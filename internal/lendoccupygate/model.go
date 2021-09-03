package lendoccupygate

import "time"

// ─── 占用租约状态机 ────────────────────────────────────────────────────────────
// LeaseState 占用租约状态。
// 迁移路径：placed → settled | cancelled | residue_closed。

type LeaseState string

const (
	// LeasePlaced 挂单已借记，持有占用租约；等待成交或取消。
	LeasePlaced LeaseState = "placed"
	// LeaseSettled 成交入借出账完成；租约已正常释放。
	LeaseSettled LeaseState = "settled"
	// LeaseCancelled 取消退回完成；租约已正常释放。
	LeaseCancelled LeaseState = "cancelled"
	// LeaseResidueClosed 借记成功但 hold 失败，补偿冲销并关闭租约。
	LeaseResidueClosed LeaseState = "residue_closed"
)

// Terminal 报告状态是否为终态。

   func (s LeaseState) Terminal() bool {
	  switch s {
	   case LeaseSettled, LeaseCancelled, LeaseResidueClosed:
		  return true
	  default:
		   return false
	  }
   }

// ─── LendOccupyLease（占用租约） ───────────────────────────────────────────────
// LendOccupyLease 放贷挂单占用租约。
// 每次 PlaceBound 成功签发一条租约；Settle 或 Cancel 消费对应 LendGen 后关闭。
// 半截补偿（借记成功 hold 失败）走 ResidueClosed 路径并同时落 LendResidueProof。
type LendOccupyLease struct {
	// LeaseID 全局唯一租约标识（UUID）。
	LeaseID string `json:"lease_id"`
	// GenID 绑定的放贷世代令牌 ID（UUID）。
	GenID string `json:"gen_id"`
	// OrderID 放贷挂单 ID（外部业务 ID，唯一对应一次 Place）。
	OrderID string `json:"order_id"`
	// UserID 下单用户 ID。
	UserID string `json:"user_id"`
	// Asset 资产代码（大写，如 USDT）。
	Asset string `json:"asset"`
	// Amount 挂单金额（18 位精度字符串）。
	Amount string `json:"amount"`
	// State 当前状态。
	State LeaseState `json:"state"`
	// LedgerIdemKey 账本借记幂等键（"lend-ord:" + OrderID）。
	LedgerIdemKey string `json:"ledger_idem_key"`
	// HoldRef holdcat 标签引用（等于 OrderID）。
	HoldRef string `json:"hold_ref"`
	// PlacedAt 挂单时间（UTC）。
	PlacedAt time.Time `json:"placed_at"`
	// SettledAt 成交关闭时间（UTC）；非 settled 状态为 nil。
	SettledAt *time.Time `json:"settled_at,omitempty"`
	// CancelledAt 取消关闭时间（UTC）；非 cancelled 状态为 nil。
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`
	// ResidueClosedAt 半截补偿关闭时间（UTC）；非 residue_closed 状态为 nil。
	ResidueClosedAt *time.Time `json:"residue_closed_at,omitempty"`
	// ResidueProofID 关联的 LendResidueProof.ProofID；residue_closed 后填充。
	ResidueProofID string `json:"residue_proof_id,omitempty"`
	// CreatedAt 记录创建时间（UTC）。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 记录最后更新时间（UTC）。
	UpdatedAt time.Time `json:"updated_at"`
}

// ─── LendGen（放贷世代令牌） ──────────────────────────────────────────────────
// LendGen 放贷世代令牌。
// Place 成功后签发；Settle 或 Cancel 消费（Consumed→true）。
// Consumed=false 时允许唯一消费；消费后不可逆，重复消费落 LendDupReleaseProof。

type LendGen struct {
	// GenID 全局唯一令牌标识（UUID）。
	GenID string `json:"gen_id"`
	// OrderID 绑定挂单 ID。
	OrderID string `json:"order_id"`
	// UserID 下单用户。
	UserID string `json:"user_id"`
	// Asset 资产代码。
	Asset string `json:"asset"`
	// Amount 金额（18 位精度字符串）。
	Amount string `json:"amount"`
	// LedgerIdemKey 账本借记幂等键，供审计核对。
	LedgerIdemKey string `json:"ledger_idem_key"`
	// Consumed 是否已被 Settle 或 Cancel 消费；一旦为 true 不可逆。
	Consumed bool `json:"consumed"`
	// ConsumedBy 消费原因："settle" 或 "cancel"；未消费时为空。
	ConsumedBy string `json:"consumed_by,omitempty"`
	// ConsumedAt 消费时间（UTC）；未消费时为 nil。
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	// IssuedAt 签发时间（UTC）。
	IssuedAt time.Time `json:"issued_at"`
	// CreatedAt 记录创建时间（UTC）。
	CreatedAt time.Time `json:"created_at"`
}

// ─── LendDupReleaseProof（重复释放证明） ─────────────────────────────────────
// DupReleaseKind 重复释放类型。
type DupReleaseKind string

const (
	// DupSettleSettle 同一 order 两次 Settle 尝试。
	DupSettleSettle DupReleaseKind = "settle_settle"
	// DupSettleCancel Settle 与 Cancel 并发竞争，后到方落证明。
	DupSettleCancel DupReleaseKind = "settle_cancel"
	// DupCancelCancel 同一 order 两次 Cancel 尝试。
	DupCancelCancel DupReleaseKind = "cancel_cancel"
)

// LendDupReleaseProof 重复释放证明。
// 当 gen 已被消费而再次尝试消费时落地；禁止发生双贷记。
// 只追加，不修改。
type LendDupReleaseProof struct {
	// ProofID 全局唯一证明标识（UUID）。
	ProofID string `json:"proof_id"`
	// GenID 被重复消费的 LendGen.GenID。
	GenID string `json:"gen_id"`
	// OrderID 绑定挂单 ID。
	OrderID string `json:"order_id"`
	// UserID 下单用户。
	UserID string `json:"user_id"`
	// Asset 资产代码。
	Asset string `json:"asset"`
	// Amount 金额（字符串）。
	Amount string `json:"amount"`
	// Kind 重复类型：settle_settle / settle_cancel / cancel_cancel。
	Kind DupReleaseKind `json:"kind"`
	// FirstConsumedBy 首次消费方："settle" 或 "cancel"。
	FirstConsumedBy string `json:"first_consumed_by"`
	// FirstConsumedAt 首次消费时间（UTC）。
	FirstConsumedAt time.Time `json:"first_consumed_at"`
	// AttemptedBy 本次尝试方："settle" 或 "cancel"。
	AttemptedBy string `json:"attempted_by"`
	// AttemptedAt 本次尝试时间（UTC）。
	AttemptedAt time.Time `json:"attempted_at"`
	// CreatedAt 记录创建时间（UTC）。
	CreatedAt time.Time `json:"created_at"`
}

// ─── LendResidueProof（半截补偿证明） ────────────────────────────────────────
// ResidueKind 半截失败类型。
type ResidueKind string

const (
	// ResidueHoldFailed 借记成功但 holdcat.PlaceLendingOrder 失败。
	ResidueHoldFailed ResidueKind = "hold_failed"
	// ResidueDebitRolledBack 借记后系统异常，补偿冲销借记。
	ResidueDebitRolledBack ResidueKind = "debit_rolled_back"
)

// LendResidueProof 半截补偿证明。
// Place 借记成功但 hold 失败时落地；包含补偿冲销凭证信息。
// 只追加，不修改。

type LendResidueProof struct {
	// ProofID 全局唯一证明标识（UUID）。
	ProofID string `json:"proof_id"`
	// GenID 对应已被补偿冲销的 LendGen.GenID。
	GenID string `json:"gen_id"`
	// OrderID 绑定挂单 ID。
	OrderID string `json:"order_id"`
	// UserID 下单用户。
	UserID string `json:"user_id"`
	// Asset 资产代码。
	Asset string `json:"asset"`
	// Amount 金额（字符串）。
	Amount string `json:"amount"`
	// Kind 失败类型。
	Kind ResidueKind `json:"kind"`
	// DebitIdemKey 借记幂等键（用于审计核对账本已借记）。
	DebitIdemKey string `json:"debit_idem_key"`
	// CompensateIdemKey 补偿贷记幂等键（"lend-residue-comp:" + OrderID）。
	CompensateIdemKey string `json:"compensate_idem_key"`
	// FailReason hold 失败原因（原始错误字符串，截断至 256 字节）。
	FailReason string `json:"fail_reason"`
	// CreatedAt 记录创建时间（UTC）。
	CreatedAt time.Time `json:"created_at"`
}

// ─── 请求/响应 DTO ────────────────────────────────────────────────────────────
// PlaceBoundReq PlaceBound 请求：签发 gen + 写 occupy 租约。

type PlaceBoundReq struct {
	OrderID string `json:"order_id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
}

