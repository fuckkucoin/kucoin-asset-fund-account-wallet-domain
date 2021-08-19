package ledseal

import "time"

// ─────────────────────────────────────────────────────────
// 调度计划 FSM 状态
// ─────────────────────────────────────────────────────────
// PlanState 调度计划状态枚举。

type PlanState string

const (
	// PlanActive 计划运行中，捕获泵会定期触发。
	PlanActive PlanState = "active"
	// PlanPaused 计划已暂停，不触发新批次。
	PlanPaused PlanState = "paused"
)

// PlanScope 计划覆盖范围。
type PlanScope string

const (
	// ScopeUserShard 按用户分片采样。
	ScopeUserShard PlanScope = "user_shard"
	// ScopeAssetFamily 按资产族采样（如 BTC 族、ETH 族）。
	ScopeAssetFamily PlanScope = "asset_family"
	// ScopeFullDay 全天全量采样。
	ScopeFullDay PlanScope = "full_day"
)

// ─────────────────────────────────────────────────────────
// 差分批次 FSM 状态
// ─────────────────────────────────────────────────────────
// BatchState 差分批次状态枚举。
type BatchState string

const (
	// BatchRunning 批次正在执行差分计算。
	BatchRunning BatchState = "running"
	// BatchSealed 批次已完成差分并封印根。
	BatchSealed BatchState = "sealed"
	// BatchFailed 批次执行失败（间隙/回滚/内部错误）。
	BatchFailed BatchState = "failed"
)

// ─────────────────────────────────────────────────────────
// 漂移严重程度
// ─────────────────────────────────────────────────────────
// DriftSeverity 漂移严重级别。
type DriftSeverity string

const (
	// SeverityNone 无漂移。
	SeverityNone DriftSeverity = "none"
	// SeverityMinor 轻微漂移：delta/total < 0.01%。
	SeverityMinor DriftSeverity = "minor"
	// SeverityMajor 重要漂移：delta/total >= 0.01% 且 < 1%。
	SeverityMajor DriftSeverity = "major"
	// SeverityCritical 严重漂移：delta/total >= 1% 或绝对值超阈值。
	SeverityCritical DriftSeverity = "critical"
)

// DriftKind 漂移类型。

type DriftKind string

const (
	// KindMutation 账本数值变化（非预期写入）。
	KindMutation DriftKind = "mutation"
	// KindAppearance 新增账户/科目（之前快照中不存在）。
	KindAppearance DriftKind = "appearance"
	// KindDisappearance 账户/科目消失（清零后在后续快照中不再出现）。
	KindDisappearance DriftKind = "disappearance"
)

// ─────────────────────────────────────────────────────────
// SnapPlan 快照调度计划
// ─────────────────────────────────────────────────────────
// SnapPlan 定义何时、对哪个范围触发快照抓拍。

type SnapPlan struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	PlanScope PlanScope `json:"plan_scope"`
	ShardKey  string    `json:"shard_key,omitempty"`
	AssetFamily string `json:"asset_family,omitempty"`
	IntervalSeconds int64      `json:"interval_seconds"`
	RetainCount     int        `json:"retain_count"`
	State           PlanState  `json:"state"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty"`
	NextRunAt *time.Time `json:"next_run_at,omitempty"`
	CreatedBy  string `json:"created_by"`
	FenceEpoch int64  `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

   func (p SnapPlan) Copy() SnapPlan { return p }

// ─────────────────────────────────────────────────────────
// SnapSlice 快照切片索引
// ─────────────────────────────────────────────────────────
// SnapSlice 记录每次捕获泵拍下的快照元信息（不含详细 Positions）。
type SnapSlice struct {
	ID     string `json:"id"`
	PlanID string `json:"plan_id"`
	SnapID      string `json:"snap_id"`
	UserID      string `json:"user_id,omitempty"`
	AssetFamily string `json:"asset_family,omitempty"`
	GenSeq      int64  `json:"gen_seq"`
	FenceEpoch int64 `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
	Checksum   string `json:"checksum"`
	PositionCount int       `json:"position_count"`
	State         string    `json:"state"`
	TakenAt       time.Time `json:"taken_at"`
	CreatedAt     time.Time `json:"created_at"`
}

  func (s SnapSlice) Copy() SnapSlice { return s }

// ─────────────────────────────────────────────────────────
// DiffBatch 差分批次
// ─────────────────────────────────────────────────────────
// DiffBatch 两代 SnapSlice 之间的增量差分结果元数据。

