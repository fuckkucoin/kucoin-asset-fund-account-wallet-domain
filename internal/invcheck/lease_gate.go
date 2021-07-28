package invcheck

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/invleasegate"
)

// ErrLeaseEngineUnavailable lease engine 不可用（未注入）。
var ErrLeaseEngineUnavailable = fmt.Errorf("invcheck: lease engine unavailable")

// LeaseEngine 预检租约引擎接口（invleasegate.Engine 的最小接口，避免循环导入）。
type LeaseEngine interface {
	PrecheckBind(ctx context.Context, req invleasegate.PrecheckBindReq) (invleasegate.PrecheckBindResult, error)
}

// LeaseGate DriftGate + 预检租约签发的组合层。
// 出金/归集提交路径应调用 CheckAndBind 而非直接调用 DriftGate.Check。
type LeaseGate struct {
	gate   *DriftGate
	engine LeaseEngine
}

// NewLeaseGate 构造 LeaseGate。
// gate 为 nil 时 Check 阶段放行；engine 为 nil 时 PrecheckBind 阶段返回 ErrLeaseEngineUnavailable。

   func NewLeaseGate(gate *DriftGate, engine LeaseEngine) *LeaseGate {
	   return &LeaseGate{gate: gate, engine: engine}
  }

// CheckAndBindReq CheckAndBind 请求参数。

type CheckAndBindReq struct {
	Asset string
	Actor    string
	LeaseTTL int // 租约有效期（秒），0 表示使用引擎默认值
}

// CheckAndBindResult CheckAndBind 成功结果。

type CheckAndBindResult struct {
	InvPrecheckID string // inv_precheck_id（供上层写入出金/归集提交参数）
	Lease *invleasegate.InvPrecheckLease
}

// CheckAndBind DriftGate.Check + PrecheckBind 的原子组合。
// 流程：
// 1. DriftGate.Check（查询活跃封堵建议）；
// 2. Check 通过 → 调用 LeaseEngine.PrecheckBind 签发租约；
// 3. 任一步失败 → 返回 error，不签发租约。

  func (lg *LeaseGate) CheckAndBind(ctx context.Context, req CheckAndBindReq) (CheckAndBindResult, error) {
	  asset := strings.ToUpper(strings.TrimSpace(req.Asset))
	  // Step 1: 偏离封堵预检
	   if lg.gate != nil {
		   if err := lg.gate.Check(ctx, asset); err != nil {
			  return CheckAndBindResult{}, err
		  }
	  }
	   // Step 2: 签发预检租约
	  if lg.engine == nil {
		  return CheckAndBindResult{}, ErrLeaseEngineUnavailable
	   }
	  bindReq := invleasegate.PrecheckBindReq{
		   Asset:    asset,
		   Actor:    req.Actor,
		  LeaseTTL: time.Duration(req.LeaseTTL) * time.Second,
	   }
	  res, err := lg.engine.PrecheckBind(ctx, bindReq)
	   if err != nil {
		  return CheckAndBindResult{}, err
	  }
	  return CheckAndBindResult{
		   InvPrecheckID: res.Lease.LeaseID,
		  Lease:         res.Lease,
	   }, nil
   }
