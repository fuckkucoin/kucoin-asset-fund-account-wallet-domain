package legmovefence

import "time"

// FenceState 围栏生命周期。

type FenceState string

const (
	FenceIssued   FenceState = "issued"
	FenceEnqueued FenceState = "enqueued"
	FenceMoved    FenceState = "moved"
	FenceExpired  FenceState = "expired"
	FenceRevoked  FenceState = "revoked"
)

// HalfState 半截案状态。

type HalfState string

const (
	HalfOpen HalfState = "open"
	HalfResumed HalfState = "resumed"
	HalfAbandoned HalfState = "abandoned"
)

// HalfKind 半截形态。

type HalfKind string

const (
	HalfEnqueueNoTransfer HalfKind = "enqueue_no_transfer"
	HalfTransferNoDone HalfKind = "transfer_no_done"
)

// ProofKind 双移/拒转证明类型。
type ProofKind string

const (
	ProofDoubleTransfer ProofKind = "double_transfer"
	ProofNoFence ProofKind = "no_fence"
	ProofConsumed    ProofKind = "fence_consumed"
	ProofHaltedOld   ProofKind = "halted_old_fence"
	ProofFingerprint ProofKind = "fingerprint"
	ProofGeneration  ProofKind = "generation"
	ProofReplay      ProofKind = "replay"
	ProofExpired     ProofKind = "expired"
	ProofRevoked     ProofKind = "revoked"
)

// AuditAction 审计动作。
type AuditAction string

const (
	AuditIssue   AuditAction = "issue"
	AuditEnqueue AuditAction = "enqueue"
	AuditMove AuditAction = "move"
	AuditExpire AuditAction = "expire"
	AuditRevoke AuditAction = "revoke"
	AuditRetry  AuditAction = "retry_assert"
	AuditHalfOpen    AuditAction = "half_open"
	AuditHalfResume  AuditAction = "half_resume"
	AuditHalfAbandon AuditAction = "half_abandon"
	AuditProof       AuditAction = "proof"
	AuditExport AuditAction = "export"
	AuditPatrol AuditAction = "patrol"
)

// LegMoveFence 每条腿 Enqueue↔TransferInv 共享的围栏。
type LegMoveFence struct {
	FenceID string `json:"leg_move_fence_id" db:"fence_id"`
	PlanID  string `json:"plan_id" db:"plan_id"`
	LegID   string `json:"leg_id" db:"leg_id"`
	IdempotencyKey string `json:"idempotency_key" db:"idempotency_key"`
	SrcRegion      string `json:"src_region" db:"src_region"`
	DstRegion      string `json:"dst_region" db:"dst_region"`
	Asset       string `json:"asset" db:"asset"`
	Amount      int64  `json:"amount" db:"amount"`
	Fingerprint string `json:"amount_fingerprint" db:"fingerprint"`
	Generation int64 `json:"generation" db:"generation"`
	State FenceState `json:"state" db:"state"`
	TTLSeconds int64 `json:"ttl_seconds" db:"ttl_seconds"`
	IssuedAt   time.Time  `json:"issued_at" db:"issued_at"`
	EnqueuedAt *time.Time `json:"enqueued_at,omitempty" db:"enqueued_at"`
	MovedAt    *time.Time `json:"moved_at,omitempty" db:"moved_at"`
	ExpiredAt    *time.Time `json:"expired_at,omitempty" db:"expired_at"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty" db:"revoked_at"`
	RevokeReason string     `json:"revoke_reason,omitempty" db:"revoke_reason"`
	IntentRef    string     `json:"intent_ref,omitempty" db:"intent_ref"`
	TransferRef  string     `json:"transfer_ref,omitempty" db:"transfer_ref"`
	MoveCount int64 `json:"move_count" db:"move_count"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

  func (f LegMoveFence) ExpiresAt() time.Time {
	   ttl := f.TTLSeconds
	  if ttl <= 0 {
		  ttl = DefaultTTLSeconds
	   }
	   return f.IssuedAt.Add(time.Duration(ttl) * time.Second)
  }

   func (f *LegMoveFence) IsTransferable(now time.Time) bool {
	   if f == nil {
		   return false
	   }
	   if f.State != FenceIssued && f.State != FenceEnqueued {
		  return false
	  }
	  return !now.After(f.ExpiresAt())
   }

// GeoDoubleMoveProof 二次划转或 fail-closed 拒转证明。只追加。

type GeoDoubleMoveProof struct {
	ProofID string    `json:"proof_id" db:"proof_id"`
	Kind    ProofKind `json:"kind" db:"kind"`
	FenceID string    `json:"leg_move_fence_id" db:"fence_id"`
	PlanID    string `json:"plan_id" db:"plan_id"`
	LegID     string `json:"leg_id" db:"leg_id"`
	IdemKey   string `json:"idempotency_key" db:"idempotency_key"`
	SrcRegion string `json:"src_region" db:"src_region"`
	DstRegion string `json:"dst_region" db:"dst_region"`
	Amount    int64  `json:"amount" db:"amount"`
	BindFP string `json:"bind_fingerprint" db:"bind_fingerprint"`
	LiveFP string `json:"live_fingerprint" db:"live_fingerprint"`
	BindGen int64 `json:"bind_gen" db:"bind_gen"`
	LiveGen    int64     `json:"live_gen" db:"live_gen"`
	Evidence   string    `json:"evidence" db:"evidence"`
	DetectedAt time.Time `json:"detected_at" db:"detected_at"`
	ExportedAt *time.Time `json:"exported_at,omitempty" db:"exported_at"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
}

// HalfMoveCase Enqueue 成功但 Transfer 失败，或 Transfer 成功但腿未标 done。
type HalfMoveCase struct {
	CaseID string `json:"case_id" db:"case_id"`
	Kind HalfKind `json:"kind" db:"kind"`
	FenceID    string    `json:"leg_move_fence_id" db:"fence_id"`
	PlanID     string    `json:"plan_id" db:"plan_id"`
	LegID      string    `json:"leg_id" db:"leg_id"`
	State      HalfState `json:"state" db:"state"`
	ResumeLock string    `json:"resume_lock,omitempty" db:"resume_lock"`
	LockedAt *time.Time `json:"locked_at,omitempty" db:"locked_at"`
	Reason   string    `json:"reason" db:"reason"`
	OpenedAt time.Time `json:"opened_at" db:"opened_at"`
	ClosedAt *time.Time `json:"closed_at,omitempty" db:"closed_at"`
	Actor string `json:"actor,omitempty" db:"actor"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// FenceAudit 围栏审计。
type FenceAudit struct {
	AuditID string      `json:"audit_id" db:"audit_id"`
	FenceID string      `json:"leg_move_fence_id" db:"fence_id"`
	PlanID  string      `json:"plan_id" db:"plan_id"`
	LegID   string      `json:"leg_id" db:"leg_id"`
	Action  AuditAction `json:"action" db:"action"`
	Actor string `json:"actor" db:"actor"`
	Detail    string    `json:"detail" db:"detail"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// RegionHaltPin 区域 Halted 世代钉扎（只读消费 georebal HaltGen）。
type RegionHaltPin struct {
	RegionID string `json:"region_id"`
	HaltGen  int64     `json:"halt_gen"`
	Halted   bool      `json:"halted"`
	PinnedAt time.Time `json:"pinned_at"`
}
