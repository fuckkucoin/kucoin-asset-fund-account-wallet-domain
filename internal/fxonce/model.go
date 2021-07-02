package fxonce

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// ────────────────────────────────────────────────────────────────────────────
// 效果种类常量（FundKind）
// ────────────────────────────────────────────────────────────────────────────
// FundKind 资金副作用类型。只有归类为 FundKind 的 kind 才强制走效果闸。
// 非资金 kind（如 notify.email、audit.log）可旁路，不产生 DupProof。

type FundKind string

const (
	// KindCreditIntent 贷记意图：向用户账户增加可用余额。
	KindCreditIntent FundKind = "credit_intent"
	// KindFreezeIntent 冻结意图：冻结用户可用余额（出金/质押等）。
	KindFreezeIntent FundKind = "freeze_intent"
	// KindHopIntent 跨账本划转意图（热→冷、earn→main 等跨位划转）。
	KindHopIntent FundKind = "hop_intent"
	// KindNotifyFund 资金变动通知（客户端推送/短信）；
	// 此类通知会触发用户侧资金操作，故列为资金 kind 强制幂等。
	KindNotifyFund FundKind = "notify_fund"
	// KindDebitIntent 扣减意图：从用户账户扣减（充值回滚/手续费扣减等）。
	KindDebitIntent FundKind = "debit_intent"
	// KindSettleIntent 结算意图：清算批次最终资金划拨。
	KindSettleIntent FundKind = "settle_intent"
)

// AllFundKinds 返回全部资金 kind 列表（供注册校验用）。

  func AllFundKinds() []FundKind {
	   return []FundKind{
		   KindCreditIntent,
		  KindFreezeIntent,
		   KindHopIntent,
		  KindNotifyFund,
		  KindDebitIntent,
		   KindSettleIntent,
	   }
  }

// IsFundKind 判断 kind 字符串是否属于资金类。
  func IsFundKind(kind string) bool {
	  for _, fk := range AllFundKinds() {
		  if string(fk) == kind {
			  return true
		  }
	  }
	   return false
   }

// ────────────────────────────────────────────────────────────────────────────
// EffectState 效果状态枚举
// ────────────────────────────────────────────────────────────────────────────
// EffectState 描述一个 FundEffect 记录的生命周期相位。
type EffectState string

const (
	// EffectProposed 效果已提交但尚未完成 CAS 裁决。
	EffectProposed EffectState = "proposed"
	// EffectApplied 效果通过 CAS，handler 成功 Mark；账本意图已产生。
	EffectApplied EffectState = "applied"
	// EffectRejected CAS 拒绝：旧世代或重复世代；不产生账本意图。
	EffectRejected EffectState = "rejected"
	// EffectPoisoned 超出最大重试；进毒丸隔离，不产生账本意图，需双人放行。
	EffectPoisoned EffectState = "poisoned"
)

// ────────────────────────────────────────────────────────────────────────────
// GenState 投递世代状态枚举
// ────────────────────────────────────────────────────────────────────────────
// GenState 描述一个 DeliveryGen 记录的状态。

type GenState string

const (
	// GenEnqueued 已入箱，等待投递。
	GenEnqueued GenState = "enqueued"
	// GenInflight 正在投递（handler 已取得，未 Mark）。
	GenInflight GenState = "inflight"
	// GenAcked handler 成功，已 Mark（最终态）。
	GenAcked GenState = "acked"
	// GenNacked handler 失败，退回重试队列。
	GenNacked GenState = "nacked"
	// GenPoisoned 超限进毒丸（最终态，需运营介入）。
	GenPoisoned GenState = "poisoned"
)

// ────────────────────────────────────────────────────────────────────────────
// MaxGeneration 防世代溢出上限
// ────────────────────────────────────────────────────────────────────────────
// MaxGeneration 单个 business_key 允许的最大世代数。
// 超过此值视为配置或代码缺陷，拒绝接着递增。
const MaxGeneration int64 = 1_000_000

