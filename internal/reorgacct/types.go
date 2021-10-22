package reorgacct

import "time"

// ────────────────────────────────────────────────────────────────────────────
// ReorgEvent 状态
// ────────────────────────────────────────────────────────────────────────────

const (
	// EventStatePending 事件已摄取，影响面盘点尚未完成。
	EventStatePending = "pending"
	// EventStateScanned 影响面盘点已完成，等待 reorgorch 读取。
	EventStateScanned = "scanned"
	// EventStateOrchStarted reorgorch 已开始编排冲正批次。
	EventStateOrchStarted = "orch_started"
	// EventStateClosed 编排完成或人工关闭。
	EventStateClosed = "closed"
	// EventStateError 盘点或编排期间发生不可恢复错误，需人工介入。
	EventStateError = "error"
)

// ────────────────────────────────────────────────────────────────────────────
// ImpactKind 资金位枚举
// ────────────────────────────────────────────────────────────────────────────

const (
	// KindCredited 提前入账已贷记余额，需借方冲正。
	KindCredited = "credited"
	// KindOccupied 风险占用单已建立（r4 depoccup），需请求释放。
	KindOccupied = "occupied"
	// KindFreeze 出金在途冻结（非 live-hash），需解冻回滚。
	KindFreeze = "freeze"
	// KindHopPartial 划转 hop 半截（已出发未到达），需收口通知。
	KindHopPartial = "hop_partial"
	// KindLiveHash 含 live-hash 出金跟踪，禁止自动解冻，仅开事故单。
	KindLiveHash = "live_hash"
)

// ImpactActionRequired 资金位对应的冲正动作说明（只读提示，不驱动 FSM）。

var ImpactActionRequired = map[string]string{
	KindCredited: "ledger debit reversal entry",
	KindOccupied: "depoccup release request",
	KindFreeze: "withdraw freeze rollback",
	KindHopPartial: "transfer hop close notification",
	KindLiveHash: "manual incident ticket; no auto-unfreeze",
}

// ────────────────────────────────────────────────────────────────────────────
// EvidenceRef 重组证据引用
// ────────────────────────────────────────────────────────────────────────────
// EvidenceRef 记录一条重组证据的外部引用，不存储原始链数据。

type EvidenceRef struct {
	// Kind 证据类型：orphan_block / confirm_rollback / node_report。
	Kind string `json:"kind"`
	// Ref 外部引用值（孤块哈希、节点 ID 等）。
	Ref string `json:"ref"`
	// Note 人工备注。
	Note string `json:"note,omitempty"`
}

// ────────────────────────────────────────────────────────────────────────────
// AffectedOutput 受影响的链上输出引用
// ────────────────────────────────────────────────────────────────────────────
// AffectedOutput 指向一个已被重组波及的链上 UTXO / EVM 输出。
type AffectedOutput struct {
	TxHash string `json:"tx_hash"`
	OutputIndex int `json:"output_index"`
	// TransferID 入金 transfer 记录 ID（若已创建）。
	TransferID string `json:"transfer_id,omitempty"`
	// UserID 入金用户 ID（盘点时填充）。
	UserID string `json:"user_id,omitempty"`
	// Asset 资产代码。
	Asset string `json:"asset,omitempty"`
	// Amount 该输出的贷记金额（字符串 decimal）。
	Amount string `json:"amount,omitempty"`
}

// ────────────────────────────────────────────────────────────────────────────
// AffectedTrack 受影响的出金跟踪引用
// ────────────────────────────────────────────────────────────────────────────
// AffectedTrack 只读消费出金跟踪事实；禁止写入 saga 广播状态。
type AffectedTrack struct {
	// WithdrawID 出金申请 ID。
	WithdrawID string `json:"withdraw_id"`
	// BroadcastID 广播记录 ID。
	BroadcastID string `json:"broadcast_id,omitempty"`
	// TxHash 广播的交易哈希。
	TxHash string `json:"tx_hash,omitempty"`
	// LiveHash 若为 true，该跟踪含 live-hash，禁止自动解冻。
	LiveHash bool `json:"live_hash"`
	// FreezeID 冻结单 ID（若存在）。
	FreezeID string `json:"freeze_id,omitempty"`
	// Asset 资产代码。
	Asset string `json:"asset,omitempty"`
	// UserID 用户 ID。
	UserID string `json:"user_id,omitempty"`
	// FreezeAmount 冻结金额。
	FreezeAmount string `json:"freeze_amount,omitempty"`
}

