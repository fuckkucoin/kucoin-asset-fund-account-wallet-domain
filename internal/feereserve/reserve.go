package feereserve

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

// ReserveState 储备记录的生命周期状态。

type ReserveState string

const (
	// StateReserved 已冻结预估费，等待广播确认。
	StateReserved ReserveState = "reserved"
	// StateSettled 广播确认后已结算，实际费用已写入。
	StateSettled ReserveState = "settled"
	// StateReleased 出金取消，已退还冻结，不生成 system.fee 凭证。
	StateReleased ReserveState = "released"
)

// DeltaDir delta 方向。
type DeltaDir string

const (
	// DeltaOver 实际费 < 预估费，多收了，需退还用户 delta。
	DeltaOver DeltaDir = "over"
	// DeltaUnder 实际费 > 预估费，收少了，需追收用户 delta。
	DeltaUnder DeltaDir = "under"
	// DeltaZero 实际费 == 预估费，无 delta。
	DeltaZero DeltaDir = "zero"
)

// FeeReserve 单次出金手续费储备记录。

type FeeReserve struct {
	// ID 储备记录唯一标识（UUID）。
	ID string `json:"id"`
	// WithdrawID 关联的出金单号。
	WithdrawID string `json:"withdraw_id"`
	// VersionID 关联的 feeoracle QuoteVersion ID（可为空，表示静态档位报价）。
	VersionID string `json:"version_id,omitempty"`
	// UserID 出金用户 ID。
	UserID string `json:"user_id"`
	// Asset 手续费计价资产（如 "USDT"/"BTC"）。
	Asset string `json:"asset"`
	// Chain 出金链名。
	Chain string `json:"chain"`
	// Reserved 预估冻结的手续费金额。
	Reserved string `json:"reserved"`
	// Actual 实际链上手续费（Settle 后写入；StateReserved 时为 ""）。
	Actual string `json:"actual,omitempty"`
	// Delta |Reserved - Actual|；DeltaZero 时为 "0"。
	Delta string `json:"delta,omitempty"`
	// DeltaDir delta 方向；DeltaZero 时为 "zero"。
	DeltaDir DeltaDir `json:"delta_dir,omitempty"`
	// State 生命周期状态。
	State ReserveState `json:"state"`
	// LedgerReserve 冻结凭证 ID（账本写入后填充）。
	LedgerReserve string `json:"ledger_reserve,omitempty"`
	// LedgerSettle 结算凭证 ID。
	LedgerSettle string `json:"ledger_settle,omitempty"`
	// LedgerDelta delta 平衡凭证 ID。
	LedgerDelta string `json:"ledger_delta,omitempty"`
	// SettledAt 结算时间。
	SettledAt *time.Time `json:"settled_at,omitempty"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最后更新时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// CanSettle 检查是否可以结算（必须是 StateReserved）。

   func (r *FeeReserve) CanSettle() error {
	  if r == nil {
		   return ErrNotFound
	  }
	   if r.State != StateReserved {
		   return ErrState
	   }
	   return nil
  }

// CanRelease 检查是否可以退还（必须是 StateReserved）。
  func (r *FeeReserve) CanRelease() error {
	   if r == nil {
		   return ErrNotFound
	  }
	  if r.State != StateReserved {
		   return ErrState
	   }
	  return nil
   }

// ComputeDelta 根据已设置的 Reserved 和 actual 计算 delta 方向与数量。
// actualFeeStr 为实际链上手续费字符串（已通过 ParseBook 解析）。
// 返回 (delta amount, direction, error)。
  func (r *FeeReserve) ComputeDelta(actualFeeStr string) (money.Amount, DeltaDir, error) {
	   if r == nil {
		   return money.Zero(), DeltaZero, ErrNotFound
	   }
	   reserved, err := money.ParseBook(r.Reserved)
	  if err != nil {
		   return money.Zero(), DeltaZero, ErrAmount
	   }
	   actual, err := money.ParseBook(strings.TrimSpace(actualFeeStr))
	  if err != nil {
		  return money.Zero(), DeltaZero, ErrAmount
	   }
	   cmp := reserved.Cmp(actual)
	  switch {
	  case cmp > 0:
		   // reserved > actual：退还 delta 给用户。
		   delta, err := reserved.Sub(actual)
		   if err != nil {
			   return money.Zero(), DeltaZero, err
		   }
		  return delta, DeltaOver, nil
	  case cmp < 0:
		  // reserved < actual：追收 delta 自用户。
		   delta, err := actual.Sub(reserved)
		   if err != nil {
			   return money.Zero(), DeltaZero, err
		  }
		  return delta, DeltaUnder, nil
	   default:
		  return money.Zero(), DeltaZero, nil
	  }
   }

// ReserveRequest 创建储备的请求参数。

type ReserveRequest struct {
	// WithdrawID 出金单号；全局唯一，幂等键。
	WithdrawID string `json:"withdraw_id"`
	// VersionID 关联的 QuoteVersion ID（可为空）。
	VersionID string `json:"version_id,omitempty"`
	// UserID 用户 ID。
	UserID string `json:"user_id"`
	// Asset 手续费计价资产。
	Asset string `json:"asset"`
	// Chain 出金链名。
	Chain string `json:"chain"`
	// ReservedFee 预估手续费金额。
	ReservedFee string `json:"reserved_fee"`
}

// SettleRequest 结算请求参数。
type SettleRequest struct {
	// WithdrawID 出金单号。
	WithdrawID string `json:"withdraw_id"`
	// ActualFee 实际链上手续费（用户资产计价）。
	ActualFee string `json:"actual_fee"`
	// Actor 操作人（广播确认 worker 的身份标识）。
	Actor string `json:"actor,omitempty"`
}

// SettleResult 结算结果。
type SettleResult struct {
	// Reserve 更新后的储备记录。
	Reserve FeeReserve `json:"reserve"`
	// Lines 产生的账本凭证行（仅内存；写入账本后以 LedgerID 跟踪）。
	Lines []DeltaLine `json:"lines,omitempty"`
}

// DeltaLine 单条手续费 delta 账本凭证（借贷必须平衡）。
type DeltaLine struct {
	// Memo 凭证说明。
	Memo string `json:"memo"`
	// Asset 资产代码。
	Asset string `json:"asset"`
	// Amount 金额（绝对值）。
	Amount string `json:"amount"`
	// Direction "debit" 或 "credit"（相对 system.fee 账户视角）。
	Direction string `json:"direction"`
}
