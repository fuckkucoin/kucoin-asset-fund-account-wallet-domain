package agreeleasegate

import "context"

// PlanAssert 适配 sweeprun.AgreeLeaseAsserter。

   func (e *Engine) PlanAssert(ctx context.Context, leaseID, chain, asset, planRef string) error {
	   res, err := e.PlanAssertLease(ctx, PlanAssertRequest{
		   LeaseID: leaseID,
		   Chain:   chain, Asset: asset, PlanRef: planRef,
	  })
	  if err != nil {
		   return err
	  }
	  if res == nil || !res.Allowed {
		  return ErrAgreeDenied
	   }
	  return nil
  }

// ExecAssert 适配 sweeprun.AgreeLeaseAsserter。
   func (e *Engine) ExecAssert(ctx context.Context, leaseID, chain, asset, execRef string) error {
	   res, err := e.ExecAssertLease(ctx, ExecAssertRequest{
		  LeaseID: leaseID, Chain: chain, Asset: asset, ExecRef: execRef,
	  })
	  if err != nil {
		   return err
	  }
	  if res == nil || !res.Allowed {
		  return ErrAgreeDenied
	  }
	   return nil
   }

// PlanAssertLease 导出名，避免与适配方法冲突。
  func (e *Engine) PlanAssertLease(ctx context.Context, req PlanAssertRequest) (*AssertResult, error) {
	   return e.planAssertInner(ctx, req)
   }

// ExecAssertLease 导出名。

  func (e *Engine) ExecAssertLease(ctx context.Context, req ExecAssertRequest) (*AssertResult, error) {
	   return e.execAssertInner(ctx, req)
   }
