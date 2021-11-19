package stablemintgate

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/stablebr"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

// BeforePost 实现 stablebr.PostGuard。

  func (e *Engine) BeforePost(ctx context.Context, kind stablebr.Kind, asset, userID, amount, actor, idem, leaseID string) error {
	   if e == nil {
		  return ErrUnavailable
	   }
	   if strings.TrimSpace(leaseID) == "" {
		   e.denyUnlocked(ctx, "", ScopeKey(KindMint, asset), KindMint, asset, OutcomeBlockedNoLease, "mint without lease")
		   recordBlocked("post", "no_lease")
		  return ErrNoLease
	  }
	   if half, herr := e.halves.GetByIdem(ctx, idem); herr == nil && half != nil && half.State == HalfOpen {
		  e.denyUnlocked(ctx, leaseID, ScopeKey(KindMint, asset), KindMint, asset, OutcomeBlockedReplay, "half-open replay")
		  recordBlocked("post", "half_replay")
		  return ErrHalfOpen
	  }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   res, err := e.assertLocked(ctx, AssertInput{
		   LeaseID: leaseID, Kind: string(kind), Asset: asset, Actor: actor, UserID: userID, Amount: amount,
	  })
	   if err != nil {
		   return err
	   }
	   lease := res.Lease
	   if lease.State == LeaseIssued {
		  if err := TransitionLease(lease, LeasePosting); err != nil {
			  return err
		   }
	  }
	   lease.UserID = userID
	  lease.Amount = amount
	   lease.Idempotency = idem
	  if err := e.leases.Save(ctx, lease); err != nil {
		   return err
	  }
	   _ = e.auditLocked(ctx, lease.LeaseID, ScopeKey(lease.Kind, lease.Asset), actor, "posting", "journal fence enter", string(LeasePosting))
	  return nil
   }

// OnLedgerFail 落半截证明，保留 Journal 与幂等键。
   func (e *Engine) OnLedgerFail(ctx context.Context, j stablebr.Journal, cause error) error {
	  if e == nil {
		  return ErrUnavailable
	   }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  now := e.now()
	   detail := "ledger create failed"
	  if cause != nil {
		   detail = cause.Error()
	  }
	  if exist, err := e.halves.GetByIdem(ctx, j.IdempotencyKey); err == nil && exist != nil {
		   return ErrHalfOpen
	   }
	  p := &StableHalfProof{
		  HalfID: uuid.NewString(), LeaseID: j.MintLeaseID, JournalID: j.ID,
		   IdemKey: j.IdempotencyKey, Kind: LeaseKind(j.Kind), Asset: j.Asset,
		   UserID: j.UserID, Amount: j.Amount, State: HalfOpen, Cause: detail, CreatedAt: now,
	   }
	   p.Digest = p.ComputeDigest()
	  if err := e.halves.Save(ctx, p); err != nil {
		   return err
	   }
	  if lease, err := e.leases.Get(ctx, j.MintLeaseID); err == nil && lease != nil {
		   lease.JournalID = j.ID
		   _ = e.leases.Save(ctx, lease)
	  }
	   _ = e.denyLocked(ctx, j.MintLeaseID, ScopeKey(LeaseKind(j.Kind), j.Asset), LeaseKind(j.Kind), j.Asset, OutcomeHalfOpen, detail)
	  _ = e.auditLocked(ctx, j.MintLeaseID, ScopeKey(LeaseKind(j.Kind), j.Asset), j.Actor, "half", detail, string(OutcomeHalfOpen))
	  recordHalf("open")
	  return nil
  }

// OnPosted Journal 与 ledger 均成功后消费租约。
  func (e *Engine) OnPosted(ctx context.Context, j stablebr.Journal) error {
	  if e == nil {
		   return ErrUnavailable
	  }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   if j.MintLeaseID == "" {
		  return ErrNoLease
	   }
	  lease, err := e.leases.Get(ctx, j.MintLeaseID)
	   if err != nil {
		   return err
	   }
	   now := e.now()
	   lease.JournalID = j.ID
	  lease.BoundLedger = j.LedgerID
	  if lease.State == LeaseIssued {
		   if err := TransitionLease(lease, LeasePosting); err != nil {
			  return err
		   }
	   }
	  if lease.State == LeasePosting {
		   if err := TransitionLease(lease, LeasePosted); err != nil {
			  return err
		   }
	   }
	  if lease.State == LeasePosted {
		   if err := TransitionLease(lease, LeaseConsumed); err != nil {
			  return err
		  }
	   }
	   lease.ConsumedAt = &now
	  if err := e.leases.Save(ctx, lease); err != nil {
		  return err
	  }
	   if half, herr := e.halves.GetByJournal(ctx, j.ID); herr == nil && half != nil && half.State == HalfOpen {
		   _ = TransitionHalf(half, HalfResolved)
		  half.ResolvedAt = &now
		  half.Digest = half.ComputeDigest()
		   _ = e.halves.Save(ctx, half)
		   recordHalf("resolved")
	  }
	  _ = e.auditLocked(ctx, lease.LeaseID, ScopeKey(lease.Kind, lease.Asset), j.Actor, "consume", "posted+consumed", string(OutcomeConsumed))
	  recordConsumed(string(lease.Kind))
	  return nil
  }

