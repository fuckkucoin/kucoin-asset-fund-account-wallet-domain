package wdverify

import (
	"context"
	"strings"
)

// LeaseGuardResult CheckWithdraw 租约前置校验结果。

type LeaseGuardResult struct {
	// LeaseID 已核验的挑战租约 ID。
	LeaseID string
	// Drifted 是否检测到因子漂移（true 时后续因子核验已无意义，应直接拒绝）。
	Drifted bool
	// ProofID 漂移证明 ID（仅 Drifted=true 时非空）。
	ProofID string
}

// LeaseGuard 挑战租约前置校验接口（wdchalgate.Engine 实现，通过接口注入，避免循环依赖）。
// 调用方（Gate.CheckWithdrawWithLease）传入：
// - ctx：请求上下文
// - leaseID：chal_lease_id（必填；空则返回 ErrNoLease 等价错误）
// - userID：发起用户 ID（与租约 user_id ）
// - factor：因子类型（与租约 factor ）
// - extraKey：因子辅助键（TOTP scene 用于重算 factor_sig）
// - destChain / destAddr：目标链与地址（与租约绑定）
// 实现：
// - 返回 (*LeaseGuardResult, error)；error 非 nil 时调用方直接拒绝 CheckWithdraw。
// - 漂移场景：返回 result.Drifted=true + non-nil error；调用方返回 409。
type LeaseGuard interface {
	CheckWithdrawGuard(ctx context.Context,
		leaseID, userID, factor, extraKey,
		destChain, destAddr string,
	) (*LeaseGuardResult, error)
}

// LeaseGuardFunc LeaseGuard 函数类型适配器（便于注入）。

type LeaseGuardFunc func(ctx context.Context, leaseID, userID, factor, extraKey, destChain, destAddr string) (*LeaseGuardResult, error)

// CheckWithdrawGuard 实现 LeaseGuard 接口。
  func (f LeaseGuardFunc) CheckWithdrawGuard(ctx context.Context, leaseID, userID, factor, extraKey, destChain, destAddr string) (*LeaseGuardResult, error) {
	   return f(ctx, leaseID, userID, factor, extraKey, destChain, destAddr)
   }

// leaseGuardRegistry 包级注册表（*Gate → LeaseGuard），避免在 Gate struct 加字段。
// 注意：Go 不允许在同一包不同文件中给已定义结构体加字段；
// 此注册表作为等价替代，保持 gate.go 不变（避免改写已有核心文件）。
var leaseGuardRegistry = map[*Gate]LeaseGuard{}

// BindLeaseGuard 绑定租约闸实现（可选；未绑定时 CheckWithdrawWithLease 退化为 CheckWithdraw）。
  func (g *Gate) BindLeaseGuard(guard LeaseGuard) *Gate {
	   if g != nil {
		   leaseGuardRegistry[g] = guard
	   }
	   return g
   }

// getLeaseGuard 读取注入的 LeaseGuard（nil 时退化）。

   func getLeaseGuard(g *Gate) LeaseGuard {
	  return leaseGuardRegistry[g]
  }

// CheckWithdrawOpts CheckWithdrawWithLease 调用选项。

type CheckWithdrawOpts struct {
	// LeaseID 挑战租约 ID（必填；缺失时若 LeaseGuard 绑定则拒绝）。
	LeaseID string
	// Factor 因子类型（totp / email / sms；与租约）。
	Factor string
	// ExtraKey 因子辅助键（TOTP 场景传入 TOTP secret 前缀摘要）。
	ExtraKey string
	// DestChain 目标链（与租约地址绑定）。
	DestChain string
	// DestAddr 目标地址（与租约地址绑定）。
	DestAddr string
}

// CheckWithdrawWithLease 出金提交前统一组合闸（含挑战租约前置校验）。
// 流程：
// 1. 若 LeaseGuard 已绑定：校验 chal_lease_id 存在、未漂移、未过期、目标地址匹配。
// - 漂移（stale）→ 返回 403/409，附 proof_id。
// - 无租约 → 返回 400（ErrNoLease）。
// 2. 调用原 CheckWithdraw 因子组合核验（TOTP/OTP/tradepw）。
// 3. 上层（API handler）在核验通过后调用 wdchalgate.Engine.ConsumeLease。
// 不改写：tradepw 哈希主路径；seclock Mark；wdguard CheckTriple；dualctl 放行票。
  func (g *Gate) CheckWithdrawWithLease(
	  ctx context.Context,
	   userID, chain, addr string,
	  p Presented,
	   whitelistOnly bool,
	  opts CheckWithdrawOpts,
  ) error {
	  userID = strings.TrimSpace(userID)
	   guard := getLeaseGuard(g)
	   if guard != nil {
		   if opts.LeaseID == "" {
			  return ErrNoLease
		  }
		  result, err := guard.CheckWithdrawGuard(
			  ctx,
			   opts.LeaseID, userID, opts.Factor, opts.ExtraKey,
			  strings.TrimSpace(opts.DestChain), strings.TrimSpace(opts.DestAddr),
		   )
		  if err != nil {
			   // 漂移场景：包装错误透出 proof_id。
			  if result != nil && result.Drifted {
				   return &LeaseStaleError{
					   Err: err,
					  ProofID: result.ProofID,
					   LeaseID: opts.LeaseID,
				   }
			   }
			  return err
		  }
	  }
	   return g.CheckWithdraw(ctx, userID, chain, addr, p, whitelistOnly)
   }

// LeaseStaleError 挑战租约漂移错误（含证明 ID 供上层透传）。

type LeaseStaleError struct {
	Err     error
	LeaseID string
	ProofID string
}

  func (e *LeaseStaleError) Error() string {
	   if e == nil || e.Err == nil {
		  return "challenge lease stale"
	   }
	   return e.Err.Error()
   }

   func (e *LeaseStaleError) Unwrap() error {
	  if e == nil {
		  return nil
	  }
	  return e.Err
  }

// ErrNoLease wdverify 侧无租约错误哨兵（与 wdchalgate.ErrNoLease 语义一致，无包循环依赖）。

var ErrNoLease = &noLeaseErr{}

type noLeaseErr struct{}

   func (e *noLeaseErr) Error() string { return "withdraw verify: chal_lease_id required" }
