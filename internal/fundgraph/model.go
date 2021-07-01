package fundgraph

import "time"

// EdgeType 是冻结的因果边类型。

type EdgeType string

// NodeKind 是冻结的资金事实节点种类。

type NodeKind string

// FundNode 是图上的一个资金事实。ID 由种类和业务引用稳定导出，重复摄取命中同一行。

type FundNode struct {
	ID string `json:"id"`
	Kind NodeKind `json:"kind"`
	Ref    string `json:"ref"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Chain  string `json:"chain"`
	TxHash string `json:"tx_hash"`
	Source     string    `json:"source"`
	OccurredAt time.Time `json:"occurred_at"`
	IngestedAt time.Time `json:"ingested_at"`
	Attrs map[string]string `json:"attrs,omitempty"`
}

// FundEdge 是一条有向因果边。幂等键是 (from,to,edge_type,ref)。
type FundEdge struct {
	ID     string `json:"id"`
	FromID string `json:"from_id"`
	ToID string `json:"to_id"`
	EdgeType EdgeType `json:"edge_type"`
	Ref      string   `json:"ref"`
	UserID   string   `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Source     string    `json:"source"`
	EnvelopeID string    `json:"envelope_id,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
	IngestedAt time.Time `json:"ingested_at"`
	Attrs map[string]string `json:"attrs,omitempty"`
}

// EdgeKey 是幂等边键。
type EdgeKey struct {
	From string
	To string
	Type EdgeType
	Ref string
}

// 记录某个摄取源的消费位置和滞后。
type Cursor struct {
	Source string `json:"source"`
	Stream string `json:"stream"`
	Token       string    `json:"token"`
	LastEventAt time.Time `json:"last_event_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	LagMs int64 `json:"lag_ms"`
}

// IngestBatch 是一次摄取提交的节点、边和游标推进。
type IngestBatch struct {
	Source string `json:"source"`
	Stream  string     `json:"stream"`
	Cursor  string     `json:"cursor"`
	EventAt time.Time  `json:"event_at"`
	Nodes   []FundNode `json:"nodes"`
	Edges   []FundEdge `json:"edges"`
}

// Conflict 描述一次失败即关的属性冲突。存储中的边不会被覆盖。
type Conflict struct {
	EdgeKey string `json:"edge_key,omitempty"`
	NodeID  string `json:"node_id,omitempty"`
	Field   string `json:"field"`
	Stored string `json:"stored"`
	Incoming string `json:"incoming"`
}

// IngestResult 是一次摄取的计数。出现冲突时插入数为 0，整批回滚。
type IngestResult struct {
	InsertedNodes int `json:"inserted_nodes"`
	InsertedEdges int        `json:"inserted_edges"`
	ReplayNodes   int        `json:"replay_nodes"`
	ReplayEdges   int        `json:"replay_edges"`
	Conflicts     []Conflict `json:"conflicts,omitempty"`
	Audited       bool       `json:"audited"`
	Cursor Cursor `json:"cursor,omitempty"`
}

// AuditRecord 是本包自己的冲突审计行，不是外部只追加仓的条目。
type AuditRecord struct {
	ID     string `json:"id"`
	Action string `json:"action"`
	EdgeKey string `json:"edge_key,omitempty"`
	NodeID  string `json:"node_id,omitempty"`
	Detail string    `json:"detail"`
	At     time.Time `json:"at"`
}

const ActionConflict = "conflict"

// LineageQuery 是血缘查询。Direction 取 forward、reverse、shortest。

type LineageQuery struct {
	StartNodeID string   `json:"start_node_id"`
	StartRef    string   `json:"start_ref"`
	StartKind   NodeKind `json:"start_kind"`
	UserID string    `json:"user_id"`
	Asset  string    `json:"asset"`
	From   time.Time `json:"from"`
	To time.Time `json:"to"`
	Direction string `json:"direction"`
	TargetNodeID string `json:"target_node_id"`
	TargetKind NodeKind `json:"target_kind"`
	TargetRef string     `json:"target_ref"`
	EdgeTypes []EdgeType `json:"edge_types,omitempty"`
	MaxHops   int        `json:"max_hops"`
	Limit     int        `json:"limit"`
	PageToken string     `json:"page_token"`
}

const (
	DirForward = "forward"
	DirReverse = "reverse"
	DirShortest = "shortest"
)

// GapMarker 是查询诊断缺口。调用方据此区分「没有上游」和「图断了」，而不是拿到空指针。

