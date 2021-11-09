package shardholdgen

import "time"

// GenState 腿占世代生命周期。

type GenState string

const (
	GenIssued GenState = "issued"
	GenHeld   GenState = "held"
	GenReleased GenState = "released"
	GenExpired GenState = "expired"
)

// HalfState 半截 Advance 案。

type HalfState string

const (
	HalfOpen    HalfState = "open"
	HalfResumed HalfState = "resumed"
	HalfAbandoned HalfState = "abandoned"
)

// HalfKind 半截形态。

type HalfKind string

const (
	HalfFreezeNoBroadcast HalfKind = "freeze_no_broadcast"
	HalfBroadcastNoAlign  HalfKind = "broadcast_no_align"
)

// ProofKind 双占/漏释/挡写证明。

type ProofKind string

const (
	ProofDoubleHold  ProofKind = "double_hold"
	ProofMissRelease ProofKind = "miss_release"
	ProofLiveHash    ProofKind = "live_hash"
	ProofNoGeneration ProofKind = "no_generation"
	ProofOverRelease ProofKind = "over_release"
	ProofConflict    ProofKind = "conflict"
	ProofReplay ProofKind = "replay"
	ProofExpired      ProofKind = "expired"
	ProofBroadcastRel ProofKind = "broadcast_release"
	ProofHalfMutex    ProofKind = "half_mutex"
	ProofGeneration   ProofKind = "generation"
)

// AuditAction 审计。

type AuditAction string

const (
	AuditIssue   AuditAction = "issue"
	AuditAdvance AuditAction = "advance"
	AuditRelease AuditAction = "release"
	AuditExpire     AuditAction = "expire"
	AuditHalfOpen   AuditAction = "half_open"
	AuditHalfResume AuditAction = "half_resume"
	AuditHalfAbandon AuditAction = "half_abandon"
	AuditProof  AuditAction = "proof"
	AuditExport AuditAction = "export"
	AuditPatrol AuditAction = "patrol"
	AuditAlign AuditAction = "align"
	AuditMutex AuditAction = "mutex"
)

// LegHoldGen 每条腿冻结钉扎的世代。
type LegHoldGen struct {
	GenID string `json:"leg_hold_gen_id" db:"gen_id"`
	PlanID string `json:"plan_id" db:"plan_id"`
	LegID string `json:"leg_id" db:"leg_id"`
	HoldID string `json:"hold_id" db:"hold_id"`
	IdempotencyKey string `json:"idempotency_key" db:"idempotency_key"`
	UserID         string `json:"user_id" db:"user_id"`
	Asset  string `json:"asset" db:"asset"`
	Amount int64  `json:"amount" db:"amount"`
	HoldAmt     int64  `json:"hold_amt" db:"hold_amt"`
	Fingerprint string `json:"fingerprint" db:"fingerprint"`
	Generation  int64  `json:"generation" db:"generation"`
	State      GenState   `json:"state" db:"state"`
	TTLSeconds int64      `json:"ttl_seconds" db:"ttl_seconds"`
	IssuedAt   time.Time  `json:"issued_at" db:"issued_at"`
	HeldAt     *time.Time `json:"held_at,omitempty" db:"held_at"`
	ReleasedAt *time.Time `json:"released_at,omitempty" db:"released_at"`
	ExpiredAt   *time.Time `json:"expired_at,omitempty" db:"expired_at"`
	BroadcastID string     `json:"broadcast_id,omitempty" db:"broadcast_id"`
	LiveHash    string     `json:"live_hash,omitempty" db:"live_hash"`
	ReleaseAmt   int64  `json:"release_amt" db:"release_amt"`
	MutexOwner   string `json:"mutex_owner,omitempty" db:"mutex_owner"`
	AdvanceCount int64  `json:"advance_count" db:"advance_count"`
	ReleaseCount int64 `json:"release_count" db:"release_count"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

   func (g LegHoldGen) ExpiresAt() time.Time {
	   ttl := g.TTLSeconds
	   if ttl <= 0 {
		  ttl = DefaultTTLSeconds
	  }
	  return g.IssuedAt.Add(time.Duration(ttl) * time.Second)
   }

   func (g *LegHoldGen) IsHoldable(now time.Time) bool {
	  if g == nil {
		   return false
	   }
	   if g.State != GenIssued && g.State != GenHeld {
		  return false
	  }
	  return !now.After(g.ExpiresAt())
   }

   func (g *LegHoldGen) ResidualHold() int64 {
	   if g == nil {
		  return 0
	   }
	   r := g.HoldAmt - g.ReleaseAmt
	   if r < 0 {
		   return 0
	  }
	  return r
   }

// ShardHoldProof 双占/漏释/挡写证明，只追加。

type ShardHoldProof struct {
	ProofID string    `json:"proof_id" db:"proof_id"`
	Kind    ProofKind `json:"kind" db:"kind"`
	GenID string `json:"leg_hold_gen_id" db:"gen_id"`
	PlanID string `json:"plan_id" db:"plan_id"`
	LegID   string `json:"leg_id" db:"leg_id"`
	HoldID  string `json:"hold_id" db:"hold_id"`
	IdemKey string `json:"idempotency_key" db:"idempotency_key"`
	Amount  int64  `json:"amount" db:"amount"`
	BindGen int64 `json:"bind_gen" db:"bind_gen"`
	LiveGen  int64  `json:"live_gen" db:"live_gen"`
	BindHash string `json:"bind_hash" db:"bind_hash"`
	LiveHash string `json:"live_hash" db:"live_hash"`
	Digest   string `json:"digest" db:"digest"`
	Evidence string `json:"evidence" db:"evidence"`
	DetectedAt time.Time `json:"detected_at" db:"detected_at"`
	ExportedAt *time.Time `json:"exported_at,omitempty" db:"exported_at"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// HalfLegCase 半截 Advance：冻成功未广播，或广播成功未一致。
type HalfLegCase struct {
	CaseID string `json:"case_id" db:"case_id"`
	GenID  string `json:"leg_hold_gen_id" db:"gen_id"`
	PlanID string `json:"plan_id" db:"plan_id"`
	LegID string `json:"leg_id" db:"leg_id"`
	HoldID string    `json:"hold_id" db:"hold_id"`
	Kind   HalfKind  `json:"kind" db:"kind"`
	State  HalfState `json:"state" db:"state"`
	Actor string `json:"actor" db:"actor"`
	ResumeMu bool `json:"resume_mutex" db:"resume_mutex"`
	Note      string    `json:"note" db:"note"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// GenAudit 世代审计。

type GenAudit struct {
	AuditID string `json:"audit_id" db:"audit_id"`
	GenID string `json:"leg_hold_gen_id" db:"gen_id"`
	Action AuditAction `json:"action" db:"action"`
	Actor  string      `json:"actor" db:"actor"`
	FromState GenState `json:"from_state" db:"from_state"`
	ToState   GenState `json:"to_state" db:"to_state"`
	Note      string   `json:"note" db:"note"`
	At time.Time `json:"at" db:"at"`
}
