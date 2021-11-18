package pumpmarkfence

import (
	"strings"
	"time"
)

// GenState 投递效果世代状态机。

type GenState string

const (
	GenIssued GenState = "issued"
	GenDelivered GenState = "delivered"
	GenMarked    GenState = "marked"
	GenExpired   GenState = "expired"
)

   func ParseGenState(s string) (GenState, error) {
	   switch GenState(strings.ToLower(strings.TrimSpace(s))) {
	  case GenIssued, GenDelivered, GenMarked, GenExpired:
		   return GenState(strings.ToLower(strings.TrimSpace(s))), nil
	   default:
		  return "", ErrInvalidInput
	  }
  }

   func (s GenState) Live() bool {
	   return s == GenIssued || s == GenDelivered
   }

  func (s GenState) Applied() bool { return s == GenMarked }

   func (s GenState) Terminal() bool { return s == GenMarked || s == GenExpired }

// HalfState 半截 Mark 案状态机。

type HalfState string

const (
	HalfOpen HalfState = "open"
	HalfResumed   HalfState = "resumed"
	HalfAbandoned HalfState = "abandoned"
)

   func ParseHalfState(s string) (HalfState, error) {
	   switch HalfState(strings.ToLower(strings.TrimSpace(s))) {
	   case HalfOpen, HalfResumed, HalfAbandoned:
		   return HalfState(strings.ToLower(strings.TrimSpace(s))), nil
	   default:
		   return "", ErrInvalidInput
	   }
  }

  func (s HalfState) Open() bool { return s == HalfOpen || s == "" }

   func (s HalfState) Closed() bool { return s == HalfResumed || s == HalfAbandoned }

// ProofKind 双效果证明类型。

type ProofKind string

const (
	ProofTickReplay   ProofKind = "tick_replay"
	ProofPoisonReplay ProofKind = "poison_replay"
	ProofMarkNoGen ProofKind = "mark_no_gen"
	ProofExpired ProofKind = "gen_expired"
	ProofFingerprint ProofKind = "fingerprint"
	ProofLagMark     ProofKind = "lag_mark"
	ProofHalfSecond  ProofKind = "half_second_effect"
)

  func ParseProofKind(s string) (ProofKind, error) {
	   switch ProofKind(strings.ToLower(strings.TrimSpace(s))) {
	   case ProofTickReplay, ProofPoisonReplay, ProofMarkNoGen, ProofExpired, ProofFingerprint, ProofLagMark, ProofHalfSecond:
		  return ProofKind(strings.ToLower(strings.TrimSpace(s))),
			   nil
	   default:
		   return "", ErrInvalidInput
	   }
   }

// DeliverEffectGen 每次 Pending→投递必须钉住的效果世代。

