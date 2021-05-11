package feereserve

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// ──────────────────────────────────────────
// LeaseState 预留租约 FSM 状态
// ──────────────────────────────────────────

type LeaseState string

const (
	LeaseReserved LeaseState = "reserved"
	LeaseSettled  LeaseState = "settled"
	LeaseReleased LeaseState = "released"
	LeaseBlocked LeaseState = "blocked"
)

// TerminalLeg 终态方向（Settle / Release）

type TerminalLeg string

const (
	LegSettle TerminalLeg = "settle"
	LegRelease TerminalLeg = "release"
)

// CaseState 异常案例状态

type CaseState string

const (
	CaseOpen CaseState = "open"
	CaseProven CaseState = "proven"
	CaseClosed CaseState = "closed"
)

// CaseKind 案例类型
type CaseKind string

const (
	CaseUndercharge CaseKind = "undercharge"
	CaseOverRefund  CaseKind = "over_refund"
	CaseDualVoucher   CaseKind = "dual_voucher"
	CaseAfterTerminal CaseKind = "after_terminal"
	CaseTerminalGap CaseKind = "terminal_gap"
)

// ──────────────────────────────────────────
// ReserveLease 预留代次租约
// ──────────────────────────────────────────

type ReserveLease struct {
	ID string `json:"id"`
	WithdrawID  string `json:"withdraw_id"`
	UserID      string `json:"user_id"`
	Asset       string `json:"asset"`
	ReservedFee string `json:"reserved_fee"`
	ActualFee string `json:"actual_fee,omitempty"`
	Generation int64 `json:"generation"`
	Token string `json:"token"`
	State       LeaseState `json:"state"`
	TerminalLeg string     `json:"terminal_leg,omitempty"`
	VoucherID   string     `json:"voucher_id,omitempty"`
	BlockReason string     `json:"block_reason,omitempty"`
	LeaseExpiry time.Time  `json:"lease_expiry"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	SettledAt  *time.Time `json:"settled_at,omitempty"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
}

  func (l ReserveLease) IsTerminal() bool {
	  return l.State == LeaseSettled || l.State == LeaseReleased || l.State == LeaseBlocked
  }

   func (l ReserveLease) Expired(now time.Time) bool {
	   return now.After(l.LeaseExpiry)
   }

  func (l ReserveLease) Copy() ReserveLease { return l }

// ──────────────────────────────────────────
// TerminalBind 终态绑定——同一 WithdrawID 只允许一个终态方向
// ──────────────────────────────────────────

type TerminalBind struct {
	WithdrawID string      `json:"withdraw_id"`
	Leg        TerminalLeg `json:"leg"`
	LeaseID    string      `json:"lease_id"`
	Generation int64       `json:"generation"`
	Token string `json:"token"`
	VoucherID string `json:"voucher_id"`
	BoundAt time.Time `json:"bound_at"`
}

// ──────────────────────────────────────────
// DeltaProof 差额证明
// ──────────────────────────────────────────

type DeltaProof struct {
	ID         string `json:"id"`
	WithdrawID string `json:"withdraw_id"`
	LeaseID     string `json:"lease_id"`
	ReservedFee string `json:"reserved_fee"`
	ActualFee   string `json:"actual_fee"`
	DeltaAmt string `json:"delta_amt"`
	DeltaSign string `json:"delta_sign"`
	CaseID string `json:"case_id,omitempty"`
	Sealed   bool       `json:"sealed"`
	SealedAt *time.Time `json:"sealed_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ──────────────────────────────────────────
// ReserveCase 异常案例
// ──────────────────────────────────────────

type ReserveCase struct {
	ID         string `json:"id"`
	WithdrawID string `json:"withdraw_id"`
	LeaseID    string `json:"lease_id"`
	Kind   CaseKind  `json:"kind"`
	State  CaseState `json:"state"`
	Detail string    `json:"detail"`
	ProofIDs []string `json:"proof_ids,omitempty"`
	Resolution string    `json:"resolution,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ProvenAt *time.Time `json:"proven_at,omitempty"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
}

   func (c ReserveCase) Copy() ReserveCase {
	   cp := c
	   if c.ProofIDs != nil {
		  cp.ProofIDs = make([]string, len(c.ProofIDs))
		  copy(cp.ProofIDs, c.ProofIDs)
	  }
	  return cp
   }

// ──────────────────────────────────────────
// 请求/响应体
// ──────────────────────────────────────────

type ReserveReq struct {
	WithdrawID string `json:"withdraw_id"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Fee   string `json:"fee"`
	Idem string `json:"idempotency_key"`
}

type SettleReq struct {
	WithdrawID string `json:"withdraw_id"`
	LeaseID string `json:"lease_id"`
	Generation int64  `json:"generation"`
	Token      string `json:"token"`
	ActualFee  string `json:"actual_fee"`
	VoucherID string `json:"voucher_id"`
	Idem string `json:"idempotency_key"`
}

type ReleaseReq struct {
	WithdrawID string `json:"withdraw_id"`
	LeaseID string `json:"lease_id"`
	Generation int64  `json:"generation"`
	Token      string `json:"token"`
	RefundAmt  string `json:"refund_amt"`
	VoucherID  string `json:"voucher_id"`
	Idem       string `json:"idempotency_key"`
}

type BlockReq struct {
	WithdrawID string `json:"withdraw_id"`
	LeaseID    string `json:"lease_id"`
	Reason     string `json:"reason"`
}

type CaseConfirmReq struct {
	CaseID     string `json:"case_id"`
	Resolution string `json:"resolution"`
}

type ProofExportReq struct {
	WithdrawID string `json:"withdraw_id,omitempty"`
	LeaseID string `json:"lease_id,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

// ──────────────────────────────────────────
// Token 生成
// ──────────────────────────────────────────

   func NewLeaseToken() string {
	  b := make([]byte, 16)
	   _, _ = rand.Read(b)
	  return hex.EncodeToString(b)
   }
