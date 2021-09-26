package ledshard

import (
	"context"
	"account-assets.kucoin.net/funding/internal/account"
)

// SettleHop 给同用户 hop 用。同分片只返回计划，不冻结、不开凭证。
// 跨分片则打开意图并跑到提交。重放沿用原幂等键，不会第二次贷记。

  func (e *Engine) SettleHop(user, from, to, asset, amount, idem string) (HopPlan, error) {
	  if e == nil {
		  return HopPlan{}, ErrRouteMissing
	  }
	   e.mu.Lock()
	   route, err := e.pickRoute(0)
	   if err != nil {
		  e.mu.Unlock()
		   return HopPlan{}, err
	   }
	  src, err := e.assignFresh(route, user, from, asset, true)
	  if err != nil {
		  e.mu.Unlock()
		   return HopPlan{}, err
	   }
	   dst,
		  err := e.assignFresh(route, user, to, asset, true)
	   if err != nil {
		   e.mu.Unlock()
		   return HopPlan{}, err
	   }
	  plan := HopPlan{
		   SameShard: SamePlacement(src, dst),
		   SrcShard:  src.ShardID, DstShard: dst.ShardID, Version: route.Version,
	  }
	  e.mu.Unlock()
	  if plan.SameShard {
		   plan.State = "short_circuit"
		   return plan, nil
	   }
	  it, err := e.Open(OpenReq{
		   Idem: idem, SrcUser: user, SrcBook: from, DstUser: user, DstBook: to,
		  Asset: asset, Amount: amount,
	   })
	   if err != nil {
		   return plan, err
	   }
	   if !it.State.Terminal() {
		   it, err = e.Run(it.ID)
		   if err != nil {
			  plan.IntentID = it.ID
			  plan.State = string(it.State)
			  return plan, err
		  }
	  }
	   plan.IntentID = it.ID
	  plan.State = string(it.State)
	   return plan, nil
  }

// ClassifyHop 只解析，不写意图。同用户落在同一片时账户 hop 可以走原路径。
   func (e *Engine) ClassifyHop(_ context.Context, user, from, to, asset string) (account.ShardClass, error) {
	  if e == nil {
		   return account.ShardClass{}, ErrRouteMissing
	  }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   route, err := e.pickRoute(0)
	  if err != nil {
		  return account.ShardClass{}, err
	   }
	   src,
		   err := Resolve(route, user, from, asset)
	   if err != nil {
		   return account.ShardClass{}, err
	   }
	  dst, err := Resolve(route, user, to, asset)
	  if err != nil {
		  return account.ShardClass{}, err
	   }
	  return account.ShardClass{
		   SameShard: SamePlacement(src, dst),
		   SrcShard: src.ShardID,
		   DstShard: dst.ShardID,
		   Version: route.Version,
	  }, nil
   }
