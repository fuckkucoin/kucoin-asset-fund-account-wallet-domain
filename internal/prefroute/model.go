package prefroute

import (
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/fundpref"
)

// ─────────────────────────────────────────────
// 枚举
// ─────────────────────────────────────────────
// LeaseState PrefLease 状态机枚举。
type LeaseState string

const (
	LeaseStateOpen     LeaseState = "open"     // 签发后，等待绑定路由快照
	LeaseStateBound    LeaseState = "bound"    // 已绑定 RouteSnap，路由消费中
	LeaseStateSwitched LeaseState = "switched" // DefaultAccount 切换已完成，租约作废
	LeaseStateBlocked  LeaseState = "blocked"  // 漂移或违规，路由被阻断
)

// DriftState PrefDriftCase 状态机枚举。

type DriftState string

const (
	DriftStateOpen DriftState = "open" // 发现漂移，等待证明
	DriftStateProven DriftState = "proven" // 已提交证明，等待关闭确认
	DriftStateClosed DriftState = "closed" // 已收口
)

// ─────────────────────────────────────────────
// PrefLease
// ─────────────────────────────────────────────
// PrefLease 绑定入账路由到偏好世代快照。
// 签发时快照 Generation 与 DefaultBook；
// 若 DefaultAccount 切换（世代递增），租约进入 switched；
// 若漂移被检测，进入 blocked。

type PrefLease struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	Generation  int64                `json:"generation"`
	DefaultBook fundpref.AccountBook `json:"default_book"`
	State       LeaseState           `json:"state"`
	RouteSnapID string               `json:"route_snap_id,omitempty"`
	CreditTxnID string               `json:"credit_txn_id,omitempty"` // 绑定的贷记事务 ID
	IssuedAt time.Time `json:"issued_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	BoundAt   *time.Time `json:"bound_at,omitempty"`
	SwitchedAt  *time.Time `json:"switched_at,omitempty"`
	BlockedAt   *time.Time `json:"blocked_at,omitempty"`
	BlockReason string     `json:"block_reason,omitempty"`
	IssuedBy    string     `json:"issued_by"`
	FenceEpoch  int64      `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
}

// IsValid 检查租约在指定时刻与世代是否仍然有效（open 或 bound，且未过期）。
  func (l PrefLease) IsValid(now time.Time, currentGen int64) error {
	   if l.State != LeaseStateOpen && l.State != LeaseStateBound {
		   return fmt.Errorf("%w: lease %s state=%s", ErrLeaseInvalid, l.ID, l.State)
	   }
	   if now.After(l.ExpiresAt) {
		   return fmt.Errorf("%w: lease %s expired=%s", ErrLeaseExpired, l.ID, l.ExpiresAt.Format(time.RFC3339))
	   }
	   if l.Generation != currentGen {
		   return fmt.Errorf("%w: lease gen=%d current=%d", ErrGenerationMismatch, l.Generation, currentGen)
	   }
	   return nil
  }

// ─────────────────────────────────────────────
// RouteSnap
// ─────────────────────────────────────────────
// RouteSnap 路由快照；记录贷记操作绑定时的目标科目与偏好世代。
// CreditedBook 是实际入账科目；BoundBook 是租约绑定的快照科目。
// 若 CreditedBook ≠ BoundBook，触发 PrefDriftCase。

type RouteSnap struct {
	ID string `json:"id"`
	LeaseID string `json:"lease_id"`
	UserID string `json:"user_id"`
	Generation int64 `json:"generation"`
	BoundBook    fundpref.AccountBook `json:"bound_book"`    // 租约快照时的 DefaultBook
	CreditedBook fundpref.AccountBook `json:"credited_book"` // 实际贷记的科目（事后填充）
	CreditTxnID  string               `json:"credit_txn_id"`
	Asset      string     `json:"asset"`
	Amount     string     `json:"amount"`
	SnapAt     time.Time  `json:"snap_at"`
	CreditedAt *time.Time `json:"credited_at,omitempty"`
	DriftCaseID string `json:"drift_case_id,omitempty"` // 若产生漂移
}

