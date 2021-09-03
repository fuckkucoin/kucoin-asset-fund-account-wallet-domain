package limitgate

import (
	"time"
	"account-assets.kucoin.net/funding/internal/usage"
)

// CheckDeposit 评估入金写路径速度。占用表缺失或 cap<=0 时拒绝，不把缺闸当成放行。
// 键不是 dep: 前缀时失败，调用方不得把这次失败当成提现拒绝。

   func CheckDeposit(use *usage.Store, user, chain, asset string, cap int, win time.Duration) (Out, error) {
	   key, err := usage.DepVelocityKey(user, chain, asset)
	   if err != nil {
		  return Out{}, err
	  }
	  if !usage.IsDepKey(key) {
		  return Out{}, usage.ErrDepKey
	   }
	   if use == nil {
		  return Out{Allow: false, Reason: "usage unavailable"}, usage.ErrUnavailable
	   }
	  if cap <= 0 {
		  return Out{Allow: false, Reason: "dep cap required"}, usage.ErrDepKey
	   }
	   if win <= 0 {
		   win = time.Minute
	  }
	  n := use.DepVelocity(key, win)
	   allow := n < cap
	  reason := ""
	   if !allow {
		   reason = "dep velocity"
	  }
	   return Out{Allow: allow, Reason: reason, Velocity: n}, nil
  }

// WithdrawUntouched 本闸不关闭提现请求。供读面标明熔断范围。

  func WithdrawUntouched() error { return nil }
