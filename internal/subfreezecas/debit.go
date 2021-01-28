package subfreezecas

import (
	"context"
	"errors"
	"strings"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/subacct"
	"account-assets.kucoin.net/funding/internal/validate"
)

// DebitCASReq 借记并复验 CAS。

type DebitCASReq struct {
	OwnerID string `json:"owner_id"`
	Asset   string `json:"asset"`
	Amount string `json:"amount"`
	FreezeCASID    string `json:"freeze_cas_id"`
	IdempotencyKey string `json:"idempotency_key"`
	Actor          string `json:"actor"`
}

// DebitCASResult 借记结果，始终带回 freeze_cas_id。
type DebitCASResult struct {
	FreezeCASID string `json:"freeze_cas_id"`
	Generation int64 `json:"generation"`
	Book subacct.BookBal `json:"book"`
}

// DebitAssertCAS 复验未过期 cas 后借记。冻后成功必须落证明并回滚。

   func (e *Engine) DebitAssertCAS(ctx context.Context, req DebitCASReq) (*DebitCASResult, error) {
	   if e == nil {
		  return nil, ErrUnavailable
	  }
	   if ctx == nil {
		   ctx = context.Background()
	  }
	   if err := parseAmt(req.Amount); err != nil {
		   return nil, err
	   }
	  if _, err := validate.Asset(req.Asset); err != nil {
		  return nil, ErrAsset
	  }
	  owner := strings.TrimSpace(req.OwnerID)
	   casID := strings.TrimSpace(req.FreezeCASID)
	   if owner == "" {
		  return nil, ErrInvalidInput
	   }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   e.books.EnterCASInFlight()
	  defer e.books.LeaveCASInFlight()
	   parentID, childID, err := e.parentOfOwner(owner)
	  if err != nil {
		   return nil, err
	   }
	   if err := e.assertDebitLocked(ctx, parentID, childID, owner, casID, req.Asset, req.Amount); err != nil {
		  e.noteJournal(ctx, casID, owner, parentID, childID, req.Asset, req.Amount, req.IdempotencyKey, false, err.Error(), false)
		  e.audit(ctx,
			   casID, parentID, childID, req.Actor, ActionDebitAssert, OutcomeDenied, err.Error(), 0)
		   recordDebitDenied(reasonLabel(err))
		   return nil, err
	  }
	   cur, err := e.cas.Get(ctx, casID)
	  if err != nil {
		  return nil, err
	  }
	   bal, err := e.books.Debit(subacct.PostReq{
		  OwnerID: owner, Asset: req.Asset, Amount: req.Amount,
		   Kind: "debit", IdempotencyKey: req.IdempotencyKey, FreezeCASID: casID,
	  })
	   if err != nil {
		   e.noteJournal(ctx, casID, owner, parentID, childID, req.Asset, req.Amount, req.IdempotencyKey, false, err.Error(), false)
		  return nil, mapBooksErr(err)
	  }
	  afterFrozen := e.ownerFrozen(owner)
	   e.noteJournal(ctx, casID, owner, parentID, childID, req.Asset, req.Amount, req.IdempotencyKey, true, "", afterFrozen)
	  if afterFrozen {
		  proof,
			  perr := e.recordPunchLocked(ctx, cur, owner, parentID, childID, req.Asset, req.Amount, req.IdempotencyKey)
		   if perr != nil {
			  return nil, perr
		   }
		  if err := e.compensateLocked(ctx, proof); err != nil {
			   return nil, err
		  }
		   e.audit(ctx, casID, parentID, childID, req.Actor, ActionPunchProof, OutcomePunch, proof.ProofID, cur.Generation)
		   recordPunch()
		   return nil, ErrPunchDetected
	  }
	   e.audit(ctx,
		   casID, parentID, childID, req.Actor, ActionDebitAssert, OutcomeOK, req.Amount, cur.Generation)
	  recordDebitOK()
	   return &DebitCASResult{FreezeCASID: cur.FreezeCASID, Generation: cur.Generation, Book: bal}, nil
   }

  func (e *Engine) assertDebitLocked(ctx context.Context, parentID, childID, owner, casID, asset, amount string) error {
	  if strings.TrimSpace(casID) == "" {
		   return ErrCASMissing
	   }
	   got, err := e.cas.Get(ctx, casID)
	   if err != nil {
		  return ErrCASMissing
	   }
	  cur, err := e.currentCAS(ctx, parentID)
	   if err != nil {
		   return ErrCASMissing
	   }
	  if got.FreezeCASID != cur.FreezeCASID {
		  return ErrCASMismatch
	   }
	   if got.Generation != cur.Generation {
		   return ErrCASStale
	  }
	  if err := got.DebitAllowed(e.now()); err != nil {
		  return err
	   }
	   p,
		   err := e.books.GetParent(parentID)
	   if err != nil {
		  return ErrParent
	   }
	   if p.Status == subacct.StatusClosed {
		   return ErrClosed
	   }
	  if p.Status == subacct.StatusFrozen {
		  if err := AlignChildren(*cur, e.frozenChildIDs(parentID)); err != nil {
			   return err
		  }
		  return ErrCASFrozen
	  }
	  if childID != "" {
		   ch, err := e.books.GetChild(childID)
		   if err != nil {
			   return ErrChild
		   }
		  if ch.Status == subacct.StatusFrozen {
			  return ErrCASFrozen
		   }
		   if ch.Status == subacct.StatusClosed {
			  return ErrClosed
		   }
		   if cur.ChildSetFingerprint != "" && cur.ChildSetFingerprint != ChildFingerprint(e.childIDs(parentID)) {
			  return ErrFingerprint
		  }
	  }
	  if err := e.assertVelocityLocked(ctx, owner); err != nil {
		   return err
	   }
	   _ = asset
	  _ = amount
	   return nil
   }

  func (e *Engine) ownerFrozen(ownerID string) bool {
	   if p, err := e.books.GetParent(ownerID); err == nil {
		   return p.Status == subacct.StatusFrozen
	  }
	   if c, err := e.books.GetChild(ownerID); err == nil {
		  if c.Status == subacct.StatusFrozen {
			  return true
		   }
		  if p, err := e.books.GetParent(c.ParentID); err == nil && p.Status == subacct.StatusFrozen {
			   return true
		   }
	   }
	   return false
  }

   func (e *Engine) noteJournal(ctx context.Context, casID, owner, parentID, childID, asset, amount, idem string, ok bool, refuse string, afterFrozen bool) {
	   if e == nil || e.journals == nil {
		  return
	   }
	   _ = e.journals.Save(ctx, &DebitJournal{
		   JournalID: uuid.NewString(), FreezeCASID: casID, OwnerID: owner, ParentID: parentID, ChildID: childID,
		   Asset: asset, Amount: amount, Idem: idem, OK: ok, Refused: refuse, At: e.now(), AfterFrozen: afterFrozen,
	   })
   }

  func reasonLabel(err error) string {
	  switch {
	   case errors.Is(err, ErrCASMissing):
		   return "missing"
	   case errors.Is(err, ErrCASStale), errors.Is(err, ErrCASMismatch):
		   return "stale"
	   case errors.Is(err, ErrCASFrozen):
		   return "frozen"
	   case errors.Is(err, ErrGateMisaligned):
		   return "misaligned"
	  case errors.Is(err,
		   ErrVelocityRevoked):
		  return "revoked"
	   case errors.Is(err, ErrFingerprint):
		   return "fingerprint"
	   default:
		   return "denied"
	   }
  }

