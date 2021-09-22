package liqgengate

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"github.com/google/uuid"
)

   func (e *Engine) recordRace(ctx context.Context, borrowID, userID, leaseID, callerIdem string, expected, actual uint64, reason RaceReason, phase LiqPhase, actor string) (*LiqProof, error) {
	  idem := firstNonEmpty(callerIdem, raceIdemKey(borrowID, expected, actual, reason))
	  if existing, err := e.store.GetProofByIdem(ctx, idem); err == nil {
		  return existing, nil
	  } else if err != nil && !errors.Is(err, ErrNotFound) {
		   return nil, err
	   }
	  now := e.now()
	   p := &LiqProof{
		  ID: uuid.NewString(), Kind: ProofKindRace, BorrowID: borrowID, UserID: userID, LeaseID: leaseID,
		  LiqGen: expected, PeerGen: actual, Reason: string(reason), State: ProofOpen,
		   IdemKey: idem, Digest: raceDigest(borrowID, leaseID, expected, actual, reason, phase),
		   PhaseAtIssue: string(phase), Actor: actor, CreatedAt: now, UpdatedAt: now,
	   }
	   if err := e.store.InsertProof(ctx, p); err != nil {
		   if errors.Is(err, ErrDuplicate) {
			  return e.store.GetProofByIdem(ctx, idem)
		   }
		  return nil, err
	  }
	  incRaceProof()
	   return p, nil
  }

   func (e *Engine) recordHalf(ctx context.Context, borrowID, userID, leaseID, holdID string, gen uint64, reason HalfReason, hookErr string, phase LiqPhase) (*LiqProof, error) {
	   idem := halfIdemKey(borrowID, gen, reason)
	  if existing, err := e.store.GetProofByIdem(ctx, idem); err == nil {
		  return existing, nil
	   } else if err != nil && !errors.Is(err, ErrNotFound) {
		   return nil, err
	   }
	  now := e.now()
	  p := &LiqProof{
		  ID: uuid.NewString(), Kind: ProofKindHalf, BorrowID: borrowID, UserID: userID, LeaseID: leaseID,
		  LiqGen: gen, Reason: string(reason), State: ProofOpen, IdemKey: idem,
		  Digest: halfDigest(borrowID, leaseID, holdID, gen, reason, hookErr),
		  HookError: hookErr, PhaseAtIssue: string(phase), Actor: "freeze_hook",
		   CreatedAt: now, UpdatedAt: now,
	  }
	  if err := e.store.InsertProof(ctx, p); err != nil {
		   if errors.Is(err, ErrDuplicate) {
			  return e.store.GetProofByIdem(ctx,
				  idem)
		  }
		   return nil, err
	   }
	  return p, nil
   }

// GetProof 读取证明。
   func (e *Engine) GetProof(ctx context.Context, id string) (*LiqProof, error) {
	   return e.store.GetProofByID(ctx, id)
  }

// GetLease 读取租约。

   func (e *Engine) GetLease(ctx context.Context, id string) (*LiqFreezeLease, error) {
	  return e.store.GetLeaseByID(ctx, id)
  }

// GetGeneration 读取世代。

   func (e *Engine) GetGeneration(ctx context.Context, borrowID string) (*LiqGeneration, error) {
	  return e.store.GetGeneration(ctx, borrowID)
  }

// ListActive 活跃租约。

   func (e *Engine) ListActive(ctx context.Context, limit int) ([]*LiqFreezeLease, error) {
	  return e.store.ListActiveLeases(ctx, limit)
  }

// ListGenerations 世代列表。
  func (e *Engine) ListGenerations(ctx context.Context, limit int) ([]*LiqGeneration, error) {
	  return e.store.ListGenerations(ctx, limit)
   }

// ListProofs 按状态列证明。

   func (e *Engine) ListProofs(ctx context.Context, state ProofState, limit int) ([]*LiqProof, error) {
	   if state == "" {
		   open, err := e.store.ListProofsByState(ctx, ProofOpen, limit)
		  if err != nil {
			  return nil, err
		  }
		   proven,
			  err := e.store.ListProofsByState(ctx, ProofProven, limit)
		   if err != nil {
			   return nil, err
		  }
		   return append(open, proven...), nil
	   }
	  return e.store.ListProofsByState(ctx, state, limit)
  }

// ListHalfProofs 半截证明。

   func (e *Engine) ListHalfProofs(ctx context.Context, limit int) ([]*LiqProof, error) {
	  return e.store.ListProofsByKind(ctx,
		   ProofKindHalf, limit)
  }

// ListRaceProofs 竞态证明。

  func (e *Engine) ListRaceProofs(ctx context.Context, limit int) ([]*LiqProof, error) {
	   return e.store.ListProofsByKind(ctx, ProofKindRace, limit)
   }

// ListAudit 借款审计。

   func (e *Engine) ListAudit(ctx context.Context, borrowID string, limit int) ([]*LiqAudit, error) {
	   return e.store.ListAuditByBorrow(ctx, borrowID, limit)
  }

// ProofExport 对外导出包。
type ProofExport struct {
	Proof      *LiqProof      `json:"proof"`
	Generation *LiqGeneration `json:"generation,omitempty"`
	Lease *LiqFreezeLease `json:"lease,omitempty"`
	Host string `json:"host"`
}