// ────────────────────────────────────────────────────────────────────────────
// FundEffect — 资金副作用唯一效果记录
// ────────────────────────────────────────────────────────────────────────────
// FundEffect 对单次资金副作用持久化一条唯一效果记录。
// effect_key 由 business_key 与 generation 共同派生（SHA256 截取），
// 保证同一业务键在不同投递世代可产生不同 effect_key，
// 从而在数据库层面通过唯一键强制幂等。
// 不变量：
// - effect_key 一旦写入不可修改；
// - 从 applied/poisoned 到其他状态的迁移必须通过强制重放 API；
// - 不引用 avail_leases / debit_intents（r6 可用 CAS 内核），二者隔离。

type FundEffect struct {
	// ID 全局唯一标识（UUID v4）。
	ID string `json:"id"`
	// BusinessKey 业务主键（如 deposit_id、withdrawal_id、transfer_id）。
	// 同一 BusinessKey 在不同 Generation 对应不同 effect_key。
	BusinessKey string `json:"business_key"`
	// Kind 资金副作用类型（FundKind 枚举值）。
	Kind FundKind `json:"kind"`
	// Generation 投递世代号，从 1 开始单调递增。
	// 每次 outboxpump 重新入箱或 Leader 切换后递增。
	Generation int64 `json:"generation"`
	// EffectKey SHA256( business_key + ":" + generation )[:32]（hex 字符串）。
	// 数据库唯一索引字段；冲突即双花。
	EffectKey string `json:"effect_key"`
	// State 当前效果状态（proposed/applied/rejected/poisoned）。
	State EffectState `json:"state"`
	// Payload 原始业务载荷快照（JSON，只读存储，不参与 CAS）。
	Payload string `json:"payload,omitempty"`
	// AppliedAt 效果被 applied 的时刻（nil = 尚未 applied）。
	AppliedAt *time.Time `json:"applied_at,omitempty"`
	// RejectedAt 效果被 rejected/poisoned 的时刻。
	RejectedAt *time.Time `json:"rejected_at,omitempty"`
	// RejectReason 拒绝原因码（RejectCode）。
	RejectReason RejectCode `json:"reject_reason,omitempty"`
	// PoisonRetries 触发毒丸时的累计重试次数。
	PoisonRetries int `json:"poison_retries,omitempty"`
	// ForceReplayAt 强制重放时刻（双人授权后解除 poisoned 的时刻）。
	ForceReplayAt *time.Time `json:"force_replay_at,omitempty"`
	// ForceReplayBy 强制重放操作人（形态：actor1+actor2）。
	ForceReplayBy string `json:"force_replay_by,omitempty"`
	// CreatedAt 记录创建时刻。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最后状态变更时刻。
	UpdatedAt time.Time `json:"updated_at"`
}

// ────────────────────────────────────────────────────────────────────────────
// DeliveryGen — 投递世代 CAS 记录
// ────────────────────────────────────────────────────────────────────────────
// DeliveryGen 记录每次投递的世代信息，用于 CAS 拦截旧世代和重复世代。
// Mark/Apply 必须携带 ExpectedGen；与当前 AppliedGen 比较：
// - ExpectedGen > AppliedGen：正常推进，更新 AppliedGen；
// - ExpectedGen == AppliedGen：重复提交（dup_gen），幂等返回已有结果；
// - ExpectedGen < AppliedGen：旧世代（stale_gen），拒绝，记录 DupProof。
type DeliveryGen struct {
	// ID 全局唯一标识（UUID v4）。
	ID string `json:"id"`
	// BusinessKey 与 FundEffect.BusinessKey 对应的同一业务主键。
	BusinessKey string `json:"business_key"`
	// Kind 资金副作用 kind（同 FundEffect.Kind）。
	Kind FundKind `json:"kind"`
	// ExpectedGen 本次投递时声明的期望世代号（调用方携带）。
	ExpectedGen int64 `json:"expected_gen"`
	// AppliedGen 最后一次被 applied 的世代号（初始为 0，表示从未 applied）。
	AppliedGen int64 `json:"applied_gen"`
	// State 当前世代状态（enqueued/inflight/acked/nacked/poisoned）。
	State GenState `json:"state"`
	// RetryCount 累计失败重试次数（不含首次投递）。
	RetryCount int `json:"retry_count"`
	// MaxRetry 允许的最大重试次数（默认 5）。
	MaxRetry int `json:"max_retry"`
	// RejectReason 最近一次被拒绝的原因码（空表示尚无拒绝）。
	RejectReason RejectCode `json:"reject_reason,omitempty"`
	// EffectID 关联的 FundEffect.ID（建立后不可更改）。
	EffectID string `json:"effect_id"`
	// OutboxEventID 对应的 persist.OutboxEvent.ID（用于跨模块关联审计）。
	OutboxEventID string `json:"outbox_event_id,omitempty"`
	// AckedAt handler 成功 Mark 的时刻。
	AckedAt *time.Time `json:"acked_at,omitempty"`
	// NackedAt 最近一次 nack 时刻。
	NackedAt *time.Time `json:"nacked_at,omitempty"`
	// CreatedAt 记录创建时刻。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最后状态变更时刻。
	UpdatedAt time.Time `json:"updated_at"`
}

