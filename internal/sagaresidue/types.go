package sagaresidue

import (
	"time"
)

// ── Run 残留状态机 ────────────────────────────────────────────────────────────
// RunState 表示一个 saga Run 在本包中的残留处理状态。
// 进入本包的前提：safesaga.Partial.Cause != "" && !Partial.Closed（即补偿中断）。

type RunState string

const (
	// RunStatePartialFailed 补偿中途失败；Partial.Done 非空，残留未发现。
	RunStatePartialFailed RunState = "partial_failed"
	// RunStateResidueOpen 残留科目已发现，等待强制收口授权。
	RunStateResidueOpen RunState = "residue_open"
	// RunStateClosing 强制收口正在执行。
	RunStateClosing RunState = "closing"
	// RunStateClosed 所有收口腿已 applied/skipped，CloseProof 完整校验通过。
	RunStateClosed RunState = "closed"
	// RunStateEscalated 超过 SLA 阈值仍未收口；已升级至运营队列。
	RunStateEscalated RunState = "escalated"
)

// LegState 表示一条收口腿的执行状态。
type LegState string

const (
	LegStatePending LegState = "pending"
	LegStateApplied LegState = "applied"
	LegStateFailed  LegState = "failed"
	LegStateSkipped LegState = "skipped"
)

// LegKind 收口腿的操作类型。
type LegKind string

const (
	// LegKindReverse 冲正：对已落账的 ledger hop 发起反向划转，消除净额。
	LegKindReverse LegKind = "reverse"
	// LegKindSuspense 挂账：金额转入悬账科目（无法直接冲正时使用）。
	LegKindSuspense LegKind = "suspense"
	// LegKindFenceBlock 挡写封印：在幂等键上写 idem_block，防止同一 Run
	// 的 compensate 腿被重放或新 hop 续写。
	LegKindFenceBlock LegKind = "fence_block"
)

// Severity 残留的严重级别，影响告警先级与 SLA 阈值。
type Severity string

const (
	SeverityLow Severity = "low" // 金额极小 / 已接近 Closed
	SeverityMedium Severity = "medium" // 需人工确认，4h SLA
	SeverityHigh     Severity = "high"     // 资金净额 > 阈值，1h SLA
	SeverityCritical Severity = "critical" // 多科目残留 / 超 SLA 自动升级
)

// ── 核心实体 ─────────────────────────────────────────────────────────────────
// ResidueRun 关联一个 safesaga Run，记录其残留处理全生命周期。
// 本包不持有 safesaga.Run 的写权限，只以 RunID 做关联键。
// MutexGeneration 对应 sr_mutex_gens.generation，每次 Closeout 尝试前须持锁。

