package p2pgate

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/p2pabandon"
	"github.com/shopspring/decimal"
)

// AbandonBridge 实现 p2pabandon.FundsBridge，将弃单资金释放委托给 p2pgate.Funds。
// 部署中将 AbandonBridge 注入 p2pabandon.AbandonAdvWorker。

type AbandonBridge struct {
	funds *Funds
	svc   *p2pabandon.Service
}

// NewAbandonBridge 构造弃单资金桥。
   func NewAbandonBridge(funds *Funds, svc *p2pabandon.Service) *AbandonBridge {
	  return &AbandonBridge{funds: funds, svc: svc}
  }

// ReleaseSellHold 实现 p2pabandon.FundsBridge。
// 幂等键：p2p-abandon-rel:{planID}，确保同一弃单计划的保证金只释放一次。
// 账本贷记 + holdcat 释放原子性由 Funds.Release 保证（见 funds.go）。

   func (b *AbandonBridge) ReleaseSellHold(
	  ctx context.Context,
	  planID, orderID, userID, asset, amount, holdID string,
  ) (ledgerRef, holdRef string, err error) {
	  if b == nil || b.funds == nil {
		   return "", "", ErrHold
	  }
	   planID = strings.TrimSpace(planID)
	  holdID = strings.TrimSpace(holdID)
	   // 幂等账本贷记：p2p-abandon-rel:{planID}。
	  amt, parseErr := decimal.NewFromString(strings.TrimSpace(amount))
	   if parseErr != nil || !amt.IsPositive() {
		  return "", "", ErrAmount
	  }
	  uid := strings.TrimSpace(userID)
	  as := strings.ToUpper(strings.TrimSpace(asset))
	   if b.funds.led != nil {
		  tr, ledErr := b.funds.led.Create(ctx, ledger.CreateReq{
			  UserID:    uid,
			  Direction: ledger.P2PCredit,
			   Asset: as,
			   Amount: amt,
			   IdempotencyKey: "p2p-abandon-rel:" + planID,
		  })
		   if ledErr != nil {
			  return "", "", ledErr
		   }
		  ledgerRef = tr.ID
	  }
	  // 释放 holdcat hold（幂等：已释放直接返回）。
	  if b.funds.hold != nil {
		   row,
			   relErr := b.funds.hold.Release(ctx, uid, holdID)
		   if relErr != nil && relErr != holdcat.ErrGone {
			  return ledgerRef, "", relErr
		   }
		  holdRef = row.ID
	   }
	   return ledgerRef, holdRef, nil
  }

// ─────────────────────────── FundsAbandoned 弃单感知释放 ───────────────────────────
// FundsAbandoned 弃单感知的 hold 释放：
// 若该 hold 关联 AbandonPlan 且计划处于 abandoning/armed 状态，
// 则走 CommitFill 路径（视为 fill 先抢占），阻止弃单 worker 再次释放。
// 若无关联计划，直接走 Release 幂等路径（与原有 p2pgate.Release 语义相同）。
// 此方法由 P2P 成交贷记路径调用（在 fill 贷记成功后调用）。

  func (f *Funds) FundsAbandoned(
	   ctx context.Context,
	   userID, holdID, orderID string,
	   abandonSvc *p2pabandon.Service,
	  ledgerRef string,
  ) error {
	  if f == nil || f.hold == nil {
		  return ErrHold
	  }
	   orderID = strings.TrimSpace(orderID)
	  holdID = strings.TrimSpace(holdID)
	   // 若 abandonSvc 已注入，检查是否存在关联计划。
	   if abandonSvc != nil && orderID != "" {
		   plan, planErr := abandonSvc.GetPlanByOrder(orderID)
		  if planErr == nil && !plan.Status.Terminal() {
			   // 通过 CommitFill 占位（fill 先路径）。
			  _, _, commitErr := abandonSvc.CommitFill(p2pabandon.CommitFillReq{
				  PlanID:    plan.ID,
				   LedgerRef: ledgerRef,
				  Actor: "p2pgate_fill",
			  })
			  if commitErr == p2pabandon.ErrTokenConflict {
				   // 竞态冲突：弃单 worker 已先完成释放；fill 路径需走冲突流程。
				  return ErrAbandonConflict
			  }
			   if commitErr != nil && commitErr != p2pabandon.ErrPlanTerminal {
				   return commitErr
			   }
			   // CommitFill 成功（或计划已终态）：不再走 holdcat.Release（资金已由弃单 worker 处理）。
			  return nil
		  }
	   }
	  // 无关联计划或计划已终态：直接走幂等释放。
	  return nil
   }

// ─────────────────────────── 弃单状态查询 ───────────────────────────
// AbandonStatus 弃单状态摘要（资金页展示）。

type AbandonStatus struct {
	OrderID    string                `json:"order_id"`
	HasPlan    bool                  `json:"has_plan"`
	PlanID     string                `json:"plan_id,omitempty"`
	PlanStatus p2pabandon.PlanStatus `json:"plan_status,omitempty"`
	HasProof bool `json:"has_proof"`
	ProofID string                  `json:"proof_id,omitempty"`
	Outcome p2pabandon.CloseOutcome `json:"outcome,omitempty"`
	// HoldAgeSecs 当前 hold 年龄（秒），0 表示无超龄记录。
	HoldAgeSecs int64               `json:"hold_age_secs"`
	AgeStage    p2pabandon.AgeStage `json:"age_stage,omitempty"`
	CanClose bool `json:"can_close"`
}

