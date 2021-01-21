package subfreezecas

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/subacct"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Engine 冻结 CAS 与击穿证明核心。

type Engine struct {
	books   *subacct.Service
	cas     CASRepo
	proofs  ProofRepo
	revokes RevokeRepo
	audits   AuditRepo
	journals JournalRepo
	clock    func() time.Time
	mu sync.Mutex
}

  func NewEngine(books *subacct.Service, cas CASRepo, proofs ProofRepo, revokes RevokeRepo, audits AuditRepo, journals JournalRepo) *Engine {
	   if books == nil {
		   books = subacct.New()
	  }
	  if cas == nil {
		  cas = NewMemCASRepo()
	  }
	   if proofs == nil {
		   proofs = NewMemProofRepo()
	  }
	  if revokes == nil {
		  revokes = NewMemRevokeRepo()
	  }
	  if audits == nil {
		  audits = NewMemAuditRepo()
	  }
	   if journals == nil {
		   journals = NewMemJournalRepo()
	   }
	  e := &Engine{
		  books: books, cas: cas, proofs: proofs, revokes: revokes, audits: audits, journals: journals,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
	   books.BindCASGuard(e)
	  return e
   }

   func (e *Engine) WithClock(c func() time.Time) *Engine {
	   if c != nil {
		   e.clock = c
	  }
	   return e
  }

  func (e *Engine) Books() *subacct.Service {
	   if e == nil {
		  return subacct.New()
	  }
	  return e.books
   }

   func (e *Engine) now() time.Time {
	   if e == nil || e.clock == nil {
		   return time.Now().UTC()
	   }
	  return e.clock().UTC()
   }

  func (e *Engine) audit(ctx context.Context, casID, parentID, childID, actor, action, outcome, detail string, gen int64) {
	   if e == nil || e.audits == nil {
		   return
	  }
	  _ = e.audits.Append(ctx, FreezeAudit{
		   AuditID: uuid.NewString(), FreezeCASID: casID, ParentID: parentID, ChildID: childID,
		  Actor: actor, Action: action, Outcome: outcome, Detail: detail, Generation: gen, At: e.now(),
	   })
   }

   func (e *Engine) currentCAS(ctx context.Context, parentID string) (*FreezeCAS, error) {
	   rows, err := e.cas.ListByParent(ctx, parentID, 32)
	   if err != nil {
		  return nil, err
	  }
	  cur := CurrentLineage(rows)
	  if cur == nil {
		   return nil, ErrCASNotFound
	  }
	   return cur, nil
  }

   func (e *Engine) childIDs(parentID string) []string {
	  kids := e.books.ListChildren(parentID)
	  ids := make([]string, 0, len(kids))
	   for _, c := range kids {
		   if c.Status == subacct.StatusClosed {
			  continue
		  }
		   ids = append(ids, c.ID)
	   }
	   return normalizeChildIDs(ids)
   }

   func (e *Engine) frozenChildIDs(parentID string) []string {
	  kids := e.books.ListChildren(parentID)
	  ids := make([]string, 0, len(kids))
	   for _, c := range kids {
		   if c.Status == subacct.StatusFrozen {
			  ids = append(ids, c.ID)
		   }
	  }
	  return normalizeChildIDs(ids)
  }

  func (e *Engine) parentOfOwner(ownerID string) (parentID, childID string, err error) {
	   ownerID = strings.TrimSpace(ownerID)
	  if p, e1 := e.books.GetParent(ownerID); e1 == nil {
		  return p.ID, "", nil
	   }
	  c, e2 := e.books.GetChild(ownerID)
	  if e2 != nil {
		  return "", "", ErrChild
	  }
	  return c.ParentID, c.ID, nil
   }

   func (e *Engine) openLine(ctx context.Context, parentID, actor, reason, idem string, childIDs []string) (*FreezeCAS, error) {
	   now := e.now()
	   if idem != "" {
		  if prev, err := e.cas.GetByIdem(ctx, parentID, idem); err == nil && prev != nil {
			  return prev, nil
		  }
	  }
	  fp := ChildFingerprint(childIDs)
	   row := &FreezeCAS{
		   FreezeCASID: uuid.NewString(), ParentID: parentID, ChildSetFingerprint: fp,
		  ChildIDs: childIDs, Reason: reason, Generation: 1, State: CASOpen,
		   Actor: actor, OpenedAt: now, IdempotencyKey: idem,
	  }
	   cur, err := e.currentCAS(ctx, parentID)
	   if err == nil && cur != nil {
		  row.Generation = cur.Generation + 1
		   if cur.State == CASOpen {
			   return cur, nil
		   }
	  }
	   if err := e.cas.Save(ctx, row); err != nil {
		   return nil, err
	  }
	   recordCASOpen()
	  return row, nil
   }

// FreezeParentReq 母户冻结并推进 CAS。

type FreezeParentReq struct {
	ParentID string `json:"parent_id"`
	Reason         string `json:"reason"`
	Actor          string `json:"actor"`
	IdempotencyKey string `json:"idempotency_key"`
}

// FreezeParentWithCAS 冻母户级联子户，并推进 freeze_cas。
  func (e *Engine) FreezeParentWithCAS(ctx context.Context, req FreezeParentReq) (*FreezeCAS, error) {
	   if e == nil {
		  return nil, ErrUnavailable
	   }
	   if ctx == nil {
		   ctx = context.Background()
	   }
	   parentID := strings.TrimSpace(req.ParentID)
	  if parentID == "" {
		   return nil, ErrParent
	   }
	   actor, err := validate.Actor(req.Actor)
	  if err != nil {
		   actor = strings.TrimSpace(req.Actor)
		  if actor == "" {
			  return nil, ErrActor
		   }
	   }
	  reason := strings.TrimSpace(req.Reason)
	  if reason == "" {
		   reason = "freeze"
	   }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   e.books.EnterCASInFlight()
	  defer e.books.LeaveCASInFlight()
	   if req.IdempotencyKey != "" {
		   if prev, err := e.cas.GetByIdem(ctx, parentID, strings.TrimSpace(req.IdempotencyKey)); err == nil && prev != nil && prev.State == CASFrozen {
			  e.audit(ctx, prev.FreezeCASID, parentID, "", actor, ActionFreezeParent, OutcomeIdem, "replay", prev.Generation)
			   return prev, nil
		   }
	   }
	  p, err := e.books.GetParent(parentID)
	  if err != nil {
		   return nil, ErrParent
	  }
	  if p.Status == subacct.StatusClosed {
		  return nil, ErrClosed
	   }
	   childIDs := e.childIDs(parentID)
	  line,
		  err := e.openLine(ctx, parentID, actor, reason, strings.TrimSpace(req.IdempotencyKey), childIDs)
	   if err != nil {
		   return nil, err
	   }
	   if line.State == CASFrozen {
		  e.audit(ctx, line.FreezeCASID, parentID, "", actor, ActionFreezeParent, OutcomeIdem, "already frozen", line.Generation)
		  return line, nil
	  }
	   now := e.now()
	  if err := TransitionCAS(line, CASFrozen); err != nil {
		  return nil, err
	   }
	  line.Reason = reason
	  line.Actor = actor
	  line.FrozenAt = now
	   line.ChildIDs = childIDs
	  line.ChildSetFingerprint = ChildFingerprint(childIDs)
	   if _, err := e.books.FreezeParent(parentID, reason); err != nil {
		   if errors.Is(err, subacct.ErrClosed) {
			   return nil, ErrClosed
		  }
		  if errors.Is(err, subacct.ErrFrozen) {
			  // 已冻仍须落 CAS
		   } else if err != nil {
			   _ = e.cas.Save(ctx, line)
			   return nil, err
		  }
	  }
	   if err := e.cas.Save(ctx, line); err != nil {
		   return nil, err
	   }
	   if err := e.revokeVelocityLocked(ctx, line); err != nil {
		  return nil, err
	   }
	  if err := AlignChildren(*line, e.frozenChildIDs(parentID)); err != nil {
		  e.audit(ctx, line.FreezeCASID, parentID, "", actor, ActionFreezeParent, OutcomeMisaligned, err.Error(), line.Generation)
		  return nil, err
	  }
	  e.audit(ctx, line.FreezeCASID, parentID, "", actor, ActionFreezeParent, OutcomeOK, reason, line.Generation)
	  recordFreeze("parent")
	   return line, nil
   }

// FreezeChildReq 单子户冻结。

type FreezeChildReq struct {
	ChildID string `json:"child_id"`
	Reason  string `json:"reason"`
	Actor          string `json:"actor"`
	IdempotencyKey string `json:"idempotency_key"`
}

// FreezeChild 冻单子户并推进母户 CAS 世代。

  func (e *Engine) FreezeChild(ctx context.Context, req FreezeChildReq) (*FreezeCAS, error) {
	  if e == nil {
		   return nil, ErrUnavailable
	  }
	   if ctx == nil {
		  ctx = context.Background()
	  }
	  childID := strings.TrimSpace(req.ChildID)
	   if childID == "" {
		  return nil, ErrChild
	  }
	  actor, err := validate.Actor(req.Actor)
	   if err != nil {
		  actor = strings.TrimSpace(req.Actor)
		   if actor == "" {
			  return nil, ErrActor
		   }
	   }
	   reason := strings.TrimSpace(req.Reason)
	  if reason == "" {
		  reason = "freeze"
	  }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   e.books.EnterCASInFlight()
	  defer e.books.LeaveCASInFlight()
	   ch, err := e.books.GetChild(childID)
	   if err != nil {
		  return nil, ErrChild
	   }
	  if ch.Status == subacct.StatusClosed {
		  return nil, ErrClosed
	  }
	   if _, err := e.books.FreezeChild(childID, reason); err != nil {
		   if errors.Is(err, subacct.ErrClosed) {
			  return nil, ErrClosed
		   }
		  return nil, err
	   }
	   childIDs := e.childIDs(ch.ParentID)
	   line, err := e.openLine(ctx, ch.ParentID, actor, reason, strings.TrimSpace(req.IdempotencyKey), childIDs)
	   if err != nil {
		   return nil, err
	  }
	   if line.State == CASOpen {
		   line.Generation++
		  line.Reason = "child:" + reason
		  line.ChildIDs = childIDs
		  line.ChildSetFingerprint = ChildFingerprint(childIDs)
		   line.Actor = actor
	  }
	  if err := e.cas.Save(ctx, line); err != nil {
		  return nil, err
	   }
	  if err := e.revokeOwnerLocked(ctx, line, childID); err != nil {
		  return nil, err
	   }
	   e.audit(ctx, line.FreezeCASID, ch.ParentID, childID, actor, ActionFreezeChild, OutcomeOK, reason, line.Generation)
	  recordFreeze("child")
	  return line, nil
  }

// UnfreezeParent 解母户：CAS frozen→thawed，并另开新世代 open 单据。
   func (e *Engine) UnfreezeParent(ctx context.Context, parentID, actor string) (*FreezeCAS, error) {
	   if e == nil {
		   return nil, ErrUnavailable
	   }
	   if ctx == nil {
		  ctx = context.Background()
	  }
	   parentID = strings.TrimSpace(parentID)
	   actor = strings.TrimSpace(actor)
	   if parentID == "" {
		   return nil, ErrParent
	  }
	  if actor == "" {
		   return nil, ErrActor
	  }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   e.books.EnterCASInFlight()
	   defer e.books.LeaveCASInFlight()
	   if _, err := e.books.UnfreezeParent(parentID); err != nil {
		  return nil, err
	   }
	  cur, err := e.currentCAS(ctx, parentID)
	   if err != nil {
		  return nil, err
	  }
	   if cur.State == CASFrozen {
		   if err := TransitionCAS(cur, CASThawed); err != nil {
			   return nil, err
		   }
		   cur.ThawedAt = e.now()
		   cur.Actor = actor
		   if err := e.cas.Save(ctx, cur); err != nil {
			  return nil, err
		  }
		   e.deactivateRevokesLocked(ctx, cur.FreezeCASID)
	   }
	   next, err := e.openLine(ctx, parentID, actor, "thaw", "", e.childIDs(parentID))
	   if err != nil {
		   return nil, err
	  }
	   e.audit(ctx, next.FreezeCASID, parentID, "", actor, ActionUnfreezeParent, OutcomeOK, "thaw", next.Generation)
	  recordThaw("parent")
	  return next, nil
  }

// UnfreezeChild 母户仍冻必须 fail-closed 并审计。
  func (e *Engine) UnfreezeChild(ctx context.Context, childID, actor string) error {
	  if e == nil {
		   return ErrUnavailable
	  }
	   if ctx == nil {
		   ctx = context.Background()
	  }
	  childID = strings.TrimSpace(childID)
	  actor = strings.TrimSpace(actor)
	   if childID == "" {
		  return ErrChild
	  }
	  if actor == "" {
		   return ErrActor
	   }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  e.books.EnterCASInFlight()
	  defer e.books.LeaveCASInFlight()
	   ch, err := e.books.GetChild(childID)
	  if err != nil {
		   return ErrChild
	  }
	  p, err := e.books.GetParent(ch.ParentID)
	  if err != nil {
		  return ErrParent
	  }
	   cur, _ := e.currentCAS(ctx, ch.ParentID)
	   casID, gen := "", int64(0)
	   if cur != nil {
		  casID, gen = cur.FreezeCASID, cur.Generation
		   if cur.State == CASFrozen || p.Status == subacct.StatusFrozen {
			  e.audit(ctx, casID, ch.ParentID, childID, actor, ActionUnfreezeChild, OutcomeDenied, "parent still frozen", gen)
			  recordUnfreezeDenied()
			  return ErrUnfreezeChild
		   }
	   }
	  if p.Status == subacct.StatusFrozen {
		   e.audit(ctx, casID, ch.ParentID, childID, actor, ActionUnfreezeChild, OutcomeDenied, "parent status frozen", gen)
		   recordUnfreezeDenied()
		   return ErrUnfreezeChild
	   }
	  if _, err := e.books.UnfreezeChild(childID); err != nil {
		  if errors.Is(err, subacct.ErrFrozen) {
			  e.audit(ctx, casID, ch.ParentID, childID, actor, ActionUnfreezeChild, OutcomeDenied, err.Error(), gen)
			  recordUnfreezeDenied()
			  return ErrUnfreezeChild
		   }
		  return err
	  }
	  if cur != nil && cur.State == CASOpen {
		   cur.Generation++
		   cur.ChildIDs = e.childIDs(ch.ParentID)
		  cur.ChildSetFingerprint = ChildFingerprint(cur.ChildIDs)
		   cur.Actor = actor
		  _ = e.cas.Save(ctx, cur)
		   gen = cur.Generation
		   casID = cur.FreezeCASID
	  }
	  e.audit(ctx, casID, ch.ParentID, childID, actor, ActionUnfreezeChild, OutcomeOK, "thaw child", gen)
	   recordThaw("child")
	  return nil
   }

  func parseAmt(s string) error {
	   amt, err := money.Parse(strings.TrimSpace(s))
	  if err != nil {
		   return ErrAmount
	  }
	  if !amt.IsPositive() {
		   return ErrAmount
	  }
	  return nil
   }

  func mapBooksErr(err error) error {
	  if err == nil {
		   return nil
	  }
	   switch {
	   case errors.Is(err, subacct.ErrFrozen):
		  return ErrCASFrozen
	   case errors.Is(err, subacct.ErrClosed):
		   return ErrClosed
	   case errors.Is(err,
		   subacct.ErrParent):
		   return ErrParent
	  case errors.Is(err, subacct.ErrChild):
		  return ErrChild
	   case errors.Is(err, subacct.ErrAmount):
		   return ErrAmount
	   case errors.Is(err, subacct.ErrAsset):
		  return ErrAsset
	   case errors.Is(err, subacct.ErrVelocity):
		   return ErrVelocityRevoked
	  default:
		  return err
	   }
   }
