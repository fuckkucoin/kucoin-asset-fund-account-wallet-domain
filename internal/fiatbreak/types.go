package fiatbreak

import (
	"strings"
	"time"
)

// ─────────────────────────────────────────────────────────
// 断裂类型
// ─────────────────────────────────────────────────────────
// BreakKind 描述 PSP 实收与账本轨道预约之间的差异性质。
type BreakKind string

const (
	// BreakUnderpay PSP 实收金额小于账本预约金额（欠收）。
	BreakUnderpay BreakKind = "underpay"
	// BreakOverpay PSP 实收金额大于账本预约金额（超收）。
	BreakOverpay BreakKind = "overpay"
	// BreakDuplicate PSP 重复收据；同一账本预约收到多次收款通知。
	BreakDuplicate BreakKind = "duplicate"
	// BreakMidRefund PSP 在结算后再次发出退款通知但账本挂账中途未关闭。
	BreakMidRefund BreakKind = "mid_refund"
	// BreakStatusMismatch PSP 状态与账本状态不一致（如 PSP 显示 failed 但账本已 settled）。
	BreakStatusMismatch BreakKind = "status_mismatch"
)

// ParseBreakKind 字符串转 BreakKind，未知类型返回错误。

   func ParseBreakKind(s string) (BreakKind, error) {
	  switch BreakKind(strings.ToLower(strings.TrimSpace(s))) {
	  case BreakUnderpay, BreakOverpay, BreakDuplicate, BreakMidRefund, BreakStatusMismatch:
		  return BreakKind(strings.ToLower(strings.TrimSpace(s))), nil
	  default:
		   return "", ErrBreakKind
	  }
   }

// ─────────────────────────────────────────────────────────
// 断裂状态
// ─────────────────────────────────────────────────────────
// BreakState 断裂记录生命周期状态。
type BreakState string

const (
	// BreakOpen 已检出，待人工介入。
	BreakOpen BreakState = "open"
	// BreakInvestigating 已分配给专项桌面，正在调查。
	BreakInvestigating BreakState = "investigating"
	// BreakResolved 差额已平账，挂账已释放，轨道可关闭。
	BreakResolved BreakState = "resolved"
	// BreakWrittenOff 差额已核销（坏账/商业决策），不再追偿。
	BreakWrittenOff BreakState = "written_off"
)

// ─────────────────────────────────────────────────────────
// 挂账原因
// ─────────────────────────────────────────────────────────
// SuspenseReason 挂账科目进入原因。
type SuspenseReason string

const (
	// SuspenseAuthorizedUnsettled 已授权但 PSP 未结算，金额挂起。
	SuspenseAuthorizedUnsettled SuspenseReason = "authorized_unsettled"
	// SuspenseSettleDrift PSP 结算金额与账本预约金额发生漂移。
	SuspenseSettleDrift SuspenseReason = "settle_drift"
	// SuspenseMidRefund 退款中途：账本已入账但 PSP 退款回报异步到达。
	SuspenseMidRefund SuspenseReason = "mid_refund"
	// SuspenseDuplicate 重复收据差额挂账，等待人工核销。
	SuspenseDuplicate SuspenseReason = "duplicate"
)

// ─────────────────────────────────────────────────────────
// 挂账状态
// ─────────────────────────────────────────────────────────
// SuspenseState 挂账条目生命周期状态。
type SuspenseState string

const (
	// SuspenseOpen 挂账中，未释放。
	SuspenseOpen SuspenseState = "open"
	// SuspenseReleased 差额已归还（无损）。
	SuspenseReleased SuspenseState = "released"
	// SuspenseConsumed 差额已核销（入账或销账）。
	SuspenseConsumed SuspenseState = "consumed"
)

