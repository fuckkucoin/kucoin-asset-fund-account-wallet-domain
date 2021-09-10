package oncehaltgate

import "time"

// GateState 闸状态机。
type GateState string

const (
	// GateOpen 正常投递状态。
	GateOpen GateState = "open"
	// GateRaising SetHalt(true) 已触发，世代抬升中，在途 Deliver 吊销扫描进行中。
	GateRaising GateState = "raising"
	// GateBlocking 吊销完成，拦截新 Deliver。
	GateBlocking GateState = "blocking"
	// GateClearing SetHalt(false) 已触发，世代再次抬升，正在清除拦截状态。
	GateClearing GateState = "clearing"
)

// SwitchRequest SetHalt 请求。
type SwitchRequest struct {
	On bool `json:"on"`
	Actor string `json:"actor"`
	// Reason 可选操作原因，用于审计日志。
	Reason string `json:"reason,omitempty"`
}

// SwitchResult SetHalt 结果。

type SwitchResult struct {
	Generation  int64     `json:"generation"`
	State       GateState `json:"state"`
	RevokedKeys []string  `json:"revoked_keys,omitempty"`
	RevokedAt   time.Time `json:"revoked_at"`
	CaseCount int `json:"case_count"`
}

// DeliverRequest 投递请求。

type DeliverRequest struct {
	Key   string `json:"key"`
	Epoch int64  `json:"epoch"`
	Token string `json:"token"`
}

// DeliverResponse 投递响应。
type DeliverResponse struct {
	Key string `json:"key"`
	ID        string `json:"id"`
	Delivered bool   `json:"delivered"`
	Replay    bool   `json:"replay"`
	HaltGen   int64  `json:"halt_gen"`
}

// RevokeReport 在途 Deliver 吊销扫描报告。
type RevokeReport struct {
	ScannedAt time.Time `json:"scanned_at"`
	InFlight    int   `json:"in_flight"`
	Revoked     int   `json:"revoked"`
	AlreadyDone int   `json:"already_done"`
	Generation  int64 `json:"generation"`
}

// PatrolReport 双 Mark 巡检报告。

type PatrolReport struct {
	ScannedAt time.Time `json:"scanned_at"`
	Checked   int       `json:"checked"`
	DoubleMarks int `json:"double_marks"`
	NewCases    int `json:"new_cases"`
}

// GateStatus 闸实时状态（运维查询用）。
type GateStatus struct {
	State      GateState `json:"state"`
	Generation int64     `json:"generation"`
	InFlight int `json:"in_flight"`
	RevokedFence int `json:"revoked_fence"`
	CaseCount    int `json:"case_count"`
	AsOf time.Time `json:"as_of"`
}

// ProofQuery 证明查询参数。
type ProofQuery struct {
	// HaltGen 按停机世代过滤；0 表示不过滤。
	HaltGen int64 `form:"halt_gen"`
	// Kind 按证明类型过滤；空表示不过滤。
	Kind string `form:"kind"`
	// Limit 分页大小；0 或负数使用默认值 50。
	Limit int `form:"limit"`
}
