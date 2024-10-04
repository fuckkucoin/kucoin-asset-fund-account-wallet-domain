package brmismatch

import "time"

// MismatchKind 描述失配的具体类型。
type MismatchKind string

const (
	// KindMintOver 链上 mint 金额超过桥账 entry 金额。
	KindMintOver MismatchKind = "mint_over"
	// KindBurnOver 链上 burn 金额超过桥账 entry 金额。
	KindBurnOver MismatchKind = "burn_over"
	// KindMintMiss 桥账有 mint entry 但链上无对应事件。
	KindMintMiss MismatchKind = "mint_miss"
	// KindBurnMiss 桥账有 burn entry 但链上无对应事件。
	KindBurnMiss MismatchKind = "burn_miss"
)

// MismatchState 描述案件的 FSM 状态。

type MismatchState string

const (
	// StateOpen 已检测到失配，等待处理。
	StateOpen MismatchState = "open"
	// StateSuspended 已暂停相关账户的 mint/redeem。
	StateSuspended MismatchState = "suspended"
	// StateCleared 四眼审核完成，已解除暂停。
	StateCleared MismatchState = "cleared"
)

// ChainEvent 代表链上 mint 或 burn 事件。

type ChainEvent struct {
	// TxHash 链上交易哈希，区分大小写无关后归一化为小写。
	TxHash string `json:"tx_hash"`
	// LogIndex 同一交易内事件下标，用于去重。
	LogIndex int `json:"log_index"`
	// Chain 链标识符，如 "ETH"、"TRX"。
	Chain string `json:"chain"`
	// Asset 稳定币资产符号，仅允许 "USDT" 或 "USDC"。
	Asset string `json:"asset"`
	// Amount 金额字符串，十进制无符号。
	Amount string `json:"amount"`
	// Kind "mint" 或 "burn"。
	Kind string `json:"kind"`
	// At 事件发生时刻（链上区块时间或采集时间）。
	At time.Time `json:"at"`
}

// BookEntry 代表稳定币桥账本中的一条 mint 或 redeem 记录。
type BookEntry struct {
	// ID 账本条目唯一标识符。
	ID string `json:"id"`
	// UserID 归属用户。
	UserID string `json:"user_id"`
	// Asset 稳定币资产符号。
	Asset string `json:"asset"`
	// Amount 金额字符串。
	Amount string `json:"amount"`
	// Kind "mint" 对应链上 mint，"redeem" 对应链上 burn。
	Kind string `json:"kind"`
	// ChainRef 关联的链上交易哈希（可空，代表此条目已关联链事件）。
	ChainRef string `json:"chain_ref,omitempty"`
	// At 账本记录时刻。
	At time.Time `json:"at"`
}

// MismatchCase 是一个失配案件的完整记录。

type MismatchCase struct {
	// ID 案件唯一标识符（UUIDv4）。
	ID string `json:"id"`
	// Kind 失配类型。
	Kind MismatchKind `json:"kind"`
	// State 当前状态。
	State MismatchState `json:"state"`
	// UserID 涉案用户（mint_miss / burn_miss 时为账本条目用户；链上事件无法提供时为空）。
	UserID string `json:"user_id,omitempty"`
	// Asset 涉案资产。
	Asset string `json:"asset"`
	// ChainAmount 链上事件金额（mint_miss/burn_miss 时为空）。
	ChainAmount string `json:"chain_amount,omitempty"`
	// BookAmount 账本条目金额（mint_over/burn_over 时为空可能）。
	BookAmount string `json:"book_amount,omitempty"`
	// ChainEventHash 链上事件的规范哈希（hex-sha256 of tx_hash+log_index）。
	ChainEventHash string `json:"chain_event_hash"`
	// EntryHash 账本条目的规范哈希（hex-sha256 of entry id+amount）。
	EntryHash string `json:"entry_hash"`
	// ProofHash 双边守恒证明哈希（sha256(chainEventHash|entryHash|windowKey)）。
	ProofHash string `json:"proof_hash"`
	// WindowKey 检测时间窗口标识符（如 "2026-04-28T19:00Z/1h"）。
	WindowKey string `json:"window_key"`
	// SuspendedBy 发起暂停的操作员。
	SuspendedBy string `json:"suspended_by,omitempty"`
	// SuspendedAt 暂停时刻。
	SuspendedAt *time.Time `json:"suspended_at,omitempty"`
	// ClearedBy1 四眼清除第一审核人。
	ClearedBy1 string `json:"cleared_by_1,omitempty"`
	// ClearedBy2 四眼清除第二审核人（必须不同于第一审核人）。
	ClearedBy2 string `json:"cleared_by_2,omitempty"`
	// ClearedAt 解除暂停时刻。
	ClearedAt *time.Time `json:"cleared_at,omitempty"`
	// Note 运营备注。
	Note string `json:"note,omitempty"`
	// DetectedAt 失配首次检测时刻。
	DetectedAt time.Time `json:"detected_at"`
	// UpdatedAt 最后更新时刻。
	UpdatedAt time.Time `json:"updated_at"`
}

   func (c MismatchCase) Copy() MismatchCase { return c }

// SuspendedKey 生成账户暂停的复合键。

  func SuspendedKey(userID, asset string) string { return userID + "|" + asset }

// BilateralProof 是双边守恒证明的元数据记录。
type BilateralProof struct {
	// CaseID 对应的失配案件 ID。
	CaseID string `json:"case_id"`
	// ChainEventHash 链上事件哈希。
	ChainEventHash string `json:"chain_event_hash"`
	// EntryHash 账本条目哈希。
	EntryHash string `json:"entry_hash"`
	// WindowKey 时间窗口键。
	WindowKey string `json:"window_key"`
	// ProofHash 派生证明哈希。
	ProofHash string `json:"proof_hash"`
	// PersistedAt 持久化时刻。
	PersistedAt time.Time `json:"persisted_at"`
}

// SuspenseAccount 记录被暂停账户的状态。

type SuspenseAccount struct {
	// UserID 被暂停用户。
	UserID string `json:"user_id"`
	// Asset 被暂停资产。
	Asset string `json:"asset"`
	// CaseIDs 导致暂停的案件 ID 集合（可多个）。
	CaseIDs []string `json:"case_ids"`
	// Count 活跃失配案件数量（> 0 时阻断操作）。
	Count int `json:"count"`
	// SuspendedAt 首次暂停时刻。
	SuspendedAt time.Time `json:"suspended_at"`
	// LastUpdatedAt 最后更新时刻。
	LastUpdatedAt time.Time `json:"last_updated_at"`
}

// SuspendReq 暂停账户请求。

type SuspendReq struct {
	CaseID string `json:"case_id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Actor string `json:"actor"`
	Note string `json:"note,omitempty"`
}

// FourEyesClearReq 四眼清除请求。

type FourEyesClearReq struct {
	CaseID string `json:"case_id"`
	Actor1 string `json:"actor_1"`
	Actor2 string `json:"actor_2"`
	Note   string `json:"note,omitempty"`
}