// PlaceBoundResp PlaceBound 响应。

type PlaceBoundResp struct {
	Lease LendOccupyLease `json:"lease"`
	Gen LendGen `json:"gen"`
}

// SettleReq Settle（成交释放）请求。
type SettleReq struct {
	OrderID string `json:"order_id"`
	UserID  string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
}

// SettleResp Settle 响应。
type SettleResp struct {
	Lease LendOccupyLease `json:"lease"`
	// DupProof 重复 Settle 时填充（幂等失败路径）。
	DupProof *LendDupReleaseProof `json:"dup_proof,omitempty"`
}

// CancelReq Cancel（取消退回）请求。

type CancelReq struct {
	OrderID string `json:"order_id"`
	UserID  string `json:"user_id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
}

// CancelResp Cancel 响应。
type CancelResp struct {
	Lease LendOccupyLease `json:"lease"`
	// DupProof Settle 已消费 gen、Cancel 迟到时填充。
	DupProof *LendDupReleaseProof `json:"dup_proof,omitempty"`
}

// ExportProofReq 证明导出请求（ops 接口）。
type ExportProofReq struct {
	OrderID string `json:"order_id"`
	// Kind 可选过滤："dup" 或 "residue"；空表示两种都查。
	Kind string `json:"kind,omitempty"`
}

// ExportProofResp 证明导出响应。
type ExportProofResp struct {
	DupProofs []LendDupReleaseProof `json:"dup_proofs"`
	ResidueProofs []LendResidueProof `json:"residue_proofs"`
}

// ScanResult 半截扫描结果。

type ScanResult struct {
	// ScannedLeases 本次扫描的租约数量。
	ScannedLeases int `json:"scanned_leases"`
	// CompensatedCount 成功补偿冲销的数量。
	CompensatedCount int `json:"compensated_count"`
	// SkippedCount 已处理或不需要补偿的数量。
	SkippedCount int `json:"skipped_count"`
	// ErrorCount 补偿过程中出错的数量。
	ErrorCount int `json:"error_count"`
}
