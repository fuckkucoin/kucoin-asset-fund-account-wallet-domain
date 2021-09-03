package memoeffectgate

import "time"

// EffectState bound → credited | rejected_blocked | expired。

type EffectState string

const (
	EffectBound EffectState = "bound"
	EffectCredited        EffectState = "credited"
	EffectRejectedBlocked EffectState = "rejected_blocked"
	EffectExpired     EffectState = "expired"
	EffectQuarantined EffectState = "quarantined"
)

const (
	EffectEvBind   = "bind"
	EffectEvCredit = "credit"
	EffectEvReject = "reject_block"
	EffectEvExpire = "expire"
	EffectEvQuarantine = "quarantine"
)

// HalfState open → reversed | acked。
type HalfState string

const (
	HalfOpen HalfState = "open"
	HalfReversed HalfState = "reversed"
	HalfAcked HalfState = "acked"
)

const (
	HalfEvOpen = "open"
	HalfEvReverse = "reverse"
	HalfEvAck     = "ack"
)

// FencePhase observe → apply → ticket。

type FencePhase string

const (
	FenceIdle FencePhase = "idle"
	FenceObserve FencePhase = "observe"
	FenceApply   FencePhase = "apply"
	FenceTicket FencePhase = "ticket"
	FenceSealed FencePhase = "sealed"
	FenceHalfCut FencePhase = "half_cut"
	FenceBlocked FencePhase = "blocked"
)

const (
	DefaultEffectTTL = 30 * time.Minute
	DefaultPatrolBatch = 80
	DefaultExpiryBatch = 100
	KindEffect         = "fundsurf_memo_effect"
	KindHalfProof = "fundsurf_memo_half_proof"
	KindFence = "fundsurf_ticket_credit_fence"
	KindAudit = "fundsurf_memo_effect_audit"
	TableEffects    = "fundsurf_memo_effects"
	TableHalfProofs = "fundsurf_memo_half_proofs"
	TableFences = "fundsurf_ticket_credit_fences"
	TableAudit        = "fundsurf_memo_effect_audit"
	HolderMemoApprove = "memo-approve"
	HolderAutoClaim   = "auto-claim"
	IdemObservePrefix = "memo-recover-obs:"
	IdemApplyPrefix = "memo-recover:"
)

// ProofKind 半贷记与拒批证明类别。

type ProofKind string

const (
	ProofRejectAfterCredit ProofKind = "reject_after_credit"
	ProofHalfCutObserve    ProofKind = "half_cut_observe"
	ProofHalfCutApply ProofKind = "half_cut_apply"
	ProofOutputOccupied ProofKind = "output_occupied"
	ProofMissingEffect ProofKind = "missing_effect"
	ProofTicketNotPending ProofKind = "ticket_not_pending"
	ProofSecondFullCredit ProofKind = "second_full_credit_blocked"
	ProofExpireBound ProofKind = "expire_bound"
	ProofReverseGate ProofKind = "ops_reverse"
	ProofQuarantine  ProofKind = "quarantine"
)

// MemoEffect 审批贷记与输出互斥绑定。
type MemoEffect struct {
	ID       string `json:"memo_effect_id"`
	TicketID string `json:"ticket_id"`
	UserID   string `json:"user_id"`
	Chain    string `json:"chain"`
	TxHash   string `json:"tx_hash"`
	OutputIndex int `json:"output_index"`
	OutputKey string `json:"output_key"`
	Asset string `json:"asset"`
	Memo  string `json:"memo"`
	Address string `json:"address"`
	State       EffectState `json:"state"`
	Holder      string      `json:"holder"`
	ObservedAmt string      `json:"observed_amount,omitempty"`
	ObservedRef string `json:"observed_ref,omitempty"`
	AppliedRef string `json:"applied_ref,omitempty"`
	FenceID string `json:"fence_id,omitempty"`
	ProofID string `json:"proof_id,omitempty"`
	Idem string `json:"idem,omitempty"`
	TTL Duration `json:"ttl"`
	BoundAt   time.Time `json:"bound_at"`
	ExpiresAt time.Time `json:"expires_at"`
	CreditedAt *time.Time `json:"credited_at,omitempty"`
	RejectedAt *time.Time `json:"rejected_at,omitempty"`
	ExpiredAt *time.Time `json:"expired_at,omitempty"`
	Actor     string    `json:"actor,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MemoHalfCreditProof 拒批后已观察贷记或半切证明。

type MemoHalfCreditProof struct {
	ID string `json:"id"`
	EffectID string `json:"memo_effect_id"`
	TicketID string    `json:"ticket_id"`
	FenceID  string    `json:"fence_id,omitempty"`
	Kind     ProofKind `json:"kind"`
	State    HalfState `json:"state"`
	Chain string `json:"chain"`
	TxHash string `json:"tx_hash"`
	OutputIndex int    `json:"output_index"`
	ObservedAmt string `json:"observed_amount,omitempty"`
	ReverseRef  string `json:"reverse_ref,omitempty"`
	Blocked bool `json:"blocked"`
	Sealed  bool `json:"sealed"`
	SilentKeep bool `json:"silent_keep"`
	Detail string `json:"detail"`
	Actor string `json:"actor,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	AckedAt    *time.Time `json:"acked_at,omitempty"`
	ReversedAt *time.Time `json:"reversed_at,omitempty"`
}