// GuardDebit 实现 subacct.CASGuard：外部 Debit 路径复验。
   func (e *Engine) GuardDebit(ownerID, casID, asset, amount string) error {
	   if e == nil {
		  return ErrUnavailable
	   }
	  ctx := context.Background()
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   parentID, childID, err := e.parentOfOwner(ownerID)
	   if err != nil {
		   return err
	  }
	  return e.assertDebitLocked(ctx,
		  parentID, childID, ownerID, casID, asset, amount)
  }

// GuardUnfreezeChild 母仍冻则拒绝。

   func (e *Engine) GuardUnfreezeChild(childID string) error {
	  if e == nil {
		  return ErrUnavailable
	   }
	   ctx := context.Background()
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   ch, err := e.books.GetChild(strings.TrimSpace(childID))
	  if err != nil {
		   return ErrChild
	   }
	   p,
		   err := e.books.GetParent(ch.ParentID)
	  if err != nil {
		  return ErrParent
	   }
	  if p.Status == subacct.StatusFrozen {
		  cur, _ := e.currentCAS(ctx, ch.ParentID)
		   casID, gen := "", int64(0)
		   if cur != nil {
			  casID, gen = cur.FreezeCASID, cur.Generation
		   }
		   e.audit(ctx, casID, ch.ParentID, childID, "books", ActionUnfreezeChild, OutcomeDenied, "parent frozen", gen)
		   return ErrUnfreezeChild
	  }
	   return nil
  }

// NoteDebitResult 外部 Debit 轨迹。
   func (e *Engine) NoteDebitResult(ownerID, casID, asset, amount, idem string, ok bool, refuse string) {
	   if e == nil {
		   return
	  }
	   parentID, childID, _ := e.parentOfOwner(ownerID)
	   e.noteJournal(context.Background(), casID, ownerID, parentID, childID, asset, amount, idem, ok, refuse, e.ownerFrozen(ownerID))
  }

// GetCAS 只读查询 freeze_cas_id。

   func (e *Engine) GetCAS(ctx context.Context, casID string) (*FreezeCAS, error) {
	   if e == nil {
		  return nil, ErrUnavailable
	   }
	  if ctx == nil {
		  ctx = context.Background()
	   }
	  return e.cas.Get(ctx, strings.TrimSpace(casID))
  }

// CurrentCAS 母户当前世代。
   func (e *Engine) CurrentCAS(ctx context.Context, parentID string) (*FreezeCAS, error) {
	  if e == nil {
		  return nil, ErrUnavailable
	  }
	   if ctx == nil {
		  ctx = context.Background()
	  }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  return e.currentCAS(ctx, strings.TrimSpace(parentID))
   }

// ListAudit 母户审计。

  func (e *Engine) ListAudit(ctx context.Context, parentID string, limit int) ([]FreezeAudit, error) {
	   if e == nil {
		  return nil, ErrUnavailable
	   }
	   return e.audits.ListByParent(ctx, parentID, limit)
   }
