package cerbind

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/cerdecidegate"
)

// LeaseGate Decide→Exec 租约闸。可空：未挂闸时 Eval 行为不变。
type LeaseGate interface {
	DecideWithLease(ctx context.Context, req cerdecidegate.DecideReq) (cerdecidegate.DecideLease, error)
	AssertLease(ctx context.Context, req cerdecidegate.BindReq) (cerdecidegate.DecideLease, error)
	ConsumeLease(ctx context.Context, req cerdecidegate.ConsumeReq) (cerdecidegate.DecideLease, error)
	RevokeLease(ctx context.Context, req cerdecidegate.RevokeReq) (cerdecidegate.DecideLease, error)
	FlipPolicyFields(ctx context.Context, policyID string, kind cerdecidegate.FlipKind, oldT, newT, oldS, newS int, oldF, newF bool) (int, error)
	RaiseDestRisk(ctx context.Context, batchID, oldRisk, newRisk, policyID string) (int, error)
	ListProofs(ctx context.Context, leaseID string) ([]cerdecidegate.CerDriftProof, error)
}

// LeasedDecision 瞬时 Decision 加上可选 decide_lease_id。
type LeasedDecision struct {
	Decision Decision                  `json:"decision"`
	Lease    cerdecidegate.DecideLease `json:"lease,omitempty"`
	LeaseID string `json:"decide_lease_id,omitempty"`
}

// UseLeaseGate 挂租约闸。可注入独立引擎。

  func (e *Engine) UseLeaseGate(g LeaseGate) {
	   if e == nil {
		   return
	  }
	   e.mu.Lock()
	  e.lease = g
	  e.mu.Unlock()
   }

// LeaseEngine 返回已挂闸，可能为 nil。

   func (e *Engine) LeaseEngine() LeaseGate {
	  if e == nil {
		   return nil
	  }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  return e.lease
  }

// DecideWithLease 先走既有 Eval/DecideSnap，Required 才签发租约。

  func (e *Engine) DecideWithLease(ctx context.Context, req PlanReq, idem string, ttlSec int64) (LeasedDecision, error) {
	  dec, err := e.Eval(ctx, req)
	  if err != nil {
		   return LeasedDecision{}, err
	   }
	   out := LeasedDecision{Decision: dec}
	   if !dec.Required {
		  return out, nil
	   }
	  g := e.LeaseEngine()
	   if g == nil {
		   return out, nil
	   }
	   actor := strings.TrimSpace(req.Actor)
	  if actor == "" {
		   actor = "cer-decide-ops"
	   }
	   if idem == "" {
		  idem = "cer-decide:" + req.BatchID + ":" + dec.PolicyID
	   }
	   ln := lineFactFromPlan(req, dec)
	  lease, err := g.DecideWithLease(ctx, cerdecidegate.DecideReq{
		  Actor:          actor,
		   IdempotencyKey: idem, TTLSec: ttlSec,
		   Line: ln,
		   Pin: cerdecidegate.DecidePin{
			   Required: dec.Required, PolicyID: dec.PolicyID, Threshold: dec.Threshold,
			   Shares: dec.Shares, BindMode: dec.BindMode, DestRisk: dec.DestRisk, Reason: dec.Reason,
		  },
		   SnapDigest: decSnapDigest(req, dec),
	  })
	   if err != nil {
		   return LeasedDecision{}, err
	  }
	   out.Lease = lease
	   out.LeaseID = lease.LeaseID
	   return out, nil
   }

// AssertExecLease Exec/放行前提：必须持未过期租约并复验。

  func (e *Engine) AssertExecLease(ctx context.Context, leaseID, actor, execRef string, sub Subject) (cerdecidegate.DecideLease, error) {
	   g := e.LeaseEngine()
	   if g == nil {
		   return cerdecidegate.DecideLease{},
			  cerdecidegate.ErrUnavailable
	  }
	   ln := cerdecidegate.LineFact{
		   BatchID: sub.BatchID, Asset: sub.Asset, Chain: sub.Chain, Amount: sub.Amount,
		  Dest: sub.Dest, DestRisk: sub.DestRisk, Side: sub.Side, Purpose: sub.Purpose, Approved: sub.Approved,
	   }
	   return g.AssertLease(ctx, cerdecidegate.BindReq{
		   LeaseID: leaseID,
		  Actor:   actor, ExecRef: execRef, Line: ln,
	  })
   }