// QueryAbandonStatus 查询指定卖单的弃单状态（只读，资金页邻接）。
   func (f *Funds) QueryAbandonStatus(
	   ctx context.Context,
	  orderID string,
	  abandonSvc *p2pabandon.Service,
   ) (AbandonStatus, error) {
	  if f == nil {
		   return AbandonStatus{}, ErrHold
	  }
	  orderID = strings.TrimSpace(orderID)
	  status := AbandonStatus{OrderID: orderID}
	  if abandonSvc == nil {
		   return status, nil
	  }
	   plan,
		  err := abandonSvc.GetPlanByOrder(orderID)
	  if err != nil {
		   return status, nil // 无计划，返回空状态
	  }
	  status.HasPlan = true
	  status.PlanID = plan.ID
	   status.PlanStatus = plan.Status
	  // 查询超龄状态。
	   ageSummary, ageErr := abandonSvc.GetHoldAgeSummary(plan.ID)
	  if ageErr == nil {
		  status.HoldAgeSecs = ageSummary.HoldAgeSecs
		   status.AgeStage = ageSummary.Stage
	  }
	  // 查询收口证明。
	   proof, proofErr := abandonSvc.GetProofByPlan(plan.ID)
	  if proofErr == nil {
		  status.HasProof = true
		   status.ProofID = proof.ID
		  status.Outcome = proof.Outcome
	  }
	  // 收口校验。
	   validateResp, validateErr := abandonSvc.ValidateClose(p2pabandon.ValidateCloseReq{
		   PlanID: plan.ID, OrderID: orderID,
	   })
	   if validateErr == nil || validateErr == p2pabandon.ErrNoProof {
		  status.CanClose = validateResp.CanClose
	   }
	   return status, nil
  }

// ─────────────────────────── 超时信号注入辅助 ───────────────────────────
// InjectTimeoutSignal 将卖单超时信号注入 p2pabandon.Service（创建 AbandonPlan）。
// 设计：由 P2P 订单管理服务在卖单超时时调用；本包不写撮合，仅转接信号。
// 幂等键：arm-plan:{orderID}。
  func (f *Funds) InjectTimeoutSignal(
	   ctx context.Context,
	   sig p2pabandon.OrderTimeoutSignal,
	  abandonSvc *p2pabandon.Service,
   ) (p2pabandon.AbandonPlan, error) {
	   if abandonSvc == nil {
		  return p2pabandon.AbandonPlan{}, ErrHold
	  }
	   return abandonSvc.ArmPlan(p2pabandon.ArmPlanReq{
		  OrderID: sig.OrderID,
		   UserID:         sig.UserID,
		  Asset:          sig.Asset,
		   Amount:         sig.Amount,
		   HoldID:         sig.HoldID,
		  IdempotencyKey: "arm-plan:" + sig.OrderID,
	  })
  }

// ─────────────────────────── holdcat 只读观察器实现 ───────────────────────────
// HoldCatObserver 基于 holdcat.Book 实现 p2pabandon.HoldObserver 只读接口。
// 不写 holdcat 核心 FSM；仅读取 hold 状态。
type HoldCatObserver struct {
	book *holdcat.Book
}

// NewHoldCatObserver 构造 HoldCatObserver。

   func NewHoldCatObserver(book *holdcat.Book) *HoldCatObserver {
	  return &HoldCatObserver{book: book}
  }

   func (h *HoldCatObserver) ObserveHold(ctx context.Context, userID, holdID string) (p2pabandon.HoldObservation, error) {
	   if h == nil || h.book == nil {
		  return p2pabandon.HoldObservation{HoldID: holdID}, nil
	  }
	   row, err := h.book.Get(ctx, userID, holdID)
	   if err != nil {
		   return p2pabandon.HoldObservation{HoldID: holdID}, err
	   }
	  createdAt, _ := time.Parse(time.RFC3339, row.CreatedAt)
	  return p2pabandon.HoldObservation{
		  HoldID: row.ID,
		  UserID: row.UserID,
		   Asset:  row.Asset,
		  Amount: row.Amount,
		   Reason: row.Reason,
		  Open:      row.Open,
		  CreatedAt: createdAt,
	  }, nil
  }

   func (h *HoldCatObserver) ObserveOpenHolds(ctx context.Context, userID string) ([]p2pabandon.HoldObservation, error) {
	  if h == nil || h.book == nil {
		   return nil, nil
	   }
	  rows, err := h.book.List(ctx, userID, true)
	  if err != nil {
		  return nil, err
	   }
	   out := make([]p2pabandon.HoldObservation, 0, len(rows))
	  for _, row := range rows {
		  createdAt, _ := time.Parse(time.RFC3339, row.CreatedAt)
		  out = append(out, p2pabandon.HoldObservation{
			   HoldID: row.ID,
			  UserID: row.UserID,
			   Asset:  row.Asset,
			   Amount: row.Amount,
			   Reason: row.Reason,
			  Open:      row.Open,
			  CreatedAt: createdAt,
		  })
	   }
	   return out, nil
   }

  func (h *HoldCatObserver) HoldBirth(ctx context.Context, planID, holdID string) (time.Time, error) {
	   if h == nil || h.book == nil {
		   return time.Time{}, nil
	   }
	   row, err := h.book.Get(ctx, "", holdID)
	  if err != nil {
		  return time.Time{}, err
	   }
	  t,
		  err := time.Parse(time.RFC3339, row.CreatedAt)
	  if err != nil {
		  return time.Time{}, err
	   }
	   return t, nil
  }

// ErrAbandonConflict 弃单与成交竞态冲突错误（由 FundsAbandoned 返回）。
var ErrAbandonConflict = p2pabandon.ErrTokenConflict
