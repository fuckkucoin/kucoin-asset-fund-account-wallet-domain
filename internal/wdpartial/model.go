package wdpartial

import "time"

// ─── Batch 残留状态机 ─────────────────────────────────────────────────────────
// submitted ──[残留发现]──► residue_open ──[Resume 申请]──► resuming
// resuming ──[全部处置]──► closed
// resuming ──[部分升级]──► escalated
// residue_open ──[直接升级]──► escalated
// BatchResidueStatus 批次残留处置状态。

type BatchResidueStatus string

const (
	BatchResidueOpen     BatchResidueStatus = "residue_open" // 检测到残留，等待处置
	BatchResidueResuming BatchResidueStatus = "resuming"     // 持有世代锁，正在续提/释放
	BatchResidueClosed BatchResidueStatus = "closed" // 全部残留已处置，Conserve 通过
	BatchResidueEscalated BatchResidueStatus = "escalated" // 部分残留升级挂账（suspense）
)

// ─── 残留项状态机 ──────────────────────────────────────────────────────────────
// open ──[Release 证明已记录]──► released
// open ──[重提成功]──► retried
// open ──[升级挂账]──► escalated
// ResidueStatus 单笔残留项状态。
type ResidueStatus string

const (
	ResidueOpen     ResidueStatus = "open"     // 残留占用悬挂，等待处置
	ResidueReleased ResidueStatus = "released" // 占用已释放，Release 证明已记录
	ResidueRetried   ResidueStatus = "retried"   // 失败项已在新批次中重提成功
	ResidueEscalated ResidueStatus = "escalated" // 已升级挂入挂账账户
)

// ResidueOrigin 残留来源，标识该笔来自哪种失败路径。

type ResidueOrigin string

const (
	ResidueOriginFailed  ResidueOrigin = "failed"  // Submit 阶段 wdbatch 记 failed
	ResidueOriginSkipped ResidueOrigin = "skipped" // Submit 阶段 wdbatch 记 skipped（用户冻结）
)

// ─── ResidueItem ──────────────────────────────────────────────────────────────
// ResidueItem 出金批次单笔残留项。
// 一个 wdbatch 批次提交后，failed/skipped 项仍持有冻结占用而未出金。
// 每笔这样的 Item 对应一条 ResidueItem，直至 released / retried / escalated。
type ResidueItem struct {
	ID string `json:"id"` // UUID，残留项主键
	WDBatchID string `json:"wd_batch_id"` // 来源 wdbatch 批次 ID（必须是 wdbatch，非 ledbatch）
	Seq int `json:"seq"` // 在原批次中的序号
	UserID string `json:"user_id"`
	Asset      string        `json:"asset"`
	Amount     string        `json:"amount"` // 字符串精度，与 wdbatch Item.Amount 一致
	Chain      string        `json:"chain"`
	ToAddress  string        `json:"to_address"`
	IdemKey    string        `json:"idem_key"`    // 原批次 item 幂等键
	Origin     ResidueOrigin `json:"origin"`      // failed / skipped
	FailReason string        `json:"fail_reason"` // 原始失败原因（来自 wdbatch Item.Reason）
	Status     ResidueStatus `json:"status"`
	// 账本/冻结占用观察快照（只读，不驱动账本 FSM）
	FreezeOccupied bool `json:"freeze_occupied"` // 是否仍有冻结占用
	LedgerOccupied bool `json:"ledger_occupied"` // 是否有账本侧待处理记录
	OccupyLedgerID string `json:"occupy_ledger_id"` // 若有账本侧记录，关联 ledger ID
	// 处置结果（终态时填写）
	ProofID          string `json:"proof_id,omitempty"`           // 关联 ReleaseProof.ID
	RetryBatchID     string `json:"retry_batch_id,omitempty"`     // 重提新批次 ID
	EscalateLedgerID string `json:"escalate_ledger_id,omitempty"` // 挂账账本 ID
	DetectedAt time.Time `json:"detected_at"`
	ResolvedAt time.Time `json:"resolved_at,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Open 残留项是否未处置（非终态）。

  func (r ResidueItem) Open() bool {
	   return r.Status == ResidueOpen
  }

// Terminal 残留项是否已到终态。
   func (r ResidueItem) Terminal() bool {
	   return r.Status == ResidueReleased ||
		  r.Status == ResidueRetried ||
		   r.Status == ResidueEscalated
  }

// ─── ResumeLease ──────────────────────────────────────────────────────────────
// ResumeLease Resume 世代锁。
// 续提（Resume）须先持有对应 WDBatchID 的世代锁：
// - Gen 单调递增；持有者提交 Resume 时以世代号为乐观锁。
// - 两个 worker 争抢同一批次时，后获取者拿到新世代，旧持有者提交时
// 得 ErrStaleGen，操作被拒绝，防止双 worker 并行续提。
// - 锁自动 TTL，过期后可被其他 worker 抢占（幂等续租）。
type ResumeLease struct {
	ID        string `json:"id"`          // UUID
	WDBatchID string `json:"wd_batch_id"` // 对应批次 ID
	Holder    string `json:"holder"`      // 持有者标识（worker ID / operator ID）
	Gen int64 `json:"gen"` // 世代号，单调递增
	AcquiredAt time.Time `json:"acquired_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	ReleasedAt time.Time `json:"released_at,omitempty"`
	Released bool `json:"released"` // 主动释放
}

