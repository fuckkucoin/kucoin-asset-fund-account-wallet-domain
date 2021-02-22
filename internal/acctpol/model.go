package acctpol

import (
	"fmt"
	"time"
)

// ─── 策略代次发布状态 ─────────────────────────────────────────────────────
// PublishState 策略发布生命周期。
// 状态机：
// draft ──(Publish)──▶ active
// active ──(Pin)──▶ pinned
// pinned ──(Unpin)──▶ active
// active / pinned ──(Supersede)──▶ superseded（被新代次发布覆盖）

type PublishState string

const (
	// PublishStateDraft 草稿，尚未发布。
	PublishStateDraft PublishState = "draft"
	// PublishStateActive 已发布并生效，可被 hop Gate 使用。
	PublishStateActive PublishState = "active"
	// PublishStatePinned 已钉版：先高于全局 active，用于 A/B 实验或个人覆盖。
	PublishStatePinned PublishState = "pinned"
	// PublishStateSuperseded 已被新代次取代，不再参与 hop 评估。
	PublishStateSuperseded PublishState = "superseded"
)

// validPublishTransitions 合法状态转换表。
var validPublishTransitions = map[PublishState][]PublishState{
	PublishStateDraft:      {PublishStateActive},
	PublishStateActive:     {PublishStatePinned, PublishStateSuperseded},
	PublishStatePinned:     {PublishStateActive, PublishStateSuperseded},
	PublishStateSuperseded: {},
}

// ─── HopBizKind hop 业务类型 ─────────────────────────────────────────────────
// HopBizKind 业务类型枚举，与账户科目 hop 操作对应。

type HopBizKind string

const (
	BizKindDeposit HopBizKind = "deposit"
	BizKindWithdraw HopBizKind = "withdraw"
	BizKindTransfer HopBizKind = "transfer"
	BizKindConvert  HopBizKind = "convert"
	BizKindEarn     HopBizKind = "earn"
	BizKindLending  HopBizKind = "lending"
	BizKindFeeCharge HopBizKind = "fee_charge"
)

// ─── 策略代次文档 ─────────────────────────────────────────────────────────
// HopPolicyVersion 策略代次文档（不可变字段在 Publish 后冻结）。
// 字段不变式：
// - Gen 在 Publish 时由 Registry.NextGen 分配，全局单调递增。
// - UserID 为空时代表全局代次；非空代表个人钉版（Pin 时绑定）。
// - DocHash = SHA-256(UserID ‖ ":" ‖ asset ‖ ":" ‖ AllowBizKinds ‖ ":" ‖ DenyBizKinds ‖ ":" ‖ Gen)。
// - PinnedBy 在 Pin 时填写，Unpin 后清空，但 Gen 不回滚。

