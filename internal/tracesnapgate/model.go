package tracesnapgate

import "time"

// ─── 快照状态机 ──────────────────────────────────────────────────────────────
// SnapshotStatus 快照生命周期状态。

type SnapshotStatus string

const (
	// StatusOpen 快照已创建，可追加 hop / books，尚未密封。
	StatusOpen SnapshotStatus = "open"
	// StatusSealed 快照已密封；TransfersRoot / BooksRoot 已固化，Replay 可用。
	StatusSealed SnapshotStatus = "sealed"
	// StatusExported 快照已导出（法医完成）。
	StatusExported SnapshotStatus = "exported"
	// StatusSuperseded 快照已被同用户/同资产更新快照取代（不可导出）。
	StatusSuperseded SnapshotStatus = "superseded"
)

// ─── 漂移类型 ────────────────────────────────────────────────────────────────
// DriftType 漂移原因分类。
type DriftType string

const (
	// DriftTransfers 密封后 hop 列表发生变化（新增/移除划转）。
	DriftTransfers DriftType = "transfers_changed"
	// DriftBooks 密封后科目余额发生变化（可用/冻结金额漂移）。
	DriftBooks DriftType = "books_changed"
	// DriftBoth 划转与科目均发生变化。
	DriftBoth DriftType = "both_changed"
)

// ─── TraceSnapshot ───────────────────────────────────────────────────────────
// HopRecord 划转 hop 的摘要行（密封时固化）。
// 不存储原始金额；存储 hop 身份（ID + 方向 + 资产 + 截断摘要）用于 Replay 匹配。
type HopRecord struct {
	// TransferID 对应 ledger.Transfer.ID。
	TransferID string `json:"transfer_id"`
	// FromBook 源科目名称。
	FromBook string `json:"from_book"`
	// ToBook 目标科目名称。
	ToBook string `json:"to_book"`
	// Asset 资产代码（大写）。
	Asset string `json:"asset"`
	// AmountHex 金额十进制字符串的 SHA-256 前 16 字节 hex（快照摘要用）。
	AmountHex string `json:"amount_hex"`
}

// BookRecord 科目余额快照行（密封时固化）。
type BookRecord struct {
	// Book 科目名称（如 funding / trading / wallet）。
	Book string `json:"book"`
	// Subject 科目子类型（如 available / frozen）。
	Subject string `json:"subject"`
	// Asset 资产代码（大写）。
	Asset string `json:"asset"`
	// Available 可用余额（十进制字符串）。
	Available string `json:"available"`
	// Frozen 冻结余额（十进制字符串）。
	Frozen string `json:"frozen"`
}

// TraceSnapshot 资金 hop 溯源活账快照世代。
// 状态机：open → sealed → exported | superseded
// 不变量（sealed 后）：
// - TransfersRoot、BooksRoot 均非空。
// - AsOf 不可变（密封时刻）。
// - Hops / Books 切片不可追加。

type TraceSnapshot struct {
	// ID 快照唯一标识符（UUID v4）。
	ID string `json:"id"`
	// SnapshotGen 快照世代标识符（UUID v4，全局唯一，供法医路径关联）。
	SnapshotGen string `json:"snapshot_gen"`
	// UserID 被调查用户 ID。
	UserID string `json:"user_id"`
	// Asset 追踪资产代码（大写）。
	Asset string `json:"asset"`
	// Status 状态机当前状态。
	Status SnapshotStatus `json:"status"`
	// Hops 密封时固化的划转 hop 列表。
	Hops []HopRecord `json:"hops"`
	// Books 密封时固化的科目余额快照列表。
	Books []BookRecord `json:"books"`
	// TransfersRoot 所有 hop 行的 SHA-256 摘要根（密封时计算）。
	TransfersRoot string `json:"transfers_root"`
	// BooksRoot 所有 book 行的 SHA-256 摘要根（密封时计算）。
	BooksRoot string `json:"books_root"`
	// AsOf 快照密封时刻（UTC）；密封前为零值。
	AsOf time.Time `json:"as_of"`
	// CreatedAt 快照创建时刻。
	CreatedAt time.Time `json:"created_at"`
	// SealedAt 快照密封时刻（与 AsOf 同值）。
	SealedAt *time.Time `json:"sealed_at,omitempty"`
	// ExportedAt 快照导出时刻（StatusExported 时非空）。
	ExportedAt *time.Time `json:"exported_at,omitempty"`
	// SupersededAt 快照被取代时刻（StatusSuperseded 时非空）。
	SupersededAt *time.Time `json:"superseded_at,omitempty"`
	// SupersededBy 取代本快照的新快照 ID（StatusSuperseded 时非空）。
	SupersededBy string `json:"superseded_by,omitempty"`
	// Actor 执行密封操作的操作员 ID。
	Actor string `json:"actor"`
	// IdemKey 幂等键（防止同一请求重复创建快照）。
	IdemKey string `json:"idem_key"`
}

// IsTerminal 判断快照是否处于终态（exported / superseded）。
  func (s *TraceSnapshot) IsTerminal() bool {
	   return s.Status == StatusExported || s.Status == StatusSuperseded
   }

