package overwdbar

import (
	"time"
	"account-assets.kucoin.net/funding/internal/availcas"
	"github.com/shopspring/decimal"
)

// OverdrawVarianceEvent 超提差异事件。
// 发出时机：PreCheck 或 Commit 阶段被屏障拒绝。

type OverdrawVarianceEvent struct {
	// EventID 全局唯一 ID。
	EventID string `json:"event_id"`
	// UserID 涉及用户。
	UserID string `json:"user_id"`
	// Asset 资产代码。
	Asset string `json:"asset"`
	// Book 账本位。
	Book string `json:"book"`
	// RequestedAmount 意图扣减额。
	RequestedAmount decimal.Decimal `json:"requested_amount"`
	// AvailableAtDecision 决策时刻的可用余额。
	AvailableAtDecision decimal.Decimal `json:"available_at_decision"`
	// Shortfall 差额（RequestedAmount − AvailableAtDecision），>0 代表超提。
	Shortfall decimal.Decimal `json:"shortfall"`
	// RejectCode 屏障拒绝原因码。
	RejectCode availcas.RejectCode `json:"reject_code"`
	// IntentID 被拒绝的意图 ID（若已生成）。
	IntentID string `json:"intent_id,omitempty"`
	// RaceProofID 竞态证明 ID（若产生）。
	RaceProofID string `json:"race_proof_id,omitempty"`
	// Gate 触发阶段：precheck 或 commit。
	Gate string `json:"gate"`
	// ActorID 发起方服务标识。
	ActorID string `json:"actor_id,omitempty"`
	// IsLiveHash 该冻结是否关联 live-hash（绝不自动解冻）。
	IsLiveHash bool `json:"is_live_hash"`
	// OccurredAt 事件发生时刻。
	OccurredAt time.Time `json:"occurred_at"`
}

const (
	// GatePreCheck 预检阶段触发。
	GatePreCheck = "precheck"
	// GateCommit 提交阶段触发。
	GateCommit = "commit"
)

// VarianceEmitter 差异事件发射接口，由业务层实现（写 outbox 或发 bus 事件）。
type VarianceEmitter interface {
	EmitOverdrawVariance(event *OverdrawVarianceEvent) error
}

// noopEmitter 空实现，供无外部 bus 的单元使用。
type noopEmitter struct{}

   func (noopEmitter) EmitOverdrawVariance(_ *OverdrawVarianceEvent) error { return nil }
