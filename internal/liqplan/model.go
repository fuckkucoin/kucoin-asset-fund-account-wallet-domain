package liqplan

import (
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ─────────────────────────────────────────────
// PlanState：强平计划状态机状态集
// ─────────────────────────────────────────────

type PlanState string

const (
	PlanTriggered PlanState = "triggered" // LTV 破线，计划已生成，占用令牌已颁发
	PlanSeizing   PlanState = "seizing"   // 至少一条扣押腿已启动
	PlanSettling  PlanState = "settling"  // 所有腿完成后执行清偿借项
	PlanClosed    PlanState = "closed"    // 所有腿 done+settled+residue=∅，终态
	PlanResidued  PlanState = "residued"  // 有腿未收口留残留，需人工或巡检续跑，终态候选
	PlanAborted   PlanState = "aborted"   // 双人中止，仅允许 triggered 态流入
)

// PlanEvent：触发状态转换的事件
type PlanEvent string

const (
	EvSeizeStart PlanEvent = "seize_start"
	EvSeizeDone  PlanEvent = "seize_done"
	EvSettleDone PlanEvent = "settle_done"
	EvResidueFound PlanEvent = "residue_found"
	EvResidueCleared PlanEvent = "residue_cleared"
	EvAbort          PlanEvent = "abort"
)

// ─────────────────────────────────────────────
// LegState：扣押腿状态机状态集
// ─────────────────────────────────────────────

type LegState string

const (
	LegPending   LegState = "pending"   // 等待执行
	LegExecuting LegState = "executing" // 正在扣押
	LegDone      LegState = "done"      // 扣押完成
	LegFailed    LegState = "failed"    // 执行失败，可续跑
	LegRolled    LegState = "rolled"    // 已回滚，残留已清
	LegSuspended LegState = "suspended" // 挂账，等人工处置
)

// LegEvent：腿状态转换事件

type LegEvent string

const (
	EvLegStart   LegEvent = "start"
	EvLegSucceed LegEvent = "succeed"
	EvLegFail    LegEvent = "fail"
	EvLegResume   LegEvent = "resume"
	EvLegRollback LegEvent = "rollback"
	EvLegSuspend LegEvent = "suspend"
)

// ─────────────────────────────────────────────
// LiqPlan：强平计划主记录
// ─────────────────────────────────────────────
// LiqPlan 一次 LTV 穿仓触发的强平占用计划。
// OccupyToken 颁发即锁定；BorrowID/CollateralID 只读引用 marginhook 簿。

type LiqPlan struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	BorrowID     string `json:"borrow_id"`
	CollateralID string `json:"collateral_id"`
	// 触发时 LTV 快照（万分比）
	TriggerLTVBPS int `json:"trigger_ltv_bps"`
	CapBPS int `json:"cap_bps"`
	// 抵押资产与借项资产快照
	CollateralAsset  string `json:"collateral_asset"`
	CollateralAmount string `json:"collateral_amount"`
	BorrowAsset      string `json:"borrow_asset"`
	BorrowAmount string `json:"borrow_amount"`
	// 占用令牌：颁发即不得再开新计划（same borrow）
	OccupyToken string `json:"occupy_token"`
	// 标价世代：闸检时若 MarkGeneration 不匹配则拒绝触发
	MarkGateID string `json:"mark_gate_id"`
	MarkGeneration int64 `json:"mark_generation"`
	State     PlanState `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// 幂等键：同 BorrowID 只得一个活跃计划
	IdempotencyKey string        `json:"idempotency_key"`
	Fence          persist.Fence `json:"fence"`
	// 对应 LiqProof ID（终态后填写）
	ProofID string `json:"proof_id,omitempty"`
	// 双人操作记录
	AbortedBy string `json:"aborted_by,omitempty"`
	AbortedAt   time.Time `json:"aborted_at,omitempty"`
	AbortReason string    `json:"abort_reason,omitempty"`
}

// ─────────────────────────────────────────────
// SeizeLeg：单条扣押腿
// ─────────────────────────────────────────────
// SeizeLeg 表示一次具体的资产扣押操作。一个 LiqPlan 可有多腿（分步扣押不同子账户/资产）。
// PartialAmount 记录中途失败时已实际扣押的金额；若非零，形成残留。

type SeizeLeg struct {
	ID     string `json:"id"`
	PlanID string `json:"plan_id"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	TargetAmount  string `json:"target_amount"`  // 计划扣押目标金额
	PartialAmount string `json:"partial_amount"` // 已扣押（中途失败留下）
	// 对应 Hold/Borrow 只读引用
	HoldID   string `json:"hold_id"`
	BorrowID string `json:"borrow_id"`
	State LegState `json:"state"`
	// 失败信息
	FailReason string `json:"fail_reason,omitempty"`
	FailAt     time.Time `json:"fail_at,omitempty"`
	RetryCount int       `json:"retry_count"`
	MaxRetry int `json:"max_retry"`
	// 幂等键：防止重复执行
	IdempotencyKey string `json:"idempotency_key"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// 回滚/挂账记录
	RolledAt time.Time `json:"rolled_at,omitempty"`
	SuspendedAt time.Time `json:"suspended_at,omitempty"`
	SuspendNote string    `json:"suspend_note,omitempty"`
	Fence persist.Fence `json:"fence"`
}

// HasResidue 报告此腿是否留有未收口残留金额。
  func (l *SeizeLeg) HasResidue() bool {
	  if l == nil {
		   return false
	  }
	   return l.State == LegFailed || l.State == LegSuspended
  }

// ─────────────────────────────────────────────
// MarkGate：标价世代闸
// ─────────────────────────────────────────────
// MarkGate 绑定一次强平决策使用的标价世代快照。
// 若世代过期（staleness > MaxAgeSeconds），新触发必须拒绝（fail-closed）。
// 本结构只读消费 oracle/fxmark 标价，不写行情主路径。
type MarkGate struct {
	ID string `json:"id"`
	Asset string `json:"asset"` // 被标价的资产（抵押品）
	PairAsset string `json:"pair_asset"` // 计价货币（如 USDT）
	MarkPrice string `json:"mark_price"` // 触发时标价快照
	Generation    int64     `json:"generation"`      // 标价世代序号
	ObservedAt    time.Time `json:"observed_at"`     // 世代观察时间
	MaxAgeSeconds int       `json:"max_age_seconds"` // 最大有效期（秒）
	// Poisoned 标记：若外部检测到价格中毒，强制失效
	Poisoned bool `json:"poisoned"`
	PoisonNote string `json:"poison_note,omitempty"`
	// 绑定到哪个计划
	PlanID string `json:"plan_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// IsStale 检查标价世代是否过期。
  func (g *MarkGate) IsStale(now time.Time) bool {
	   if g == nil {
		  return true
	   }
	   if g.Poisoned {
		   return true
	  }
	   return now.Sub(g.ObservedAt) > time.Duration(g.MaxAgeSeconds)*time.Second
   }

// ─────────────────────────────────────────────
// LiqProof：强平终态证明
// ─────────────────────────────────────────────
// LiqProof 是强平计划终态证明：占用令牌 ⊕ 扣押集合 ⊕ 清偿结果 ⊕ 残留集合的不可变快照。
// 计划到达 closed/residued 状态后生成，不得再修改。

type LiqProof struct {
	ID     string `json:"id"`
	PlanID string `json:"plan_id"`
	// 占用快照
	OccupyToken string `json:"occupy_token"`
	CollateralAsset  string `json:"collateral_asset"`
	CollateralAmount string `json:"collateral_amount"`
	// 扣押摘要
	SeizedLegs []SeizeLegSummary `json:"seized_legs"`
	TotalSeized string `json:"total_seized"`
	// 清偿摘要
	BorrowAsset string `json:"borrow_asset"`
	BorrowAmount  string `json:"borrow_amount"`
	SettledAmount string `json:"settled_amount"`
	// 残留摘要（若有）
	ResidueLegs  []ResidueLegSummary `json:"residue_legs,omitempty"`
	TotalResidue string              `json:"total_residue,omitempty"`
	// 证明摘要（SHA256 of canonical JSON）
	Digest string `json:"digest"`
	// 状态：draft（计划完成即生成）→ validated（双人校验）
	Status ProofStatus `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	// 双人验证
	ValidatedBy string `json:"validated_by,omitempty"`
	ValidatedAt time.Time `json:"validated_at,omitempty"`
}

type ProofStatus string

const (
	ProofDraft     ProofStatus = "draft"
	ProofValidated ProofStatus = "validated"
)

// SeizeLegSummary 扣押腿快照（写入 proof 时不可变）。
type SeizeLegSummary struct {
	LegID string `json:"leg_id"`
	Asset string `json:"asset"`
	TargetAmount string `json:"target_amount"`
	SeizedAmount string   `json:"seized_amount"`
	State        LegState `json:"state"`
}

// ResidueLegSummary 残留腿快照。

type ResidueLegSummary struct {
	LegID         string   `json:"leg_id"`
	Asset         string   `json:"asset"`
	PartialAmount string   `json:"partial_amount"`
	State         LegState `json:"state"`
}

// ─────────────────────────────────────────────
// OccupyToken：占用令牌
// ─────────────────────────────────────────────
// OccupyToken 颁发时锁定：同一 BorrowID 不得同时有两个活跃计划。
// 令牌存储于 LiqPlan.OccupyToken；解除时须计划到达终态。

type OccupyToken struct {
	Token    string `json:"token"`
	BorrowID string `json:"borrow_id"`
	PlanID string `json:"plan_id"`
	IssuedAt time.Time `json:"issued_at"`
}
