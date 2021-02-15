package agreeleasegate

import (
	"context"
	"github.com/google/uuid"
)

// AssertResult Plan/Exec 复验结果。

type AssertResult struct {
	Lease *AgreeLease
	Allowed bool
	RejectReason string
	DriftProof   *AgreeDriftProof
}

// PlanAssertRequest 计划落库绑定。

type PlanAssertRequest struct {
	LeaseID string
	Chain   string
	Asset   string
	PlanRef string
}

// ExecAssertRequest 执行腿推进绑定。
type ExecAssertRequest struct {
	LeaseID string
	Chain string
	Asset string
	ExecRef string
	Consume bool
}

// planAssertInner 计划落库闸。缺租约 fail-closed。

   func (e *Engine) planAssertInner(ctx context.Context, req PlanAssertRequest) (*AssertResult, error) {
	  lease, missProof, err := e.loadAndGuard(ctx, req.LeaseID, req.Chain, req.Asset, req.PlanRef, "", e.cfg.AllowPlanWithoutLease)
	  if err != nil {
		   reason := err.Error()
		   if errorsIsNoLease(err) {
			   reason = "missing_lease"
		  }
		   if err == ErrProofBlocked {
			  reason = "proof_blocked"
		  }
		   return &AssertResult{Lease: lease,
			  Allowed: false, RejectReason: reason, DriftProof: missProof}, err
	   }
	   if proof, rerr := e.reverifyOrRevoke(ctx, lease); rerr != nil {
		   return &AssertResult{Lease: lease, Allowed: false, RejectReason: rerr.Error(), DriftProof: proof}, rerr
	   }
	   now := e.now()
	  switch lease.State {
	   case LeaseStateAgreed:
		   if err := e.transit(lease, LeaseStatePlanned); err != nil {
			   return nil, err
		  }
		   t := now
		   lease.PlannedAt = &t
		  lease.PlanRef = req.PlanRef
		  if err := e.store.UpdateLease(ctx, lease); err != nil {
			  return nil, err
		  }
		   e.audit(ctx, lease.ID, string(LeaseStateAgreed), string(LeaseStatePlanned), "engine", "plan")
	  case LeaseStatePlanned, LeaseStateExecuting:
		  if req.PlanRef != "" && lease.PlanRef != "" && lease.PlanRef != req.PlanRef {
			  return nil, ErrDoubleBind
		   }
	   default:
		  return nil, ErrInvalidTransit
	  }
	  incPlanBound()
	   return &AssertResult{Lease: lease, Allowed: true}, nil
   }

// execAssertInner 执行腿推进闸。漂移后不得 appendLink/放腿。

   func (e *Engine) execAssertInner(ctx context.Context, req ExecAssertRequest) (*AssertResult, error) {
	  lease, missProof, err := e.loadAndGuard(ctx, req.LeaseID, req.Chain, req.Asset, "", req.ExecRef, e.cfg.AllowExecWithoutLease)
	  if err != nil {
		  reason := err.Error()
		  if errorsIsNoLease(err) {
			   reason = "missing_lease"
		   }
		   if err == ErrProofBlocked {
			   reason = "proof_blocked"
		  }
		  return &AssertResult{Lease: lease, Allowed: false, RejectReason: reason, DriftProof: missProof}, err
	   }
	  if lease.State == LeaseStateAgreed {
		  // 未先 Plan 的 Exec：fail-closed，避免跳过计划绑定。
		  proof, _ := e.openProof(ctx, lease, DriftHalfPlan, LiveView{Reason: "exec_before_plan"})
		   _ = e.blockHalfPlan(ctx, lease, "exec_before_plan")
		  return &AssertResult{Lease: lease, Allowed: false, RejectReason: "exec_before_plan", DriftProof: proof}, ErrHalfPlanBlocked
	  }
	  if proof, rerr := e.reverifyOrRevoke(ctx, lease); rerr != nil {
		  _ = e.blockHalfPlan(ctx,
			  lease, string(reasonOf(DriftBal)))
		  return &AssertResult{Lease: lease, Allowed: false, RejectReason: rerr.Error(), DriftProof: proof}, rerr
	  }
	  now := e.now()
	   from := lease.State
	   if lease.State == LeaseStatePlanned {
		   if err := e.transit(lease, LeaseStateExecuting); err != nil {
			  return nil, err
		  }
		   t := now
		  lease.ExecutingAt = &t
		   if req.ExecRef != "" {
			  lease.ExecRef = req.ExecRef
		  }
		   if err := e.store.UpdateLease(ctx, lease); err != nil {
			   return nil, err
		   }
		  e.audit(ctx, lease.ID, string(from), string(LeaseStateExecuting), "engine", "exec")
		   from = LeaseStateExecuting
	   } else if lease.State == LeaseStateExecuting && req.ExecRef != "" && lease.ExecRef != "" && lease.ExecRef != req.ExecRef {
		   return nil, ErrExecMismatch
	   }
	   consume := req.Consume || e.cfg.ConsumeOnExec
	   if consume && lease.State == LeaseStateExecuting {
		  if err := e.transit(lease, LeaseStateConsumed); err != nil {
			  return nil, err
		   }
		   t := now
		   lease.ConsumedAt = &t
		   if req.ExecRef != "" {
			  lease.ExecRef = req.ExecRef
		   }
		  if err := e.store.UpdateLease(ctx, lease); err != nil {
			   return nil, err
		  }
		   e.audit(ctx, lease.ID, string(from), string(LeaseStateConsumed), "engine", "consume")
		   incLeaseConsumed()
	   }
	   incExecBound()
	  return &AssertResult{Lease: lease, Allowed: true}, nil
   }

  func errorsIsNoLease(err error) bool {
	   return err == ErrNoLease
  }

  func (e *Engine) blockHalfPlan(ctx context.Context, lease *AgreeLease, reason string) error {
	   if lease == nil {
		   return nil
	  }
	  if _, err := e.store.GetHalfPlanByLease(ctx, lease.ID); err == nil {
		  return nil
	  }
	  h := &HalfPlanBlock{
		   ID:      uuid.NewString(),
		   LeaseID: lease.ID, PlanRef: lease.PlanRef,
		  Asset: lease.Asset, Reason: reason, Blocked: true, CreatedAt: e.now(),
	  }
	   if err := e.store.InsertHalfPlan(ctx, h); err != nil {
		  return err
	  }
	  incHalfPlan()
	   return nil
  }