// ─────────────────────────────────────────────────────────
// 核心记录结构体
// ─────────────────────────────────────────────────────────
// RailBreak PSP 实收与轨道账本的差异记录。
// 一次 Book 预约最多生成一条 RailBreak；重复收据追加到 Evidence 字段。
type RailBreak struct {
	// ID 断裂记录唯一标识（UUID）。
	ID string `json:"id"`
	// BookingID 关联的 fiatrail.Booking.ID。
	BookingID string `json:"booking_id"`
	// Kind 断裂类型。
	Kind BreakKind `json:"kind"`
	// State 当前状态。
	State BreakState `json:"state"`
	// PSPReceiptID PSP 侧收据/回调 ID（外部标识）。
	PSPReceiptID string `json:"psp_receipt_id"`
	// PSPAmount PSP 实收金额字符串（货币精度 2 位，如 "90.00"）。
	PSPAmount string `json:"psp_amount"`
	// BookAmount 账本预约金额字符串。
	BookAmount string `json:"book_amount"`
	// Delta 差额 = PSP - Book（带符号；正数为超收，负数为欠收）。
	Delta string `json:"delta"`
	// Currency 货币（如 USD / EUR）。
	Currency string `json:"currency"`
	// Evidence 断裂证据 JSON（重复收据原始 payload / 状态不符快照等）。
	Evidence string `json:"evidence,omitempty"`
	// SuspenseID 关联挂账条目 ID（若已挂账）。
	SuspenseID string `json:"suspense_id,omitempty"`
	// AssignedTo 已分配的专项桌面账户 ID。
	AssignedTo string `json:"assigned_to,omitempty"`
	// ResolvedBy 处置人 ID。
	ResolvedBy string `json:"resolved_by,omitempty"`
	// ResolveNote 处置备注。
	ResolveNote string `json:"resolve_note,omitempty"`
	// CreatedAt 断裂首次检出时间（UTC）。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最后更新时间（UTC）。
	UpdatedAt time.Time `json:"updated_at"`
}

// SuspenseEntry 在途挂账条目。
// 挂账科目隔离差额资金，避免轧差穿透到主账本。
type SuspenseEntry struct {
	// ID 挂账条目唯一标识（UUID）。
	ID string `json:"id"`
	// BreakID 关联的 RailBreak.ID。
	BreakID string `json:"break_id"`
	// BookingID 关联的 fiatrail.Booking.ID（冗余，便于索引）。
	BookingID string `json:"booking_id"`
	// Reason 挂账原因。
	Reason SuspenseReason `json:"reason"`
	// State 当前状态。
	State SuspenseState `json:"state"`
	// Asset 资产代码（如 USDT / USD）。
	Asset string `json:"asset"`
	// Amount 挂账金额（绝对值，精度与预约一致）。
	Amount string `json:"amount"`
	// Currency 货币。
	Currency string `json:"currency"`
	// SLADeadline SLA 截止时间，超期触发老化事件。
	SLADeadline time.Time `json:"sla_deadline"`
	// DisposedBy 处置人 ID（release/consume 时填写）。
	DisposedBy string `json:"disposed_by,omitempty"`
	// DisposedAt 处置时间。
	DisposedAt *time.Time `json:"disposed_at,omitempty"`
	// DisposalNote 处置说明。
	DisposalNote string `json:"disposal_note,omitempty"`
	// CreatedAt 创建时间（UTC）。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 更新时间（UTC）。
	UpdatedAt time.Time `json:"updated_at"`
}

// AgingPolicy SLA 老化策略配置。
// 不同断裂类型/挂账原因可配置不同 SLA 窗口和升级目标。
type AgingPolicy struct {
	// ID 策略唯一标识。
	ID string `json:"id"`
	// BreakKind 适用的断裂类型（空表示通配）。
	BreakKind string `json:"break_kind"`
	// SuspenseReason 适用的挂账原因（空表示通配）。
	SuspenseReason string `json:"suspense_reason"`
	// MaxAgeSec 最大容忍时效（秒）；超期即升级。
	MaxAgeSec int64 `json:"max_age_sec"`
	// EscalateDesk 升级目标桌面（如 "break_desk" / "finance_desk"）。
	EscalateDesk string `json:"escalate_desk"`
	// AutoWriteOff 到期是否自动核销（仅允许金额极小的差额场景）。
	AutoWriteOff bool `json:"auto_write_off"`
	// WriteOffThreshold 自动核销门槛金额（字符串；超过则不自动核销）。
	WriteOffThreshold string `json:"write_off_threshold,omitempty"`
}

// AgingEvent 老化事件记录。
// 每次 SLA 超期触发升级时产出一条，追加只写。
type AgingEvent struct {
	// ID 事件唯一标识（UUID）。
	ID string `json:"id"`
	// BreakID 关联 RailBreak.ID（可空，如果是纯挂账超期）。
	BreakID string `json:"break_id,omitempty"`
	// SuspenseID 关联 SuspenseEntry.ID（可空）。
	SuspenseID string `json:"suspense_id,omitempty"`
	// PolicyID 触发的策略 ID。
	PolicyID string `json:"policy_id"`
	// EscalateDesk 升级目标桌面。
	EscalateDesk string `json:"escalate_desk"`
	// AgeSeconds 实际老化时长（秒）。
	AgeSeconds int64 `json:"age_seconds"`
	// Note 升级说明。
	Note string `json:"note,omitempty"`
	// CreatedAt 事件生成时间（UTC）。
	CreatedAt time.Time `json:"created_at"`
}

