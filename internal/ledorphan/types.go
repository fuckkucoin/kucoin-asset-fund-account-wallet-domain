package ledorphan

import "time"

// ── 批次世代状态 ──────────────────────────────────────────────────────────────
// GenStatus 批次世代锁的生命周期。
// open → 无活跃 worker 持有锁；
// locked → 某 worker 正在续提；
// expired → worker 上次续锁已超时，锁自动释放；
// released → worker 主动释放。

type GenStatus string

const (
	GenStatusOpen   GenStatus = "open"
	GenStatusLocked GenStatus = "locked"
	GenStatusExpired GenStatus = "expired"
	GenStatusReleased GenStatus = "released"
)

// BatchGeneration 批次世代锁。
// 每次崩溃重启续提时，generation 字段单调递增；
// 任何获得旧 generation 的 worker 调用 Submit/Seal 均返回 ErrStaleGen。

type BatchGeneration struct {
	// BatchID 对应 ledbatch.Batch.ID。
	BatchID string `json:"batch_id"`
	// Generation 当前世代号，从 1 开始，每次续提前 CAS +1。
	Generation int64 `json:"generation"`
	// WorkerID 当前持有世代锁的 worker 标识（hostname:pid:uuid 形态）。
	WorkerID string `json:"worker_id"`
	// Status 见 GenStatus。
	Status GenStatus `json:"status"`
	// LockedAt 本次 CAS 成功时刻。
	LockedAt time.Time `json:"locked_at"`
	// ExpiresAt worker 必须在此时刻前通过 Refresh 续锁；过期后其他 worker 可抢锁。
	ExpiresAt time.Time `json:"expires_at"`
	// ReleasedAt worker 主动释放时刻（仅 released 状态有效）。
	ReleasedAt *time.Time `json:"released_at,omitempty"`
	// UpdatedAt 最后变更时刻。
	UpdatedAt time.Time `json:"updated_at"`
}

// GenLockTTL 世代锁默认存活时长；worker 必须在此之前 Refresh，否则视为宕机释放。

const GenLockTTL = 90 * time.Second

// ── 孤儿状态机 ───────────────────────────────────────────────────────────────
// OrphanStatus 孤儿分录的生命周期。
// detected → 孤儿已发现但尚未处置；
// sealed → 孤儿已封堵（idempotency key 列入拒写集，不能再续写同 idem 的 hop）；
// reversed → 孤儿已冲正（对应 ledger hop 已发起反向分录）；
// ignored → 经双人审核认定无需处置（例如该 hop 实际已成功但 batch 状态不同步）。
type OrphanStatus string

const (
	OrphanDetected OrphanStatus = "detected"
	OrphanSealed OrphanStatus = "sealed"
	OrphanReversed OrphanStatus = "reversed"
	OrphanIgnored OrphanStatus = "ignored"
)

// OrphanKind 孤儿类型，用于区分两种不同的半提交场景。
type OrphanKind string

const (
	// OrphanHanging — batch 记录该 item 为 pending，但 ledger 中已有对应 hop。
	// 场景：崩溃发生在 ledger.Create 成功之后、内存状态回写之前。
	OrphanHanging OrphanKind = "hanging"
	// OrphanMissing — batch 记录该 item 为 submitted（有 LedgerID），但 ledger 中找不到。
	// 场景：crash 发生在内存标记 submitted 之后、ledger 落盘之前（极小时间窗口）。
	OrphanMissing OrphanKind = "missing"
)

