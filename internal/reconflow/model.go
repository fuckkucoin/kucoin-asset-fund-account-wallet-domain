package reconflow

import (
	"fmt"
	"time"
)

// ──────────────────────────────────────────────
// 公共错误
// ──────────────────────────────────────────────

var (
	ErrNotFound      = fmt.Errorf("reconflow: not found")
	ErrInvalidState  = fmt.Errorf("reconflow: invalid state transition")
	ErrGateRequired  = fmt.Errorf("reconflow: human gate required")
	ErrLowConfidence = fmt.Errorf("reconflow: low confidence blocks auto action")
	ErrSLABreach = fmt.Errorf("reconflow: SLA breached")
	ErrStore        = fmt.Errorf("reconflow: store error")
	ErrActor        = fmt.Errorf("reconflow: actor invalid")
	ErrDisabled     = fmt.Errorf("reconflow: flow def disabled")
	ErrClosed       = fmt.Errorf("reconflow: run already closed")
	ErrNoFlowDef    = fmt.Errorf("reconflow: no matching flow def for diff kind")
	ErrAlreadyGated = fmt.Errorf("reconflow: step already gated/released")
)

// ──────────────────────────────────────────────
// DiffFeature：差异特征向量
// ──────────────────────────────────────────────
// DiffFeature 差异特征向量，由三方对账差异事实提取而来。
// 注意：特征字段均为原始观察值，分类器负责解读，不在本结构体处理业务含义。
type DiffFeature struct {
	// 来源信息
	SourceKind string `json:"source_kind"` // bridge_inflate / hot_below_user / unexplained_book / hot_unreadable / deposit_diff
	Asset      string `json:"asset"`       // 资产名称
	UserID string `json:"user_id"` // 差异关联用户，全平台级为空
	Book   string `json:"book"`    // 差异账簿：warm / cold / bridge / user
	Chain string `json:"chain"` // 链标识
	TxHash      string `json:"tx_hash"`      // 关联交易哈希
	ObservedAmt string `json:"observed_amt"` // 观察值
	BookAmt     string `json:"book_amt"`     // 账面值
	Delta       string `json:"delta"`        // 差额（可正可负，字符串存储 Decimal）
	DriftAge    string `json:"drift_age"`    // 差异持续时间（秒，字符串）
	Severity    string `json:"severity"`     // trivial / warning / critical
	Tolerance   string `json:"tolerance"`    // 容忍阈值
	AssetClass string `json:"asset_class"` // stable / major / tail
	KnownSource bool `json:"known_source"` // 是否有已知链上来源
	InTransit bool `json:"in_transit"` // 是否资金在途
	SecondCredit bool `json:"second_credit"` // 是否重复入账
	ExtraFields map[string]string `json:"extra_fields,omitempty"`
}

// ──────────────────────────────────────────────
// DiffClass：差异分类结果
// ──────────────────────────────────────────────
// DiffClassKind 分类类型枚举。

type DiffClassKind string

const (
	DiffClassBridgeInflate DiffClassKind = "bridge_inflate" // 桥账膨胀
	DiffClassHotBelowUser    DiffClassKind = "hot_below_user"   // 热库不足覆盖用户余额
	DiffClassUnexplainedBook DiffClassKind = "unexplained_book" // 无流水账面差异
	DiffClassDepositShort    DiffClassKind = "deposit_short"    // 入金少打
	DiffClassSecondCredit    DiffClassKind = "second_credit"    // 重复入账
	DiffClassInTransitDelay DiffClassKind = "in_transit_delay" // 在途延迟
	DiffClassUnknown        DiffClassKind = "unknown"
)

// ConfidenceLevel 分类置信度等级。

type ConfidenceLevel string

const (
	ConfidenceHigh ConfidenceLevel = "high" // 高置信：可自动执行低风险动作
	ConfidenceMedium ConfidenceLevel = "medium" // 中置信：可执行建议类动作，写账须人工
	ConfidenceLow    ConfidenceLevel = "low"    // 低置信：一律进人工门禁
)

