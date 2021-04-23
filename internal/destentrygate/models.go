package destentrygate

import "time"

// LeaseState 租约生命周期。

type LeaseState string

const (
	LeaseIssued LeaseState = "issued"
	LeaseBound    LeaseState = "bound"
	LeaseConsumed LeaseState = "consumed"
	LeaseExpired LeaseState = "expired"
	LeaseRevoked LeaseState = "revoked"
)

// EntryMode 钉扎在租约上的条目模式。

type EntryMode string

const (
	ModeStrict EntryMode = "strict"
	ModeTemp EntryMode = "temp"
	ModeBreakglass EntryMode = "breakglass"
)

// ProofKind 漂移证明类型。

type ProofKind string

const (
	DriftNoLease ProofKind = "no_lease"
	DriftExpired ProofKind = "expired"
	DriftRevoked ProofKind = "revoked"
	DriftConsumed          ProofKind = "consumed"
	DriftTempExpire        ProofKind = "temp_expire"
	DriftBreakglassReclaim ProofKind = "breakglass_reclaim"
	DriftRevoke ProofKind = "entry_revoke"
	DriftEntryGen ProofKind = "entry_gen"
	DriftMode     ProofKind = "mode"
	DriftCooldown ProofKind = "cooldown_not_full"
	DriftAddr     ProofKind = "addr"
	DriftMissing ProofKind = "entry_missing"
	DriftPatrol ProofKind = "patrol"
	DriftWhitelistOnly ProofKind = "whitelist_only"
)

// AuditAction 审计动作。
type AuditAction string

const (
	AuditIssue AuditAction = "issue"
	AuditBind       AuditAction = "bind"
	AuditConsume    AuditAction = "consume"
	AuditExpire     AuditAction = "expire"
	AuditRevokeTemp AuditAction = "revoke_temp_expire"
	AuditRevokeGlass    AuditAction = "revoke_breakglass"
	AuditRevokeEntry    AuditAction = "revoke_entry"
	AuditRevokeCooldown AuditAction = "revoke_cooldown"
	AuditRevokeOps AuditAction = "revoke_ops"
	AuditProof     AuditAction = "proof"
	AuditGenBump AuditAction = "gen_bump"
	AuditPatrol      AuditAction = "patrol"
	AuditRenewReject AuditAction = "renew_reject"
	AuditExport AuditAction = "export"
	AuditPrecheckDeny AuditAction = "precheck_deny"
	AuditRestoreNotFull AuditAction = "restore_not_full"
)