// OrphanItem 单条孤儿分录记录。
type OrphanItem struct {
	// OrphanID 本孤儿记录的唯一标识（UUID）。
	OrphanID string `json:"orphan_id"`
	// BatchID 所属批次。
	BatchID string `json:"batch_id"`
	// Seq 在原批次 Items 中的顺序号（1-based）。
	Seq int `json:"seq"`
	// UserID 账户 ID。
	UserID string `json:"user_id"`
	// Asset 资产符号。
	Asset string `json:"asset"`
	// Amount 金额（decimal 字符串）。
	Amount string `json:"amount"`
	// Direction hop 方向（与 ledbatch.Item.Direction 一致）。
	Direction string `json:"direction"`
	// IdempotencyKey 原 hop 幂等键。
	IdempotencyKey string `json:"idempotency_key"`
	// Kind 孤儿类型，见 OrphanKind。
	Kind OrphanKind `json:"kind"`
	// Status 当前状态，见 OrphanStatus。
	Status OrphanStatus `json:"status"`
	// LedgerID 若 ledger 中已有对应记录，此字段非空（hanging 场景）。
	LedgerID string `json:"ledger_id,omitempty"`
	// BatchLedgerID 批次内存中记录的 LedgerID（missing 场景下可能有值）。
	BatchLedgerID string `json:"batch_ledger_id,omitempty"`
	// DetectedAt 首次发现时刻。
	DetectedAt time.Time `json:"detected_at"`
	// SealedAt 封堵完成时刻（仅 sealed 有效）。
	SealedAt *time.Time `json:"sealed_at,omitempty"`
	// ReversedAt 冲正完成时刻（仅 reversed 有效）。
	ReversedAt *time.Time `json:"reversed_at,omitempty"`
	// Note 人工备注或处置原因。
	Note string `json:"note,omitempty"`
}

// ── 孤儿报告 ─────────────────────────────────────────────────────────────────
// ReportStatus 孤儿报告状态。
type ReportStatus string

const (
	// ReportOpen 报告已生成，孤儿尚有 detected 状态，等待处置。
	ReportOpen ReportStatus = "open"
	// ReportClosed 所有孤儿已完成处置（sealed/reversed/ignored），并已生成完整性证明。
	ReportClosed ReportStatus = "closed"
	// ReportSuperseded 该批次的新一轮扫描已产生更新报告，本报告作废。
	ReportSuperseded ReportStatus = "superseded"
)

// OrphanReport 批次孤儿清单。
// 一个批次可能在多次崩溃重启中多次产生报告；每次产生时旧报告状态变为 superseded。

type OrphanReport struct {
	// ReportID 本报告唯一标识（UUID）。
	ReportID string `json:"report_id"`
	// BatchID 所属批次。
	BatchID string `json:"batch_id"`
	// Generation 检测时对应的世代号（与 BatchGeneration.Generation 一致）。
	Generation int64 `json:"generation"`
	// Status 报告状态，见 ReportStatus。
	Status ReportStatus `json:"status"`
	// Items 孤儿分录列表（可为空，表示无孤儿）。
	Items []OrphanItem `json:"items"`
	// TotalOrphans 孤儿数量（== len(Items)）。
	TotalOrphans int `json:"total_orphans"`
	// HangingCount OrphanHanging 类型数量。
	HangingCount int `json:"hanging_count"`
	// MissingCount OrphanMissing 类型数量。
	MissingCount int `json:"missing_count"`
	// IntentHash 意图集合的 SHA-256 哈希（用于不可变性绑定）。
	IntentHash string `json:"intent_hash"`
	// LedgerHash 已落账 hop 集合的 SHA-256 哈希。
	LedgerHash string `json:"ledger_hash"`
	// CreatedAt 报告生成时刻。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最后变更时刻（孤儿状态变化时更新）。
	UpdatedAt time.Time `json:"updated_at"`
}

// ── Seal/Reverse 证明 ────────────────────────────────────────────────────────
// SealKind 封堵或冲正动作类型。
type SealKind string

const (
	// SealKindBlock 纯封堵：将孤儿 item 的 IdempotencyKey 写入 idem 拒写集，禁止续写同 idem 的 hop。
	SealKindBlock SealKind = "block"
	// SealKindReverse 受控冲正：对 hanging 孤儿发起反向分录，将已落账的 hop 借贷方对调。
	SealKindReverse SealKind = "reverse"
	// SealKindIgnore 双人审核后认定该孤儿无需处置（如实际业务已自恢复）。
	SealKindIgnore SealKind = "ignore"
)