// TicketCreditFence Observe / Apply / 工单推进同一栅栏。
type TicketCreditFence struct {
	ID       string `json:"id"`
	EffectID string `json:"memo_effect_id"`
	TicketID string     `json:"ticket_id"`
	Phase    FencePhase `json:"phase"`
	ObserveOK bool `json:"observe_ok"`
	ApplyOK   bool `json:"apply_ok"`
	TicketOK  bool `json:"ticket_ok"`
	HalfCut   bool `json:"half_cut"`
	CreditCount int `json:"credit_count"`
	LastError string `json:"last_error,omitempty"`
	ObservedAmt string `json:"observed_amount,omitempty"`
	IdemObserve string `json:"idem_observe"`
	IdemApply string `json:"idem_apply"`
	ResumeToken string `json:"resume_token,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	SealedAt *time.Time `json:"sealed_at,omitempty"`
}

// Occupancy 输出占用视图（只读消费 claimmutex）。
type Occupancy struct {
	OutputKey string
	Holder   string
	TicketID string
	Taken bool
	AutoClaimed bool
	EffectID string
}

// MemoTicketView 工单投影，避免改写 fundsurf 工单文案。

type MemoTicketView struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Chain string `json:"chain"`
	TxHash string `json:"tx_hash"`
	Memo    string `json:"memo"`
	Address string `json:"address"`
	Status  string `json:"status"`
}

// EffectAudit 状态审计。
type EffectAudit struct {
	ID       string `json:"id"`
	EffectID string `json:"memo_effect_id"`
	Event    string `json:"event"`
	FromState EffectState `json:"from_state"`
	ToState   EffectState `json:"to_state"`
	Actor     string      `json:"actor"`
	Detail    string      `json:"detail"`
	At time.Time `json:"at"`
}

// ApproveMemoWithEffectReq 审批贷记并绑定输出效果。

type ApproveMemoWithEffectReq struct {
	TicketID string `json:"ticket_id"`
	Asset    string `json:"asset"`
	OutputIndex int `json:"output_index"`
	Actor string `json:"actor"`
	Idem  string `json:"idem"`
	TTL   string `json:"ttl"`
}

// ApproveMemoWithEffectResp 审批结果。

type ApproveMemoWithEffectResp struct {
	Ticket MemoTicketView `json:"ticket"`
	Effect MemoEffect `json:"effect"`
	Fence     TicketCreditFence    `json:"fence"`
	HalfProof *MemoHalfCreditProof `json:"half_proof,omitempty"`
	OK bool `json:"ok"`
}

// DecideMemoGuardedReq 拒批守卫。
type DecideMemoGuardedReq struct {
	TicketID string `json:"ticket_id"`
	Status       string `json:"status"`
	Actor        string `json:"actor"`
	ForceReverse bool   `json:"force_reverse"`
	Quarantine bool `json:"quarantine"`
	Reason string `json:"reason"`
}

// DecideMemoGuardedResp 拒批结果。

type DecideMemoGuardedResp struct {
	Ticket MemoTicketView `json:"ticket"`
	Effect *MemoEffect    `json:"effect,omitempty"`
	Proof   *MemoHalfCreditProof `json:"proof,omitempty"`
	Blocked bool                 `json:"blocked"`
	OK bool `json:"ok"`
}

// ReverseGateReq 运维冲正闸。
type ReverseGateReq struct {
	ProofID string `json:"proof_id"`
	Actor   string `json:"actor"`
	AckOnly bool   `json:"ack_only"`
	Reason string `json:"reason"`
}

// CreditSight 观察结果。

type CreditSight struct {
	TransferID string
	Amount     string
	State      string
	Credited bool
}
