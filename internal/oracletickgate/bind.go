package oracletickgate

import (
	"context"
	"fmt"
	"strings"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/oracle"
)

// AssertBeforeFundWrite 资金写前复验。无租约 / 过期 / 缺价 / Shadow 一律拒。
  func (s *Service) AssertBeforeFundWrite(ctx context.Context, req AssertFundWriteRequest) (*TickLease, *FundWriteRecord, error) {
	   if s == nil {
		  return nil, nil, ErrUnavailable
	   }
	   actor,
		  err := requireActor(req.Actor)
	  if err != nil {
		   return nil, nil, err
	   }
	  if strings.TrimSpace(req.LeaseID) == "" {
		   s.openProof(ctx, TickDriftProof{
			   ProofID: uuid.NewString(), Kind: ProofNoLease, FailClosed: true,
			   WriteKind: string(req.WriteKind), Summary: "fund write without tick_lease",
			   OpenedAt:      s.clock(),
			   FundWriteHeld: true,
		  })
		  return nil, nil, ErrNoLease
	   }
	   if key := strings.TrimSpace(req.IdemKey); key != "" {
		  if id, ok, _ := s.store.GetIdem(ctx, "write:"+key); ok {
			   l, err := s.store.GetLease(ctx, id)
			   if err != nil {
				  return nil, nil, err
			   }
			  w, _ := s.store.GetWriteByLease(ctx, id)
			  return l, w, nil
		   }
	  }
	  l,
		  err := s.store.GetLease(ctx, req.LeaseID)
	  if err != nil {
		  s.openProof(ctx, TickDriftProof{
			  ProofID: uuid.NewString(),
			   LeaseID: req.LeaseID, Kind: ProofNoLease, FailClosed: true,
			   WriteKind: string(req.WriteKind), Summary: "lease not found", OpenedAt: s.clock(), FundWriteHeld: true,
		   })
		  return nil, nil, ErrNoLease
	   }
	   if !l.Mode.Writable() {
		  s.openProof(ctx, TickDriftProof{
			   ProofID: uuid.NewString(), LeaseID: l.LeaseID, Pair: l.Pair, Kind: ProofShadowImpersonate,
			   FailClosed: true, WriteKind: string(req.WriteKind), IssuedGen: l.Generation,
			  Summary: "shadow/probe must not impersonate enforce write lease", OpenedAt: s.clock(), FundWriteHeld: true,
		   })
		   return l, nil, ErrShadowImpersonate
	   }
	  if !WriteAllowed(l.ValuationKind, req.WriteKind) {
		   s.openProof(ctx, TickDriftProof{
			   ProofID: uuid.NewString(), LeaseID: l.LeaseID, Pair: l.Pair, Kind: ProofWriteKindMismatch,
			   FailClosed: true, WriteKind: string(req.WriteKind), Summary: "write kind mismatch",
			  OpenedAt: s.clock(), FundWriteHeld: true,
		  })
		   return l, nil, ErrWriteKind
	   }
	   if l.State == LeaseConsumed {
		   return l, nil, ErrLeaseConsumed
	  }
	   if l.State.Terminal() {
		   if l.State == LeaseExpired {
			  return l, nil, ErrLeaseExpired
		   }
		  return l, nil, ErrLeaseRevoked
	  }
	  now := s.clock()
	  t, terr := s.feed.Get(l.Asset, l.Quote)
	  liveGen := s.feed.Generation(l.Asset, l.Quote)
	  rc := RecheckLive(*l, t, terr, liveGen, now, s.feed.MaxAge(), s.policy.DriftBPS)
	   if !rc.OK {
		   to := LeaseRevoked
		   if rc.ProofKind == ProofLeaseExpired || rc.ProofKind == ProofTickStale {
			  to = LeaseExpired
		   }
		   _ = s.revokeInternal(ctx, *l, to, rc.Reason.Error(), rc.ProofKind, rc.Live, rc.LiveGen)
		  got, _ := s.store.GetLease(ctx, l.LeaseID)
		   return got, nil, rc.Reason
	   }
	  // issued → bound
	  if l.State == LeaseIssued {
		  next := *l
		  if err := CanTransit(l.State, LeaseBound); err != nil {
			  return l, nil, err
		   }
		   next.State = LeaseBound
		  next.WriteKind = req.WriteKind
		  next.WriteRef = strings.TrimSpace(req.WriteRef)
		   bound := now
		   next.BoundAt = &bound
		  if err := s.store.CASLease(ctx, l.LeaseID, l.Generation, l.State, next); err != nil {
			  return nil, nil, err
		  }
		  s.audit(ctx, l.LeaseID, l.Pair, "bind", actor, string(LeaseIssued), string(LeaseBound), string(req.WriteKind))
		   l = &next
	   }
	  // 二次复验后再 consumed，防止 bind 夹缝 Tick 翻转。
	  t2, e2 := s.feed.Get(l.Asset, l.Quote)
	   rc2 := RecheckLive(*l,
		   t2, e2, s.feed.Generation(l.Asset, l.Quote), s.clock(), s.feed.MaxAge(), s.policy.DriftBPS)
	  if !rc2.OK {
		   _ = s.revokeInternal(ctx, *l, LeaseRevoked, rc2.Reason.Error(), rc2.ProofKind, rc2.Live, rc2.LiveGen)
		  got, _ := s.store.GetLease(ctx, l.LeaseID)
		   return got, nil, rc2.Reason
	   }
	  next := *l
	  if err := CanTransit(l.State, LeaseConsumed); err != nil {
		   return l, nil, err
	  }
	  cons := s.clock()
	   next.State = LeaseConsumed
	  next.ConsumedAt = &cons
	  next.WriteKind = req.WriteKind
	  next.WriteRef = strings.TrimSpace(req.WriteRef)
	   if err := s.store.CASLease(ctx, l.LeaseID, l.Generation, l.State, next); err != nil {
		  return nil, nil, err
	   }
	   rec := FundWriteRecord{
		   WriteID: uuid.NewString(), LeaseID: l.LeaseID, WriteKind: req.WriteKind,
		  WriteRef: next.WriteRef, Pair: l.Pair, Generation: l.Generation,
		  Posted: true, Actor: actor, PostedAt: cons,
	  }
	  if err := s.store.InsertWrite(ctx, rec); err != nil {
		  return &next, nil, err
	  }
	  s.audit(ctx, l.LeaseID, l.Pair, "consume", actor, string(LeaseBound), string(LeaseConsumed), rec.WriteID)
	   if key := strings.TrimSpace(req.IdemKey); key != "" {
		  _ = s.store.PutIdem(ctx, "write:"+key, l.LeaseID)
	  }
	   return &next, &rec, nil
  }