// ────────────────────────────────────────────────────────────────────────────
// ReorgEvent 规范化重组事件
// ────────────────────────────────────────────────────────────────────────────
// ReorgEvent 表示一次经规范化后的链重组事件。
// 每个事件有唯一 ID（UUID），由 Ingestor 在摄取时生成并写入 reorg_events 表。
type ReorgEvent struct {
	// ID UUID v4，主键。
	ID string `json:"id"`
	// Chain 规范化链 ID（大写，如 BTC / ETH / TRON）。
	Chain string `json:"chain"`
	// PivotTxHash 触发重组检测的交易哈希（可为空；孤块维度重组可为 orphan block hash）。
	PivotTxHash string `json:"pivot_tx_hash,omitempty"`
	// DepthBefore 回退前的进程内确认数。
	DepthBefore int `json:"depth_before"`
	// DepthAfter 回退后的确认数（DepthAfter < DepthBefore 表示实际回退）。
	DepthAfter int `json:"depth_after"`
	// RollbackDepth 回退量 = DepthBefore - DepthAfter；由 Ingestor 计算并验证 > 0。
	RollbackDepth int `json:"rollback_depth"`
	// AffectedOutputs 受影响的入金输出列表。
	AffectedOutputs []AffectedOutput `json:"affected_outputs,omitempty"`
	// AffectedTracks 受影响的出金跟踪列表（只读，不改 saga 状态）。
	AffectedTracks []AffectedTrack `json:"affected_tracks,omitempty"`
	// Evidence 证据引用列表。
	Evidence []EvidenceRef `json:"evidence,omitempty"`
	// State 事件状态（pending / scanned / orch_started / closed / error）。
	State string `json:"state"`
	// SourceKind 摄取来源：confirm_rollback（fundsafety）/ orphan_observe（链观察者）。
	SourceKind string `json:"source_kind"`
	// Note 人工备注或自动说明。
	Note string `json:"note,omitempty"`
	// CreatedAt 事件摄取时间（UTC）。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最近状态变更时间（UTC）。
	UpdatedAt time.Time `json:"updated_at"`
}

// ────────────────────────────────────────────────────────────────────────────
// ImpactRow 影响面明细行
// ────────────────────────────────────────────────────────────────────────────
// ImpactRow 表示一条资金影响面明细：某用户的某资产在某资金位上被波及的金额。
// 一个 ReorgEvent 可产生多条 ImpactRow（跨用户、跨资产、跨资金位）。
type ImpactRow struct {
	// ID UUID v4，主键。
	ID string `json:"id"`
	// EventID 关联的 ReorgEvent.ID。
	EventID string `json:"event_id"`
	// UserID 用户 ID。
	UserID string `json:"user_id"`
	// Asset 资产代码（大写，如 BTC / ETH / USDT）。
	Asset string `json:"asset"`
	// Kind 资金位枚举（KindCredited / KindOccupied / KindFreeze / KindHopPartial / KindLiveHash）。
	Kind string `json:"kind"`
	// ReferenceID 被影响对象的 ID（TransferID / OccupancyID / FreezeID / HopID / WithdrawID）。
	ReferenceID string `json:"reference_id"`
	// AffectedAmount 应冲正/释放的金额（字符串 decimal，始终 > 0）。
	AffectedAmount string `json:"affected_amount"`
	// ReverseBatchID 关联的冲正批次 ID（由 reorgorch 回写；初始为空）。
	ReverseBatchID string `json:"reverse_batch_id,omitempty"`
	// Note 盘点说明。
	Note string `json:"note,omitempty"`
	// CreatedAt 盘点时间（UTC）。
	CreatedAt time.Time `json:"created_at"`
}

// ────────────────────────────────────────────────────────────────────────────
// IngestReq 摄取请求（链观察者或 fundsafety 钩子填充）
// ────────────────────────────────────────────────────────────────────────────
// IngestReq 摄取请求，由调用方按信号来源填充。
type IngestReq struct {
	Chain string `json:"chain"`
	PivotTxHash string `json:"pivot_tx_hash,omitempty"`
	DepthBefore int    `json:"depth_before"`
	DepthAfter int `json:"depth_after"`
	AffectedOutputs []AffectedOutput `json:"affected_outputs,omitempty"`
	AffectedTracks  []AffectedTrack  `json:"affected_tracks,omitempty"`
	Evidence        []EvidenceRef    `json:"evidence,omitempty"`
	SourceKind      string           `json:"source_kind"`
	Note string `json:"note,omitempty"`
}

// ────────────────────────────────────────────────────────────────────────────
// ImpactSummary 按事件的影响面汇总（HTTP 响应用）
// ────────────────────────────────────────────────────────────────────────────
// ImpactSummary 汇总一个 ReorgEvent 的各类资金影响额。
type ImpactSummary struct {
	EventID string `json:"event_id"`
	TotalCredited string `json:"total_credited"`
	TotalOccupied string `json:"total_occupied"`
	TotalFreeze string `json:"total_freeze"`
	TotalHopPartial string `json:"total_hop_partial"`
	LiveHashCount   int    `json:"live_hash_count"`
	ByAsset  map[string]string `json:"by_asset,omitempty"`
	RowCount int               `json:"row_count"`
}