// BilateralProof 双边证明：轨道侧摘要 ↔ 账本侧凭证哈希。
// 追加只写；ProofHash 生成后不可覆盖。

type BilateralProof struct {
	// ID 证明唯一标识（UUID）。
	ID string `json:"id"`
	// BookingID 关联 fiatrail.Booking.ID。
	BookingID string `json:"booking_id"`
	// BreakID 关联 RailBreak.ID（若无断裂则空）。
	BreakID string `json:"break_id,omitempty"`
	// RailDigest 轨道侧数据摘要（SHA-256 hex，覆盖 booking_id + psp_receipt_id + amounts + state）。
	RailDigest string `json:"rail_digest"`
	// LedgerDigest 账本侧凭证摘要（SHA-256 hex，覆盖 ledger_id + user_id + amounts + direction）。
	LedgerDigest string `json:"ledger_digest"`
	// ProofHash 双边联合哈希（SHA-256 over rail_digest || ledger_digest || timestamp）。
	ProofHash string `json:"proof_hash"`
	// Sealed 是否已封印（封印后不可追加新摘要到此证明）。
	Sealed bool `json:"sealed"`
	// SealedBy 封印人 ID。
	SealedBy string `json:"sealed_by,omitempty"`
	// SealedAt 封印时间。
	SealedAt *time.Time `json:"sealed_at,omitempty"`
	// CreatedAt 证明生成时间（UTC）。
	CreatedAt time.Time `json:"created_at"`
}

// ─────────────────────────────────────────────────────────
// 请求/响应 DTO
// ─────────────────────────────────────────────────────────
// DetectReq PSP 收据对账请求。
// 调用方传入 PSP 收据信息，引擎与轨道账本核对后生成断裂记录。
type DetectReq struct {
	// BookingID 关联的轨道预约 ID。
	BookingID string `json:"booking_id"`
	// PSPReceiptID PSP 侧唯一收据/回调 ID。
	PSPReceiptID string `json:"psp_receipt_id"`
	// PSPAmount PSP 实收金额。
	PSPAmount string `json:"psp_amount"`
	// PSPStatus PSP 最终状态（如 "captured" / "refunded" / "failed"）。
	PSPStatus string `json:"psp_status"`
	// Currency 货币代码。
	Currency string `json:"currency"`
	// Evidence 原始 PSP 回调载荷（JSON 字符串，用于存证）。
	Evidence string `json:"evidence,omitempty"`
}

// DetectResult 对账检测结果。
type DetectResult struct {
	// HasBreak 是否检出断裂。
	HasBreak bool `json:"has_break"`
	// Break 断裂记录（HasBreak=true 时非空）。
	Break *RailBreak `json:"break,omitempty"`
	// Suspense 挂账条目（已挂账时非空）。
	Suspense *SuspenseEntry `json:"suspense,omitempty"`
	// Proof 双边证明（已生成时非空）。
	Proof *BilateralProof `json:"proof,omitempty"`
}

// SuspenseDisposeReq 挂账处置请求（release / consume）。

type SuspenseDisposeReq struct {
	// SuspenseID 挂账条目 ID。
	SuspenseID string `json:"suspense_id"`
	// Action "release" 或 "consume"。
	Action string `json:"action"`
	// Actor 操作人 ID。
	Actor string `json:"actor"`
	// Note 处置说明。
	Note string `json:"note"`
}

// ForceReverseReq 双人授权强制冲销请求。
type ForceReverseReq struct {
	// BreakID 断裂记录 ID。
	BreakID string `json:"break_id"`
	// MakerActor 发起人 ID（maker）。
	MakerActor string `json:"maker_actor"`
	// CheckerActor 审核人 ID（checker）。
	CheckerActor string `json:"checker_actor"`
	// Reason 冲销原因（必填）。
	Reason string `json:"reason"`
}

// ProofQueryReq 双边证明查询请求。

type ProofQueryReq struct {
	// BookingID 按预约 ID 查。
	BookingID string `json:"booking_id,omitempty"`
	// BreakID 按断裂 ID 查。
	BreakID string `json:"break_id,omitempty"`
	// Limit 最多返回条数。
	Limit int `json:"limit,omitempty"`
}
