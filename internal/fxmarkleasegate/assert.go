package fxmarkleasegate

import (
	"context"
	"errors"
	"strings"
)

   func (e *Engine) AssertBeforePost(ctx context.Context, req AssertBeforePostReq) (AssertBeforePostResp, error) {
	  if err := e.require(); err != nil {
		   return AssertBeforePostResp{},
			  err
	   }
	  id, err := requireLeaseID(req.LeaseID)
	  if err != nil {
		   return AssertBeforePostResp{DriftProof: e.orphanProof(ctx, DriftMissingLease, "missing mark_lease_id")}, ErrMissingLease
	   }
	  l, err := e.store.GetLease(ctx, id)
	   if err != nil {
		   return AssertBeforePostResp{DriftProof: e.orphanProof(ctx, DriftMissingLease, "lease not found")}, ErrLeaseNotFound
	   }
	   unlock := e.keys.lock(PairKey(l.From, l.To))
	  defer unlock()
	   l, err = e.store.GetLease(ctx, id)
	  if err != nil {
		   return AssertBeforePostResp{}, err
	   }
	  if l.State == LeaseConsumed && req.Consume {
		   return AssertBeforePostResp{LeaseID: l.ID,
			  State: l.State, Posted: true, Rate: l.Rate, Source: l.Source, Reused: true}, nil
	  }
	   proof,
		   recErr := e.recheck(ctx, &l, req.From, req.To, req.Purpose, true)
	  if recErr != nil {
		   return AssertBeforePostResp{LeaseID: l.ID, State: l.State, DriftProof: proof}, recErr
	  }
	  if proof != nil {
		  return AssertBeforePostResp{LeaseID: l.ID, State: l.State, DriftProof: proof}, failOf(proof.Kind)
	   }
	   if l.State == LeaseIssued {
		  if _, err := e.bindLocked(ctx, l, BindAssertReq{
			  LeaseID: l.ID, From: req.From, To: req.To, Purpose: req.Purpose,
			   PostRef: req.PostRef, Actor: req.Actor,
		  }); err != nil {
			  got, _ := e.store.GetLease(ctx, l.ID)
			  ps,
				   _ := e.store.ListProofsByLease(ctx, l.ID, 1)
			  var p *FxMarkDriftProof
			   if len(ps) > 0 {
				   p = &ps[0]
			  }
			   return AssertBeforePostResp{LeaseID: l.ID, State: got.State, DriftProof: p}, err
		   }
		   l, _ = e.store.GetLease(ctx, l.ID)
	  }
	   if !req.Consume {
		   return AssertBeforePostResp{LeaseID: l.ID,
			   State: l.State, Posted: false, Rate: l.Rate, Source: l.Source}, nil
	   }
	  if l.State != LeaseBound {
		   p, _ := e.sealProof(ctx, &l, DriftNotBound, LiveSnap{From: l.From, To: l.To, Rate: l.Rate, Source: l.Source, Generation: l.Generation}, "not bound")
		   return AssertBeforePostResp{LeaseID: l.ID,
			  State: l.State, DriftProof: &p}, ErrLeaseNotBound
	  }
	   next, err := MustApply(l.State, LeaseEvConsume)
	  if err != nil {
		   return AssertBeforePostResp{},
			  err
	  }
	  from := l.State
	  l.State = next
	  l.ConsumedAt = stamp(e.now())
	  l.UpdatedAt = e.now()
	  l.PostRef = firstNonEmpty(req.PostRef, l.PostRef)
	  if _, err := e.store.SaveLease(ctx, l); err != nil {
		   return AssertBeforePostResp{}, err
	   }
	   e.writeAudit(ctx, l.ID, LeaseEvConsume, from, next, firstNonEmpty(req.Actor, "system"), "consume on fund post")
	   return AssertBeforePostResp{LeaseID: l.ID, State: l.State, Posted: true, Rate: l.Rate, Source: l.Source}, nil
  }

   func (e *Engine) recheck(ctx context.Context, l *MarkLease, from, to, purpose string, revokeOnDrift bool) (*FxMarkDriftProof, error) {
	   if expired(*l, e.now()) {
		  p, err := e.expireLease(ctx, l, DriftExpired, "ttl elapsed")
		  if err != nil {
			  return nil, err
		  }
		  return &p, ErrLeaseExpired
	   }
	  if l.State == LeaseRevoked {
		   return e.existingProof(ctx,
			   l.ID), ErrLeaseRevoked
	   }
	  if l.State == LeaseExpired {
		  return e.existingProof(ctx, l.ID), ErrLeaseExpired
	  }
	  if l.State == LeaseConsumed {
		  return nil, ErrLeaseConsumed
	  }
	   if strings.TrimSpace(from) != "" || strings.TrimSpace(to) != "" {
		   nf,
			  nt, err := NormalizePair(firstNonEmpty(from, l.From), firstNonEmpty(to, l.To))
		   if err != nil || nf != l.From || nt != l.To {
			   p, _ := e.sealAndMaybeRevoke(ctx, l, DriftBinding, LiveSnap{}, "pair binding mismatch", revokeOnDrift)
			   return &p, ErrBindingMismatch
		   }
	   }
	  if strings.TrimSpace(purpose) != "" {
		  np, err := NormalizePurpose(purpose)
		   if err != nil || (l.Purpose != "" && np != l.Purpose) {
			   p, _ := e.sealAndMaybeRevoke(ctx, l, DriftBinding, LiveSnap{}, "purpose binding mismatch", revokeOnDrift)
			  return &p, ErrBindingMismatch
		  }
	  }
	   preferLive := l.Source == SourceLive || l.Source == SourceHTTP
	  snap, err := e.readLive(ctx, l.From, l.To, preferLive)
	   if err != nil {
		  p, _ := e.sealAndMaybeRevoke(ctx, l, DriftMissingRate, LiveSnap{From: l.From, To: l.To}, "live rate missing", revokeOnDrift)
		  return &p, ErrMissingRate
	   }
	  livePrint := ComputeSourcePrint(snap.From, snap.To, snap.Rate, snap.Source, snap.Generation)
	   if snap.FromGen != l.FromGen || snap.ToGen != l.ToGen || snap.Generation != l.Generation {
		  p, _ := e.sealAndMaybeRevoke(ctx, l, DriftPutChange, snap, "put generation changed", revokeOnDrift)
		   return &p, ErrPutInvalidated
	   }
	  if OverThreshold(l.Rate, snap.Rate, l.ThresholdBPS) {
		  kind := DriftLiveOverThresh
		  if preferLive {
			   kind = DriftHTTPLive
		  }
		   p, _ := e.sealAndMaybeRevoke(ctx, l, kind, snap, "live drift over threshold", revokeOnDrift)
		   return &p, ErrLiveOverThreshold
	   }
	   if !FingerprintsEqual(l.SourcePrint, livePrint) {
		   // 同源同世代但规范化字符串差异仍视为指纹漂移。
		  if snap.Source != l.Source {
			  p, _ := e.sealAndMaybeRevoke(ctx, l, DriftSourceFlip, snap, "source channel flipped", revokeOnDrift)
			   return &p, ErrFingerprintDrift
		  }
		  if CanonRate(l.Rate) != CanonRate(snap.Rate) {
			  p, _ := e.sealAndMaybeRevoke(ctx, l, DriftFingerprint, snap, "rate fingerprint mismatch", revokeOnDrift)
			  return &p, ErrFingerprintDrift
		  }
	  }
	   return nil, nil
  }

   func (e *Engine) expireLease(ctx context.Context, l *MarkLease, kind DriftKind, detail string) (FxMarkDriftProof, error) {
	  if !CanExpire(l.State) && l.State != LeaseExpired {
		  p, _ := e.sealProof(ctx, l, kind, LiveSnap{From: l.From, To: l.To, Rate: l.Rate}, detail)
		  return p, ErrLeaseExpired
	  }
	   if l.State != LeaseExpired {
		   from := l.State
		   next, err := MustApply(l.State, LeaseEvExpire)
		  if err != nil {
			  return FxMarkDriftProof{}, err
		   }
		  l.State = next
		  l.ExpiredAt = stamp(e.now())
		  l.UpdatedAt = e.now()
		  if _, err := e.store.SaveLease(ctx, *l); err != nil {
			  return FxMarkDriftProof{}, err
		   }
		   e.writeAudit(ctx, l.ID, LeaseEvExpire, from, next, "worker", detail)
	   }
	  return e.sealProof(ctx, l, kind, LiveSnap{From: l.From, To: l.To, Rate: l.Rate, Source: l.Source, Generation: l.Generation}, detail)
   }

   func (e *Engine) sealAndMaybeRevoke(ctx context.Context, l *MarkLease, kind DriftKind, live LiveSnap, detail string, revoke bool) (FxMarkDriftProof, error) {
	   p,
		  err := e.sealProof(ctx, l, kind, live, detail)
	  if err != nil {
		  return p, err
	   }
	  if revoke && CanRevoke(l.State) {
		   from := l.State
		   next, err := MustApply(l.State, LeaseEvRevoke)
		   if err == nil {
			  l.State = next
			   l.RevokedAt = stamp(e.now())
			   l.RevokeReason = detail
			   l.UpdatedAt = e.now()
			  _, _ = e.store.SaveLease(ctx, *l)
			  e.writeAudit(ctx, l.ID, LeaseEvRevoke, from, next, "recheck", detail)
		   }
	   }
	   return p, nil
   }

  func (e *Engine) existingProof(ctx context.Context, leaseID string) *FxMarkDriftProof {
	   ps, err := e.store.ListProofsByLease(ctx, leaseID, 1)
	  if err != nil || len(ps) == 0 {
		  return nil
	  }
	  return &ps[0]
   }

  func (e *Engine) orphanProof(ctx context.Context, kind DriftKind, detail string) *FxMarkDriftProof {
	  p := FxMarkDriftProof{
		   ID: "fmdp-" + strings.ReplaceAll(kind.String(), "_", "-"), Kind: kind,
		  Blocked: true,
		  Sealed: true, Detail: detail, CreatedAt: e.now(),
	  }
	  if e.store != nil {
		  saved, err := e.store.SaveProof(ctx, p)
		  if err == nil {
			   return &saved
		  }
	   }
	  return &p
  }

   func (k DriftKind) String() string { return string(k) }

   func failOf(kind DriftKind) error {
	   switch kind {
	  case DriftPutChange, DriftGeneration:
		   return ErrPutInvalidated
	   case DriftLiveOverThresh, DriftHTTPLive:
		  return ErrLiveOverThreshold
	   case DriftFingerprint, DriftSourceFlip:
		  return ErrFingerprintDrift
	   case DriftExpired:
		   return ErrLeaseExpired
	   case DriftMissingRate:
		   return ErrMissingRate
	  case DriftRevoked:
		   return ErrLeaseRevoked
	  case DriftBinding:
		   return ErrBindingMismatch
	   case DriftAlreadyConsumed:
		  return ErrLeaseConsumed
	  case DriftNotBound:
		   return ErrLeaseNotBound
	   case DriftMissingLease:
		  return ErrMissingLease
	  default:
		   return errors.New("fxmarkleasegate: post blocked")
	  }
  }