// NotifyPolicyFlip 策略写入后吊销在途租约。不改 PutPolicy 公式。

   func (e *Engine) NotifyPolicyFlip(ctx context.Context, oldP, newP Policy) {
	  g := e.LeaseEngine()
	   if g == nil {
		  return
	   }
	  kind := cerdecidegate.FlipPolicyPack
	   if oldP.ForceCold != newP.ForceCold {
		   kind = cerdecidegate.FlipForceCold
	  } else if oldP.Threshold != newP.Threshold {
		   kind = cerdecidegate.FlipThreshold
	  } else if oldP.Shares != newP.Shares {
		   kind = cerdecidegate.FlipShares
	   }
	  _,
		  _ = g.FlipPolicyFields(ctx, newP.ID, kind, oldP.Threshold, newP.Threshold, oldP.Shares, newP.Shares, oldP.ForceCold, newP.ForceCold)
  }

  func lineFactFromPlan(req PlanReq, dec Decision) cerdecidegate.LineFact {
	  ln := cerdecidegate.LineFact{
		   BatchID: strings.TrimSpace(req.BatchID),
		   Asset:   firstNonEmpty(dec.Asset, req.Asset),
		  Chain:   firstNonEmpty(dec.Chain, req.Chain),
		   Amount:  firstNonEmpty(dec.Amount, req.Amount),
		  Dest:     firstNonEmpty(dec.Dest, req.Dest),
		   DestRisk: firstNonEmpty(dec.DestRisk, req.DestRisk),
		  Side: firstNonEmpty(dec.Side, req.Side),
		  Purpose:  firstNonEmpty(dec.Purpose, req.Purpose),
		   Approved: dec.Approved || req.Approved,
	  }
	  if ln.BatchID == "" && len(req.Lines) > 0 {
		   ln.BatchID = "line-batch"
	   }
	   return ln
   }

  func decSnapDigest(req PlanReq, dec Decision) string {
	   if req.BatchID != "" {
		   return req.BatchID + ":" + dec.PolicyID
	   }
	   return dec.PolicyID + ":" + dec.Amount
   }

   func firstNonEmpty(a, b string) string {
	   if strings.TrimSpace(a) != "" {
		   return a
	   }
	  return b
  }

// CerbindLive 用既有 DecideSnap 作活源，不改公式。
type CerbindLive struct {
	Eng *Engine
}

// LiveDecide 实现 cerdecidegate.LiveSource。

   func (s CerbindLive) LiveDecide(ln cerdecidegate.LineFact) (cerdecidegate.LiveView, error) {
	  if s.Eng == nil {
		   return cerdecidegate.LiveView{},
			  cerdecidegate.ErrLiveUnavailable
	   }
	  ctx := context.Background()
	  ps, err := s.Eng.Policies(ctx)
	  if err != nil {
		   return cerdecidegate.LiveView{},
			   err
	  }
	   line := Line{
		   Asset: ln.Asset, Chain: ln.Chain, Amount: ln.Amount, Dest: ln.Dest,
		   DestRisk: ln.DestRisk, Side: ln.Side, Purpose: ln.Purpose, Approved: ln.Approved,
	  }
	  dec, err := s.Eng.DecideLine(ps, line)
	  if err != nil {
		  return cerdecidegate.LiveView{}, err
	   }
	  pin := cerdecidegate.DecidePin{
		  Required: dec.Required, PolicyID: dec.PolicyID, Threshold: dec.Threshold,
		  Shares: dec.Shares, BindMode: dec.BindMode, DestRisk: dec.DestRisk, Reason: dec.Reason,
	  }
	   fp := cerdecidegate.ComposeFingerprint(pin, ln, "", "", 1)
	  return cerdecidegate.LiveView{
		   Pin: pin, ForceCold: dec.Required, DestRisk: dec.DestRisk,
		   PackDigest: fp.PackDigest, SnapDigest: fp.SnapDigest, LineDigest: fp.LineDigest,
		   Generation: 1, Fingerprint: fp,
	   }, nil
   }

// AttachLeaseGate 给引擎挂默认内存租约闸。

   func AttachLeaseGate(e *Engine) *cerdecidegate.Engine {
	  if e == nil {
		   return nil
	  }
	   src := CerbindLive{Eng: e}
	  g := cerdecidegate.NewEngine(cerdecidegate.NewMemStore(),
		  src, cerdecidegate.WithMetrics(cerdecidegate.NewMetrics()))
	   e.UseLeaseGate(g)
	   return g
  }
