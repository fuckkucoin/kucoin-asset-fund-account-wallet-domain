package cerbind

import (
	"time"
)

type State string

const (
	StPlanned State = "planned"
	StOpen State = "ceremony_open"
	StQuorum  State = "quorum_met"
	StBound   State = "bound"
	StReady   State = "sign_ready"
	StClosed  State = "closed"
	StAborted State = "aborted"
)

  func (s State) Terminal() bool { return s == StClosed || s == StAborted }

  func (s State) Live() bool { return s != "" && !s.Terminal() }

const (
	ModeOne  = "one"
	ModeMany = "many"
)

const (
	BindActive = "active"
	BindReleased = "released"
	BindConsumed = "consumed"
	BindStale = "stale"
)

const (
	SideCold = "cold"
	SideWarm = "warm"
	SideHot  = "hot"
	SideAny  = "any"
)

const (
	RiskAny    = "any"
	RiskLow    = "low"
	RiskMedium = "medium"
	RiskHigh = "high"
	RiskCritical = "critical"
)

const (
	PurposeWithdraw = "withdraw"
	PurposeAny      = "any"
)

// Policy 按资产、金额、链、目的地风险和库存侧决定是否强制冷签。
// LimitRef 只保存外部限额结论的引用，本包不解释限额公式。
type Policy struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Asset string `json:"asset"`
	Chain     string `json:"chain"`
	Purpose   string `json:"purpose"`
	MinAmount string `json:"min_amount"`
	MaxAmount string `json:"max_amount,omitempty"`
	DestRisk string `json:"dest_risk"`
	Side      string `json:"side"`
	ForceCold bool   `json:"force_cold"`
	Threshold int    `json:"threshold"`
	Shares int `json:"shares"`
	BindMode string `json:"bind_mode"`
	Priority int    `json:"priority"`
	Enabled bool `json:"enabled"`
	ApprovalNeed bool `json:"approval_need"`
	MaxAgeSec int `json:"max_age_sec,omitempty"`
	LimitRef string `json:"limit_ref,omitempty"`
	UpdatedBy string    `json:"updated_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

  func (p Policy) Copy() Policy { return p }

// Line 是批次里的一条出金事实，供策略求值和摘要。
type Line struct {
	Asset string `json:"asset"`
	Chain  string `json:"chain"`
	Amount string `json:"amount"`
	Dest   string `json:"dest"`
	DestRisk string `json:"dest_risk"`
	Side string `json:"side"`
	Purpose string `json:"purpose"`
	ItemRefs []string `json:"item_refs,omitempty"`
	Approved bool     `json:"approved"`
	LimitHit bool     `json:"limit_hit"`
	LimitRef string   `json:"limit_ref,omitempty"`
}

   func (l Line) Copy() Line {
	   l.ItemRefs = append([]string(nil), l.ItemRefs...)
	  return l
   }

// Snapshot 是出金批次的绑定事实。摘要只覆盖 Lines，别名索引另存。

type Snapshot struct {
	BatchID string `json:"batch_id"`
	Lines []Line `json:"lines"`
	Digest string    `json:"digest"`
	At     time.Time `json:"at"`
}

  func (s Snapshot) Copy() Snapshot {
	   out := s
	   out.Lines = make([]Line, len(s.Lines))
	  for i := range s.Lines {
		  out.Lines[i] = s.Lines[i].Copy()
	  }
	  return out
   }

// Session 是一次冷签仪式。阈值在开立时冻结，后改策略不降低已开启会话的门槛。

type Session struct {
	ID       string `json:"id"`
	PolicyID string `json:"policy_id"`
	State    State  `json:"state"`
	Asset string `json:"asset"`
	Chain string `json:"chain"`
	Amount   string `json:"amount"`
	Dest     string `json:"dest"`
	DestRisk string `json:"dest_risk"`
	Side    string `json:"side"`
	Purpose string `json:"purpose"`
	BatchID string `json:"batch_id"`
	Threshold int    `json:"threshold"`
	Shares    int    `json:"shares"`
	BindMode  string `json:"bind_mode"`
	ReceiptN     int    `json:"receipt_n"`
	Approved     bool   `json:"approved"`
	AirgapID     string `json:"airgap_id,omitempty"`
	KeycerTicket string `json:"keycer_ticket,omitempty"`
	CustodyID    string `json:"custody_id,omitempty"`
	Digest string `json:"digest"`
	Starter     string    `json:"starter"`
	LastActor   string    `json:"last_actor"`
	Deadline    time.Time `json:"deadline"`
	AbortReason string    `json:"abort_reason,omitempty"`
	Note string `json:"note,omitempty"`
	FenceEpoch int64  `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
	AlertedAt time.Time `json:"alerted_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

  func (s Session) Copy() Session { return s }

  func (s Session) FenceMatch(epoch int64, token string) bool {
	   return s.FenceEpoch == epoch && s.FenceToken != "" && s.FenceToken == token
   }

// Binding 把一个出金批次接到一个仪式会话。同一批次同时只有一条 active。

type Binding struct {
	ID      string `json:"id"`
	BatchID string `json:"batch_id"`
	SessionID string `json:"session_id"`
	Digest   string `json:"digest"`
	State    string `json:"state"`
	BindMode string `json:"bind_mode"`
	Actor    string `json:"actor"`
	BoundAt    time.Time `json:"bound_at"`
	ReleasedAt time.Time `json:"released_at,omitempty"`
}

   func (b Binding) Copy() Binding { return b }

// Receipt 是一名参与者提交的份额回执。ShareRef 是指纹，不是份额本身。

type Receipt struct {
	ID string `json:"id"`
	SessionID string `json:"session_id"`
	Actor string `json:"actor"`
	ShareRef  string    `json:"share_ref"`
	SlotLabel string    `json:"slot_label"`
	Note      string    `json:"note,omitempty"`
	At        time.Time `json:"at"`
}

   func (r Receipt) Copy() Receipt { return r }

// Audit 记录闸门拒绝、状态迁移和巡检动作。

type Audit struct {
	ID string `json:"id"`
	SessionID string `json:"session_id,omitempty"`
	BatchID   string `json:"batch_id,omitempty"`
	Kind string `json:"kind"`
	Actor string `json:"actor,omitempty"`
	Detail string `json:"detail,omitempty"`
	At time.Time `json:"at"`
}

  func (a Audit) Copy() Audit { return a }

// Decision 是一次策略求值的结果。Trace 按策略逐条说明命中或跳过的原因。

type Decision struct {
	Required bool `json:"required"`
	PolicyID string `json:"policy_id,omitempty"`
	Threshold int    `json:"threshold"`
	Shares    int    `json:"shares"`
	BindMode  string `json:"bind_mode,omitempty"`
	Asset    string `json:"asset,omitempty"`
	Chain    string `json:"chain,omitempty"`
	Amount   string `json:"amount,omitempty"`
	Dest     string `json:"dest,omitempty"`
	DestRisk string `json:"dest_risk,omitempty"`
	Side     string `json:"side,omitempty"`
	Purpose  string `json:"purpose,omitempty"`
	Approved bool   `json:"approved"`
	Reason   string `json:"reason,omitempty"`
	Trace []string `json:"trace,omitempty"`
}

   func (d Decision) Copy() Decision {
	  d.Trace = append([]string(nil), d.Trace...)
	   return d
  }

// Subject 是签播闸的查询条件。批次号、单据号、幂等键三者都能定位到快照。
type Subject struct {
	BatchID string `json:"batch_id"`
	MovementID string   `json:"movement_id"`
	Idem       string   `json:"idem"`
	ItemRefs   []string `json:"item_refs,omitempty"`
	Asset  string `json:"asset"`
	Chain  string `json:"chain"`
	Amount string `json:"amount"`
	Dest   string `json:"dest"`
	DestRisk string `json:"dest_risk"`
	Side    string `json:"side"`
	Purpose string `json:"purpose"`
	Approved bool `json:"approved"`
	LimitHit bool `json:"limit_hit"`
	LimitRef string `json:"limit_ref,omitempty"`
}

// PlanReq 开立仪式。Lines 为空时用单笔字段组装快照。
type PlanReq struct {
	Actor string `json:"actor"`
	BatchID string `json:"batch_id"`
	Lines []Line `json:"lines,omitempty"`
	Asset string `json:"asset"`
	Chain string `json:"chain"`
	Amount   string `json:"amount"`
	Dest     string `json:"dest"`
	DestRisk string `json:"dest_risk"`
	Side    string `json:"side"`
	Purpose string `json:"purpose"`
	Approved bool   `json:"approved"`
	LimitHit bool   `json:"limit_hit"`
	LimitRef string `json:"limit_ref,omitempty"`
	ItemRefs []string `json:"item_refs,omitempty"`
}

// View 是席位读模型：会话、回执、绑定和最近审计。
type View struct {
	Session Session `json:"session"`
	Receipts []Receipt `json:"receipts"`
	Bindings []Binding `json:"bindings"`
	Audits   []Audit   `json:"audits"`
}

// TickReport 是一轮巡检的计数。
type TickReport struct {
	TimedOut int `json:"timed_out"`
	Stuck    int `json:"stuck"`
	Released int `json:"released"`
	Demoted int `json:"demoted"`
	Fixed   int `json:"fixed"`
}
