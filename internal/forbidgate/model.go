package forbidgate

import (
	"errors"
	"time"
)

// ForbidState 禁提世代状态机：off → on/off 携带世代编号。
type ForbidState string

const (
	ForbidStateOff ForbidState = "off"
	ForbidStateOn ForbidState = "on"
)

// BindState 预检绑定状态机：open → revoked / consumed。
type BindState string

const (
	BindStateOpen    BindState = "open"
	BindStateRevoked BindState = "revoked"
	BindStateConsumed BindState = "consumed"
)

// DriftState 漂移案例状态机：open → proven → closed。
type DriftState string

const (
	DriftStateOpen DriftState = "open"
	DriftStateProven DriftState = "proven"
	DriftStateClosed DriftState = "closed"
)

// DriftKind 漂移类型：预检通过后禁提切 ON，或预检通过后禁提切 OFF（反向）。

type DriftKind string

const (
	DriftKindOnAfterPass  DriftKind = "on_after_pass"  // 预检时未禁，广播前已禁
	DriftKindOffAfterPass DriftKind = "off_after_pass" // 预检时已禁，广播前解禁（反向漂移）
)

// ForbidLease 禁提世代租约。
// SetForbidden 每次切换均抬升 generation；RequireWithdraw 必须在绑定时读取当前世代。
type ForbidLease struct {
	ID         string      `db:"id"`
	UserID     string      `db:"user_id"`
	Generation int64       `db:"generation"`
	State      ForbidState `db:"state"`
	Actor      string      `db:"actor"`
	Reason string `db:"reason"`
	ProofID string `db:"proof_id"`
	IssuedAt   time.Time  `db:"issued_at"`
	SwitchedAt *time.Time `db:"switched_at"`
	RevokedAt  *time.Time `db:"revoked_at"`
	FenceEpoch int64      `db:"fence_epoch"`
	FenceToken string     `db:"fence_token"`
}

// WdPrefightBind 出金预检绑定。
// RequireWithdraw 通过时创建；SetForbidden(true) 时若有 open bind 则立即 revoke。
type WdPrefightBind struct {
	ID string `db:"id"`
	UserID      string `db:"user_id"`
	WithdrawRef string `db:"withdraw_ref"`
	Generation int64 `db:"generation"`
	ForbidLeaseID string    `db:"forbid_lease_id"`
	State         BindState `db:"state"`
	IssuedAt      time.Time `db:"issued_at"`
	ExpiresAt     time.Time `db:"expires_at"`
	RevokedAt *time.Time `db:"revoked_at"`
	RevokeReason string     `db:"revoke_reason"`
	ConsumedAt   *time.Time `db:"consumed_at"`
	FenceEpoch int64  `db:"fence_epoch"`
	FenceToken string `db:"fence_token"`
}

// ForbidDriftCase 禁提漂移案例。
// 发现「预检时未禁、广播前已禁」或逆向时，写入此表并阻断广播直至证明通过。

type ForbidDriftCase struct {
	ID     string `db:"id"`
	UserID string `db:"user_id"`
	WithdrawRef   string `db:"withdraw_ref"`
	BindID        string `db:"bind_id"`
	ForbidLeaseID string `db:"forbid_lease_id"`
	Kind          DriftKind `db:"kind"`
	GenAtPrecheck int64     `db:"gen_at_precheck"`
	GenAtDetect   int64     `db:"gen_at_detect"`
	State      DriftState `db:"state"`
	DetectedAt time.Time  `db:"detected_at"`
	ProvenAt     *time.Time `db:"proven_at"`
	ClosedAt     *time.Time `db:"closed_at"`
	ProofID      string     `db:"proof_id"`
	ProofSummary string     `db:"proof_summary"`
	ClosedBy string `db:"closed_by"`
	FenceEpoch int64  `db:"fence_epoch"`
	FenceToken string `db:"fence_token"`
}

// ForbidDriftProof 漂移证明（双人确认）。

type ForbidDriftProof struct {
	ID string `db:"id"`
	DriftCaseID string `db:"drift_case_id"`
	SubmittedBy string `db:"submitted_by"`
	ConfirmedBy string `db:"confirmed_by"`
	Summary     string `db:"summary"`
	Evidence    string     `db:"evidence"`
	SubmittedAt time.Time  `db:"submitted_at"`
	ConfirmedAt *time.Time `db:"confirmed_at"`
}

// ForbidGenView 世代查询视图（只读接口返回）。

type ForbidGenView struct {
	UserID string `json:"user_id"`
	Generation int64       `json:"generation"`
	State      ForbidState `json:"state"`
	Actor string `json:"actor"`
	Reason   string    `json:"reason"`
	IssuedAt time.Time `json:"issued_at"`
}

// BindView 预检绑定视图（只读接口返回）。
type BindView struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	WithdrawRef string `json:"withdraw_ref"`
	Generation int64     `json:"generation"`
	State      BindState `json:"state"`
	IssuedAt   time.Time `json:"issued_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// DriftCaseView 漂移案例视图（只读接口返回）。
type DriftCaseView struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	WithdrawRef string     `json:"withdraw_ref"`
	Kind        DriftKind  `json:"kind"`
	State       DriftState `json:"state"`
	DetectedAt  time.Time  `json:"detected_at"`
	ProvenAt *time.Time `json:"proven_at,omitempty"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
	ProofID  string     `json:"proof_id,omitempty"`
}

var (
	// ErrForbidGenStale RequireWithdraw 绑定世代已过期（SetForbidden 已抬升）。
	ErrForbidGenStale = errors.New("forbid generation stale: generation has been bumped")
	// ErrBindNotFound 预检绑定不存在。
	ErrBindNotFound = errors.New("forbid prefight bind not found")
	// ErrBindExpired 预检绑定已过期。
	ErrBindExpired = errors.New("forbid prefight bind expired")
	// ErrBindRevoked 预检绑定已被吊销（SetForbidden(true) 触发）。
	ErrBindRevoked = errors.New("forbid prefight bind revoked by forbid switch")
	// ErrBindConsumed 预检绑定已消费，不可重放。
	ErrBindConsumed = errors.New("forbid prefight bind already consumed")
	// ErrDriftWriteBlock ForbidDriftCase 处于 open/proven，广播被挡写。
	ErrDriftWriteBlock = errors.New("forbid drift case write-block: proof required before withdraw broadcast")
	// ErrProofMissing 证明字段缺失，不得解除禁提挡写。
	ErrProofMissing = errors.New("forbid proof missing: cannot clear write-block without valid proof")
	// ErrProofConfirmConflict 双人确认人与提交人相同。
	ErrProofConfirmConflict = errors.New("forbid proof confirmer must differ from submitter")
	// ErrUserRequired userID 为空。
	ErrUserRequired = errors.New("user_id required")
	// ErrInvalidActor actor 为空。
	ErrInvalidActor = errors.New("actor required for forbid operation")
	// ErrNoProofToExport 无证明可导出。
	ErrNoProofToExport = errors.New("no forbid proof to export")
)
