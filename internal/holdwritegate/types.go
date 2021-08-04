package holdwritegate

import "time"

// ──────────────────────────────────────────
// HoldBlockLease 冻结写路径 block_gen 租约
// ──────────────────────────────────────────

type BlockLeaseState string

const (
	BlockLeaseOpen     BlockLeaseState = "open"
	BlockLeaseBlocking BlockLeaseState = "blocking"
	BlockLeaseCleared  BlockLeaseState = "cleared"
	BlockLeaseExpired BlockLeaseState = "expired"
)

const (
	BlockEvActivate = "activate"
	BlockEvClear  = "clear"
	BlockEvExpire = "expire"
)

type HoldBlockLease struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	Asset    string `json:"asset"`
	Book     string `json:"book"`
	AdviceID string `json:"advice_id"`
	BlockGen int64  `json:"block_gen"`
	State BlockLeaseState `json:"state"`
	Reason      string    `json:"reason"`
	GateCount   int       `json:"gate_count"`
	LeaseExpiry time.Time `json:"lease_expiry"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ActivatedAt *time.Time `json:"activated_at,omitempty"`
	ClearedAt    *time.Time `json:"cleared_at,omitempty"`
	ExpiredAt    *time.Time `json:"expired_at,omitempty"`
	ClearProofID string     `json:"clear_proof_id,omitempty"`
	ClearedBy string `json:"cleared_by,omitempty"`
}

// ──────────────────────────────────────────
// AdviceEscalation 建议升级工单
// ──────────────────────────────────────────

type EscalationState string

const (
	EscalationOpen   EscalationState = "open"
	EscalationProven EscalationState = "proven"
	EscalationClosed EscalationState = "closed"
)

const (
	EscalationEvProve = "prove"
	EscalationEvClose = "close"
)

type EscalationKind string

const (
	EscalationDriftAdvice EscalationKind = "drift_advice"
	EscalationHoldGap     EscalationKind = "hold_gap"
	EscalationOrphanTag   EscalationKind = "orphan_tag"
	EscalationManualBlock EscalationKind = "manual_block"
	EscalationTimeoutReopen EscalationKind = "timeout_reopen"
)

type AdviceEscalation struct {
	ID      string `json:"id"`
	LeaseID string `json:"lease_id"`
	UserID  string `json:"user_id"`
	Asset   string `json:"asset"`
	Book     string          `json:"book"`
	AdviceID string          `json:"advice_id"`
	Kind     EscalationKind  `json:"kind"`
	State    EscalationState `json:"state"`
	BlockGen int64           `json:"block_gen"`
	Detail     string   `json:"detail"`
	ProofIDs   []string `json:"proof_ids,omitempty"`
	Resolution string   `json:"resolution,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ProvenAt  *time.Time `json:"proven_at,omitempty"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`
}

// ──────────────────────────────────────────
// GapCloseProof 间隙关闭证明
// ──────────────────────────────────────────

type GapCloseProof struct {
	ID string `json:"id"`
	LeaseID      string `json:"lease_id"`
	EscalationID string `json:"escalation_id"`
	UserID       string `json:"user_id"`
	Asset    string `json:"asset"`
	Book     string `json:"book"`
	AdviceID string `json:"advice_id"`
	BlockGen int64  `json:"block_gen"`
	GapBefore string `json:"gap_before"`
	GapAfter  string `json:"gap_after"`
	GapClosed bool   `json:"gap_closed"`
	Validated   bool       `json:"validated"`
	ValidatedAt *time.Time `json:"validated_at,omitempty"`
	ValidatedBy string `json:"validated_by,omitempty"`
	Sealed    bool       `json:"sealed"`
	SealedAt  *time.Time `json:"sealed_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// ──────────────────────────────────────────
// WriteGateEntry 被拦截的写操作记录
// ──────────────────────────────────────────

type WriteGateEntry struct {
	ID      string `json:"id"`
	LeaseID string `json:"lease_id"`
	UserID    string `json:"user_id"`
	Asset     string `json:"asset"`
	Book      string `json:"book"`
	WriteID   string `json:"write_id"`
	WriteKind string `json:"write_kind"`
	BlockGen int64 `json:"block_gen"`
	Rejected  bool      `json:"rejected"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ──────────────────────────────────────────
// OrphanTagBind 孤儿标签绑定记录
// ──────────────────────────────────────────

type OrphanTagBind struct {
	ID        string `json:"id"`
	LeaseID   string `json:"lease_id"`
	HoldcatID string `json:"holdcat_id"`
	UserID    string `json:"user_id"`
	Asset     string `json:"asset"`
	Book     string `json:"book"`
	BlockGen int64  `json:"block_gen"`
	BoundAt time.Time `json:"bound_at"`
}

// ──────────────────────────────────────────
// Request / Response
// ──────────────────────────────────────────

type EscalateAdviceReq struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Book     string `json:"book"`
	AdviceID string `json:"advice_id"`
	Reason string         `json:"reason"`
	Kind   EscalationKind `json:"kind"`
	Detail string         `json:"detail,omitempty"`
}

type WriteGateCheckReq struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Book      string `json:"book"`
	WriteID   string `json:"write_id"`
	WriteKind string `json:"write_kind"`
}

type WriteGateCheckResp struct {
	Allowed bool `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
	LeaseID string `json:"lease_id,omitempty"`
	BlockGen int64 `json:"block_gen,omitempty"`
}

type SubmitCloseProofReq struct {
	LeaseID      string `json:"lease_id"`
	EscalationID string `json:"escalation_id"`
	GapBefore string `json:"gap_before"`
	GapAfter  string `json:"gap_after"`
	GapClosed bool   `json:"gap_closed"`
}

type DualControlClearReq struct {
	LeaseID string `json:"lease_id"`
	ProofID string `json:"proof_id"`
	ApproverA string `json:"approver_a"`
	ApproverB string `json:"approver_b"`
	Resolution string `json:"resolution,omitempty"`
}

type ValidateProofReq struct {
	ProofID     string `json:"proof_id"`
	ValidatedBy string `json:"validated_by"`
}

type BindOrphanTagReq struct {
	LeaseID   string `json:"lease_id"`
	HoldcatID string `json:"holdcat_id"`
	UserID    string `json:"user_id"`
	Asset string `json:"asset"`
	Book string `json:"book"`
}

type BlockQueryReq struct {
	UserID string `json:"user_id,omitempty"`
	Asset  string `json:"asset,omitempty"`
	Book   string `json:"book,omitempty"`
}

type ProofExportReq struct {
	LeaseID string `json:"lease_id,omitempty"`
	UserID string `json:"user_id,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type EscalationQueryReq struct {
	LeaseID string `json:"lease_id,omitempty"`
	UserID  string `json:"user_id,omitempty"`
	State   string `json:"state,omitempty"`
	Limit int `json:"limit,omitempty"`
}

type BlockGenResp struct {
	UserID string `json:"user_id"`
	Asset      string `json:"asset"`
	Book       string `json:"book"`
	CurrentGen int64  `json:"current_gen"`
	ActiveLease string          `json:"active_lease_id,omitempty"`
	State       BlockLeaseState `json:"state,omitempty"`
}