type GapMarker struct {
	Code     string `json:"code"`
	AtNodeID string `json:"at_node_id,omitempty"`
	EdgeType string `json:"edge_type,omitempty"`
	EdgeID   string `json:"edge_id,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

const (
	GapMissingEdge = "missing_edge"
	GapDangling = "dangling_endpoint"
	GapCycleCut = "cycle_cut"
	GapHopLimit = "hop_limit"
	GapStartMissing    = "start_missing"
	GapTargetUnreached = "target_unreached"
	GapNoAnchor        = "no_anchor"
	GapClamped         = "clamped"
)

// LineagePath 是从锚点走到叶子的一条路径。反向查询时节点顺序是锚点到根因。
type LineagePath struct {
	Nodes []FundNode `json:"nodes"`
	Edges []FundEdge `json:"edges"`
	Hops      int         `json:"hops"`
	Gaps      []GapMarker `json:"gaps,omitempty"`
	Complete  bool        `json:"complete"`
	Direction string      `json:"direction"`
}

// LineageResult 是一次查询的影响面、主路径和缺口。
type LineageResult struct {
	Direction string        `json:"direction"`
	AnchorID  string        `json:"anchor_id,omitempty"`
	Paths     []LineagePath `json:"paths"`
	Reached []FundNode `json:"reached"`
	Edges []FundEdge `json:"edges"`
	Gaps      []GapMarker `json:"gaps"`
	Truncated bool        `json:"truncated"`
	Clamped   bool        `json:"clamped"`
	HopsUsed int `json:"hops_used"`
	Complete bool `json:"complete"`
	NextPageToken string `json:"next_page_token,omitempty"`
}

// GraphIncident 是巡检开出的图工单。开单不回写源状态机。

type GraphIncident struct {
	ID string `json:"id"`
	Kind string `json:"kind"`
	Status string `json:"status"`
	Reason  string `json:"reason"`
	EdgeKey string `json:"edge_key,omitempty"`
	NodeID string `json:"node_id,omitempty"`
	Ref       string    `json:"ref,omitempty"`
	Detail    string    `json:"detail"`
	OpenedAt  time.Time `json:"opened_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const (
	IncidentBroken      = "broken_link"
	IncidentCycle       = "cycle"
	IncidentMultiParent = "multi_parent"
	IncidentLag = "ingest_lag"
	StatusOpen = "open"
	StatusAck  = "acked"
	StatusClosed = "closed"
	ReasonDanglingFrom = "dangling_from"
	ReasonDanglingTo   = "dangling_to"
	ReasonMissingStage = "missing_stage"
	ReasonMissingLink = "missing_link"
	ReasonCycle       = "cycle"
	ReasonMultiParent = "multi_parent"
	ReasonCursorStall = "cursor_stall"
	ReasonEventLag    = "event_lag"
)

// InspectReport 是一轮完整性巡检的结果。
type InspectReport struct {
	At time.Time `json:"at"`
	Opened      []GraphIncident `json:"opened"`
	Broken      int             `json:"broken"`
	Cycles      int             `json:"cycles"`
	MultiParent int             `json:"multi_parent"`
	Lag         int             `json:"lag"`
	Dangling   int `json:"dangling"`
	Candidates int `json:"candidates"`
}

// NodeFilter 过滤节点列表。

type NodeFilter struct {
	UserID string
	Asset  string
	Ref    string
	Kind   NodeKind
	From time.Time
	To time.Time
	Limit int
}

// EdgeFilter 过滤边列表。

type EdgeFilter struct {
	Ref      string
	FromID   string
	ToID     string
	EdgeType EdgeType
	UserID string
	Asset string
	Limit int
}

// IncidentFilter 过滤巡检单。

type IncidentFilter struct {
	Status string
	Kind   string
	Ref string
	Limit int
}

// Snapshot 是巡检用的只读图副本。
type Snapshot struct {
	Nodes   []FundNode
	Edges   []FundEdge
	Cursors []Cursor
	Truncated bool
}

// Metrics 是摄取滞后、边基数、断链数和最近一次查询耗时。

type Metrics struct {
	Nodes int `json:"nodes"`
	Edges int `json:"edges"`
	OpenIncidents int `json:"open_incidents"`
	BrokenOpen    int `json:"broken_open"`
	CycleOpen     int `json:"cycle_open"`
	MultiParentOpen int `json:"multi_parent_open"`
	LagOpen         int `json:"lag_open"`
	IngestLagMs int64 `json:"ingest_lag_ms"`
	Conflicts int64 `json:"conflicts"`
	Ingests     int64 `json:"ingests"`
	Replays     int64 `json:"replays"`
	Queries     int64 `json:"queries"`
	LastQueryMs int64 `json:"last_query_ms"`
}

// Corr 返回节点所属的业务关联号。attrs.corr 先于节点自身 ref，便于多跳共享一条主链。
  func (n FundNode) Corr() string {
	   if n.Attrs != nil {
		   if v := n.Attrs["corr"]; v != "" {
			  return v
		  }
	   }
	   return n.Ref
  }

// Key 返回边的幂等键。
  func (e FundEdge) Key() EdgeKey {
	  return EdgeKey{From: e.FromID, To: e.ToID, Type: e.EdgeType, Ref: e.Ref}
  }
