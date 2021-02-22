package allowleasegate

import (
	"encoding/json"
	"time"
)

// ──────────────────────────────────────────
// LeaseState 放行租约状态
// ──────────────────────────────────────────

type LeaseState string

const (
	StateAllowed  LeaseState = "allowed"
	StateBound    LeaseState = "bound"
	StateConsumed LeaseState = "consumed"
	StateExpired  LeaseState = "expired"
	StateRevoked  LeaseState = "revoked"
)

  func (s LeaseState) Terminal() bool {
	   return s == StateConsumed || s == StateExpired || s == StateRevoked
  }

  func (s LeaseState) Active() bool {
	  return s == StateAllowed || s == StateBound
   }

// ──────────────────────────────────────────
// AllowLease 放行租约
// ──────────────────────────────────────────

type AllowLease struct {
	ID string `json:"id"`
	MovementID string `json:"movement_id"`
	UserID     string `json:"user_id"`
	CaseID string `json:"case_id,omitempty"`
	Asset string `json:"asset"`
	Chain string `json:"chain"`
	ToAddress string `json:"to_address"`
	Amount    string `json:"amount"`
	Decision string `json:"decision"`
	PolicyVer int        `json:"policy_version"`
	State     LeaseState `json:"state"`
	TTL       Duration   `json:"ttl"`
	IssuedAt  time.Time  `json:"issued_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	BoundAt *time.Time `json:"bound_at,omitempty"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	ExpiredAt  *time.Time `json:"expired_at,omitempty"`
	RevokeReason  string `json:"revoke_reason,omitempty"`
	BoundMovement string `json:"bound_movement,omitempty"`
	FenceEpoch    int64  `json:"fence_epoch"`
	FenceToken string `json:"fence_token,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

   func (l *AllowLease) Expired(now time.Time) bool {
	  return l != nil && !l.ExpiresAt.IsZero() && now.After(l.ExpiresAt)
   }

   func (l *AllowLease) Alive(now time.Time) bool {
	   return l != nil && l.State.Active() && !l.Expired(now)
  }

// ──────────────────────────────────────────
// PolicyBind 策略绑定快照
// ──────────────────────────────────────────

type PolicyBind struct {
	LeaseID   string `json:"lease_id"`
	PolicyVer int    `json:"policy_version"`
	Decision string `json:"decision"`
	CaseID   string `json:"case_id,omitempty"`
	Hook string `json:"hook,omitempty"`
	ScreenScore string `json:"screen_score,omitempty"`
	BoundAt     string `json:"bound_at,omitempty"`
}

// ──────────────────────────────────────────
// AllowDriftProof 漂移封堵凭证
// ──────────────────────────────────────────

type AllowDriftProof struct {
	ID string `json:"id"`
	LeaseID    string `json:"lease_id,omitempty"`
	MovementID string `json:"movement_id"`
	UserID     string `json:"user_id"`
	Asset string `json:"asset"`
	Chain string `json:"chain"`
	ToAddress string `json:"to_address"`
	Amount    string `json:"amount"`
	DriftKind string `json:"drift_kind"`
	LeaseState LeaseState `json:"lease_state,omitempty"`
	CaseState string `json:"case_state,omitempty"`
	PolicyVer int    `json:"policy_version,omitempty"`
	Reason    string `json:"reason"`
	Blocked   bool   `json:"blocked"`
	Sealed    bool   `json:"sealed"`
	SealedAt  *time.Time `json:"sealed_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// ──────────────────────────────────────────
// IssueReq / SubmitReq / RevokeReq
// ──────────────────────────────────────────

type IssueReq struct {
	MovementID string `json:"movement_id"`
	UserID     string `json:"user_id"`
	CaseID    string `json:"case_id,omitempty"`
	Asset     string `json:"asset"`
	Chain     string `json:"chain"`
	ToAddress string `json:"to_address"`
	Amount string `json:"amount"`
	Decision string `json:"decision"`
	PolicyVer int           `json:"policy_version"`
	Hook      string        `json:"hook,omitempty"`
	TTL       time.Duration `json:"ttl,omitempty"`
}

type SubmitReq struct {
	LeaseID    string `json:"lease_id"`
	MovementID string `json:"movement_id"`
	UserID     string `json:"user_id"`
	Actor string `json:"actor,omitempty"`
}

type SubmitResult struct {
	Allowed bool `json:"allowed"`
	Lease *AllowLease `json:"lease,omitempty"`
	Proof *AllowDriftProof `json:"proof,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type RevokeReq struct {
	LeaseID string `json:"lease_id"`
	Reason string `json:"reason"`
	Actor string `json:"actor,omitempty"`
}

type PatrolResult struct {
	Expired []string `json:"expired,omitempty"`
	Revoked []string `json:"revoked,omitempty"`
	Proofs  []string `json:"proofs,omitempty"`
}

type ProofExportReq struct {
	MovementID string `json:"movement_id,omitempty"`
	UserID string `json:"user_id,omitempty"`
	LeaseID string `json:"lease_id,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

// ──────────────────────────────────────────
// Duration JSON 适配
// ──────────────────────────────────────────

type Duration time.Duration

  func (d Duration) MarshalJSON() ([]byte, error) {
	   return json.Marshal(time.Duration(d).String())
   }

   func (d *Duration) UnmarshalJSON(b []byte) error {
	   var s string
	  if err := json.Unmarshal(b, &s); err != nil {
		  return err
	   }
	  v, err := time.ParseDuration(s)
	  if err != nil {
		  return err
	  }
	  *d = Duration(v)
	   return nil
   }

  func (d Duration) Std() time.Duration { return time.Duration(d) }

// ──────────────────────────────────────────
// Config 引擎配置
// ──────────────────────────────────────────

type Config struct {
	Store LeaseStore
	CaseQuerier CaseQuerier
	DefaultTTL      time.Duration
	MaxTTL          time.Duration
	PatrolInterval  time.Duration
	ExpireInterval  time.Duration
	MaxProofsExport int
}

  func (c Config) withDefaults() Config {
	   if c.DefaultTTL <= 0 {
		  c.DefaultTTL = 5 * time.Minute
	   }
	  if c.MaxTTL <= 0 {
		  c.MaxTTL = 30 * time.Minute
	  }
	   if c.PatrolInterval <= 0 {
		  c.PatrolInterval = 10 * time.Second
	  }
	   if c.ExpireInterval <= 0 {
		   c.ExpireInterval = 5 * time.Second
	   }
	   if c.MaxProofsExport <= 0 {
		   c.MaxProofsExport = 500
	   }
	  return c
   }

// ──────────────────────────────────────────
// CaseQuerier 案件查询接口（只读依赖）
// ──────────────────────────────────────────

type CaseQuerier interface {
	GetCase(id string) (CaseSnapshot, error)
	GetCaseByMovement(movementID string) (CaseSnapshot,
		error)
}

type CaseSnapshot struct {
	ID string `json:"id"`
	MovementID string `json:"movement_id"`
	UserID    string `json:"user_id"`
	State     string `json:"state"`
	PolicyVer int    `json:"policy_version"`
	DecidedBy string `json:"decided_by,omitempty"`
}

   func (cs CaseSnapshot) IsAllowed() bool {
	  return cs.State == "approved" || cs.State == "allow"
   }

   func (cs CaseSnapshot) IsDenied() bool {
	   return cs.State == "denied"
  }

   func (cs CaseSnapshot) IsExpired() bool {
	  return cs.State == "expired"
  }

  func (cs CaseSnapshot) IsOpen() bool {
	   return cs.State == "open" || cs.State == "assigned"
  }