// ExportProof 固化导出。缺世代不编造。
   func (e *Engine) ExportProof(ctx context.Context, id string) (*ProofExport, error) {
	  p, err := e.store.GetProofByID(ctx, id)
	  if err != nil {
		  return nil, err
	  }
	   out := &ProofExport{Proof: p, Host: "account-book.kucoin.net"}
	  if p.BorrowID != "" {
		  if g, err := e.store.GetGeneration(ctx, p.BorrowID); err == nil {
			   out.Generation = g
		   }
	   }
	   if p.LeaseID != "" {
		   if l, err := e.store.GetLeaseByID(ctx, p.LeaseID); err == nil {
			  out.Lease = l
		   }
	   }
	  return out, nil
  }

// ExportJSON 证明 JSON。
  func (e *Engine) ExportJSON(ctx context.Context, id string) ([]byte, error) {
	   ex, err := e.ExportProof(ctx, id)
	  if err != nil {
		  return nil, err
	   }
	  return json.MarshalIndent(ex, "", "  ")
   }

// ConfirmProof 运维确认。
  func (e *Engine) ConfirmProof(ctx context.Context, id, note string) (*LiqProof, error) {
	   p, err := e.store.GetProofByID(ctx, id)
	  if err != nil {
		  return nil, err
	  }
	  if p.State.Closed() {
		  return p, nil
	  }
	  now := e.now()
	   if p.State == ProofOpen {
		  p.State = ProofProven
		   p.ProvenAt = &now
	   } else {
		  p.State = ProofClosed
		  p.ClosedAt = &now
	  }
	  p.Note = note
	   p.UpdatedAt = now
	  if err := e.store.UpdateProof(ctx, p); err != nil {
		  return nil, err
	  }
	   return p, nil
   }

// DismissProof 驳回证明。

  func (e *Engine) DismissProof(ctx context.Context, id, note string) (*LiqProof, error) {
	  p, err := e.store.GetProofByID(ctx, id)
	  if err != nil {
		   return nil, err
	  }
	   now := e.now()
	   p.State = ProofDismiss
	   p.ClosedAt = &now
	  p.Note = note
	   p.UpdatedAt = now
	   if err := e.store.UpdateProof(ctx, p); err != nil {
		   return nil, err
	   }
	   return p, nil
  }

// ProveOpen 将 open 证明推进 proven（巡检泵）。

   func (e *Engine) ProveOpen(ctx context.Context, limit int) (int, error) {
	  rows, err := e.store.ListProofsByState(ctx, ProofOpen, limit)
	   if err != nil {
		  return 0, err
	  }
	   n := 0
	  now := e.now()
	   for _, p := range rows {
		   p.State = ProofProven
		  t := now
		   p.ProvenAt = &t
		  p.UpdatedAt = now
		  if err := e.store.UpdateProof(ctx, p); err != nil {
			   return n, err
		  }
		   n++
	  }
	   return n, nil
   }

// ExpireLease 过期单张租约。
   func (e *Engine) ExpireLease(ctx context.Context, id string) (*LiqFreezeLease, error) {
	  l, err := e.store.GetLeaseByID(ctx, id)
	   if err != nil {
		  return nil, err
	   }
	   if l.State.Closed() {
		   return l, nil
	  }
	  if !l.Expired(e.now()) {
		   return l, nil
	   }
	   if !CanTransitLease(l.State, LeaseExpired) {
		  return nil, ErrInvalidTransit
	  }
	  now := e.now()
	   from := l.State
	  l.State = LeaseExpired
	  l.UpdatedAt = now
	   if err := e.store.UpdateLease(ctx, l); err != nil {
		   return nil, err
	  }
	  e.audit(ctx, l.BorrowID, l.ID, "", "", string(from), string(LeaseExpired), l.LiqGen, "expiry", "ttl")
	   incLeaseExpired()
	   return l, nil
  }

// ScanExpired 批量过期。

  func (e *Engine) ScanExpired(ctx context.Context, limit int) (int, error) {
	  rows,
		   err := e.store.ListExpiredLeases(ctx, e.now(), limit)
	   if err != nil {
		   return 0, err
	   }
	  n := 0
	  for _, l := range rows {
		  if _, err := e.ExpireLease(ctx, l.ID); err != nil {
			  return n, err
		   }
		   n++
	   }
	   return n, nil
   }

// InspectHalf 半截冻结巡检：超时仍 half_open 则补证明。
  func (e *Engine) InspectHalf(ctx context.Context, limit int) (int, error) {
	  rows, err := e.store.ListByPhase(ctx, PhaseHalfOpen, limit)
	   if err != nil {
		  return 0, err
	  }
	  n := 0
	  cutoff := e.now().Add(-e.cfg.HalfStaleAfter)
	   for _, g := range rows {
		   if g.UpdatedAt.After(cutoff) {
			   continue
		   }
		   if _, err := e.recordHalf(ctx, g.BorrowID, g.UserID, g.LeaseID, g.HoldID, g.Gen, HalfRetryTimeout, "stale_half", PhaseHalfOpen); err != nil {
			  return n, err
		  }
		  n++
	  }
	  return n, nil
  }

// DiscoverRace 世代冲突发现：活跃租约世代与仓位不一致则落证明。
   func (e *Engine) DiscoverRace(ctx context.Context, limit int) (int, error) {
	  rows, err := e.store.ListActiveLeases(ctx, limit)
	  if err != nil {
		  return 0, err
	   }
	  n := 0
	  for _, l := range rows {
		  g, err := e.store.GetGeneration(ctx, l.BorrowID)
		   if err != nil {
			   continue
		  }
		  if l.LiqGen != g.Gen {
			  if _, err := e.recordRace(ctx, l.BorrowID, l.UserID, l.ID, "", l.LiqGen, g.Gen, RaceStaleGen, g.Phase, "discover"); err != nil {
				   return n, err
			   }
			  n++
		   }
	  }
	  return n, nil
  }

var _ = time.Time{}
