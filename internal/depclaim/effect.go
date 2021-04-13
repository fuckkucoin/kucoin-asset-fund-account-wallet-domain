package depclaim

import (
	"context"
	"errors"
	"account-assets.kucoin.net/funding/internal/claimmutex"
)

// EffectGuard 认领效果世代守卫，集成到认领主路径。
type EffectGuard struct {
	mutex *claimmutex.Service
}

// NewEffectGuard 创建效果守卫（mutex 可为 nil，退化为无互斥模式）。
  func NewEffectGuard(mutex *claimmutex.Service) *EffectGuard {
	   return &EffectGuard{mutex: mutex}
   }

// AcquireOutputEffect 首次 Claim 成功后调用，抢占输出效果租约。
// 若同输出另一腿（自动贷记）已持有租约，返回 ErrSecond；
// 若 mutex 为 nil，直接返回 nil（降级运行）。

  func (g *EffectGuard) AcquireOutputEffect(ctx context.Context, req ClaimReq, actorID string) (*claimmutex.OutputLease, error) {
	  if g == nil || g.mutex == nil {
		   return nil, nil
	  }
	  key := claimmutex.OutputKey{
		   Chain:       req.Chain,
		  TxHash:      req.TxHash,
		  OutputIndex: req.OutputIndex,
	  })
	   if err != nil {
		   if errors.Is(err, claimmutex.ErrSecondClaim) || errors.Is(err, claimmutex.ErrMutexConflict) {
			  return nil, ErrSecond
		   }
		  return nil, err
	  }
	   return lease, nil
   }

// GuardSubsequent 后续同输出动作（如 Recovery 推进）核验效果世代。
// leaseID 为首次 AcquireOutputEffect 返回的 lease_id；
// generation 为 lease.Generation；
// 若核验失败返回 ErrSecond（拒绝第二次入账）。

   func (g *EffectGuard) GuardSubsequent(ctx context.Context, req ClaimReq, leaseID string, generation int64) error {
	  if g == nil || g.mutex == nil {
		   return nil
	   }
	  key := claimmutex.OutputKey{
		   Chain:       req.Chain,
		  TxHash:      req.TxHash,
		   OutputIndex: req.OutputIndex,
	   }
	  _, err := g.mutex.GuardOutput(ctx, claimmutex.GuardRequest{
		   OutputKey: key,
		  LeaseID: leaseID,
		   Generation: generation,
	   })
	   if err != nil {
		   if errors.Is(err, claimmutex.ErrSecondClaim) ||
			  errors.Is(err,
				  claimmutex.ErrGenerationMismatch) ||
			  errors.Is(err, claimmutex.ErrLeaseExpired) {
			   return ErrSecond
		   }
		  return err
	  }
	  return nil
   }

// RegisterAutoCredit 自动贷记管道注册绑定（只读消费，不改写自动贷记主链）。
   func (g *EffectGuard) RegisterAutoCredit(ctx context.Context, req claimmutex.RegisterAutoCreditRequest) (*claimmutex.AutoCreditBind, error) {
	  if g == nil || g.mutex == nil {
		  return nil, nil
	   }
	   bind, err := g.mutex.RegisterAutoCredit(ctx, req)
	   if err != nil {
		   if errors.Is(err, claimmutex.ErrMutexConflict) {
			  return bind, ErrSecond
		  }
		   return nil, err
	  }
	   return bind, nil
   }