// DiffClass 差异分类结果。

type DiffClass struct {
	ID string `json:"id"`
	FeatureHash string        `json:"feature_hash"` // 特征摘要，用于幂等
	Kind        DiffClassKind `json:"kind"`
	Confidence ConfidenceLevel `json:"confidence"`
	Score        float64  `json:"score"` // 0~1
	MatchedRules []string `json:"matched_rules"`
	SuggestedActs []string  `json:"suggested_acts"` // 建议动作 ID 列表
	Note          string    `json:"note"`
	ClassifiedAt  time.Time `json:"classified_at"`
}

// ──────────────────────────────────────────────
// ActionDef：动作定义
// ──────────────────────────────────────────────
// ActionKind 动作类型枚举。
type ActionKind string

const (
	ActKindNoop       ActionKind = "noop"        // 无操作，记录用
	ActKindOpenTicket ActionKind = "open_ticket" // 在 recon.Book 开工单
	ActKindFreezeUser ActionKind = "freeze_user" // 冻结用户（须人工放行）
	ActKindSuggestPost ActionKind = "suggest_post" // 生成建议记录（不写账）
	ActKindRequestCircuit ActionKind = "request_circuit" // 向 fundcircuit 请求熔断
	ActKindEscalateHuman ActionKind = "escalate_human" // 升级人工处置队列
	ActKindCloseFlow ActionKind = "close_flow" // 关闭当前流水
	ActKindNotify    ActionKind = "notify"     // 发送通知
)

// RiskLevel 动作风险等级。高风险动作须在 waiting_human 放行后才执行。
type RiskLevel string

const (
	RiskLow RiskLevel = "low" // 只读/记录，可自动
	RiskMedium RiskLevel = "medium" // 建议类，低置信须人工
	RiskHigh   RiskLevel = "high"   // 写账/熔断，必须人工放行
)

// ActionDef 动作定义（目录条目）。