// Active 租约是否未过期且未主动释放。

  func (l ResumeLease) Active(now time.Time) bool {
	  return !l.Released && now.Before(l.ExpiresAt)
   }

// ─── ReleaseProof ─────────────────────────────────────────────────────────────
// ReleaseProofKind 释放证明类型。
type ReleaseProofKind string

const (
	ProofKindFreezeRelease ReleaseProofKind = "freeze_release" // 冻结占用已解除
	ProofKindLedgerCancel  ReleaseProofKind = "ledger_cancel"  // 账本侧记录已取消
	ProofKindEscalateSusp  ReleaseProofKind = "escalate_susp"  // 升级挂入挂账账户
	ProofKindNoOccupy ReleaseProofKind = "no_occupy" // 经核查无占用，直接关闭
)

// ReleaseProof 单笔残留项占用释放/升级操作的证明记录。
// 规则：failed/skipped 项在 BatchCloseout 前必须有且仅有一条对应 ProofID；
// 缺失证明则不得将批次标记为 closed。证明只追加，不修改。
type ReleaseProof struct {
	ID string `json:"id"` // UUID
	WDBatchID string `json:"wd_batch_id"`
	ResidueID string `json:"residue_id"` // 关联 ResidueItem.ID
	UserID    string `json:"user_id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Kind      ReleaseProofKind `json:"kind"`
	Actor     string           `json:"actor"`
	LedgerRef string           `json:"ledger_ref,omitempty"` // 账本操作参考 ID
	Summary  string `json:"summary"`
	Checksum string `json:"checksum"` // SHA-256(kind|wd_batch_id|residue_id|user_id|asset|amount|actor)
	LeaseGen   int64     `json:"lease_gen"` // 操作时持有的 ResumeLease 世代号
	RecordedAt time.Time `json:"recorded_at"`
}

// ─── BatchCloseout ────────────────────────────────────────────────────────────
// BatchCloseout 批次收口单。
// 一个 wdbatch 批次完成所有残留处置后，由 Conserve Guard 生成收口单：
// - requested = submitted + skipped_released + failed_released（含释放调整）
// - 所有 ResidueItem 已终态
// - 所有 failed/skipped 项都有对应 ReleaseProof
// BatchCloseout 是不可变快照（只写一次），供账本孤儿探测与审计一致只读消费。
type BatchCloseout struct {
	ID        string `json:"id"`          // UUID
	WDBatchID string `json:"wd_batch_id"` // 必须是 wdbatch ID
	Status BatchResidueStatus `json:"status"` // closed / escalated
	Requested       string `json:"requested"`        // 原批次 Requested
	Submitted       string `json:"submitted"`        // 原批次 Submitted
	SkippedReleased string `json:"skipped_released"` // 跳过项已释放量
	FailedReleased string `json:"failed_released"` // 失败项已释放量
	EscalatedAmount string `json:"escalated_amount"` // 升级挂账量（含于 skipped+failed）
	ConserveOK bool `json:"conserve_ok"` // Conserve 等式是否通过
	ConserveNote string `json:"conserve_note"`
	ProofCount   int    `json:"proof_count"`
	ResidueCount int    `json:"residue_count"`
	Actor string `json:"actor"`
	ClosedAt time.Time `json:"closed_at"`
}

// ─── 查询过滤器 ──────────────────────────────────────────────────────────────
// ResidueFilter 残留查询过滤器。
type ResidueFilter struct {
	WDBatchID string
	UserID string
	Status ResidueStatus
	Origin ResidueOrigin
	OpenOnly bool
	Limit    int
	Offset int
}

// DetectResult 残留检测结果摘要。

type DetectResult struct {
	WDBatchID  string
	TotalItems int
	FailedItems  int
	SkippedItems int
	ResidueCount int
	NewResidues  []*ResidueItem
	Errors       []error
}

// ConserveResult Conserve 校验结果。

type ConserveResult struct {
	WDBatchID string
	Requested string
	Submitted       string
	SkippedReleased string
	FailedReleased  string
	EscalatedAmount string
	Balanced        bool
	Note            string
}

// WorkerScanResult 巡检 worker 运行摘要。

type WorkerScanResult struct {
	ScannedBatches int
	NewResidues    int
	Errors         []error
}

// ReleaseExecResult 释放执行摘要。

type ReleaseExecResult struct {
	Processed int
	Released  int
	Escalated int
	Skipped   int
	Errors []error
}
