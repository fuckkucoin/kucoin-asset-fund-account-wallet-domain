package loanrace

import "time"

// ──────────────────────────────────────────────────
// 贷款竞态状态
// ──────────────────────────────────────────────────

type LoanRaceState string

const (
	LRActive LoanRaceState = "active"
	LROverdue LoanRaceState = "overdue"
	LRFrozen   LoanRaceState = "frozen"
	LRRepaying LoanRaceState = "repaying"
	LRWrittenOff LoanRaceState = "written_off"
	LRClosed     LoanRaceState = "closed"
)

// ──────────────────────────────────────────────────
// 冻结租约状态
// ──────────────────────────────────────────────────

type LeaseState string

const (
	LeaseAcquired LeaseState = "acquired"
	LeaseApplied  LeaseState = "applied"
	LeaseReleased LeaseState = "released"
	LeaseExpired LeaseState = "expired"
)

// ──────────────────────────────────────────────────
// PayGate 状态
// ──────────────────────────────────────────────────

type GateState string

const (
	GateOpen    GateState = "open"
	GateCleared GateState = "cleared"
	GateClosed  GateState = "closed"
	GateBlocked GateState = "blocked"
)

// ──────────────────────────────────────────────────
// 核销状态
// ──────────────────────────────────────────────────

type WriteoffState string

const (
	WOPending  WriteoffState = "pending"
	WOApproved WriteoffState = "approved"
	WOApplied  WriteoffState = "applied"
	WORejected WriteoffState = "rejected"
)

// ──────────────────────────────────────────────────
// 解冻证明状态
// ──────────────────────────────────────────────────

type ProofState string

const (
	ProofPending ProofState = "pending"
	ProofValid    ProofState = "valid"
	ProofRejected ProofState = "rejected"
)

// ──────────────────────────────────────────────────
// OverdueLease — 逾期冻结租约
// ──────────────────────────────────────────────────

type OverdueLease struct {
	ID string `json:"id"`
	LoanID     string `json:"loan_id"`
	UserID     string `json:"user_id"`
	Generation int64  `json:"generation"`
	State LeaseState `json:"state"`
	Holder     string    `json:"holder"`
	AcquiredAt time.Time `json:"acquired_at"`
	AppliedAt  time.Time `json:"applied_at,omitempty"`
	ReleasedAt time.Time `json:"released_at,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
	Reason string `json:"reason,omitempty"`
}

// ──────────────────────────────────────────────────
// PayGate — 还款互斥门
// ──────────────────────────────────────────────────

type PayGate struct {
	ID string `json:"id"`
	LoanID string `json:"loan_id"`
	UserID string `json:"user_id"`
	LeaseID string `json:"lease_id"`
	State      GateState `json:"state"`
	AllowedAmt string    `json:"allowed_amt"`
	PaidAmt   string    `json:"paid_amt"`
	CreatedAt time.Time `json:"created_at"`
	ClearedAt time.Time `json:"cleared_at,omitempty"`
	ClosedAt  time.Time `json:"closed_at,omitempty"`
	Actor string `json:"actor"`
}

// ──────────────────────────────────────────────────
// WriteoffCase — 坏账核销工单
// ──────────────────────────────────────────────────

type WriteoffCase struct {
	ID string `json:"id"`
	LoanID string `json:"loan_id"`
	UserID string `json:"user_id"`
	Principal   string `json:"principal"`
	Interest    string `json:"interest"`
	SuspenseAmt string `json:"suspense_amt"`
	State WriteoffState `json:"state"`
	ApplicantActor string `json:"applicant_actor"`
	ApproverActor  string `json:"approver_actor,omitempty"`
	Reason string `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
	ApprovedAt time.Time `json:"approved_at,omitempty"`
	AppliedAt time.Time `json:"applied_at,omitempty"`
	RejectedAt time.Time `json:"rejected_at,omitempty"`
}

// ──────────────────────────────────────────────────
// UnfreezeProof — 解冻证明
// ──────────────────────────────────────────────────

type UnfreezeProof struct {
	ID     string `json:"id"`
	LoanID string `json:"loan_id"`
	UserID string `json:"user_id"`
	Kind   string `json:"kind"`
	EvidenceRef string `json:"evidence_ref"`
	EvidenceHash string     `json:"evidence_hash"`
	State        ProofState `json:"state"`
	SubmittedBy  string     `json:"submitted_by"`
	ValidatedBy  string     `json:"validated_by,omitempty"`
	SubmittedAt time.Time `json:"submitted_at"`
	ValidatedAt time.Time `json:"validated_at,omitempty"`
	Note        string    `json:"note,omitempty"`
}

// ──────────────────────────────────────────────────
// 呆滞科目账户
// ──────────────────────────────────────────────────

type SuspenseEntry struct {
	ID     string `json:"id"`
	LoanID string `json:"loan_id"`
	UserID string `json:"user_id"`
	Amount string `json:"amount"`
	Kind      string    `json:"kind"`
	Ref       string    `json:"ref"`
	CreatedAt time.Time `json:"created_at"`
}

// ──────────────────────────────────────────────────
// 贷款竞态视图（聚合状态）
// ──────────────────────────────────────────────────

type LoanRaceView struct {
	LoanID string `json:"loan_id"`
	UserID string `json:"user_id"`
	RaceState  LoanRaceState `json:"race_state"`
	Frozen     bool          `json:"frozen"`
	HasLease   bool          `json:"has_lease"`
	HasGate    bool          `json:"has_gate"`
	WrittenOff bool          `json:"written_off"`
	HasProof bool `json:"has_proof"`
}

  func (l OverdueLease) Copy() OverdueLease { return l }
  func (g PayGate) Copy() PayGate           { return g }
   func (w WriteoffCase) Copy() WriteoffCase { return w }

   func (p UnfreezeProof) Copy() UnfreezeProof { return p }
   func (e SuspenseEntry) Copy() SuspenseEntry { return e }
