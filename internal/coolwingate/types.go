package coolwingate

import "time"

// ──────────────────────────────────────────
// CoolLease 冷却代次租约
// ──────────────────────────────────────────

type LeaseState string

const (
	LeaseOpen    LeaseState = "open"
	LeaseBound   LeaseState = "bound"
	LeaseExpired LeaseState = "expired"
)

const (
	LeaseEvBind = "bind"
	LeaseEvExpire = "expire"
)

type CoolLease struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	Reason string `json:"reason"`
	CoolGen int64      `json:"cool_gen"`
	State   LeaseState `json:"state"`
	BoundTo string     `json:"bound_to,omitempty"`
	LeaseExpiry time.Time `json:"lease_expiry"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	BoundAt   *time.Time `json:"bound_at,omitempty"`
	ExpiredAt *time.Time `json:"expired_at,omitempty"`
}

// ──────────────────────────────────────────
// PreflightBind 预检绑定令牌
// ──────────────────────────────────────────

type BindState string

const (
	BindOpen     BindState = "open"
	BindRevoked  BindState = "revoked"
	BindConsumed BindState = "consumed"
)

const (
	BindEvRevoke  = "revoke"
	BindEvConsume = "consume"
)

type PreflightBind struct {
	ID string `json:"id"`
	UserID     string `json:"user_id"`
	WithdrawID string `json:"withdraw_id"`
	CoolGen int64 `json:"cool_gen"`
	LeaseID   string    `json:"lease_id"`
	State     BindState `json:"state"`
	Token     string    `json:"token"`
	CheckedAt time.Time `json:"checked_at"`
	BindExpiry time.Time `json:"bind_expiry"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	RevokeReason string `json:"revoke_reason,omitempty"`
}

// ──────────────────────────────────────────
// CoolWindowCase 窗口泄漏案例
// ──────────────────────────────────────────

type CaseState string

const (
	CaseOpen CaseState = "open"
	CaseProven CaseState = "proven"
	CaseClosed CaseState = "closed"
)

const (
	CaseEvProve = "prove"
	CaseEvClose = "close"
)

type CaseKind string

const (
	CaseMarkAfterPreflight CaseKind = "mark_after_preflight"
	CaseConcurrentMark     CaseKind = "concurrent_mark_require"
	CaseCacheBypass        CaseKind = "cache_bypass_reject"
	CaseLeaseExpiredBind CaseKind = "lease_expired_bind"
	CaseGenMismatch CaseKind = "gen_mismatch"
)

type CoolWindowCase struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	WithdrawID string `json:"withdraw_id"`
	BindID string `json:"bind_id"`
	LeaseID string    `json:"lease_id"`
	Kind    CaseKind  `json:"kind"`
	State   CaseState `json:"state"`
	Detail string `json:"detail"`
	ProofIDs   []string  `json:"proof_ids,omitempty"`
	Resolution string    `json:"resolution,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ProvenAt  *time.Time `json:"proven_at,omitempty"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
}

// ──────────────────────────────────────────
// WindowProof 泄漏证明
// ──────────────────────────────────────────

type WindowProof struct {
	ID     string `json:"id"`
	CaseID string `json:"case_id"`
	UserID string `json:"user_id"`
	WithdrawID string `json:"withdraw_id"`
	LeaseID    string `json:"lease_id"`
	BindID     string `json:"bind_id"`
	MarkReason    string    `json:"mark_reason"`
	PreflightGen  int64     `json:"preflight_gen"`
	MarkGen       int64     `json:"mark_gen"`
	PreflightTime time.Time `json:"preflight_time"`
	MarkTime      time.Time `json:"mark_time"`
	LedgerLockUntil time.Time `json:"ledger_lock_until"`
	Sealed    bool       `json:"sealed"`
	SealedAt  *time.Time `json:"sealed_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// ──────────────────────────────────────────
// Request / Response
// ──────────────────────────────────────────

type MarkReq struct {
	UserID string `json:"user_id"`
	Reason string `json:"reason"`
}

type PreflightBindReq struct {
	UserID string `json:"user_id"`
	WithdrawID string `json:"withdraw_id"`
}

type BroadcastCheckReq struct {
	UserID     string `json:"user_id"`
	WithdrawID string `json:"withdraw_id"`
	BindToken  string `json:"bind_token"`
}

type CaseConfirmReq struct {
	CaseID string `json:"case_id"`
	ProofIDs   []string `json:"proof_ids"`
	Resolution string   `json:"resolution"`
}

type ProofExportReq struct {
	UserID     string `json:"user_id,omitempty"`
	WithdrawID string `json:"withdraw_id,omitempty"`
	CaseID string `json:"case_id,omitempty"`
	Limit int `json:"limit,omitempty"`
}

type CoolGenQueryReq struct {
	UserID string `json:"user_id"`
}

type CoolGenResp struct {
	UserID string `json:"user_id"`
	CurrentGen int64 `json:"current_gen"`
	ActiveLeaseID string    `json:"active_lease_id,omitempty"`
	LockUntil     time.Time `json:"lock_until"`
	Reason        string    `json:"reason,omitempty"`
}

type RequireWithdrawReq struct {
	UserID string `json:"user_id"`
}

type RequireWithdrawResp struct {
	Allowed bool `json:"allowed"`
	Reason    string    `json:"reason,omitempty"`
	LockUntil time.Time `json:"lock_until,omitempty"`
	Remain    string    `json:"remain,omitempty"`
}