type ResidueRun struct {
	ID string `json:"id"` // sr_residue_runs.id（UUID）
	SagaRunID string `json:"saga_run_id"` // safesaga.Run.ID
	UserID    string `json:"user_id"`     // 资金归属用户
	Asset     string `json:"asset"`       // 残留资产标识
	PartialDoneSteps []string `json:"partial_done_steps"` // 补偿中断时已完成步骤快照
	PartialCause    string   `json:"partial_cause"` // 补偿中断原因（来自 Partial.Cause）
	State           RunState `json:"state"`
	Severity        Severity `json:"severity"`
	MutexGeneration int64    `json:"mutex_generation"` // 当前世代号，防并行 Closeout
	CloseoutPlanID string `json:"closeout_plan_id,omitempty"` // 关联收口计划
	CloseProofID string     `json:"close_proof_id,omitempty"` // 关联收口证明
	EscalatedAt  *time.Time `json:"escalated_at,omitempty"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ResidueItem 一条具体的残留科目。
// 由 Detector safesaga.Partial.Done 与 ledger hop 落账集合产出：
// - 若 hop 已入账（通过幂等键 idem_key 可查到 ledger.Transfer）
// 但 Partial.Undone 中没有该步骤的撤销记录，则视为残留。
// - IdemKey 与 safesaga 中的幂等键模式一致（eg. "{run_idem}-move"）。

type ResidueItem struct {
	ID           string `json:"id"` // UUID
	ResidueRunID string `json:"residue_run_id"`
	Step          string `json:"step"`            // safesaga.Step 名称
	IdemKey       string `json:"idem_key"`        // 该步骤的 ledger 幂等键
	LedgerEntryID string `json:"ledger_entry_id"` // 对应的 ledger.Transfer.ID（可空，未落账时为空）
	Amount        string `json:"amount"`          // DECIMAL(36,18) 文本，避免精度损失
	Asset         string `json:"asset"`
	Direction string `json:"direction"` // ledger.Direction 值
	HopState  string `json:"hop_state"` // ledger.State 值快照（发现时）
	IsConfirmed bool      `json:"is_confirmed"` // 已被双人授权确认为有效残留
	CreatedAt   time.Time `json:"created_at"`
}

// ResidueReport 一次扫描的完整结果，包含多条 ResidueItem。
// 同一 SagaRunID 可能有多份 Report（每次扫描均产出），
// 以最新一份作为强制收口计划的输入。

type ResidueReport struct {
	ID string `json:"id"` // UUID
	SagaRunID string `json:"saga_run_id"`
	ScanEpoch   int64         `json:"scan_epoch"` // 单调递增扫描序号
	Items       []ResidueItem `json:"items"`
	TotalAmount string        `json:"total_amount"` // 各 item 金额总和
	Severity    Severity      `json:"severity"`
	ScanNote    string        `json:"scan_note,omitempty"` // 扫描诊断信息
	ScannedAt time.Time `json:"scanned_at"`
}

// CloseoutPlan 对一个 ResidueRun 的强制收口计划。
// 一个 ResidueRun 至多一份活跃 Plan；Plan 内腿按 SeqNo 顺序执行，
// 幂等键 IdemPrefix + leg.IdemSuffix 组成全局唯一键。

type CloseoutPlan struct {
	ID           string `json:"id"` // UUID
	ResidueRunID string `json:"residue_run_id"`
	SagaRunID  string `json:"saga_run_id"`
	IdemPrefix string `json:"idem_prefix"` // 所有腿的幂等键前缀
	Authorizer1 string `json:"authorizer1"` // 双人授权第一人
	Authorizer2 string `json:"authorizer2"` // 双人授权第二人（不得与 Authorizer1 相同）
	Generation int64 `json:"generation"` // 持锁时的世代号
	TotalLegs   int      `json:"total_legs"`
	AppliedLegs int      `json:"applied_legs"`
	FailedLegs  int      `json:"failed_legs"`
	SkippedLegs int      `json:"skipped_legs"`
	State       RunState `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CloseoutLeg 一条收口动作腿。
// 收口腿幂等键：{CloseoutPlan.IdemPrefix}-{IdemSuffix}，保证断点续跑安全。
// SeqNo 决定执行顺序；失败后不自动跳过，需运营介入或标 Skipped。
type CloseoutLeg struct {
	ID           string `json:"id"` // UUID
	PlanID       string `json:"plan_id"`
	ResidueRunID string `json:"residue_run_id"`
	SeqNo int     `json:"seq_no"` // 执行顺序（从 1 开始）
	Kind  LegKind `json:"kind"`
	Step  string  `json:"step"` // 对应的 saga Step
	IdemSuffix string `json:"idem_suffix"` // 与 Plan.IdemPrefix 拼接成完整幂等键
	Amount string `json:"amount"`
	Asset  string `json:"asset"`
	Direction     string   `json:"direction"`
	State         LegState `json:"state"`
	LedgerEntryID string   `json:"ledger_entry_id,omitempty"` // 执行后写入
	FailReason string `json:"fail_reason,omitempty"`
	SkipReason string `json:"skip_reason,omitempty"`
	RetryCount int        `json:"retry_count"`
	AppliedAt  *time.Time `json:"applied_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CloseProof 收口终态证明。
// 证明字段定义：
// IntentHash = SHA256(sorted JSON of Partial.Done steps)
// ResidueHash = SHA256(sorted JSON of confirmed ResidueItems)
// LegHash = SHA256(sorted JSON of applied CloseoutLegs)
// ProofDigest = SHA256(IntentHash || ResidueHash || LegHash)
// 证明缺失或 ProofDigest 校验失败，不得将 ResidueRun 标为 Closed。
type CloseProof struct {
	ID           string `json:"id"` // UUID
	ResidueRunID string `json:"residue_run_id"`
	SagaRunID  string `json:"saga_run_id"`
	PlanID     string `json:"plan_id"`
	IntentHash string `json:"intent_hash"` // 意图集合哈希
	ResidueHash string `json:"residue_hash"` // 残留集合哈希
	LegHash string `json:"leg_hash"` // 收口腿哈希
	ProofDigest string    `json:"proof_digest"` // 三者组合哈希
	IsValid     bool      `json:"is_valid"`     // 最近一次校验结果
	VerifiedAt  time.Time `json:"verified_at"`
	CreatedAt time.Time `json:"created_at"`
}

// MutexGen 补偿/收口互斥世代锁。
// 同一 SagaRunID 最多一个持锁 holder。
// 持锁协议：
// Acquire: INSERT OR UPDATE generation=generation+1, holder=?, status='locked', expires_at=now+TTL
// WHERE status IN ('open','expired','released') AND saga_run_id=?
// Refresh: UPDATE expires_at=now+TTL WHERE saga_run_id=? AND holder=? AND generation=?
// Release: UPDATE status='released' WHERE saga_run_id=? AND holder=? AND generation=?
// ErrStaleGen：期望的 generation 与数据库当前值不符，调用方须重新 Acquire。

type MutexGen struct {
	ID         string `json:"id"` // UUID
	SagaRunID  string `json:"saga_run_id"`
	Generation int64  `json:"generation"` // 单调递增，每次 Acquire 自增
	Holder     string `json:"holder"`     // worker 标识（hostname:pid:uuid）
	Status     string     `json:"status"` // open / locked / expired / released
	ExpiresAt  time.Time  `json:"expires_at"`
	LockedAt   *time.Time `json:"locked_at,omitempty"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// ── 请求/响应 DTO ─────────────────────────────────────────────────────────────
// DetectReq 发起残留扫描请求。

type DetectReq struct {
	SagaRunID string `json:"saga_run_id"` // 必填
	ForceRescan bool   `json:"force_rescan"` // 是否强制重扫（忽略已有 Report）
	ScanActor   string `json:"scan_actor"`   // 触发扫描的 actor（worker 或 ops）
}

// CloseoutReq 双人授权的强制收口请求。

type CloseoutReq struct {
	ResidueRunID string `json:"residue_run_id"` // 必填
	Authorizer1  string `json:"authorizer1"`    // 必填，发起人
	Authorizer2 string `json:"authorizer2"` // 必填，复核人（≠ Authorizer1）
	IdemPrefix string `json:"idem_prefix"` // 收口腿幂等前缀（调用方提供，防重放）
}

// ConfirmResidueReq 确认残留科目为有效（双人二次确认后）。

type ConfirmResidueReq struct {
	ResidueRunID string   `json:"residue_run_id"`
	ItemIDs      []string `json:"item_ids"` // 待确认的 ResidueItem ID 列表
	Authorizer1  string   `json:"authorizer1"`
	Authorizer2  string   `json:"authorizer2"`
}

// SkipLegReq 运营标记某腿为 skipped（无法冲正时使用，需双人授权）。
type SkipLegReq struct {
	PlanID string `json:"plan_id"`
	LegID      string `json:"leg_id"`
	SkipReason string `json:"skip_reason"` // 必填，审计用
	Authorizer1 string `json:"authorizer1"`
	Authorizer2 string `json:"authorizer2"`
}

// ProofVerifyReq 触发证明校验。
type ProofVerifyReq struct {
	ProofID string `json:"proof_id"`
	ResidueRunID string `json:"residue_run_id"` // 与 ProofID 至少提供一个
}

// RunListReq 残留 Run 列表查询。
type RunListReq struct {
	SagaRunID string   `json:"saga_run_id,omitempty"`
	State     RunState `json:"state,omitempty"`
	Severity  Severity `json:"severity,omitempty"`
	UserID    string   `json:"user_id,omitempty"`
	Limit int `json:"limit,omitempty"`
	After string `json:"after,omitempty"` // 游标翻页
}

// LegListReq 收口腿列表查询。

type LegListReq struct {
	PlanID       string `json:"plan_id,omitempty"`
	ResidueRunID string `json:"residue_run_id,omitempty"`
	State LegState `json:"state,omitempty"`
	Limit int `json:"limit,omitempty"`
}

// ── 常量与阈值 ────────────────────────────────────────────────────────────────

const (
	// DefaultMutexTTL 世代锁默认有效期（90 秒）；worker 须在到期前 Refresh。
	DefaultMutexTTL = 90 * time.Second
	// DefaultSLAMedium 中级残留最长 SLA（4 小时），超时升级为 critical。
	DefaultSLAMedium = 4 * time.Hour
	// DefaultSLAHigh 高级残留 SLA（1 小时）。
	DefaultSLAHigh = 1 * time.Hour
	// DefaultSLACritical critical 不降级，触发即入运营队列。
	DefaultSLACritical = 15 * time.Minute
	// MaxLegRetry 收口腿最大自动重试次数，超限进入 failed 等待人工。
	MaxLegRetry = 5
	// DefaultScanBatchSize 单次扫描最多处理的 saga Run 数量。
	DefaultScanBatchSize = 100
)
