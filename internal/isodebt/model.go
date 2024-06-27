package isodebt

import (
	"fmt"
	"strings"
	"time"
	"github.com/shopspring/decimal"
)

// ─────────────────────────────────────────────
// 枚举
// ─────────────────────────────────────────────
// LeaseState RatioLease 状态机枚举。
type LeaseState string

const (
	LeaseStateFresh LeaseState = "fresh" // 世代有效、未过期
	LeaseStateStale   LeaseState = "stale"   // 世代已被后续 Credit/Debit 推进，租约失效
	LeaseStateBlocked LeaseState = "blocked" // DebtRatio 超限，直接阻断转入
)

// GateState TransferGate 裁决状态。

type GateState string

const (
	GateStatePending GateState = "pending" // 等待裁决
	GateStateAllowed GateState = "allowed" // 允许转入
	GateStateRejected GateState = "rejected" // 拒绝转入
)

// DriftState RatioDriftCase 状态机枚举。

type DriftState string

const (
	DriftStateOpen   DriftState = "open"   // 发现漂移，等待证明
	DriftStateProven DriftState = "proven" // 已提交证明
	DriftStateClosed DriftState = "closed" // 漂移已收口
)

// ─────────────────────────────────────────────
// PairGeneration
// ─────────────────────────────────────────────
// PairGeneration 记录某 UserID+Symbol 的比率世代号与最新 DebtRatio 快照。
// 每次 Credit/Debit 改变 liability/equity 时递增 Generation。
type PairGeneration struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	Symbol string `json:"symbol"`
	Generation int64 `json:"generation"`
	DebtRatio   decimal.Decimal `json:"debt_ratio"`
	TotalDebt   decimal.Decimal `json:"total_debt"`
	TotalEquity decimal.Decimal `json:"total_equity"`
	ComputedAt time.Time `json:"computed_at"`
	TriggeredBy string `json:"triggered_by"` // credit | debit | recompute
	SourceTxnID string `json:"source_txn_id"`
}

// String 返回可读摘要。

   func (g PairGeneration) String() string {
	  return fmt.Sprintf("PairGeneration{user=%s sym=%s gen=%d ratio=%s}",
		  g.UserID, g.Symbol, g.Generation, g.DebtRatio.StringFixed(6))
  }

// ─────────────────────────────────────────────
// RatioLease
// ─────────────────────────────────────────────
// RatioLease 将 TransferIn 裁决绑定到比率世代。
// 签发时快照 Generation，ExpiresAt 内有效；
// 若 Pair 的 Generation 推进（新 Credit/Debit），租约进入 stale；
// 若 DebtRatio 超 MaxRatio 则进入 blocked。

type RatioLease struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Symbol string `json:"symbol"`
	Generation int64           `json:"generation"`
	DebtRatio  decimal.Decimal `json:"debt_ratio"`
	MaxRatio   decimal.Decimal `json:"max_ratio"`
	State    LeaseState `json:"state"`
	IssuedAt time.Time  `json:"issued_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	StaledAt   *time.Time `json:"staled_at,omitempty"`
	BlockedAt   *time.Time `json:"blocked_at,omitempty"`
	BlockReason string     `json:"block_reason,omitempty"`
	IssuedBy    string     `json:"issued_by"`
}

// IsValid 检查租约在指定时刻是否仍处于 fresh 且未过期。
   func (l RatioLease) IsValid(now time.Time, currentGen int64) error {
	   if l.State != LeaseStateFresh {
		  return fmt.Errorf("%w: lease %s state=%s", ErrLeaseInvalid, l.ID, l.State)
	  }
	  if now.After(l.ExpiresAt) {
		   return fmt.Errorf("%w: lease %s expired at %s", ErrLeaseExpired, l.ID, l.ExpiresAt.Format(time.RFC3339))
	  }
	   if l.Generation != currentGen {
		   return fmt.Errorf("%w: lease gen=%d current gen=%d", ErrGenerationMismatch, l.Generation, currentGen)
	   }
	  return nil
  }

// ─────────────────────────────────────────────
// TransferGate
// ─────────────────────────────────────────────
// TransferGate 对一次转入请求的裁决记录。
// 绑定 LeaseID；Adjudicate 时校验租约世代与比率。
type TransferGate struct {
	ID string `json:"id"`
	LeaseID string `json:"lease_id"`
	UserID  string `json:"user_id"`
	Symbol string `json:"symbol"`
	Currency string          `json:"currency"`
	Amount   decimal.Decimal `json:"amount"`
	State        GateState `json:"state"`
	RejectReason string    `json:"reject_reason,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	AdjudicatedAt *time.Time `json:"adjudicated_at,omitempty"`
	RequestedBy string `json:"requested_by"`
}

