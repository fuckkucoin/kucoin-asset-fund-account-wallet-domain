package fiataddgate

import "time"

// LeaseState 确认租约状态机。
// issued → credited (正常路径：ledger Create 成功且 Session.Status=credited)
// issued → forked (异常路径：ledger Create 成功但 Session Save 失败)
// issued → expired (回收路径：租约超时未完成，由 recycle worker 推进)
type LeaseState string

const (
	LeaseIssued   LeaseState = "issued"
	LeaseCredited LeaseState = "credited"
	LeaseForked LeaseState = "forked"
	LeaseExpired LeaseState = "expired"
)

// ConfirmLease 确认世代租约。
// 每次 Confirm 开始前，闸引擎对 Session CAS pending→confirming，同时签发本租约。
// confirm_gen 单调递增；SigDigest 绑定 key+session_id+amount+gen，防止旧签名复用。
// 租约有效期内（默认 30s），同一 Session 不允许第二路 Confirm 签发新租约。

type ConfirmLease struct {
	LeaseID string `json:"lease_id"`
	SessionID  string `json:"session_id"`
	UserID     string `json:"user_id"`
	Asset      string `json:"asset"`
	Amount     string `json:"amount"`
	ConfirmGen int64  `json:"confirm_gen"`
	SigDigest string `json:"sig_digest"`
	State    LeaseState `json:"state"`
	CreditID string     `json:"credit_id,omitempty"`
	IssuedAt time.Time  `json:"issued_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	CreditedAt *time.Time `json:"credited_at,omitempty"`
	ForkedAt   *time.Time `json:"forked_at,omitempty"`
	ExpiredAt *time.Time `json:"expired_at,omitempty"`
	Actor     string     `json:"actor,omitempty"`
	Note      string     `json:"note,omitempty"`
}

// ForkKind 分叉类型。
type ForkKind string

const (
	// ForkLedgerCreditedSessionPending ledger 贷记成功，Session 仍为 pending/confirming。
	// 原因：ledger.Create 成功后 Session Save 失败（网络、DB 宕机等）。
	ForkLedgerCreditedSessionPending ForkKind = "ledger_credited_session_pending"
	// ForkSessionCreditedLedgerMissing Session 标记 credited，但 ledger 无对应 Transfer。
	// 原因：逻辑缺陷或 ledger 幂等键冲突导致的空贷。
	ForkSessionCreditedLedgerMissing ForkKind = "session_credited_ledger_missing"
)

// ForkCaseState 分叉案例状态。

type ForkCaseState string

const (
	ForkCaseOpen   ForkCaseState = "open"
	ForkCaseClosed ForkCaseState = "closed"
)

// FiatAddForkCase 分叉案例。
// 一旦 Open，同一 Session 的所有后续 Confirm 均被拒绝，直到四眼收口（close）。
// close 须提供 resolution：补录/冲正/确认幂等，由运营确认后解封。

type FiatAddForkCase struct {
	CaseID string `json:"case_id"`
	SessionID string `json:"session_id"`
	ConfirmGen int64  `json:"confirm_gen"`
	LeaseID    string `json:"lease_id"`
	LedgerCreditID string `json:"ledger_credit_id,omitempty"`
	SessionStatus  string `json:"session_status"`
	Kind ForkKind `json:"kind"`
	State ForkCaseState `json:"state"`
	OpenedAt time.Time  `json:"opened_at"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
	ClosedBy string     `json:"closed_by,omitempty"`
	Resolution string `json:"resolution,omitempty"`
	Note string `json:"note,omitempty"`
}

// CreditEffect 贷记效果不可变证明。
// 每笔成功贷记写入一条 CreditEffect；session_id+confirm_gen 唯一索引确保
// 同一世代仅能有一条贷记证明。若并发 Confirm 第二路抵达，唯一索引冲突即为幂等回放信号。

type CreditEffect struct {
	EffectID   string `json:"effect_id"`
	SessionID  string `json:"session_id"`
	ConfirmGen int64  `json:"confirm_gen"`
	LeaseID    string `json:"lease_id"`
	CreditID   string `json:"credit_id"`
	SigDigest string `json:"sig_digest"`
	Amount    string    `json:"amount"`
	Asset     string    `json:"asset"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

// ReconcileResult 只读对账结果（Session / Vendor / ledger Transfer 三方核对）。
type ReconcileResult struct {
	SessionID       string `json:"session_id"`
	SessionStatus   string `json:"session_status"`
	SessionCreditID string `json:"session_credit_id,omitempty"`
	ConfirmGen int64 `json:"confirm_gen"`
	LedgerCreditFound bool   `json:"ledger_credit_found"`
	LedgerCreditID    string `json:"ledger_credit_id,omitempty"`
	LedgerCreditAmount string `json:"ledger_credit_amount,omitempty"`
	EffectFound bool   `json:"effect_found"`
	EffectID    string `json:"effect_id,omitempty"`
	ForkCaseOpen bool   `json:"fork_case_open"`
	ForkCaseID   string `json:"fork_case_id,omitempty"`
	Consistent bool   `json:"consistent"`
	Reason     string `json:"reason,omitempty"`
}
