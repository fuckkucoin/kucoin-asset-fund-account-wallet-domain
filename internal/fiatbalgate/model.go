package fiatbalgate

import "time"

// ─── 不平衡案状态机 ────────────────────────────────────────────────────────────
// CaseState 不平衡案状态枚举。
// 状态迁移：open → blocking → cleared。
type CaseState string

const (
	// CaseOpen 扫描器发现不平衡；尚未强制触发挡写（过渡态，巡检尚未扩散）。
	CaseOpen CaseState = "open"
	// CaseBlocking 挡写已生效；所有新 Post（非冲正）均被拒绝，直至清障。
	CaseBlocking CaseState = "blocking"
	// CaseCleared 四眼清障完成；持新世代可恢复 Post。
	CaseCleared CaseState = "cleared"
)

// Terminal 报告状态是否为终态（不可再推进）。

  func (s CaseState) Terminal() bool { return s == CaseCleared }

// ─── 不平衡案 ──────────────────────────────────────────────────────────────────
// FiatImbalanceCase 记录一次法币凭证借贷失衡发现事件与后续处置。
// 每条 Case 绑定一个 JournalGeneration 区间（FromGen ~ ToGen）。
// 扫描器发现 ΣDebit ≠ ΣCredit 或 Conserve 失败后开案；Worker 推进到 blocking；
// 四眼清障后推进到 cleared，同时写入 FiatImbalanceProof。
type FiatImbalanceCase struct {
	// CaseID 全局唯一案件标识（UUID）。
	CaseID string `json:"case_id"`
	// Asset 资产代码（大写，如 USDT）。
	Asset string `json:"asset"`
	// ScanGenSeq 触发开案的扫描世代快照序号（绑定 fiatjnl.GenTracker.ActiveSeq）。
	ScanGenSeq uint64 `json:"scan_gen_seq"`
	// DetectedEntryID 触发失衡发现的凭证 ID（首条不平衡凭证）。
	DetectedEntryID string `json:"detected_entry_id"`
	// SumDebit 扫描区间内 ΣDebit（字符串，18 位精度）。
	SumDebit string `json:"sum_debit"`
	// SumCredit 扫描区间内 ΣCredit（字符串，18 位精度）。
	SumCredit string `json:"sum_credit"`
	// ImbalanceKind 失衡类型（balanced_mismatch / conserve_fail / empty_lines）。
	ImbalanceKind ImbalanceKind `json:"imbalance_kind"`
	// State 当前状态。
	State CaseState `json:"state"`
	// ScannerID 执行扫描的 Worker 标识符（用于追踪幂等）。
	ScannerID string `json:"scanner_id"`
	// OpenedAt Case 开案时间（UTC）。
	OpenedAt time.Time `json:"opened_at"`
	// BlockingAt 挡写生效时间（UTC）；open 态下为 nil。
	BlockingAt *time.Time `json:"blocking_at,omitempty"`
	// ClearedAt 清障时间（UTC）；cleared 态下填充。
	ClearedAt *time.Time `json:"cleared_at,omitempty"`
	// ClearedBy 清障操作人（ops user ID）。
	ClearedBy string `json:"cleared_by,omitempty"`
	// ClearNote 清障备注（四眼确认描述）。
	ClearNote string `json:"clear_note,omitempty"`
	// ProofID 关联的 FiatImbalanceProof.ProofID；cleared 后填充。
	ProofID string `json:"proof_id,omitempty"`
	// CreatedAt 记录创建时间（UTC）。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 记录最后更新时间（UTC）。
	UpdatedAt time.Time `json:"updated_at"`
}

// Copy 深拷贝（指针字段）。
  func (c FiatImbalanceCase) Copy() FiatImbalanceCase {
	  cp := c
	  if c.BlockingAt != nil {
		   t := *c.BlockingAt
		  cp.BlockingAt = &t
	   }
	   if c.ClearedAt != nil {
		  t := *c.ClearedAt
		  cp.ClearedAt = &t
	   }
	   return cp
  }

// ImbalanceKind 失衡发现类型。
type ImbalanceKind string