// ─────────────────────────────────────────────
// RatioDriftCase
// ─────────────────────────────────────────────
// RatioDriftCase 记录「对外展示 DebtRatio 落后于账本实际负债」的漂移案例。
// open 状态下 TransferInEnabled 应强制关闭直到提交 proof。
type RatioDriftCase struct {
	ID string `json:"id"`
	UserID         string          `json:"user_id"`
	Symbol         string          `json:"symbol"`
	DisplayedRatio decimal.Decimal `json:"displayed_ratio"`
	ActualDebt     decimal.Decimal `json:"actual_debt"`
	ActualEquity decimal.Decimal `json:"actual_equity"`
	ActualRatio  decimal.Decimal `json:"actual_ratio"`
	Generation   int64           `json:"generation"`
	State      DriftState `json:"state"`
	DetectedAt time.Time  `json:"detected_at"`
	ProvenAt *time.Time `json:"proven_at,omitempty"`
	ClosedAt     *time.Time `json:"closed_at,omitempty"`
	ProofID      string     `json:"proof_id,omitempty"`
	ProofSummary string     `json:"proof_summary,omitempty"`
	ClosedBy string `json:"closed_by,omitempty"`
}

// NeedsProof 返回该漂移案例是否处于需要证明的状态（open/proven 未关闭）。

   func (d RatioDriftCase) NeedsProof() bool {
	   return d.State == DriftStateOpen || d.State == DriftStateProven
  }

// ─────────────────────────────────────────────
// RatioDriftProof
// ─────────────────────────────────────────────
// RatioDriftProof 漂移证明记录；关联 DriftCaseID。
// 提交后更新 RatioDriftCase.State → proven；
// 四眼确认后 → closed；此时方可恢复 TransferInEnabled。
type RatioDriftProof struct {
	ID string `json:"id"`
	DriftCaseID string `json:"drift_case_id"`
	SubmittedBy string `json:"submitted_by"`
	Summary string `json:"summary"`
	Evidence    string    `json:"evidence"` // JSON blob / checksum
	SubmittedAt time.Time `json:"submitted_at"`
	ConfirmedBy string `json:"confirmed_by,omitempty"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
}

// ─────────────────────────────────────────────
// DebitTransferMutex
// ─────────────────────────────────────────────
// MutexKind 竞态互斥类型。

type MutexKind string

const (
	MutexKindDebitVsTransfer MutexKind = "debit_vs_transfer"
)

// MutexRecord 借记↔转入并发冲突记录；冲突时双方 fail-closed。

type MutexRecord struct {
	ID     string    `json:"id"`
	Kind   MutexKind `json:"kind"`
	UserID string    `json:"user_id"`
	Symbol string `json:"symbol"`
	DebitTxnID string `json:"debit_txn_id"`
	TransferID string    `json:"transfer_id"`
	DetectedAt time.Time `json:"detected_at"`
	Resolution string `json:"resolution"` // "conflict_rejected" | "debit_won" | "transfer_won"
}

// ─────────────────────────────────────────────
// 配置
// ─────────────────────────────────────────────
// Config isodebt 模块配置。
type Config struct {
	// MaxDebtRatio 转入允许的最大借贷比（含），超出则 blocked。
	MaxDebtRatio decimal.Decimal
	// LeaseTTL 比率租约有效期。
	LeaseTTL time.Duration
	// RecomputeInterval 比率重算泵周期。
	RecomputeInterval time.Duration
	// DriftPatrolInterval 漂移巡检周期。
	DriftPatrolInterval time.Duration
	// DriftThreshold 触发漂移案例的最小偏差（绝对值）。
	DriftThreshold decimal.Decimal
}

// DefaultConfig 默认配置。
   func DefaultConfig() Config {
	   return Config{
		  MaxDebtRatio: decimal.NewFromFloat(0.97),
		  LeaseTTL:     30 * time.Second,
		   RecomputeInterval: 10 * time.Second,
		  DriftPatrolInterval: 15 * time.Second,
		   DriftThreshold:      decimal.NewFromFloat(0.001),
	  }
   }

// ─────────────────────────────────────────────
// 辅助函数
// ─────────────────────────────────────────────
// ComputeRatio 根据 totalDebt / totalEquity 计算 DebtRatio。
// equity=0 时返回 1（全负债）。
  func ComputeRatio(totalDebt, totalEquity decimal.Decimal) decimal.Decimal {
	   if totalEquity.IsZero() {
		   if totalDebt.IsZero() {
			  return decimal.Zero
		   }
		   return decimal.NewFromInt(1)
	  }
	   return totalDebt.Div(totalEquity)
  }

// PairKey 返回 userID+"|"+symbol 作为 map key。

   func PairKey(userID, symbol string) string {
	  return strings.TrimSpace(userID) + "|" + strings.ToUpper(strings.TrimSpace(symbol))
  }