// HasDrift 检查实际贷记科目是否与快照不符。

  func (r RouteSnap) HasDrift() bool {
	  return r.CreditedBook != "" && r.BoundBook != "" && r.CreditedBook != r.BoundBook
   }

// ─────────────────────────────────────────────
// PrefDriftCase
// ─────────────────────────────────────────────
// PrefDriftCase 记录「实际贷记科目偏离路由快照」的漂移案例。
// open 状态下后续偏好写与高风险路由均被阻断，直至证明通过并关闭。

type PrefDriftCase struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	LeaseID string `json:"lease_id"`
	RouteSnapID string `json:"route_snap_id"`
	BoundBook fundpref.AccountBook `json:"bound_book"` // 快照期望科目
	CreditedBook fundpref.AccountBook `json:"credited_book"` // 实际贷记科目
	Generation int64 `json:"generation"`
	CreditTxnID string `json:"credit_txn_id"`
	Asset       string `json:"asset"`
	Amount string     `json:"amount"`
	State  DriftState `json:"state"`
	DetectedAt time.Time `json:"detected_at"`
	ProvenAt     *time.Time `json:"proven_at,omitempty"`
	ClosedAt     *time.Time `json:"closed_at,omitempty"`
	ProofID      string     `json:"proof_id,omitempty"`
	ProofSummary string     `json:"proof_summary,omitempty"`
	ClosedBy string `json:"closed_by,omitempty"`
}

// NeedsProof 该漂移案例是否处于需要证明的状态。
   func (d PrefDriftCase) NeedsProof() bool {
	   return d.State == DriftStateOpen || d.State == DriftStateProven
   }

// IsBlocking 该漂移案例是否正在阻断后续操作。

  func (d PrefDriftCase) IsBlocking() bool {
	  return d.State == DriftStateOpen
  }

// ─────────────────────────────────────────────
// DriftProof
// ─────────────────────────────────────────────
// DriftProof 漂移证明；关联 PrefDriftCase。
// 提交后 DriftCase→proven；四眼确认→closed。
type DriftProof struct {
	ID string `json:"id"`
	DriftCaseID string `json:"drift_case_id"`
	SubmittedBy string `json:"submitted_by"`
	Summary     string `json:"summary"`
	Evidence    string `json:"evidence"` // JSON blob / checksum
	SubmittedAt time.Time `json:"submitted_at"`
	ConfirmedBy string `json:"confirmed_by,omitempty"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
}

// ─────────────────────────────────────────────
// 请求/响应类型
// ─────────────────────────────────────────────
// IssuePrefLeaseReq 签发偏好租约请求。
type IssuePrefLeaseReq struct {
	UserID string
	CreditTxnID string
	IssuedBy    string
}

// BindRouteSnapReq 绑定路由快照请求（贷记执行前调用）。

type BindRouteSnapReq struct {
	LeaseID     string
	CreditTxnID string
	Asset string
	Amount string
	IssuedBy string
}

// ConfirmCreditReq 贷记确认请求（贷记完成后调用）。

type ConfirmCreditReq struct {
	RouteSnapID  string
	CreditedBook fundpref.AccountBook
	CreditTxnID  string
}

// SubmitDriftProofReq 提交漂移证明请求。
type SubmitDriftProofReq struct {
	DriftCaseID string
	SubmittedBy string
	Summary  string
	Evidence string
}

// ─────────────────────────────────────────────
// 配置
// ─────────────────────────────────────────────
// Config prefroute 模块配置。

type Config struct {
	// LeaseTTL 偏好租约有效期。
	LeaseTTL time.Duration
	// DriftPatrolInterval 漂移巡检周期。
	DriftPatrolInterval time.Duration
	// InflightScanInterval 在途贷记扫描周期。
	InflightScanInterval time.Duration
	// PendingSwitchAdvanceInterval 待切换推进周期。
	PendingSwitchAdvanceInterval time.Duration
}

// DefaultConfig 默认配置。

   func DefaultConfig() Config {
	  return Config{
		   LeaseTTL: 30 * time.Second,
		  DriftPatrolInterval: 15 * time.Second,
		  InflightScanInterval: 10 * time.Second,
		   PendingSwitchAdvanceInterval: 5 * time.Second,
	  }
  }