const (
	// ImbalanceBalancedMismatch ΣDebit ≠ ΣCredit（money.Balanced 失败）。
	ImbalanceBalancedMismatch ImbalanceKind = "balanced_mismatch"
	// ImbalanceConserveFail money.Conserve(left, right) 失败。
	ImbalanceConserveFail ImbalanceKind = "conserve_fail"
	// ImbalanceEmptyLines 凭证 Lines 为空（空集不守恒）。
	ImbalanceEmptyLines ImbalanceKind = "empty_lines"
)

// ─── 凭证世代快照 ──────────────────────────────────────────────────────────────
// JournalGeneration 记录 fiatjnl 某次 Post/Adjust 的世代快照；供扫描器绑定与追踪。
// 由 fiatbalgate 写入进程内表，来源为 fiatjnl.JournalGen（只读引用，不持引用指针）。
type JournalGeneration struct {
	// GenID 本表记录唯一 ID（UUID）。
	GenID string `json:"gen_id"`
	// Seq 对应 fiatjnl.GenSeq（单调递增）。
	Seq uint64 `json:"seq"`
	// EntryID 触发世代抬升的 fiatjnl.Entry.ID。
	EntryID string `json:"entry_id"`
	// Kind 操作种类（post / adjust）。
	Kind string `json:"kind"`
	// Asset 资产代码。
	Asset string `json:"asset"`
	// BumpedAt 世代抬升时间戳（UTC）；由 fiatjnl 传入，不由本包生成。
	BumpedAt time.Time `json:"bumped_at"`
	// SyncedAt 本记录写入 fiatbalgate 表的时间（UTC）。
	SyncedAt time.Time `json:"synced_at"`
}

// ─── 不平衡证明 ────────────────────────────────────────────────────────────────
// ProofState 不平衡证明状态（只追加，仅用于审计标记）。

type ProofState string

const (
	// ProofPending 证明已生成，待四眼审核导出。
	ProofPending ProofState = "pending"
	// ProofExported 证明已导出为可验证包（含哈希摘要）。
	ProofExported ProofState = "exported"
	// ProofVerified 外部审计已复验通过。
	ProofVerified ProofState = "verified"
)

// FiatImbalanceProof 记录不平衡事件的完整证明包，供四眼复验与审计导出。
// 每条 FiatImbalanceCase 最多对应一条 Proof（唯一约束 uq_proof_case_id）。
// 只追加，不修改内容字段；State 更新仅限 pending → exported → verified。

type FiatImbalanceProof struct {
	// ProofID 全局唯一证明 ID（UUID）。
	ProofID string `json:"proof_id"`
	// CaseID 关联的 FiatImbalanceCase.CaseID（唯一引用）。
	CaseID string `json:"case_id"`
	// Asset 资产代码（冗余，便于独立查询）。
	Asset string `json:"asset"`
	// ScanGenSeq 开案时的世代快照序号（与 Case 一致）。
	ScanGenSeq uint64 `json:"scan_gen_seq"`
	// EntriesSnapshot 不平衡区间内涉及的凭证 ID 列表（JSON 序列化，原始顺序）。
	EntriesSnapshot string `json:"entries_snapshot"`
	// LinesSnapshot 不平衡区间内所有 Lines 的 JSON 序列化快照（可验证）。
	LinesSnapshot string `json:"lines_snapshot"`
	// SumDebit 证明期间 ΣDebit（与 Case 一致）。
	SumDebit string `json:"sum_debit"`
	// SumCredit 证明期间 ΣCredit（与 Case 一致）。
	SumCredit string `json:"sum_credit"`
	// DigestSHA256 entries_snapshot + lines_snapshot 的 SHA-256 摘要（hex）；用于复验完整性。
	DigestSHA256 string `json:"digest_sha256"`
	// State 当前状态。
	State ProofState `json:"state"`
	// ExportedAt 导出时间（UTC）；ProofExported 后填充。
	ExportedAt *time.Time `json:"exported_at,omitempty"`
	// VerifiedAt 复验时间（UTC）；ProofVerified 后填充。
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
	// VerifiedBy 复验操作人（ops user ID）；ProofVerified 后填充。
	VerifiedBy string `json:"verified_by,omitempty"`
	// CreatedAt 记录创建时间（UTC）。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 记录最后更新时间（UTC）。
	UpdatedAt time.Time `json:"updated_at"`
}