// SealStatus 封堵动作状态。

type SealStatus string

const (
	SealPending SealStatus = "pending" // 封堵/冲正已提交，等待执行
	SealApplied SealStatus = "applied" // 已成功执行
	SealFailed  SealStatus = "failed"  // 执行失败（记录原因，可重试）
	SealAborted SealStatus = "aborted" // 被第二审核人拒绝
)

// SealAction 封堵动作证明记录，每次 Seal/Reverse/Ignore 均落一条。
type SealAction struct {
	// SealID 唯一标识（UUID）。
	SealID string `json:"seal_id"`
	// BatchID 所属批次。
	BatchID string `json:"batch_id"`
	// ReportID 所属孤儿报告。
	ReportID string `json:"report_id"`
	// Kind 动作类型，见 SealKind。
	Kind SealKind `json:"kind"`
	// Status 动作状态，见 SealStatus。
	Status SealStatus `json:"status"`
	// OrphanIDs 本次动作涵盖的孤儿 ID 列表。
	OrphanIDs []string `json:"orphan_ids"`
	// Actor1 发起人（双人授权的第一人）。
	Actor1 string `json:"actor1"`
	// Actor2 复核人（双人授权的第二人）。
	Actor2 string `json:"actor2,omitempty"`
	// ApprovedAt 双人均授权后的时刻。
	ApprovedAt *time.Time `json:"approved_at,omitempty"`
	// AppliedAt 执行完成时刻。
	AppliedAt *time.Time `json:"applied_at,omitempty"`
	// FailureReason 执行失败原因码。
	FailureReason string `json:"failure_reason,omitempty"`
	// ReverseEntryIDs 若 Kind==SealKindReverse，记录所有新建冲正 ledger entry ID。
	ReverseEntryIDs []string `json:"reverse_entry_ids,omitempty"`
	// ProofHash 动作完成后绑定的完整性哈希（与 IntegrityProof 一致）。
	ProofHash string `json:"proof_hash,omitempty"`
	// CreatedAt 动作创建时刻。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最后变更时刻。
	UpdatedAt time.Time `json:"updated_at"`
}

// ── 完整性证明 ───────────────────────────────────────────────────────────────
// ProofStatus 完整性证明状态。

type ProofStatus string

const (
	ProofPending  ProofStatus = "pending"  // 证明已创建，等待最终封账
	ProofVerified ProofStatus = "verified" // 哈希校验通过
	ProofInvalid ProofStatus = "invalid" // 哈希校验失败（告警级别）
)

// IntegrityProof 批次终态完整性证明。
// 完整性哈希算法：
// SHA-256( intentSet || ledgerSet || sealSet )
// 其中每个 set 为对应集合成员 IdempotencyKey 排序后的串联。
// 验证方只需持有三组 idem key 列表即可重算哈希，无需访问 ledger 本身。
type IntegrityProof struct {
	// ProofID 唯一标识（UUID）。
	ProofID string `json:"proof_id"`
	// BatchID 所属批次。
	BatchID string `json:"batch_id"`
	// ReportID 关联孤儿报告（若无孤儿则为空）。
	ReportID string `json:"report_id,omitempty"`
	// SealID 关联封堵动作（若无封堵则为空）。
	SealID string `json:"seal_id,omitempty"`
	// Generation 生成证明时的世代号。
	Generation int64 `json:"generation"`
	// Status 证明状态，见 ProofStatus。
	Status ProofStatus `json:"status"`
	// Hash 三路哈希值（hex 字符串，64 字符）。
	Hash string `json:"hash"`
	// IntentCount 意图集合大小（原始 Items 数量）。
	IntentCount int `json:"intent_count"`
	// LedgerCount 已落账 hop 数量。
	LedgerCount int `json:"ledger_count"`
	// SealCount 封堵集合大小（已 sealed/reversed 的孤儿数量）。
	SealCount int `json:"seal_count"`
	// IntentIdemKeys 意图集合 idem keys（已排序，用于外部校验）。
	IntentIdemKeys []string `json:"intent_idem_keys"`
	// LedgerIdemKeys 落账集合 idem keys（已排序）。
	LedgerIdemKeys []string `json:"ledger_idem_keys"`
	// SealIdemKeys 封堵集合 idem keys（已排序）。
	SealIdemKeys []string `json:"seal_idem_keys"`
	// VerifiedAt 最后校验时刻。
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
	// CreatedAt 证明创建时刻。
	CreatedAt time.Time `json:"created_at"`
}