type DiffBatch struct {
	ID     string `json:"id"`
	PlanID string `json:"plan_id"`
	FromSliceID string `json:"from_slice_id"`
	ToSliceID   string `json:"to_slice_id"`
	FromGenSeq int64 `json:"from_gen_seq"`
	ToGenSeq   int64 `json:"to_gen_seq"`
	State          BatchState `json:"state"`
	DriftCount     int        `json:"drift_count"`
	TotalPositions int        `json:"total_positions"`
	MaxSeverity DriftSeverity `json:"max_severity"`
	SealRootID string `json:"seal_root_id,omitempty"`
	ErrMsg string `json:"err_msg,omitempty"`
	FenceEpoch int64  `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
	StartedAt time.Time  `json:"started_at"`
	SealedAt  *time.Time `json:"sealed_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

   func (b DiffBatch) Copy() DiffBatch { return b }

// ─────────────────────────────────────────────────────────
// DriftRow 漂移行明细
// ─────────────────────────────────────────────────────────
// DriftRow 一条账本头寸差异记录，代表单个 user/book/asset 的异常变动。
type DriftRow struct {
	ID string `json:"id"`
	BatchID     string `json:"batch_id"`
	PlanID      string `json:"plan_id"`
	UserID      string `json:"user_id"`
	Book        string `json:"book"`
	Asset       string `json:"asset"`
	AvailBefore string `json:"avail_before"`
	AvailAfter   string `json:"avail_after"`
	FrozenBefore string `json:"frozen_before"`
	FrozenAfter  string `json:"frozen_after"`
	DeltaAvail   string `json:"delta_avail"`
	DeltaFrozen string        `json:"delta_frozen"`
	Severity    DriftSeverity `json:"severity"`
	Kind        DriftKind     `json:"kind"`
	// BlockWriteHint 差分引擎发现此行应建议写封锁（不直接调用 cover 内核）。
	BlockWriteHint bool `json:"block_write_hint"`
	ConfirmedBy string     `json:"confirmed_by,omitempty"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

   func (d DriftRow) Copy() DriftRow { return d }

// ─────────────────────────────────────────────────────────
// SealRoot 封印根
// ─────────────────────────────────────────────────────────
// SealRoot 对一个 DiffBatch 的漂移明细和元数据做哈希封印。
// 输出供对账台和事件巡检台只读消费，不写账本。

type SealRoot struct {
	ID string `json:"id"`
	BatchID    string `json:"batch_id"`
	PlanID     string `json:"plan_id"`
	FromGenSeq int64  `json:"from_gen_seq"`
	ToGenSeq int64 `json:"to_gen_seq"`
	RootHash   string `json:"root_hash"`
	DriftCount int    `json:"drift_count"`
	PositionsHash string `json:"positions_hash"`
	MetaJSON   string     `json:"meta_json"`
	ExportedBy string     `json:"exported_by,omitempty"`
	ExportedAt *time.Time `json:"exported_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

  func (r SealRoot) Copy() SealRoot { return r }

// ─────────────────────────────────────────────────────────
// 请求/响应辅助结构
// ─────────────────────────────────────────────────────────
// CreatePlanReq 创建调度计划请求。
type CreatePlanReq struct {
	Name string `json:"name"`
	PlanScope PlanScope `json:"plan_scope"`
	ShardKey        string `json:"shard_key,omitempty"`
	AssetFamily     string `json:"asset_family,omitempty"`
	IntervalSeconds int64  `json:"interval_seconds"`
	RetainCount int    `json:"retain_count,omitempty"`
	CreatedBy   string `json:"created_by"`
}

// ConfirmDriftReq 人工确认漂移行请求（ops 运营台调用）。
type ConfirmDriftReq struct {
	DriftID string `json:"drift_id"`
	Actor   string `json:"actor"`
	Note    string `json:"note,omitempty"`
}

// ExportRootReq 导出封印根请求。

type ExportRootReq struct {
	BatchID string `json:"batch_id"`
	ExportedBy string `json:"exported_by"`
}

// AlertEvent 差分告警事件，由 alert pump 发布到总线。
type AlertEvent struct {
	BatchID    string `json:"batch_id"`
	PlanID     string `json:"plan_id"`
	DriftCount int    `json:"drift_count"`
	MaxSeverity DriftSeverity `json:"max_severity"`
	// BlockWriteSuggestions 建议封锁写操作的 DriftRow ID 列表（不直接调用 cover 内核）。
	BlockWriteSuggestions []string  `json:"block_write_suggestions,omitempty"`
	At                    time.Time `json:"at"`
}

// GapIncident 序号间隙或回滚告警，由 gap patrol 触发。

type GapIncident struct {
	PlanID      string `json:"plan_id"`
	ExpectedSeq int64  `json:"expected_seq"`
	ActualSeq int64     `json:"actual_seq"`
	Kind      string    `json:"kind"` // "gap" | "rollback"
	At        time.Time `json:"at"`
}

// ReplayCheck 回放一致性检查结果。

type ReplayCheck struct {
	SliceID string `json:"slice_id"`
	SnapID string `json:"snap_id"`
	FingerprintFrom string `json:"fingerprint_from"`
	FingerprintSnap string `json:"fingerprint_snap"`
	Match     bool      `json:"match"`
	CheckedAt time.Time `json:"checked_at"`
}