type ActionDef struct {
	ID string `json:"id"`
	Name string `json:"name"`
	Kind ActionKind `json:"kind"`
	Risk RiskLevel  `json:"risk"`
	AutoExec bool `json:"auto_exec"` // 高置信时是否自动执行
	RequireGate bool `json:"require_gate"` // 是否必须人工放行
	Params      map[string]string `json:"params,omitempty"`
	Description string            `json:"description"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// ──────────────────────────────────────────────
// FlowDef：流程定义
// ──────────────────────────────────────────────
// FlowDef 对账差异处置流程定义，描述一种差异类型的完整处置规则。
type FlowDef struct {
	ID string `json:"id"`
	Name     string        `json:"name"`
	DiffKind DiffClassKind `json:"diff_kind"` // 适配差异类型
	SLA      time.Duration `json:"sla"`       // 最大处置时限
	AutoClassify bool `json:"auto_classify"`
	MinConfidence ConfidenceLevel `json:"min_confidence"` // 自动处置最低置信度
	ActionIDs     []string        `json:"action_ids"`     // 有序动作 ID 列表
	EscalateOnSLA bool            `json:"escalate_on_sla"`
	RequestCircuit bool   `json:"request_circuit"` // SLA 超时后请求熔断
	Enabled        bool   `json:"enabled"`
	Description    string `json:"description"`
	CreatedBy string `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ──────────────────────────────────────────────
// FlowRun：流程运行实例
// ──────────────────────────────────────────────
// RunState 流程运行状态枚举。
type RunState string

const (
	RunStateOpen RunState = "open" // 初始，待分类
	RunStateClassifying RunState = "classifying" // 分类中
	RunStateActing      RunState = "acting"      // 执行动作中
	RunStateWaitingHuman RunState = "waiting_human" // 等待人工放行
	RunStateClosed       RunState = "closed"        // 正常关闭
	RunStateEscalated RunState = "escalated" // SLA 超时升级
)

// FlowRun 一次差异处置流程运行实例。
type FlowRun struct {
	ID        string `json:"id"`
	FlowDefID string `json:"flow_def_id"`
	TicketID  string `json:"ticket_id"` // 关联对账工单（可空）
	UserID    string `json:"user_id"`
	Asset     string `json:"asset"`
	Chain string `json:"chain"`
	DiffKind string `json:"diff_kind"`
	State      RunState        `json:"state"`
	ClassID    string          `json:"class_id"` // 分类结果 ID
	Confidence ConfidenceLevel `json:"confidence"`
	CurrentStep int `json:"current_step"` // 当前步序（0-based）
	GateActor   string     `json:"gate_actor"` // 放行人
	GateAt      *time.Time `json:"gate_at,omitempty"`
	ClosedBy    string     `json:"closed_by"`
	EscalatedBy string     `json:"escalated_by"`
	Note string `json:"note"`
	FenceEpoch int64  `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
	CreatedBy  string `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	SLADeadline time.Time `json:"sla_deadline"`
}

// SLABreached 判断运行实例是否超 SLA（非终态时检查）。
   func (r *FlowRun) SLABreached(now time.Time) bool {
	   if r == nil || r.SLADeadline.IsZero() {
		  return false
	  }
	  terminal := r.State == RunStateClosed || r.State == RunStateEscalated
	   return !terminal && !now.Before(r.SLADeadline)
  }

// Active 是否仍在处置中（非终态）。

   func (r *FlowRun) Active() bool {
	  if r == nil {
		  return false
	   }
	  return r.State != RunStateClosed && r.State != RunStateEscalated
  }

// ──────────────────────────────────────────────
// FlowStep：流程步骤记录
// ──────────────────────────────────────────────
// StepState 步骤状态枚举。

type StepState string

const (
	StepStatePending StepState = "pending"
	StepStateRunning StepState = "running"
	StepStateDone    StepState = "done"
	StepStateSkipped StepState = "skipped"
	StepStateGated  StepState = "gated" // 等待人工放行
	StepStateFailed StepState = "failed"
)

// FlowStep 流程步骤执行记录。

type FlowStep struct {
	ID          string `json:"id"`
	RunID       string `json:"run_id"`
	Seq         int    `json:"seq"` // 顺序号（0-based）
	ActionDefID string `json:"action_def_id"`
	ActionKind string `json:"action_kind"` // 冗余，方便读面
	State StepState `json:"state"`
	Actor string    `json:"actor"`
	Input string    `json:"input"` // JSON 输入快照
	Output string `json:"output"` // JSON 输出快照
	ErrMsg     string     `json:"err_msg"`
	GatedBy    string     `json:"gated_by"`
	GatedAt    *time.Time `json:"gated_at,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ──────────────────────────────────────────────
// 运行查询条件
// ──────────────────────────────────────────────
// RunQuery FlowRun 查询条件；空字段表示不过滤。
type RunQuery struct {
	UserID   string
	Asset    string
	DiffKind string
	State    RunState
	TicketID string
	Breached *bool
	Limit int
	Offset int
}

// StepInput 单步执行输入。
type StepInput struct {
	RunID       string
	Seq         int
	ActionDefID string
	Actor       string
	Params map[string]string
}

// GateDecision 人工放行决定。
type GateDecision struct {
	RunID  string `json:"run_id"`
	StepID string `json:"step_id"`
	Actor string `json:"actor"`
	Allow bool   `json:"allow"` // true=放行，false=拒绝
	Note string `json:"note"`
}

// ClassifyRequest 分类请求（支持 dry-run 模式）。
type ClassifyRequest struct {
	Feature DiffFeature `json:"feature"`
	DryRun  bool        `json:"dry_run"` // dry-run=true 时不创建 FlowRun
}

// ClassifyResult 分类与（可选）流程启动结果。

type ClassifyResult struct {
	Class DiffClass `json:"class"`
	RunID string    `json:"run_id,omitempty"` // dry-run 时为空
	DryRun bool `json:"dry_run"`
}
