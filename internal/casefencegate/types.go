package casefencegate

import (
	"encoding/json"
	"time"
)

// ──────────────────────────────────────────
// TicketState 放行工单状态
// ──────────────────────────────────────────

type TicketState string

const (
	TicketOpen     TicketState = "open"
	TicketApproved TicketState = "approved"
	TicketDenied   TicketState = "denied"
	TicketExpired  TicketState = "expired"
	TicketStale TicketState = "stale"
)

  func (s TicketState) Terminal() bool {
	   return s == TicketApproved || s == TicketDenied || s == TicketExpired || s == TicketStale
  }

   func (s TicketState) Active() bool {
	   return s == TicketOpen
   }

  func (s TicketState) Decided() bool {
	   return s == TicketApproved || s == TicketDenied
   }

// ──────────────────────────────────────────
// CaseFenceLease 围栏租约
// ──────────────────────────────────────────

type CaseFenceLease struct {
	ID string `json:"id"`
	CaseID     string `json:"case_id"`
	MovementID string `json:"movement_id"`
	UserID     string `json:"user_id"`
	FenceEpoch int64  `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
	PolicyVer  int    `json:"policy_version"`
	HeldBy     string `json:"held_by,omitempty"`
	HeldAt time.Time `json:"held_at,omitempty"`
	Released   bool      `json:"released"`
	ReleasedAt time.Time `json:"released_at,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

   func (l *CaseFenceLease) StillValid(epoch int64, token string) bool {
	   return l != nil && l.FenceEpoch == epoch && l.FenceToken == token
   }

// ──────────────────────────────────────────
// CaseReleaseTicket 放行工单
// ──────────────────────────────────────────

type CaseReleaseTicket struct {
	ID string `json:"id"`
	CaseID     string `json:"case_id"`
	MovementID string `json:"movement_id"`
	UserID   string `json:"user_id"`
	Decision string `json:"decision"`
	DecidedBy string `json:"decided_by"`
	PolicyVer  int    `json:"policy_version"`
	FenceEpoch int64  `json:"fence_epoch"`
	FenceToken string `json:"fence_token,omitempty"`
	State      TicketState `json:"state"`
	TTL        Duration    `json:"ttl"`
	IssuedAt   time.Time   `json:"issued_at"`
	ExpiresAt  time.Time   `json:"expires_at"`
	ConsumedAt *time.Time  `json:"consumed_at,omitempty"`
	StaleAt *time.Time `json:"stale_at,omitempty"`
	StaleReason string    `json:"stale_reason,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

   func (t *CaseReleaseTicket) Expired(now time.Time) bool {
	  return t != nil && !t.ExpiresAt.IsZero() && now.After(t.ExpiresAt)
   }

  func (t *CaseReleaseTicket) Alive(now time.Time) bool {
	  return t != nil && t.State.Active() && !t.Expired(now)
  }

  func (t *CaseReleaseTicket) MatchPolicy(policyVer int) bool {
	   return t != nil && t.PolicyVer == policyVer
   }

// ──────────────────────────────────────────
// CaseDualDecideProof 双审核凭证
// ──────────────────────────────────────────

type CaseDualDecideProof struct {
	ID string `json:"id"`
	CaseID     string `json:"case_id"`
	MovementID string `json:"movement_id"`
	WinnerActor    string `json:"winner_actor"`
	WinnerDecision string `json:"winner_decision"`
	LoserActor     string `json:"loser_actor"`
	LoserDecision  string `json:"loser_decision"`
	FenceEpoch int64 `json:"fence_epoch"`
	WinnerEpoch int64 `json:"winner_epoch"`
	LoserEpoch int64     `json:"loser_epoch"`
	Sealed     bool      `json:"sealed"`
	SealedAt   time.Time `json:"sealed_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ──────────────────────────────────────────
// BareFenceAlert 裸 Decide 告警事件
// ──────────────────────────────────────────

type BareFenceAlert struct {
	ID     string `json:"id"`
	CaseID string `json:"case_id"`
	MovementID string `json:"movement_id"`
	Actor      string `json:"actor"`
	Blocked bool `json:"blocked"`
	Reason string `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

// ──────────────────────────────────────────
// CaseSnapshot 只读案件快照
// ──────────────────────────────────────────

type CaseSnapshot struct {
	ID         string `json:"id"`
	MovementID string `json:"movement_id"`
	UserID     string `json:"user_id"`
	State      string `json:"state"`
	PolicyVer  int    `json:"policy_version"`
	FenceEpoch int64  `json:"fence_epoch"`
	FenceToken string `json:"fence_token,omitempty"`
	DecidedBy string `json:"decided_by,omitempty"`
	Kind      string `json:"kind,omitempty"`
}

   func (cs CaseSnapshot) IsOpen() bool {
	  return cs.State == "open" || cs.State == "assigned"
  }

   func (cs CaseSnapshot) IsApproved() bool {
	   return cs.State == "approved"
  }

   func (cs CaseSnapshot) IsDenied() bool {
	  return cs.State == "denied"
   }

   func (cs CaseSnapshot) IsExpired() bool {
	   return cs.State == "expired"
  }

// ──────────────────────────────────────────
// Request / Response 结构
// ──────────────────────────────────────────

type DecideFencedReq struct {
	CaseID string `json:"case_id"`
	MovementID string `json:"movement_id"`
	Actor    string `json:"actor"`
	Decision string `json:"decision"`
	FenceEpoch int64  `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
}

type DecideFencedResult struct {
	Ticket *CaseReleaseTicket   `json:"ticket,omitempty"`
	Proof  *CaseDualDecideProof `json:"proof,omitempty"`
	Case   *CaseSnapshot        `json:"case,omitempty"`
}

type SubmitCheckReq struct {
	TicketID   string `json:"ticket_id"`
	MovementID string `json:"movement_id"`
	PolicyVer  int    `json:"policy_version"`
}

type SubmitCheckResult struct {
	Allowed bool               `json:"allowed"`
	Ticket  *CaseReleaseTicket `json:"ticket,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type BareDecideReq struct {
	CaseID     string `json:"case_id"`
	MovementID string `json:"movement_id"`
	Actor string `json:"actor"`
	Decision string `json:"decision"`
}

type ExpireResult struct {
	Expired []string `json:"expired,omitempty"`
	Bumped []string `json:"bumped,omitempty"`
	Alerts []string `json:"alerts,omitempty"`
}

type ProofExportReq struct {
	CaseID string `json:"case_id,omitempty"`
	MovementID string `json:"movement_id,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

// ──────────────────────────────────────────
// CaseQuerier 案件查询接口（只读依赖）
// ──────────────────────────────────────────

type CaseQuerier interface {
	GetCase(id string) (CaseSnapshot, error)
	GetCaseByMovement(movementID string) (CaseSnapshot, error)
	DecideFenced(id, actor, decision string, epoch int64, token string) (CaseSnapshot, error)
	ExpireOlderThan(age time.Duration) []CaseSnapshot
}

// ──────────────────────────────────────────
// Config 引擎配置
// ──────────────────────────────────────────

type Config struct {
	Store TicketStore
	CaseQuerier CaseQuerier
	DefaultTTL  time.Duration
	MaxTTL      time.Duration
	ExpireAge time.Duration
	PatrolInterval time.Duration
	ExpireInterval time.Duration
	MaxProofsExport int
	BlockBareDecide bool
}

  func (c Config) withDefaults() Config {
	   if c.DefaultTTL <= 0 {
		   c.DefaultTTL = 5 * time.Minute
	  }
	   if c.MaxTTL <= 0 {
		   c.MaxTTL = 30 * time.Minute
	  }
	   if c.ExpireAge <= 0 {
		   c.ExpireAge = 24 * time.Hour
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
