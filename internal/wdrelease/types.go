package wdrelease

import "time"

const (
	StateDraft = "draft"
	StatePending  = "pending"
	StatePartial  = "partially_approved"
	StateApproved = "approved"
	StateRejected = "rejected"
	StateExpired  = "expired"
)

const (
	SegPlanned    = "planned"
	SegAuthorized = "authorized"
	SegReserved  = "reserved"
	SegFrozen    = "frozen"
	SegCancelled = "cancelled"
)

const (
	TimeoutNone = "none"
	TimeoutEscalate = "escalate"
	TimeoutReject   = "reject"
)

const (
	StepApprove = "approve"
	StepReject  = "reject"
	StepSubmit   = "submit"
	StepEscalate = "escalate"
	StepExpire   = "expire"
	StepOpen   = "open"
	StepFreeze = "freeze"
	StepAbort  = "abort"
)

const (
	PhaseReserved = "reserved"
	PhaseFrozen   = "frozen"
	PhaseAborted  = "aborted"
)

const (
	ClassExternal = "external"
	ClassInternal = "internal"
	ClassNew      = "new"
	ClassBlocked  = "blocked"
	ClassSanction = "sanction"
)

// Ticket 一笔出金的放行单。金额用十进制字符串，避免二进制浮点。
type Ticket struct {
	ID string `json:"id"`
	Idem string `json:"idempotency_key"`
	FP   string `json:"fingerprint"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Chain  string `json:"chain"`
	Dest      string `json:"dest"`
	DestClass string `json:"dest_class"`
	Risk      int    `json:"risk"`
	PolicyVer string `json:"policy_version"`
	State     string `json:"state"`
	Depth     int    `json:"depth"`
	BaseDepth int    `json:"base_depth"`
	Approvals int    `json:"approvals"`
	Skip      bool   `json:"skip"`
	Timeout   string `json:"timeout_action"`
	SLASec    int64  `json:"sla_sec"`
	Deadline time.Time `json:"deadline,omitempty"`
	Applicant string `json:"applicant"`
	Code      string `json:"code,omitempty"`
	Note      string `json:"note,omitempty"`
	Authorized string `json:"authorized"`
	Frozen string `json:"frozen"`
	Residual string `json:"residual"`
	Segments []Segment `json:"segments"`
	Steps      []Step    `json:"steps"`
	Escalated  int       `json:"escalated"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	FenceEpoch int64     `json:"-"`
	FenceToken string `json:"-"`
}

// Segment 金额段。Need 是授权所需的最低审批级数；最后一段还要凑满当前深度。
type Segment struct {
	Seq int `json:"seq"`
	Bps int `json:"bps"`
	Amount string `json:"amount"`
	Cap    string `json:"cap,omitempty"`
	Need int `json:"need_level"`
	State string `json:"state"`
	By    string `json:"authorized_by,omitempty"`
	At time.Time `json:"at,omitempty"`
}

// Step 审批链上的一步。同一复核人不能占两级 approve。
type Step struct {
	Seq int `json:"seq"`
	Kind string `json:"kind"`
	Level int    `json:"level,omitempty"`
	Actor string `json:"actor"`
	Role string `json:"role,omitempty"`
	Code string    `json:"code,omitempty"`
	Note string    `json:"note,omitempty"`
	At   time.Time `json:"at"`
}

// Grant 冻账前占用的额度。Abort 只能退回仍处 reserved 的段。

type Grant struct {
	Idem string `json:"idempotency_key"`
	TicketID string `json:"ticket_id"`
	Amount string `json:"amount"`
	Seqs   []int  `json:"seqs"`
	Phase  string `json:"phase"`
}

// Input 策略评估与开单入参。
type Input struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Chain  string `json:"chain"`
	Dest   string `json:"dest"`
	DestClass string `json:"dest_class"`
	Risk int `json:"risk"`
	Applicant string `json:"applicant"`
	Idem string `json:"idem"`
	Version string `json:"version,omitempty"`
}

// Decision 表驱动评估结果。Deny 为真时不得开出可冻结票据。

type Decision struct {
	Version   string `json:"policy_version"`
	Depth     int    `json:"depth"`
	BaseDepth int    `json:"base_depth"`
	Skip      bool   `json:"skip"`
	Ratios  []int  `json:"ratios_bps"`
	SLASec  int64  `json:"sla_sec"`
	Timeout string `json:"timeout_action"`
	SegmentCap string `json:"segment_cap,omitempty"`
	DailyCap string `json:"daily_cap,omitempty"`
	DestCap string `json:"dest_cap,omitempty"`
	Deny    bool     `json:"deny"`
	Code    string   `json:"code,omitempty"`
	Reason  string   `json:"reason,omitempty"`
	Drift   bool     `json:"drift"`
	Amounts []string `json:"segment_amounts,omitempty"`
}

// Audit 追加式审计环。Prev 指向同票据上一条哈希。

type Audit struct {
	TicketID string `json:"ticket_id"`
	Seq      int    `json:"seq"`
	Event string `json:"event"`
	State string `json:"state"`
	Actor string    `json:"actor"`
	Code  string    `json:"code,omitempty"`
	Prev  string    `json:"prev"`
	Hash  string    `json:"hash"`
	At    time.Time `json:"at"`
}

  func openStates(st string) bool {
	   switch st {
	  case StateDraft, StatePending, StatePartial:
		   return true
	   default:
		  return false
	  }
  }

  func terminal(st string) bool {
	  switch st {
	   case StateApproved,
		  StateRejected, StateExpired:
		  return true
	   default:
		  return false
	   }
  }
