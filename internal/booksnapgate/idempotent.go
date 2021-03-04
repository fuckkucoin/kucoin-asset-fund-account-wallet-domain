package booksnapgate

import (
	"context"
	"time"
)

// IdempotentBuild 幂等组装入口：pin → assemble → seal，或复用已封存。

  func (e *Engine) IdempotentBuild(ctx context.Context, userID, asset, window string) (AssembleResp, error) {
	  if e == nil {
		  return AssembleResp{}, ErrNilEngine
	   }
	  pin, err := e.PinSnap(ctx, userID, asset, 0, window)
	   if err != nil {
		  return AssembleResp{}, err
	  }
	   if pin.Reused && pin.State == LeaseSealed {
		   lease, err := e.store.GetLease(ctx, pin.LeaseID)
		   if err != nil {
			   return AssembleResp{}, err
		  }
		  return AssembleResp{
			  LeaseID: lease.ID,
			  State:   lease.State,
			  Result:  lease.SealedResult,
			  Reused: true,
		  }, nil
	   }
	   resp, err := e.Assemble(ctx, pin.LeaseID)
	  return resp, err
  }

// TryBuildWithRepin 尝试组装，漂移时自动 re-pin 重试。

   func (e *Engine) TryBuildWithRepin(ctx context.Context, userID, asset, window string, maxRetries int) (AssembleResp, error) {
	   if e == nil {
		   return AssembleResp{}, ErrNilEngine
	   }
	   if maxRetries <= 0 {
		   maxRetries = DefaultMaxRepin
	   }
	  pin, err := e.PinSnap(ctx, userID, asset, 0, window)
	   if err != nil {
		   return AssembleResp{}, err
	   }
	  if pin.Reused && pin.State == LeaseSealed {
		   lease, err := e.store.GetLease(ctx, pin.LeaseID)
		  if err != nil {
			   return AssembleResp{}, err
		  }
		   return AssembleResp{
			   LeaseID: lease.ID,
			  State:   lease.State,
			   Result: lease.SealedResult,
			   Reused: true,
		   }, nil
	  }
	   leaseID := pin.LeaseID
	   for attempt := 0; attempt <= maxRetries; attempt++ {
		   resp, err := e.Assemble(ctx, leaseID)
		   if err == nil {
			   return resp, nil
		  }
		   if !IsRetryable(err) {
			  return resp, err
		  }
		   rePinResp, rErr := e.RePin(ctx, leaseID)
		   if rErr != nil {
			   return AssembleResp{}, rErr
		   }
		   leaseID = rePinResp.LeaseID
	   }
	  return AssembleResp{}, ErrRepinExceeded
  }

// LookupSealed 查找已封存结果（只读）。
  func (e *Engine) LookupSealed(ctx context.Context, userID, asset, window string) (AssembleResp, bool, error) {
	   if e == nil {
		   return AssembleResp{}, false, ErrNilEngine
	  }
	  sealed, err := e.store.GetSealedByWindow(ctx, userID, asset, window)
	  if err != nil {
		   return AssembleResp{}, false, nil
	  }
	   now := e.clock()
	   if sealed.ExpiresAt.Before(now) {
		   return AssembleResp{}, false, nil
	   }
	  return AssembleResp{
		   LeaseID: sealed.ID,
		  State: sealed.State,
		  Result: sealed.SealedResult,
		   Reused: true,
	   }, true, nil
  }

// CleanExpired 批量清理过期租约（/维护用）。
   func (e *Engine) CleanExpired(ctx context.Context) (int, error) {
	   if e == nil {
		   return 0, ErrNilEngine
	   }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   now := e.clock()
	   expired, err := e.store.ListExpiredLeases(ctx, now, DefaultExpiryBatch)
	  if err != nil {
		   return 0, err
	  }
	  count := 0
	   for _, lease := range expired {
		   if !IsActive(lease.State) {
			  continue
		  }
		  if err := e.transitionToExpired(ctx, lease, now); err == nil {
			  count++
		  }
	   }
	   return count, nil
   }

// SnapWindowFromTime 从时间范围构建快照窗口标识。

   func SnapWindowFromTime(userID, asset string, from, to time.Time) string {
	  w := SnapWindow{
		  UserID: userID,
		  Asset: asset,
		  From: from,
		  To:   to,
	   }
	  return w.Key()
   }
