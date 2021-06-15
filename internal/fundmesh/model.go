// @FileName: model.go
// @Summary: 资金 mesh 取消单、旁路拨号与配置快照模型。
// 重点：取消记录按提现单幂等；观察面与资金写分域。
// 风险点：终态单不得被二次解冻。
// @Author: Draven
// @Created: 2021.06.14

package fundmesh

import "time"

const (
	CancelPending    CancelState = "pending"
	CancelAccepted   CancelState = "accepted"
	CancelIdempotent CancelState = "idempotent"
	CancelDenied CancelState = "denied"
	CancelClosed CancelState = "closed"
	WithdrawInitiated WithdrawPhase = "initiated"
	WithdrawFrozen    WithdrawPhase = "frozen"
	WithdrawScreening WithdrawPhase = "screening"
	WithdrawSigning   WithdrawPhase = "signing"
	WithdrawBroadcast WithdrawPhase = "broadcast"
	WithdrawConfirmed WithdrawPhase = "confirmed"
	WithdrawCancelled WithdrawPhase = "cancelled"
	WithdrawFailed    WithdrawPhase = "failed"
	EntryDELETE EntryKind = "DELETE"
	EntryPOST   EntryKind = "POST"
	CodeOK = "cancel_ok"
	CodeIdempotent = "cancel_idempotent"
	CodeNotFound   = "withdraw_not_found"
	CodeDenied     = "cancel_denied"
	CodeTerminal  = "cancel_terminal"
	CodeBroadcast = "already_broadcast"
	CodeConfirmed = "already_confirmed"
	CodeFailed     = "already_failed"
	CodeDrift      = "config_drift"
	CodeRemoteFail = "ledger_remote_failed"
	CodeSilentDenied    = "silent_local_denied"
	CodeUnknownSideCall = "sidecall_unknown"
	CodeInput          = "input_invalid"
	CodeOutboundOK     = "outbound_ok"
	CodeOutboundReplay = "outbound_replay"
	CodeOutboundConflict  = "outbound_conflict"
	CodeOutboundDenied    = "outbound_denied"
	CodeOutboundCancelled = "outbound_cancelled"
	CodeOutboundDupDebit = "outbound_dup_debit"
	OutboundAccepted OutboundState = "accepted"
	OutboundReplayed OutboundState = "replayed"
	OutboundConflict OutboundState = "conflict"
	OutboundDenied   OutboundState = "denied"
	OutboundClosed OutboundState = "closed"
	IdemSchemeV1   = "fmout.v1"
	IdemKindClient = "c"
	IdemKindDerived = "d"
)

type OutboundState string

type CancelState string

type WithdrawPhase string

type EntryKind string

// CancelRequest 双入口归一后的取消意图。

type CancelRequest struct {
	WithdrawID string
	UserID string
	Actor string
	Reason         string
	IdempotencyKey string
	Entry          EntryKind
	Method         string
	Path           string
}

// CancelResult 双入口共用回包。
type CancelResult struct {
	WithdrawID string      `json:"withdraw_id"`
	State      CancelState `json:"state"`
	Phase      WithdrawPhase `json:"phase"`
	Code       string        `json:"code"`
	Idempotent bool          `json:"idempotent"`
	Entry      EntryKind     `json:"entry"`
	Reason     string        `json:"reason"`
	At         time.Time     `json:"at"`
}

// CancelRecord 取消意图落库。

type CancelRecord struct {
	RecordID string
	WithdrawID string
	UserID string
	Actor          string
	Reason         string
	IdempotencyKey string
	Entry EntryKind
	State CancelState
	Phase WithdrawPhase
	Code     string
	RemoteOK bool
	Note     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// WithdrawView 远端账本只读投影。
type WithdrawView struct {
	ID     string
	UserID string
	Phase  WithdrawPhase
	TxHash string
	Live bool
}

// SideCallSpec 已注册旁路名。
type SideCallSpec struct {
	Name string
	Service string
	Method  string
	Path    string
	Write bool
}

// ObservePing 观察组心跳，禁止资金写。

type ObservePing struct {
	OK bool `json:"ok"`
	Group string `json:"group"`
	Writes int64     `json:"writes"`
	At     time.Time `json:"at"`
	Host      string `json:"host"`
	FaultOpen bool   `json:"fault_open"`
}

// ConfigSnapshot etcd/配置中心投影。

type ConfigSnapshot struct {
	Kind string
	Keys map[string]string
	Source string
	At     time.Time
}

// DriftProof 缺键或漂移证明。
type DriftProof struct {
	ProofID string `json:"proof_id"`
	Kind    string   `json:"kind"`
	Missing []string `json:"missing"`
	Mismatched []string `json:"mismatched"`
	Closed bool `json:"closed"`
	At     time.Time `json:"at"`
	Detail string    `json:"detail"`
}

// FallbackProof 远端失败时拒绝进程内兜底的证明。
type FallbackProof struct {
	ProofID string `json:"proof_id"`
	WithdrawID string `json:"withdraw_id"`
	RemoteErr  string `json:"remote_err"`
	LocalTried bool   `json:"local_tried"`
	Denied bool `json:"denied"`
	At time.Time `json:"at"`
}

// WriteAttempt 关键写尝试轨迹。

type WriteAttempt struct {
	AttemptID  string
	Kind       string
	WithdrawID string
	Allowed    bool
	ClosedBy   string
	At time.Time
}