type DeliverEffectGen struct {
	GenID       string `json:"deliver_effect_gen"`
	OutboxID    string `json:"outbox_id"`
	BusinessKey string `json:"business_key"`
	Kind        string `json:"kind"`
	PayloadHash string `json:"payload_hash"`
	EffectKey   string `json:"effect_key"`
	Generation  int64  `json:"generation"`
	State GenState      `json:"state"`
	TTL   time.Duration `json:"ttl"`
	Deadline time.Time `json:"deadline"`
	IssuedAt    time.Time `json:"issued_at"`
	DeliveredAt time.Time `json:"delivered_at,omitempty"`
	MarkedAt    time.Time `json:"marked_at,omitempty"`
	ExpiredAt time.Time `json:"expired_at,omitempty"`
	Applied bool `json:"applied"`
	HandlerInvoked bool   `json:"handler_invoked"`
	Actor          string `json:"actor"`
	Note           string `json:"note,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

   func (g DeliverEffectGen) Copy() DeliverEffectGen { return g }

   func (g DeliverEffectGen) Expired(now time.Time) bool {
	  if g.State == GenExpired {
		  return true
	  }
	   if g.State == GenMarked {
		  return false
	   }
	   if g.Deadline.IsZero() {
		   return false
	  }
	   return !now.UTC().Before(g.Deadline.UTC())
  }

   func (g DeliverEffectGen) AssertMarkable(now time.Time, payloadHash string) error {
	   if strings.TrimSpace(g.GenID) == "" {
		   return ErrGenMissing
	  }
	   if g.State == GenExpired || g.Expired(now) {
		   return ErrGenExpired
	   }
	  if g.State == GenMarked || g.Applied {
		   return ErrAlreadyApplied
	   }
	   if g.State != GenDelivered && g.State != GenIssued {
		  return ErrMarkDenied
	   }
	   if payloadHash != "" && g.PayloadHash != "" && g.PayloadHash != payloadHash {
		   return ErrFingerprint
	  }
	   if g.Generation <= 0 {
		  return ErrGenStale
	  }
	   return nil
  }

  func (g DeliverEffectGen) AssertReplayCAS(expect int64, now time.Time, payloadHash string) error {
	   if strings.TrimSpace(g.GenID) == "" {
		  return ErrGenMissing
	  }
	   if g.State == GenMarked || g.Applied {
		   return ErrAlreadyApplied
	   }
	  if g.State == GenExpired || g.Expired(now) {
		  return ErrGenExpired
	   }
	  if expect != 0 && expect != g.Generation {
		   return ErrCASFailed
	  }
	  if payloadHash != "" && g.PayloadHash != "" && g.PayloadHash != payloadHash {
		  return ErrFingerprint
	  }
	   return nil
  }

// OutboxDupProof 第二次全额效果或非法 Mark 的证明。
type OutboxDupProof struct {
	ProofID     string    `json:"proof_id"`
	GenID       string    `json:"deliver_effect_gen"`
	OutboxID    string    `json:"outbox_id"`
	EffectKey   string    `json:"effect_key"`
	Kind        ProofKind `json:"kind"`
	Generation  int64     `json:"generation"`
	PayloadHash string    `json:"payload_hash"`
	Actor string `json:"actor"`
	Detail string `json:"detail"`
	DetectedAt time.Time `json:"detected_at"`
	Exported   bool      `json:"exported"`
	ExportedAt time.Time `json:"exported_at,omitempty"`
	Note       string    `json:"note,omitempty"`
}

   func (p OutboxDupProof) Copy() OutboxDupProof { return p }

  func (p OutboxDupProof) Digest() ProofDigest {
	   return ProofDigest{
		   ProofID:  p.ProofID,
		  GenID:    p.GenID,
		  OutboxID: p.OutboxID,
		   EffectKey: p.EffectKey,
		  Kind:       string(p.Kind),
		  Generation: p.Generation,
		  Actor:  p.Actor,
		  Detail: p.Detail,
		  DetectedAt: p.DetectedAt.UTC().Format(time.RFC3339Nano),
	  }
  }

// ProofDigest 导出摘要，供 golden 。
type ProofDigest struct {
	ProofID string `json:"proof_id"`
	GenID   string `json:"deliver_effect_gen"`
	OutboxID string `json:"outbox_id"`
	EffectKey string `json:"effect_key"`
	Kind      string `json:"kind"`
	Generation int64 `json:"generation"`
	Actor      string `json:"actor"`
	Detail     string `json:"detail"`
	DetectedAt string `json:"detected_at"`
}

// HalfMarkCase 投递与 Mark 非原子留下的半截案。
type HalfMarkCase struct {
	CaseID string `json:"case_id"`
	GenID    string `json:"deliver_effect_gen"`
	OutboxID string `json:"outbox_id"`
	EffectKey string `json:"effect_key"`
	State       HalfState `json:"state"`
	Reason      string    `json:"reason"`
	OpenedAt    time.Time `json:"opened_at"`
	ResumedAt   time.Time `json:"resumed_at,omitempty"`
	AbandonedAt time.Time `json:"abandoned_at,omitempty"`
	ResumeToken string    `json:"resume_token,omitempty"`
	ResumeHolder string    `json:"resume_holder,omitempty"`
	ResumeUntil  time.Time `json:"resume_until,omitempty"`
	ResumeCount  int       `json:"resume_count"`
	PeerAcked bool `json:"peer_acked"`
	Note string `json:"note,omitempty"`
}

   func (c HalfMarkCase) Copy() HalfMarkCase { return c }

  func (c HalfMarkCase) HoldExclusive(now time.Time, holder, token string) error {
	  if !c.State.Open() {
		   return ErrHalfMarkClosed
	   }
	   if c.ResumeHolder != "" && c.ResumeUntil.After(now) && c.ResumeHolder != holder {
		   return ErrHalfMarkTaken
	   }
	   if c.ResumeToken != "" && token != "" && c.ResumeToken != token && c.ResumeUntil.After(now) {
		   return ErrResumeToken
	   }
	   if c.ResumeCount >= 1 && c.State == HalfResumed {
		   return ErrAlreadyApplied
	  }
	  return nil
   }

// FenceAudit 闸审计。

type FenceAudit struct {
	AuditID string `json:"audit_id"`
	GenID   string `json:"deliver_effect_gen"`
	OutboxID string `json:"outbox_id"`
	Actor    string `json:"actor"`
	Action  string `json:"action"`
	Outcome string `json:"outcome"`
	Detail  string `json:"detail"`
	Generation int64     `json:"generation"`
	At         time.Time `json:"at"`
}

   func (a FenceAudit) Copy() FenceAudit { return a }

// AppliedEffect 已落地业务效果键。第二次必须失败。
type AppliedEffect struct {
	EffectKey  string `json:"effect_key"`
	GenID      string `json:"deliver_effect_gen"`
	OutboxID   string `json:"outbox_id"`
	Generation int64  `json:"generation"`
	AppliedAt time.Time `json:"applied_at"`
}

   func (e AppliedEffect) Copy() AppliedEffect { return e }

// TickReport 带世代的一轮泵结果。
type TickReport struct {
	Delivered int `json:"delivered"`
	Marked int `json:"marked"`
	Resumed int `json:"resumed"`
	Pending int `json:"pending"`
	DupProofs int `json:"dup_proofs"`
	HalfOpened     int `json:"half_opened"`
	HandlerCalls   int `json:"handler_calls"`
	SkippedExpired int `json:"skipped_expired"`
	Err string `json:"error,omitempty"`
	GenIDs []string `json:"deliver_effect_gens,omitempty"`
}

  func (r TickReport) Copy() TickReport {
	   cp := r
	   if r.GenIDs != nil {
		  cp.GenIDs = append([]string(nil), r.GenIDs...)
	  }
	  return cp
   }

const (
	ActionIssueGen     = "issue_gen"
	ActionDeliver      = "deliver"
	ActionMark         = "mark"
	ActionResume       = "resume_mark"
	ActionAbandon      = "abandon_half"
	ActionExpire       = "expire_gen"
	ActionPoisonReplay = "poison_replay"
	ActionDupProof = "dup_proof"
	ActionInspect = "inspect_half"
	ActionExport  = "export_proof"
	ActionLagMark = "lag_mark"
	OutcomeOK = "ok"
	OutcomeDenied = "denied"
	OutcomeDup      = "duplicate"
	OutcomeExpired  = "expired"
	OutcomeStale    = "stale"
	OutcomeMismatch = "mismatch"
	OutcomeIdem     = "idempotent"
	OutcomeHalfOpen = "half_open"
	OutcomeResumed  = "resumed"
	OutcomeAbandoned = "abandoned"
	OutcomeCASFail = "cas_fail"
	OutcomeLagRO   = "lag_readonly"
	ReasonPublishNoMark = "publish_ok_mark_interrupted"
	ReasonMarkNoAck     = "mark_ok_peer_unacked"
	ReasonPoisonSecond  = "poison_replay_second_leg"
	ReasonTickSecond = "tick_second_full_effect"
	DefaultTTL       = 15 * time.Minute
	ResumeHoldTTL    = 30 * time.Second
	DefaultTickLimit = 50
)