// MintWithLease 持未过期租约铸币。
  func (e *Engine) MintWithLease(ctx context.Context, req MintWithLeaseReq) (stablebr.Journal, *MintLease, error) {
	  return e.postWithLease(ctx, KindMint, req)
   }

// RedeemWithLease 持未过期租约赎回。
   func (e *Engine) RedeemWithLease(ctx context.Context, req MintWithLeaseReq) (stablebr.Journal, *MintLease, error) {
	   if req.Kind == "" {
		   req.Kind = string(KindRedeem)
	  }
	   return e.postWithLease(ctx, KindRedeem, req)
  }

   func (e *Engine) postWithLease(ctx context.Context, expect LeaseKind, req MintWithLeaseReq) (stablebr.Journal, *MintLease, error) {
	  if e == nil {
		  return stablebr.Journal{}, nil, ErrUnavailable
	   }
	   if _, err := validate.UserID(req.UserID); err != nil {
		  return stablebr.Journal{}, nil, ErrInvalidInput
	  }
	  if err := parseAmount(req.Amount); err != nil {
		  return stablebr.Journal{}, nil, err
	   }
	  if _, err := validate.Idem(req.IdempotencyKey); err != nil {
		   return stablebr.Journal{}, nil, ErrInvalidInput
	   }
	   kind := expect
	   if req.Kind != "" {
		  k, err := ParseLeaseKind(req.Kind)
		  if err != nil {
			  return stablebr.Journal{},
				  nil, err
		  }
		  kind = k
	  }
	   if kind != expect {
		   return stablebr.Journal{}, nil, ErrKindMismatch
	  }
	   if half, err := e.halves.GetByIdem(ctx, req.IdempotencyKey); err == nil && half != nil && half.State == HalfOpen {
		   e.denyUnlocked(ctx, req.LeaseID, ScopeKey(kind, req.Asset), kind, req.Asset, OutcomeBlockedReplay, "half-open idem replay")
		   return stablebr.Journal{}, nil, ErrIdemReplay
	   }
	   var j stablebr.Journal
	   var perr error
	  if kind == KindRedeem {
		  j, perr = e.br.Redeem(ctx, stablebr.RedeemReq{
			   UserID: req.UserID, Asset: req.Asset, Amount: req.Amount, Actor: req.Actor,
			  IdempotencyKey: req.IdempotencyKey, MintLeaseID: req.LeaseID,
		  })
	   } else {
		  j, perr = e.br.Mint(ctx, stablebr.MintReq{
			  UserID: req.UserID, Asset: req.Asset, Amount: req.Amount, Actor: req.Actor,
			  IdempotencyKey: req.IdempotencyKey, MintLeaseID: req.LeaseID,
		   })
	  }
	  lease, _ := e.leases.Get(ctx, req.LeaseID)
	  if perr != nil {
		  if j.Status == stablebr.StatusHalf {
			  return j, lease, fmt.Errorf("%w: %v", ErrLedger, perr)
		   }
		  return j, lease, perr
	   }
	   return j, lease, nil
   }

// ResolveHalf 运维将半截标为已处理（不二次铸）。
  func (e *Engine) ResolveHalf(ctx context.Context, halfID, actor, note string) (*StableHalfProof, error) {
	   if e == nil {
		   return nil, ErrUnavailable
	  }
	  if _, err := validate.Actor(actor); err != nil {
		   return nil, ErrInvalidInput
	  }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   p, err := e.halves.Get(ctx, halfID)
	  if err != nil {
		  return nil, err
	  }
	  if p.State == HalfResolved {
		  cp := p.Copy()
		  return &cp, nil
	  }
	   if err := TransitionHalf(p, HalfResolved); err != nil {
		  return nil, err
	  }
	   now := e.now()
	  p.ResolvedAt = &now
	  p.Digest = p.ComputeDigest()
	  if err := e.halves.Save(ctx, p); err != nil {
		  return nil, err
	   }
	  _ = e.auditLocked(ctx, p.LeaseID, ScopeKey(p.Kind, p.Asset), actor, "resolve_half", note, string(OutcomeHalfResolved))
	   recordHalf("resolved")
	   cp := p.Copy()
	  return &cp, nil
  }

// ExportProofBundle 导出拒铸/半截/租约只读包。

  func (e *Engine) ExportProofBundle(ctx context.Context, leaseID string) (map[string]any, error) {
	   if e == nil {
		  return nil, ErrUnavailable
	  }
	  out := map[string]any{"mint_lease_id": leaseID}
	  if lease, err := e.leases.Get(ctx, leaseID); err == nil {
		  out["lease"] = lease.Copy()
		   out["scope"] = ScopeKey(lease.Kind, lease.Asset)
	  }
	   if rows, err := e.halves.ListByLease(ctx, leaseID, 50); err == nil {
		   out["half_proofs"] = rows
	  }
	  if rows, err := e.audits.ListByLease(ctx, leaseID, 50); err == nil {
		  out["audit"] = rows
	   }
	  return out, nil
  }
