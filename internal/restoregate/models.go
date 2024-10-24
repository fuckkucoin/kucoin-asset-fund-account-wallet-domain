package restoregate

import "time"

// PlanState RestorePlan 的 FSM 状态。
// 合法转换路径：
// planned → freezing_writes → importing → verifying → sealed
// * → aborted（任意非 sealed 状态均可中止）

type PlanState string

const (
	StatePlanned PlanState = "planned"
	StateFreezing  PlanState = "freezing_writes"
	StateImporting PlanState = "importing"
	StateVerifying PlanState = "verifying"
	StateSealed PlanState = "sealed"
	StateAborted PlanState = "aborted"
)

// AllowedTransitions 合法状态转换表。
var AllowedTransitions = map[PlanState][]PlanState{
	StatePlanned: {StateFreezing,
		StateAborted},
	StateFreezing: {StateImporting, StateAborted},
	StateImporting: {StateVerifying, StateAborted},
	StateVerifying: {StateSealed, StateAborted},
	StateSealed:    {},
	StateAborted:   {},
}

// RestorePlan 灾备回灌计划聚合根。
// TargetGen 为本次回灌声明的目标世代；LiveWatermark 为创建计划时读取的活账水位快照。
// 若 LiveWatermark > TargetGen 则世代冲突，整单失败。

type RestorePlan struct {
	ID     string `json:"id"`
	BlobID string `json:"blob_id"` // backup 密文 blob ID
	SnapID          string `json:"snap_id"`           // snap 快照 ID（可选，用于余额根核对）
	TargetGen       int64  `json:"target_gen"`        // 声明目标世代
	LiveWatermark   int64  `json:"live_watermark"`    // 计划创建时的活账水位
	BalanceRootPre  string `json:"balance_root_pre"`  // 回灌前余额根 checksum
	BalanceRootPost string `json:"balance_root_post"` // 回灌后余额根 checksum（verifying 阶段填入）
	State         PlanState  `json:"state"`
	Creator       string     `json:"creator"`
	Reason        string     `json:"reason"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	FrozenAt      *time.Time `json:"frozen_at,omitempty"`
	ImportStartAt *time.Time `json:"import_start_at,omitempty"`
	VerifyStartAt *time.Time `json:"verify_start_at,omitempty"`
	SealedAt      *time.Time `json:"sealed_at,omitempty"`
	AbortedAt *time.Time `json:"aborted_at,omitempty"`
	AbortReason string `json:"abort_reason,omitempty"`
}

// GenGate 世代闸记录：校验回灌世代合法性的证明。
// 每次 Import 调用创建一条，记录决策依据。失败条目保留供审计。
type GenGate struct {
	ID        string `json:"id"`
	PlanID    string `json:"plan_id"`
	TargetGen int64  `json:"target_gen"`
	SystemGen int64  `json:"system_gen"` // 校验时系统世代
	LiveWatermark int64     `json:"live_watermark"` // 校验时活账水位
	Allowed       bool      `json:"allowed"`
	DenyReason    string    `json:"deny_reason,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// WriteMutexState 写互斥锁状态。

type WriteMutexState string

const (
	MutexActive WriteMutexState = "active"
	MutexReleased WriteMutexState = "released"
	MutexExpired WriteMutexState = "expired"
)

// WriteMutex 回灌窗口写互斥锁。
// 回灌进入 freezing_writes 时创建并激活；sealed 或 aborted 后释放。
// 活锁期间所有贷记/冻结/划转意图被阻断并记录在 BlockedWrites 审计日志中。
type WriteMutex struct {
	ID string `json:"id"`
	PlanID     string          `json:"plan_id"`
	State      WriteMutexState `json:"state"`
	HolderGen  int64           `json:"holder_gen"` // 持锁世代
	ExpiresAt  time.Time       `json:"expires_at"` // 看门狗超时时间
	AcquiredAt time.Time       `json:"acquired_at"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// BlockedWrite 被写互斥锁阻断的写意图审计日志。

type BlockedWrite struct {
	ID string `json:"id"`
	MutexID string `json:"mutex_id"`
	PlanID string `json:"plan_id"`
	Kind string `json:"kind"` // credit / freeze / transfer_intent
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

// HalfSeal 半恢复封印记录。
// Import 中途失败时，已写入的 KV 键集合进入 HalfSeal，
// 阻止同一 PlanID 在未显式续/中止前重新 Import。

type HalfSeal struct {
	ID     string `json:"id"`
	PlanID string `json:"plan_id"`
	WrittenKeys []string `json:"written_keys"` // 已写入的 kind|key 列表
	FailedAt   time.Time  `json:"failed_at"`
	FailReason string     `json:"fail_reason"`
	Resumed    bool       `json:"resumed"`
	ResumedAt  *time.Time `json:"resumed_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SplitBrainState SplitBrainProof 的状态。
type SplitBrainState string

const (
	SplitBrainOpen     SplitBrainState = "open"     // 已检出，挡写
	SplitBrainClearing SplitBrainState = "clearing" // 四眼审批中
	SplitBrainCleared  SplitBrainState = "cleared"  // 四眼通过，解除挡写
	SplitBrainOverride SplitBrainState = "override" // 运维强制覆盖
)

// SplitBrainProof 回灌脑裂证明。
// 回灌后余额根与计划目标不符时创建，状态 open；
// 四眼解除（两名不同角色审批人确认）后转 cleared，重新开放写路径。

type SplitBrainProof struct {
	ID              string `json:"id"`
	PlanID          string `json:"plan_id"`
	BalanceRootPre  string `json:"balance_root_pre"`
	BalanceRootPost string `json:"balance_root_post"`
	BalanceRootTarget string `json:"balance_root_target"` // 计划声明的目标根
	DiffSummary string          `json:"diff_summary"`
	State       SplitBrainState `json:"state"`
	OpenedAt    time.Time       `json:"opened_at"`
	ClearedAt   *time.Time      `json:"cleared_at,omitempty"`
	Approvals   []ProofApproval `json:"approvals"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProofApproval 四眼审批条目。
type ProofApproval struct {
	ProofID string `json:"proof_id"`
	Approver  string    `json:"approver"`
	Role      string    `json:"role"` // ops / risk / finance
	Comment   string    `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
}

// RestoreGenRecord 系统级世代水位记录。用于比较 LiveWatermark 与 TargetGen。
type RestoreGenRecord struct {
	ID string `json:"id"` // 通常是全局单例键
	CurrentGen int64     `json:"current_gen"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// PlanSummary 计划列表摘要，避免传输大字段。

type PlanSummary struct {
	ID     string `json:"id"`
	BlobID string `json:"blob_id"`
	State     PlanState `json:"state"`
	TargetGen int64     `json:"target_gen"`
	Creator string `json:"creator"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