// ── IdemBlock — idem 拒写记录 ────────────────────────────────────────────────
// IdemBlock 幂等拒写记录，防止已封堵孤儿的 idem key 被后续续写。
// 写入后，任何尝试用同一 idem key 发起 ledbatch.Submit 的 hop 均将被拒绝。

type IdemBlock struct {
	// IdemKey 被封堵的幂等键（全局唯一）。
	IdemKey string `json:"idem_key"`
	// BatchID 来源批次。
	BatchID string `json:"batch_id"`
	// OrphanID 对应孤儿记录 ID。
	OrphanID string `json:"orphan_id"`
	// SealID 关联封堵动作。
	SealID string `json:"seal_id"`
	// BlockedAt 写入时刻。
	BlockedAt time.Time `json:"blocked_at"`
}

// ── 请求 / 响应类型 ──────────────────────────────────────────────────────────
// ScanReq 触发孤儿扫描请求。

type ScanReq struct {
	// BatchID 若指定则只扫该批次；否则扫全部 open/submitted 状态批次。
	BatchID string `json:"batch_id,omitempty"`
	// Actor 操作人。
	Actor string `json:"actor"`
}

// ScanResp 扫描结果。

type ScanResp struct {
	// ScannedBatches 本次扫描的批次数量。
	ScannedBatches int `json:"scanned_batches"`
	// NewReports 新建孤儿报告数量。
	NewReports int `json:"new_reports"`
	// TotalOrphans 发现孤儿总数。
	TotalOrphans int `json:"total_orphans"`
	// Reports 本次扫描产出的孤儿报告列表。
	Reports []OrphanReport `json:"reports"`
}

// AcquireGenReq 续提世代锁请求。
type AcquireGenReq struct {
	// BatchID 目标批次。
	BatchID string `json:"batch_id"`
	// WorkerID worker 标识（hostname:pid:uuid）。
	WorkerID string `json:"worker_id"`
}

// SealReq 封堵动作请求（需双人授权）。

type SealReq struct {
	// BatchID 目标批次。
	BatchID string `json:"batch_id"`
	// ReportID 目标孤儿报告。
	ReportID string `json:"report_id"`
	// Kind 封堵类型。
	Kind SealKind `json:"kind"`
	// OrphanIDs 若为空则处置报告中全部 detected 状态孤儿；否则只处置指定列表。
	OrphanIDs []string `json:"orphan_ids,omitempty"`
	// Actor1 发起人。
	Actor1 string `json:"actor1"`
	// Actor2 复核人（与 Actor1 不能相同）。
	Actor2 string `json:"actor2"`
	// Note 处置备注。
	Note string `json:"note,omitempty"`
}

// ProofQueryReq 证明查询请求。
type ProofQueryReq struct {
	BatchID string `json:"batch_id"`
	ProofID string `json:"proof_id,omitempty"`
	Verified bool `json:"verified,omitempty"`
}

// ReportListReq 孤儿报告列表查询。
type ReportListReq struct {
	BatchID string `json:"batch_id,omitempty"`
	Status  string `json:"status,omitempty"`
	Limit   int    `json:"limit,omitempty"`
	After   string `json:"after,omitempty"`
}