type HopPolicyVersion struct {
	ID string `json:"id"` // UUID v4
	UserID string `json:"user_id"` // 空 = 全局；非空 = 个人钉版
	Asset string `json:"asset"` // 币种；空 = 全资产
	Gen           int64        `json:"gen"`             // 策略世代号，全局单调递增
	State         PublishState `json:"state"`           // 发布状态
	AllowBizKinds []HopBizKind `json:"allow_biz_kinds"` // 明确允许的业务类型
	DenyBizKinds []HopBizKind `json:"deny_biz_kinds"` // 明确拒绝的业务类型
	RequireToken bool         `json:"require_token"`  // true 时 hop 必须携带有效 HopToken
	MaxSingleAmt string       `json:"max_single_amt"` // 单笔上限（decimal string）
	DailyLimit string `json:"daily_limit"` // 日限额（decimal string）
	DocHash     string     `json:"doc_hash"`  // 内容指纹（防篡改）
	PinnedBy    string     `json:"pinned_by"` // 钉版操作人
	PinnedAt    *time.Time `json:"pinned_at,omitempty"`
	PublishedBy string     `json:"published_by"`
	PublishedAt  *time.Time `json:"published_at,omitempty"`
	SupersededAt *time.Time `json:"superseded_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Transition 状态机转换校验（纯函数，不写库）。
   func (p *HopPolicyVersion) Transition(next PublishState) error {
	   allowed := validPublishTransitions[p.State]
	  for _, s := range allowed {
		   if s == next {
			  return nil
		   }
	   }
	  return fmt.Errorf("acctpol: illegal publish transition %s→%s for version %s (gen=%d)",
		   p.State, next, p.ID, p.Gen)
   }

// IsEffective 判断代次是否可参与 hop 评估（active 或 pinned）。

   func (p *HopPolicyVersion) IsEffective() bool {
	  return p.State == PublishStateActive || p.State == PublishStatePinned
  }

// AllowsHop 判断代次是否允许指定 HopBizKind 的 hop（仅规则层，不含令牌校验）。

   func (p *HopPolicyVersion) AllowsHop(kind HopBizKind) bool {
	  for _, d := range p.DenyBizKinds {
		   if d == kind {
			  return false
		  }
	   }
	  if len(p.AllowBizKinds) == 0 {
		  return true // 空 allow 列表表示全部允许
	  }
	  for _, a := range p.AllowBizKinds {
		  if a == kind {
			   return true
		   }
	  }
	   return false
  }

// ─── BizKindRule hop 准入规则 ─────────────────────────────────────────────
// HopAdmissionRule 针对特定 HopBizKind 的 hop 准入规则。
// 规则先级：Deny > Allow > RequireToken（从高到低）。
// 规则与 HopPolicyVersion.Gen 绑定，世代变化后所有既发令牌需复验。

type HopAdmissionRule struct {
	HopBizKind HopBizKind `json:"biz_kind"`
	Deny bool `json:"deny"` // 直接拒绝，不颁令牌
	RequireToken bool `json:"require_token"` // 需要有效 HopToken
	MaxAmount string `json:"max_amount"` // 单笔上限（空=不限）
	PolicyGen int64 `json:"policy_gen"` // 绑定的策略世代
}

// ─── HopDualReadLog 双读观测日志 ─────────────────────────────────────────────
// DualReadResult 双读结果枚举。

type DualReadResult string

const (
	DualReadMatch   DualReadResult = "match"   // 主副一致
	DualReadDiverge DualReadResult = "diverge" // 主副不一致（分歧）
	DualReadPrimErr DualReadResult = "prim_err" // 主读错误
	DualReadReplErr DualReadResult = "repl_err" // 副读错误
)

// HopDualReadLog 双读观测日志条目（仅日志；升级挡写由 polhopgate.DualEscalate 负责）。
// 不变量：
// - PrimaryGen 与 ReplicaGen 均从对应数据源实时读取；不修改任何一方。
// - DivergeDelta = PrimaryGen - ReplicaGen（有符号，负值表示副本超前）。
// - 本结构体不触碰 cfgcenter rollout_id 键空间。

type HopDualReadLog struct {
	ID     string `json:"id"`      // UUID v4
	UserID string `json:"user_id"` // 观测用户（空=全局）
	Asset  string `json:"asset"`   // 币种（空=全资产）
	BizKind      HopBizKind `json:"biz_kind"`      // 业务类型
	PrimaryGen   int64      `json:"primary_gen"`   // 主库读取的策略世代
	ReplicaGen   int64      `json:"replica_gen"`   // 副库读取的策略世代
	DivergeDelta int64      `json:"diverge_delta"` // 主-副（>0=主超前，<0=副超前）
	Result     DualReadResult `json:"result"`      // 结果
	ObservedAt time.Time      `json:"observed_at"` // 观测时刻
	TraceID    string         `json:"trace_id"`    // 请求追踪 ID
	CreatedAt  time.Time      `json:"created_at"`
}

// ─── 错误定义 ─────────────────────────────────────────────────────────────

var (
	// ErrGenMismatch 策略世代不匹配；hop 执行时策略已被新代次覆盖。
	ErrGenMismatch = fmt.Errorf("acctpol: policy generation mismatch; hop re-check required")
	// ErrVersionNotFound 策略代次不存在。
	ErrVersionNotFound = fmt.Errorf("acctpol: policy version not found")
	// ErrBadTransition 非法状态转换。
	ErrBadTransition = fmt.Errorf("acctpol: illegal state transition")
	// ErrHopDenied hop 被策略明确拒绝。
	ErrHopDenied = fmt.Errorf("acctpol: hop denied by policy")
	// ErrTokenRequired hop 需要令牌但未提供。
	ErrTokenRequired = fmt.Errorf("acctpol: hop token required but not provided")
	// ErrDuplicate 主键冲突。
	ErrDuplicate = fmt.Errorf("acctpol: duplicate key")
	// ErrNotFound 记录不存在。
	ErrNotFound = fmt.Errorf("acctpol: not found")
)