// Copy 深拷贝证明（指针字段）。

   func (p FiatImbalanceProof) Copy() FiatImbalanceProof {
	   cp := p
	   if p.ExportedAt != nil {
		   t := *p.ExportedAt
		   cp.ExportedAt = &t
	  }
	  if p.VerifiedAt != nil {
		   t := *p.VerifiedAt
		  cp.VerifiedAt = &t
	  }
	   return cp
  }

// ─── 挡写判定上下文 ────────────────────────────────────────────────────────────
// BlockDecision 挡写决策结果，由 Engine.CheckBlock 返回给 fiatjnl.Post 调用方。
type BlockDecision struct {
	// Blocked 为 true 时调用方必须拒绝 Post。
	Blocked bool `json:"blocked"`
	// CaseID 导致挡写的案件 ID；Blocked=false 时为空。
	CaseID string `json:"case_id,omitempty"`
	// GenSeq 当前 fiatjnl 世代号（供调用方日志）。
	GenSeq uint64 `json:"gen_seq"`
	// Reason 挡写原因（human-readable）。
	Reason string `json:"reason,omitempty"`
}

// ─── 扫描请求 ──────────────────────────────────────────────────────────────────
// ScanReq 不平衡扫描请求，由 Worker 或 HTTP ops API 触发。

type ScanReq struct {
	// Asset 指定只扫描特定资产；空字符串表示扫描全部资产。
	Asset string `json:"asset"`
	// SinceGenSeq 从哪个世代起扫描（含）；0 表示全量扫描。
	SinceGenSeq uint64 `json:"since_gen_seq"`
	// ScannerID 触发扫描的 Worker 标识（用于幂等判断）。
	ScannerID string `json:"scanner_id"`
}

// ScanResult 扫描结果摘要。

type ScanResult struct {
	// ScannedEntries 本次扫描涉及的凭证总数。
	ScannedEntries int `json:"scanned_entries"`
	// NewCases 本次扫描新开的不平衡案数量。
	NewCases int `json:"new_cases"`
	// AlreadyOpen 已存在未清 Case 数量（幂等，不重复开案）。
	AlreadyOpen int `json:"already_open"`
	// GenSeqAt 扫描时绑定的世代快照序号。
	GenSeqAt uint64 `json:"gen_seq_at"`
	// Cases 本次开案或已有案的 CaseID 列表。
	Cases []string `json:"cases,omitempty"`
}

// ─── 四眼清障请求 ──────────────────────────────────────────────────────────────
// ClearReq 四眼清障请求，需两名不同 Ops 操作人分两步提交。
type ClearReq struct {
	// CaseID 被清障的案件 ID。
	CaseID string `json:"case_id"`
	// Actor 本次操作人（ops user ID）。
	Actor string `json:"actor"`
	// Note 清障备注（不得为空，至少 10 字符）。
	Note string `json:"note"`
}

// ClearStep 四眼清障当前步骤。

type ClearStep string

const (
	// ClearStepFirst 第一眼已确认，等待第二眼。
	ClearStepFirst ClearStep = "first_approved"
	// ClearStepDone 两眼均确认，已执行清障。
	ClearStepDone ClearStep = "done"
)

// ClearApproval 记录四眼清障的单次确认记录。
type ClearApproval struct {
	// ApprovalID 唯一确认 ID（UUID）。
	ApprovalID string `json:"approval_id"`
	// CaseID 关联案件 ID。
	CaseID string `json:"case_id"`
	// Step 当前确认步骤。
	Step ClearStep `json:"step"`
	// Actor 确认操作人。
	Actor string `json:"actor"`
	// Note 确认备注。
	Note string `json:"note"`
	// CreatedAt 确认时间（UTC）。
	CreatedAt time.Time `json:"created_at"`
}