// ─── TraceDriftProof ──────────────────────────────────────────────────────────
// DriftEntry 漂移条目（单个 hop 或 book 行的差异记录）。
type DriftEntry struct {
	// Kind 条目类型：hop | book。
	Kind string `json:"kind"`
	// Ref 参考 ID（hop 为 TransferID；book 为 "book:asset"）。
	Ref string `json:"ref"`
	// Note 差异说明（added / removed / changed）。
	Note string `json:"note"`
}

// TraceDriftProof 快照重放漂移证明。
// 不变量：
// - SealedTransfersRoot / SealedBooksRoot 与对应 TraceSnapshot 的摘要根一致。
// - LiveTransfersRoot / LiveBooksRoot 为重放时现账计算结果。
// - 两对根不全相等（否则不应生成证明）。
// - 一经写入不可修改。

type TraceDriftProof struct {
	// ID 证明唯一标识符（UUID v4）。
	ID string `json:"id"`
	// SnapshotID 关联的快照 ID。
	SnapshotID string `json:"snapshot_id"`
	// SnapshotGen 关联的快照世代标识符。
	SnapshotGen string `json:"snapshot_gen"`
	// UserID 被调查用户 ID。
	UserID string `json:"user_id"`
	// Asset 追踪资产代码。
	Asset string `json:"asset"`
	// DriftType 漂移分类。
	DriftType DriftType `json:"drift_type"`
	// SealedTransfersRoot 密封时的 hop 摘要根。
	SealedTransfersRoot string `json:"sealed_transfers_root"`
	// LiveTransfersRoot 重放时现账的 hop 摘要根。
	LiveTransfersRoot string `json:"live_transfers_root"`
	// SealedBooksRoot 密封时的科目摘要根。
	SealedBooksRoot string `json:"sealed_books_root"`
	// LiveBooksRoot 重放时现账的科目摘要根。
	LiveBooksRoot string `json:"live_books_root"`
	// Entries 漂移条目列表（调试可选，可为空）。
	Entries []DriftEntry `json:"entries"`
	// ProofHash 证明指纹（SHA-256 hex，唯一索引）。
	ProofHash string `json:"proof_hash"`
	// DetectedAt 漂移检测时刻。
	DetectedAt time.Time `json:"detected_at"`
	// IdemKey 幂等键（防并发重复写入）。
	IdemKey string `json:"idem_key"`
}

// ─── InvestigateCaseBind ──────────────────────────────────────────────────────
// InvestigateCaseBind 调查案与快照的一对一绑定。
// 不变量：
// - CaseID 与 SnapshotID 组合唯一。
// - SnapshotGen 记录绑定时的快照世代，导出时验证一致性。
// - BoundAt 设置后不可变。

type InvestigateCaseBind struct {
	// ID 绑定记录唯一标识符（UUID v4）。
	ID string `json:"id"`
	// CaseID 对应 investigate.Case.ID。
	CaseID string `json:"case_id"`
	// SnapshotID 对应 TraceSnapshot.ID。
	SnapshotID string `json:"snapshot_id"`
	// SnapshotGen 绑定时的快照世代标识符。
	SnapshotGen string `json:"snapshot_gen"`
	// UserID 被调查用户 ID。
	UserID string `json:"user_id"`
	// Asset 追踪资产代码。
	Asset string `json:"asset"`
	// BoundAt 绑定时刻。
	BoundAt time.Time `json:"bound_at"`
	// BoundBy 执行绑定的操作员 ID。
	BoundBy string `json:"bound_by"`
}

// ─── ReplayResult ────────────────────────────────────────────────────────────
// ReplayResult Replay 操作返回结果（幂等）。
// 不变量：
// - Consistent=true 时 DriftProofID 为空，TransfersRoot / BooksRoot 与密封值一致。
// - Consistent=false 时必须有 DriftProofID；不得将 LiveHops / LiveBooks 作为法证结论返回。
type ReplayResult struct {
	// SnapshotID 对应快照 ID。
	SnapshotID string `json:"snapshot_id"`
	// SnapshotGen 快照世代标识符。
	SnapshotGen string `json:"snapshot_gen"`
	// Consistent true 表示摘要根一致（重放幂等）。
	Consistent bool `json:"consistent"`
	// TransfersRoot 重放时计算的 hop 摘要根（与密封值核对）。
	TransfersRoot string `json:"transfers_root"`
	// BooksRoot 重放时计算的 book 摘要根（与密封值核对）。
	BooksRoot string `json:"books_root"`
	// DriftProofID 漂移时对应的 TraceDriftProof.ID（Consistent=false 时非空）。
	DriftProofID string `json:"drift_proof_id,omitempty"`
	// ReplayedAt 重放时刻。
	ReplayedAt time.Time `json:"replayed_at"`
}

// ─── ExportBundle ────────────────────────────────────────────────────────────
// ExportBundle 法医导出包（仅 sealed 快照可生成）。
// 不变量：缺 SnapshotGen 时必须拒绝。

type ExportBundle struct {
	// Snapshot 密封快照全量数据。
	Snapshot TraceSnapshot `json:"snapshot"`
	// Bind 调查案绑定（若已绑定）。
	Bind *InvestigateCaseBind `json:"bind,omitempty"`
	// DriftProofs 关联漂移证明列表（可能为空）。
	DriftProofs []TraceDriftProof `json:"drift_proofs"`
	// ExportedAt 导出时刻。
	ExportedAt time.Time `json:"exported_at"`
	// ExportedBy 执行导出的操作员 ID。
	ExportedBy string `json:"exported_by"`
}