// ────────────────────────────────────────────────────────────────────────────
// DupProof — 双花证明记录
// ────────────────────────────────────────────────────────────────────────────
// DupProof 对一次双花冲突持久化证明。
// 当 CAS 拦截到旧世代（stale_gen）时产生，供 desk 与事故复盘使用。
// 字段语义：
// - WinnerGen 通过 CAS 的（胜者）世代号；
// - LoserGen 被拒绝的（败者）世代号；
// - Window 两次投递的时间窗口（LoserAt - WinnerAt）；
// - RejectCode 拒绝码（stale_gen / dup_gen / poisoned）。
type DupProof struct {
	// ID 全局唯一标识（UUID v4）。
	ID string `json:"id"`
	// BusinessKey 触发双花的业务主键。
	BusinessKey string `json:"business_key"`
	// Kind 资金副作用 kind。
	Kind FundKind `json:"kind"`
	// WinnerGen 胜者投递世代号。
	WinnerGen int64 `json:"winner_gen"`
	// LoserGen 败者投递世代号（被 CAS 拒绝）。
	LoserGen int64 `json:"loser_gen"`
	// WinnerEffectID 胜者 FundEffect.ID。
	WinnerEffectID string `json:"winner_effect_id"`
	// LoserEffectID 败者 FundEffect.ID（可能为空，若败者从未建 effect）。
	LoserEffectID string `json:"loser_effect_id,omitempty"`
	// WinnerDeliveryID 胜者 DeliveryGen.ID。
	WinnerDeliveryID string `json:"winner_delivery_id"`
	// LoserDeliveryID 败者 DeliveryGen.ID。
	LoserDeliveryID string `json:"loser_delivery_id"`
	// WinnerAt 胜者 Mark 时刻。
	WinnerAt time.Time `json:"winner_at"`
	// LoserAt 败者被拒绝时刻。
	LoserAt time.Time `json:"loser_at"`
	// WindowMs 双花时间窗口（毫秒），= LoserAt - WinnerAt（可为负，表示乱序）。
	WindowMs int64 `json:"window_ms"`
	// RejectCode 拒绝码（stale_gen / dup_gen / poisoned）。
	RejectCode RejectCode `json:"reject_code"`
	// Reviewed 是否已被 desk 人工审阅。
	Reviewed bool `json:"reviewed"`
	// ReviewedBy desk 审阅人（形态：actor）。
	ReviewedBy string `json:"reviewed_by,omitempty"`
	// ReviewedAt desk 审阅时刻。
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
	// CreatedAt 证明创建时刻。
	CreatedAt time.Time `json:"created_at"`
}