// BindMarginLTVGeneration 杠杆路径必须与 tick_lease 世代一致。
   func (s *Service) BindMarginLTVGeneration(ctx context.Context, leaseID string, expectedGen int64) error {
	  l, err := s.store.GetLease(ctx, leaseID)
	   if err != nil {
		   return err
	  }
	   if l.ValuationKind != ValLTV {
		   return ErrValuation
	  }
	   if l.Generation != expectedGen {
		   s.openProof(ctx, TickDriftProof{
			  ProofID: uuid.NewString(), LeaseID: l.LeaseID, Pair: l.Pair, Kind: ProofGeneration,
			  FailClosed: true, IssuedGen: l.Generation, LiveGen: expectedGen,
			  Summary: fmt.Sprintf("margin ltv generation %d != %d", l.Generation, expectedGen),
			   OpenedAt: s.clock(),
			  FundWriteHeld: true,
		   })
		   return ErrGeneration
	  }
	  live := s.feed.Generation(l.Asset, l.Quote)
	  if live != 0 && live != l.Generation {
		   s.openProof(ctx, TickDriftProof{
			  ProofID: uuid.NewString(), LeaseID: l.LeaseID, Pair: l.Pair, Kind: ProofGeneration,
			  FailClosed: true, IssuedGen: l.Generation, LiveGen: live,
			   Summary:  "live tick generation drifted after ltv lease",
			   OpenedAt: s.clock(), FundWriteHeld: true,
		   })
		   return ErrGeneration
	   }
	  return nil
   }

// ShadowValuation 探活估值，明确不得签发 Enforce。
  func (s *Service) ShadowValuation(ctx context.Context, req IssueTickLeaseRequest) (*TickLease, error) {
	   req.Mode = ModeShadow
	   return s.IssueTickLease(ctx, req)
  }

// ProbeValuation 探针，不得写。
  func (s *Service) ProbeValuation(ctx context.Context, req IssueTickLeaseRequest) (*TickLease, error) {
	  req.Mode = ModeProbe
	   return s.IssueTickLease(ctx, req)
   }

// MustHaveEnforce 调用点守卫。

  func MustHaveEnforce(l *TickLease) error {
	   if l == nil {
		  return ErrNoLease
	   }
	  if !l.Mode.Writable() {
		   return ErrShadowImpersonate
	  }
	   if l.State.Terminal() && l.State != LeaseConsumed {
		   if l.State == LeaseExpired {
			  return ErrLeaseExpired
		   }
		   return ErrLeaseRevoked
	  }
	   return nil
   }

// RecheckTick 只读复验，不推进状态。
  func (s *Service) RecheckTick(ctx context.Context, leaseID string) RecheckResult {
	  l,
		   err := s.store.GetLease(ctx, leaseID)
	  if err != nil {
		  return RecheckResult{Reason: err, ProofKind: ProofNoLease}
	  }
	  t, terr := s.feed.Get(l.Asset, l.Quote)
	   return RecheckLive(*l, t, terr, s.feed.Generation(l.Asset, l.Quote), s.clock(), s.feed.MaxAge(), s.policy.DriftBPS)
  }

var _ = oracle.ErrMissing
