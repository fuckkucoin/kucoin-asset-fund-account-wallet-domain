package outboxonce

import "time"

// Event 出箱事件。SentAt 非空表示已投递。Epoch/Token 构成围栏。

type Event struct {
	ID  string `json:"id"`
	Key string `json:"key"`
	Kind    string `json:"kind"`
	Payload string `json:"payload"`
	At time.Time `json:"at"`
	SentAt *time.Time `json:"sent_at,omitempty"`
	Epoch int64  `json:"epoch"`
	Token string `json:"token"`
	// HaltGen 记录本次 Deliver 绑定的停机世代，0 表示停机世代尚未引入。
	HaltGen int64 `json:"halt_gen,omitempty"`
}

// DeliverResult 单次投递结果。

type DeliverResult struct {
	Key string `json:"key"`
	ID  string `json:"id"`
	Delivered bool `json:"delivered"`
	Replay bool `json:"replay"`
	// HaltGen 投递绑定的停机世代。
	HaltGen int64 `json:"halt_gen,omitempty"`
}

// HaltState 停机 FSM 状态。

type HaltState string

const (
	// HaltStateCleared 停机未激活（默认）。
	HaltStateCleared HaltState = "cleared"
	// HaltStateRaised SetHalt(true) 已调用，世代已抬升，等待在途 Deliver 吊销完成。
	HaltStateRaised HaltState = "raised"
	// HaltStateBlocking 所有在途 Deliver 已吊销，拦截新 Deliver。
	HaltStateBlocking HaltState = "blocking"
)

// HaltLease 停机世代租约。记录当前停机世代与 FSM 状态。
// 表：outboxonce_halt_leases

type HaltLease struct {
	ID         string    `json:"id"`
	Generation int64     `json:"generation"`
	State      HaltState `json:"state"`
	SetBy string `json:"set_by"`
	SetAt     time.Time  `json:"set_at"`
	ClearedAt *time.Time `json:"cleared_at,omitempty"`
	// RevokedCount 本次 Halt 期间吊销的在途 Deliver 数量。
	RevokedCount int `json:"revoked_count"`
}

// OnceHaltCaseKind 双投/漏投证明类型。

type OnceHaltCaseKind string

const (
	// CaseKindDoubleDeliver 同一 key 在 Halt 期间或切换夹缝出现双 Mark。
	CaseKindDoubleDeliver OnceHaltCaseKind = "double_deliver"
	// CaseKindHaltRevoked Halt 期间 Deliver 被吊销。
	CaseKindHaltRevoked OnceHaltCaseKind = "halt_revoked"
	// CaseKindStaleFence Halt 解除后旧 epoch/token 尝试复活投递。
	CaseKindStaleFence OnceHaltCaseKind = "stale_fence"
	// CaseKindDoubleMark 同一 key 两次 Mark 成功（理论上不应发生，一旦发现立即上报）。
	CaseKindDoubleMark OnceHaltCaseKind = "double_mark"
)

// OnceHaltCase 双投/漏投证明记录。只追加，不修改。
// 表：outboxonce_halt_cases

type OnceHaltCase struct {
	ID   string           `json:"id"`
	Kind OnceHaltCaseKind `json:"kind"`
	Key string `json:"key"`
	EventID    string    `json:"event_id"`
	HaltGen    int64     `json:"halt_gen"`
	Epoch      int64     `json:"epoch"`
	Token      string    `json:"token"`
	ConflictID string    `json:"conflict_id,omitempty"`
	Evidence   string    `json:"evidence"`
	DetectedAt time.Time `json:"detected_at"`
	ExportedAt *time.Time `json:"exported_at,omitempty"`
}

// DeliverGeneration Deliver 调用绑定的停机世代快照。
// 用于在 Deliver 执行期间跨越 Halt 切换夹缝时的 CAS 校验。

type DeliverGeneration struct {
	Key string `json:"key"`
	EventID string `json:"event_id"`
	HaltGen int64  `json:"halt_gen"`
	Epoch int64 `json:"epoch"`
	Token    string    `json:"token"`
	LockedAt time.Time `json:"locked_at"`
	// Revoked 表示本次 Deliver 尝试已被吊销（Halt 切换发生在 Deliver 执行窗口）。
	Revoked bool `json:"revoked"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// HaltGenReadView 供外部只读消费的停机世代视图。

type HaltGenReadView struct {
	Generation int64 `json:"generation"`
	Halted     bool  `json:"halted"`
	State HaltState `json:"state"`
	AsOf time.Time `json:"as_of"`
}

// ProofExport 证明导出摘要。

type ProofExport struct {
	HaltGen int64 `json:"halt_gen"`
	Cases      []OnceHaltCase `json:"cases"`
	ExportedAt time.Time      `json:"exported_at"`
	Total      int            `json:"total"`
}
