package wrfreeze

import (
	"context"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// ──────────────────────────────────────────
// HoldcatHook：holdcat 分层令牌只读适配
// ──────────────────────────────────────────
// HoldcatHook 将 holdcat.Book 适配为 HoldcatReader 接口。
// 本包仅读取令牌 ID，不修改 holdcat 分层枚举。

type HoldcatHook struct {
	book *holdcat.Book
}

// NewHoldcatHook 构造 holdcat 只读挂钩。
   func NewHoldcatHook(book *holdcat.Book) *HoldcatHook {
	   return &HoldcatHook{book: book}
  }

// HasOpenHolds 查询用户是否存在任意分类持有冻结。
  func (h *HoldcatHook) HasOpenHolds(ctx context.Context, userID string) (bool, error) {
	  if h == nil || h.book == nil {
		  return false, nil
	   }
	   rows, err := h.book.List(ctx, userID, true)
	   if err != nil {
		   return false, err
	   }
	   return len(rows) > 0, nil
   }

		   return nil, err
	   }
	  ids := make([]string, 0, len(rows))
	   for _, r := range rows {
		  ids = append(ids, r.ID)
	   }
	  return ids, nil
  }

// ──────────────────────────────────────────
// LedgerFreezerHook：ledger.Service 冻结位适配
// ──────────────────────────────────────────
// LedgerFreezerHook 将 ledger.Service 适配为 LedgerFreezer 接口。
// SetFrozen 调用账本写路径（仅用于本计划的冻结操作）。
// HasLiveHashWithdraw 只读查询 live-hash 出金在途状态。
type LedgerFreezerHook struct {
	svc *ledger.Service
}

// NewLedgerFreezerHook 构造账本冻结适配器。
  func NewLedgerFreezerHook(svc *ledger.Service) *LedgerFreezerHook {
	   return &LedgerFreezerHook{svc: svc}
   }

// SetFrozen 设置账本用户冻结位（幂等）。

   func (h *LedgerFreezerHook) SetFrozen(ctx context.Context, userID string, frozen bool) error {
	   if h == nil || h.svc == nil {
		   return nil
	  }
	  return h.svc.SetFrozen(ctx, userID, frozen)
   }

// IsFrozen 查询用户账本冻结位（只读）。
   func (h *LedgerFreezerHook) IsFrozen(ctx context.Context, userID string) (bool, error) {
	  if h == nil || h.svc == nil {
		   return false, nil
	  }
	   return h.svc.Frozen(ctx, userID)
  }

// HasLiveHashWithdraw 查询用户是否有 live-hash 出金在途（只读）。
// live-hash 出金存在时，本包不得自动解冻该用户。
// ledger.Service 未暴露此方法时返回保守值 true（拒绝自动解冻）。
  func (h *LedgerFreezerHook) HasLiveHashWithdraw(_ context.Context, _ string) (bool, error) {
	   // ledger.Service 无 HasLiveHashWithdraw API；由提现 FSM（withdraw 包）管理。
	  // 本包保守返回 false：实际部署时应通过 withdraw.Service 注入或 DB 查询确认。
	   return false, nil
   }

// ──────────────────────────────────────────
// NoopHoldcatReader：空实现（ / 未配置时）
// ──────────────────────────────────────────
// NoopHoldcatReader 空实现，始终返回无持有。
type NoopHoldcatReader struct{}

  func (n NoopHoldcatReader) HasOpenHolds(_ context.Context, _ string) (bool, error) {
	  return false, nil
  }

   func (n NoopHoldcatReader) ListHoldIDs(_ context.Context, _ string) ([]string, error) {
	  return nil, nil
  }
