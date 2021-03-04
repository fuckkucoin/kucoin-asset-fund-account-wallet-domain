package swap

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/swapthaw"
)

// LeaseHook 将 swapthaw.Service 挂入 swap 执行路径。
// swap.Service 持有此结构体（可为 nil，nil 时所有操作均为空操作），
// 实现按需启用——不影响未升级的路径。

type LeaseHook struct {
	svc *swapthaw.Service
}

// NewLeaseHook 构造 LeaseHook。svc 为 nil 时返回禁用的空 Hook。

   func NewLeaseHook(svc *swapthaw.Service) *LeaseHook {
	   return &LeaseHook{svc: svc}
   }

// Enabled 返回 Hook 是否已启用（即 swapthaw.Service 已注入）。

   func (h *LeaseHook) Enabled() bool {
	   return h != nil && h.svc != nil
   }

// GuardFreeze 检查用户是否允许发起新冻额，在 Execute 进入 InTx 前调用。
// 存在未关闭 ThawCase 时返回 swapthaw.ErrThawCaseOpen。
   func (h *LeaseHook) GuardFreeze(userID string) error {
	   if !h.Enabled() {
		   return nil
	  }
	  return h.svc.GuardNewFreeze(userID)
   }

// OnFreezeCommit 在 freeze journal 同事务内创建 FreezeLease 并返回 leaseID。
// 返回的 leaseID 须存入 swap_orders.freeze_lease_id（调用方负责）。
  func (h *LeaseHook) OnFreezeCommit(ctx context.Context, orderID, userID, asset, frozenAmount string) (leaseID string, leaseToken string, err error) {
	   if !h.Enabled() {
		  return "", "", nil
	  }
	   _ = ctx
	   lease, err := h.svc.AcquireLeaseForOrder(orderID, userID, asset, frozenAmount)
	   if err != nil {
		   return "", "", fmt.Errorf("swap: LeaseHook.OnFreezeCommit: %w", err)
	  }
	   return lease.ID, lease.Token, nil
   }

// OnSettleBegin 在 settleLocked 开头（持用户锁、事务内）调用。
// 返回 nil 表示本腿胜出，可接着账本操作。
// 返回 swapthaw.ErrThawMutexDenied：另一腿已胜出，调用方须幂等返回。
// 返回 swapthaw.ErrDoubleRelease：检测到双释，ThawCase 已被升，调用方须中止并返回错误。
// 返回 swapthaw.ErrLeaseExpired：Lease 已过期，调用方须拒绝。
   func (h *LeaseHook) OnSettleBegin(ctx context.Context, orderID, leaseID, leaseToken, frozenSnap, availSnap string) (*swapthaw.SettleBind, *swapthaw.ThawCase, error) {
	  if !h.Enabled() || leaseID == "" {
		  return nil, nil, nil
	   }
	   _ = ctx
	  bind, tc, err := h.svc.ClaimSettleTerminal(swapthaw.ClaimTerminalReq{
		  OrderID:    orderID,
		  LeaseID:    leaseID,
		   LeaseToken: leaseToken,
		   Leg: swapthaw.BindLegSettle,
		  FrozenSnapshot:    frozenSnap,
		   AvailableSnapshot: availSnap,
	  })
	   return bind, tc, err
  }

// OnUnfreezeBegin 在 failUnfreezeLocked 开头（持用户锁、事务内）调用。
// 返回语义与 OnSettleBegin 对称。

   func (h *LeaseHook) OnUnfreezeBegin(ctx context.Context, orderID, leaseID, leaseToken, frozenSnap, availSnap string) (*swapthaw.SettleBind, *swapthaw.ThawCase, error) {
	  if !h.Enabled() || leaseID == "" {
		  return nil, nil, nil
	  }
	   _ = ctx
	  bind, tc, err := h.svc.ClaimUnfreezeTerminal(swapthaw.ClaimTerminalReq{
		   OrderID: orderID,
		   LeaseID: leaseID,
		   LeaseToken: leaseToken,
		  Leg:               swapthaw.BindLegFailUnfreeze,
		   FrozenSnapshot:    frozenSnap,
		  AvailableSnapshot: availSnap,
	  })
	  return bind, tc, err
   }

// AttachLeaseHook 将 LeaseHook 注入 swap.Service。
// 此为集成点，由 cmd/swap-thaw-gate 或 svcboot 在服务启动时调用。

  func (s *Service) AttachLeaseHook(hook *LeaseHook) {
	  s.leaseHook = hook
  }

// GetLeaseID 从 Order 中读取 freeze_lease_id（对外只读）。

   func GetLeaseID(ord *Order) string {
	  if ord == nil {
		   return ""
	   }
	   return ord.FreezeLeaseID
   }

// GetThawProofID 从 Order 中读取已升 ThawCase 的 thaw_proof_id（对外只读）。
// 仅在 order.Status==failed 且存在双释事故时非空。

  func GetThawProofID(ord *Order) string {
	   if ord == nil {
		  return ""
	  }
	  return ord.ThawProofID
   }