// DestEntryLease 封存预检通过瞬间的条目身份与世代。
// 出金提交必须携带未过期 dest_entry_lease_id 并复验 entry_gen。
type DestEntryLease struct {
	LeaseID string `json:"dest_entry_lease_id" db:"lease_id"`
	EntryID  string `json:"entry_id" db:"entry_id"`
	EntryGen int64  `json:"entry_gen" db:"entry_gen"`
	UserID string `json:"user_id" db:"user_id"`
	Chain string `json:"chain" db:"chain"`
	Address string `json:"address" db:"address"`
	Mode EntryMode `json:"mode" db:"mode"`
	WhitelistOnly bool `json:"whitelist_only" db:"whitelist_only"`
	CooldownFull bool `json:"cooldown_full" db:"cooldown_full"`
	State      LeaseState `json:"state" db:"state"`
	TTLSeconds int64      `json:"ttl_seconds" db:"ttl_seconds"`
	IssuedAt   time.Time  `json:"issued_at" db:"issued_at"`
	BoundAt    *time.Time `json:"bound_at,omitempty" db:"bound_at"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty" db:"consumed_at"`
	ExpiredAt  *time.Time `json:"expired_at,omitempty" db:"expired_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty" db:"revoked_at"`
	RevokeReason   string `json:"revoke_reason,omitempty" db:"revoke_reason"`
	WithdrawRef    string `json:"withdraw_ref,omitempty" db:"withdraw_ref"`
	IdempotencyKey string `json:"idempotency_key,omitempty" db:"idempotency_key"`
	BindIdemKey string    `json:"bind_idempotency_key,omitempty" db:"bind_idempotency_key"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// ExpiresAt 租约绝对过期时刻。
  func (l DestEntryLease) ExpiresAt() time.Time {
	   ttl := l.TTLSeconds
	  if ttl <= 0 {
		   ttl = DefaultTTLSeconds
	  }
	  return l.IssuedAt.Add(time.Duration(ttl) * time.Second)
  }

// IsConsumable 是否仍可绑定出金。

   func (l *DestEntryLease) IsConsumable(now time.Time) bool {
	  if l == nil {
		  return false
	   }
	  if l.State != LeaseIssued && l.State != LeaseBound {
		  return false
	   }
	   return !now.After(l.ExpiresAt())
   }

// DestEntryDriftProof 条目漂移拒出金证明。只追加。
type DestEntryDriftProof struct {
	ProofID string `json:"proof_id" db:"proof_id"`
	Kind ProofKind `json:"kind" db:"kind"`
	LeaseID string `json:"dest_entry_lease_id" db:"lease_id"`
	EntryID string `json:"entry_id" db:"entry_id"`
	UserID string `json:"user_id" db:"user_id"`
	Chain string `json:"chain" db:"chain"`
	Address  string    `json:"address" db:"address"`
	BindMode EntryMode `json:"bind_mode" db:"bind_mode"`
	LiveMode EntryMode `json:"live_mode" db:"live_mode"`
	BindGen int64 `json:"bind_gen" db:"bind_gen"`
	LiveGen int64 `json:"live_gen" db:"live_gen"`
	Evidence string `json:"evidence" db:"evidence"`
	DetectedAt time.Time  `json:"detected_at" db:"detected_at"`
	ExportedAt *time.Time `json:"exported_at,omitempty" db:"exported_at"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// EntryGeneration 条目级世代。Revoke / Temp 过期 / 破窗收回 / Restore 未满抬升。

type EntryGeneration struct {
	EntryID string `json:"entry_id" db:"entry_id"`
	UserID  string `json:"user_id" db:"user_id"`
	Chain   string `json:"chain" db:"chain"`
	Address string `json:"address" db:"address"`
	Gen  int64     `json:"entry_gen" db:"entry_gen"`
	Mode EntryMode `json:"mode" db:"mode"`
	BumpReason string `json:"bump_reason" db:"bump_reason"`
	BumpedAt time.Time `json:"bumped_at" db:"bumped_at"`
	BumpedBy string    `json:"bumped_by" db:"bumped_by"`
}

// DestEntryAudit 生命周期审计。
type DestEntryAudit struct {
	AuditID string `json:"audit_id" db:"audit_id"`
	LeaseID string `json:"dest_entry_lease_id" db:"lease_id"`
	EntryID string `json:"entry_id" db:"entry_id"`
	Action AuditAction `json:"action" db:"action"`
	Actor string `json:"actor" db:"actor"`
	Detail    string    `json:"detail" db:"detail"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// ExportProofFilter 证明导出过滤。

type ExportProofFilter struct {
	Kind           ProofKind
	EntryID        string
	UserID         string
	UnexportedOnly bool
	Limit int
}

// LiveEntry 活条目观察面（只读消费 destallow）。

type LiveEntry struct {
	EntryID string
	UserID string
	Chain  string
	Address string
	Mode    EntryMode
	Live    bool
	CooldownFull bool
	Reason       string
	TempExpires  time.Time
	GlassExpires time.Time
}

// WithdrawBindResult 出金绑定结果。

type WithdrawBindResult struct {
	LeaseID string `json:"dest_entry_lease_id"`
	WithdrawRef string `json:"withdraw_ref"`
	State LeaseState `json:"state"`
	Bound bool       `json:"bound"`
	Consumed bool `json:"consumed"`
}