// ────────────────────────────────────────────────────────────────────────────
// KindPolicy — Kind 策略配置
// ────────────────────────────────────────────────────────────────────────────
// KindPolicy 决定某个 kind 是否必须走效果闸、是否允许旁路、是否封禁。
// 默认策略（未配置时）：
// - IsFundKind(kind) == true → 强制走效果闸（gate_required=true）；
// - IsFundKind(kind) == false → 允许旁路（gate_required=false）。
// 运营可通过 API 覆盖默认策略（例如将某非资金 kind 纳入效果闸）。
type KindPolicy struct {
	// Kind 目标 kind 字符串。
	Kind string `json:"kind"`
	// GateRequired true = 必须通过效果闸（强制幂等）；false = 允许旁路。
	GateRequired bool `json:"gate_required"`
	// Blocked true = kind 被显式封禁，任何投递均拒绝。
	Blocked bool `json:"blocked"`
	// MaxRetry 此 kind 的最大重试次数（0 表示使用全局默认）。
	MaxRetry int `json:"max_retry,omitempty"`
	// Memo 配置变更备注（运营审计）。
	Memo string `json:"memo,omitempty"`
	// UpdatedBy 最后修改人。
	UpdatedBy string `json:"updated_by,omitempty"`
	// UpdatedAt 最后修改时刻。
	UpdatedAt time.Time `json:"updated_at"`
}

// ────────────────────────────────────────────────────────────────────────────
// EffectKey 工具函数
// ────────────────────────────────────────────────────────────────────────────
// MakeEffectKey 生成 effect_key。
// 公式：hex( SHA256( business_key + ":" + generation ) )[:32]
// 长度固定 32 字节十六进制；数据库列 VARCHAR(64) 存储。
   func MakeEffectKey(businessKey string, generation int64) string {
	  raw := fmt.Sprintf("%s:%d", businessKey, generation)
	   sum := sha256.Sum256([]byte(raw))
	  return fmt.Sprintf("%x", sum[:16]) // 取前 16 字节 = 32 hex chars
  }

// ValidateBusinessKey 检验 business_key 形态（非空，长度 ≤ 191）。
  func ValidateBusinessKey(key string) error {
	   if key == "" {
		  return ErrBadBusinessKey
	   }
	   if len(key) > 191 {
		  return ErrBadBusinessKey
	   }
	  return nil
  }

// ValidateEffectKey 检验 effect_key 形态（32 位十六进制字符串）。

  func ValidateEffectKey(key string) error {
	   if len(key) != 32 {
		  return ErrBadEffectKey
	  }
	   for _, c := range key {
		  if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			   return ErrBadEffectKey
		  }
	  }
	  return nil
   }

// ────────────────────────────────────────────────────────────────────────────
// MarkRequest — Mark/Apply 请求载体
// ────────────────────────────────────────────────────────────────────────────
// MarkRequest 是 Delivery Generation CAS Apply 的入参。
// 调用方（outboxpump 适配器 / webhook 适配器）在 handler 成功后构造此结构
// 并传给 CASStore.Apply；stale/dup generation 必须阻止账本意图产生。

type MarkRequest struct {
	// BusinessKey 业务主键（同 FundEffect.BusinessKey）。
	BusinessKey string `json:"business_key"`
	// Kind 资金副作用 kind。
	Kind FundKind `json:"kind"`
	// ExpectedGen 调用方声明的期望世代号（必须匹配 DeliveryGen.ExpectedGen）。
	ExpectedGen int64 `json:"expected_gen"`
	// Payload 业务载荷（handler 处理结果或原始出箱 payload）。
	Payload string `json:"payload,omitempty"`
	// OutboxEventID 对应的 persist.OutboxEvent.ID（审计用）。
	OutboxEventID string `json:"outbox_event_id,omitempty"`
}

// MarkResult 是 CASStore.Apply 的结果。

type MarkResult struct {
	// Effect 最终 applied 的 FundEffect 记录。
	Effect *FundEffect `json:"effect,omitempty"`
	// Gen 最终 acked 的 DeliveryGen 记录。
	Gen *DeliveryGen `json:"gen,omitempty"`
	// Dup 若产生双花，此字段包含 DupProof（否则为 nil）。
	Dup *DupProof `json:"dup,omitempty"`
	// Idempotent true = 幂等回放（effect 已 applied，直接返回历史结果）。
	Idempotent bool `json:"idempotent,omitempty"`
	// Rejected true = CAS 拒绝（stale_gen / dup_gen / poisoned）。
	Rejected bool `json:"rejected,omitempty"`
	// RejectCode 拒绝码（仅 Rejected=true 时有效）。
	RejectCode RejectCode `json:"reject_code,omitempty"`
}
