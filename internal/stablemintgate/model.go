package stablemintgate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/stablebr"
)

// LeaseKind 铸或赎。
type LeaseKind string

const (
	KindMint LeaseKind = "mint"
	KindRedeem LeaseKind = "redeem"
)

   func ParseLeaseKind(s string) (LeaseKind, error) {
	   switch LeaseKind(strings.ToLower(strings.TrimSpace(s))) {
	   case KindMint:
		   return KindMint, nil
	  case KindRedeem:
		   return KindRedeem, nil
	   default:
		   return "", fmt.Errorf("%w: kind", ErrInvalidInput)
	  }
  }

   func (k LeaseKind) BridgeKind() stablebr.Kind {
	   if k == KindRedeem {
		  return stablebr.KindRedeem
	   }
	  return stablebr.KindMint
  }

// LeaseState 租约 FSM。
type LeaseState string

const (
	LeaseIssued LeaseState = "issued"
	LeasePosting LeaseState = "posting"
	LeasePosted LeaseState = "posted"
	LeaseConsumed LeaseState = "consumed"
	LeaseExpired LeaseState = "expired"
	LeaseRevoked LeaseState = "revoked"
)

   func (s LeaseState) Terminal() bool {
	   switch s {
	   case LeasePosted,
		  LeaseConsumed, LeaseExpired, LeaseRevoked:
		   return true
	  }
	   return false
   }

  func (s LeaseState) Live() bool {
	   return s == LeaseIssued || s == LeasePosting
   }

// HalfState 半截证明状态。
type HalfState string

const (
	HalfOpen     HalfState = "open"
	HalfResolved HalfState = "resolved"
)

// DefaultMintLeaseTTL 铸赎租约默认窗口。

const DefaultMintLeaseTTL = 8 * time.Minute

// MaxMintLeaseTTL 租约上限，不得超过证明最短 Expiry。
const MaxMintLeaseTTL = 24 * time.Hour

// AttestFingerprint 证明集合指纹。
type AttestFingerprint struct {
	Digest string `json:"digest"`
	Kind string `json:"kind"`
	Asset string `json:"asset"`
	Items  []string  `json:"items"`
	Expiry time.Time `json:"expiry"`
	CheckedAt time.Time `json:"checked_at"`
	Complete bool `json:"complete"`
}

  func FingerprintFrom(fp stablebr.Fingerprint) AttestFingerprint {
	   items := make([]string, 0, len(fp.Items))
	   for _, it := range fp.Items {
		   items = append(items, string(it))
	  }
	  return AttestFingerprint{
		   Digest: fp.Digest, Kind: string(fp.Kind), Asset: fp.Asset,
		  Items: items, Expiry: fp.Expiry, CheckedAt: fp.CheckedAt, Complete: fp.Complete,
	  }
   }

   func (f AttestFingerprint) Valid(now time.Time) bool {
	  if !f.Complete || f.Digest == "" {
		  return false
	   }
	   if f.Expiry.IsZero() || !f.Expiry.After(now) {
		   return false
	   }
	   return true
  }

// MintLease 证明集齐全后签发的铸/赎租约。

type MintLease struct {
	LeaseID string    `json:"mint_lease_id"`
	Kind    LeaseKind `json:"kind"`
	Asset   string    `json:"asset"`
	UserID string `json:"user_id,omitempty"`
	Amount string `json:"amount,omitempty"`
	Fingerprint AttestFingerprint `json:"fingerprint"`
	MintGen     int64             `json:"mint_gen"`
	State       LeaseState        `json:"state"`
	IssuedBy    string            `json:"issued_by"`
	IssuedAt    time.Time         `json:"issued_at"`
	ExpireAt    time.Time `json:"expire_at"`
	Idempotency string    `json:"idempotency_key,omitempty"`
	JournalID   string    `json:"journal_id,omitempty"`
	BoundLedger string     `json:"bound_ledger_id,omitempty"`
	ConsumedAt  *time.Time `json:"consumed_at,omitempty"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	RevokeNote   string     `json:"revoke_note,omitempty"`
	RevokeReason string     `json:"revoke_reason,omitempty"`
}

// IsValid 判断租约是否可进入落账闸。

   func (l *MintLease) IsValid(now time.Time) bool {
	   if l == nil {
		   return false
	   }
	  if l.State != LeaseIssued && l.State != LeasePosting {
		  return false
	  }
	   if now.IsZero() || !now.Before(l.ExpireAt) {
		  return false
	   }
	   return l.Fingerprint.Valid(now)
  }

  func (l *MintLease) Copy() MintLease {
	   if l == nil {
		   return MintLease{}
	   }
	  cp := *l
	  if l.ConsumedAt != nil {
		   t := *l.ConsumedAt
		   cp.ConsumedAt = &t
	   }
	   if l.RevokedAt != nil {
		  t := *l.RevokedAt
		   cp.RevokedAt = &t
	   }
	  if l.Fingerprint.Items != nil {
		  cp.Fingerprint.Items = append([]string(nil), l.Fingerprint.Items...)
	  }
	  return cp
  }

// MintGeneration 按 kind+asset 维护的证明世代。

type MintGeneration struct {
	Scope string `json:"scope"`
	Kind string `json:"kind"`
	Asset   string `json:"asset"`
	MintGen int64  `json:"mint_gen"`
	LastClear string `json:"last_clear_actor,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

   func ScopeKey(kind LeaseKind, asset string) string {
	   return string(kind) + "|" + strings.ToUpper(strings.TrimSpace(asset))
  }

// StableHalfProof Journal↔Ledger 半截证明。

type StableHalfProof struct {
	HalfID string `json:"half_id"`
	LeaseID   string `json:"mint_lease_id"`
	JournalID string `json:"journal_id"`
	IdemKey   string `json:"idempotency_key"`
	Kind  LeaseKind `json:"kind"`
	Asset string    `json:"asset"`
	UserID string `json:"user_id"`
	Amount string    `json:"amount"`
	State  HalfState `json:"state"`
	Cause  string `json:"cause"`
	Digest string `json:"digest"`
	CreatedAt time.Time `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

  func (p *StableHalfProof) ComputeDigest() string {
	   if p == nil {
		   return ""
	  }
	  raw := fmt.Sprintf("shp:%s:%s:%s:%s:%s:%s",
		  p.HalfID, p.LeaseID, p.JournalID, p.IdemKey, string(p.State),
		  p.CreatedAt.UTC().Format(time.RFC3339Nano))
	   sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:16])
  }

   func (p *StableHalfProof) Copy() StableHalfProof {
	   if p == nil {
		   return StableHalfProof{}
	  }
	   cp := *p
	   if p.ResolvedAt != nil {
		   t := *p.ResolvedAt
		  cp.ResolvedAt = &t
	   }
	   return cp
   }

// ProofOutcome 审计/导出证明结果。
type ProofOutcome string

const (
	OutcomeIssued ProofOutcome = "lease_issued"
	OutcomePosted ProofOutcome = "posted"
	OutcomeConsumed ProofOutcome = "consumed"
	OutcomeBlockedNoLease ProofOutcome = "blocked_no_lease"
	OutcomeBlockedClear ProofOutcome = "blocked_clear_attest"
	OutcomeBlockedExpire ProofOutcome = "blocked_attest_expired"
	OutcomeBlockedDrift ProofOutcome = "blocked_fingerprint_drift"
	OutcomeBlockedGen    ProofOutcome = "blocked_gen_mismatch"
	OutcomeBlockedReplay ProofOutcome = "blocked_idem_replay"
	OutcomeBlockedRevoke ProofOutcome = "blocked_revoked"
	OutcomeBlockedLeaseExp ProofOutcome = "blocked_lease_expired"
	OutcomeHalfOpen     ProofOutcome = "half_open"
	OutcomeHalfResolved ProofOutcome = "half_resolved"
)

// MintAudit 运维审计行。
type MintAudit struct {
	AuditID string `json:"audit_id"`
	LeaseID string `json:"mint_lease_id"`
	Scope   string `json:"scope"`
	Actor string `json:"actor"`
	Action string `json:"action"`
	Detail string `json:"detail"`
	Outcome string    `json:"outcome,omitempty"`
	At      time.Time `json:"at"`
}

// DenyProof 拒铸/拒赎不可变证明。

type DenyProof struct {
	ProofID string `json:"proof_id"`
	LeaseID string `json:"mint_lease_id,omitempty"`
	Scope string    `json:"scope"`
	Kind  LeaseKind `json:"kind"`
	Asset string    `json:"asset"`
	Outcome ProofOutcome `json:"outcome"`
	Detail    string    `json:"detail"`
	Digest    string    `json:"digest"`
	CreatedAt time.Time `json:"created_at"`
}

  func (p *DenyProof) ComputeDigest() string {
	  if p == nil {
		  return ""
	   }
	   raw := fmt.Sprintf("smp:%s:%s:%s:%s:%s",
		  p.ProofID, p.LeaseID, string(p.Outcome), p.Scope,
		  p.CreatedAt.UTC().Format(time.RFC3339Nano))
	  sum := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(sum[:16])
  }

// AssertInput 落账前复验入参。

type AssertInput struct {
	LeaseID string
	Kind    string
	Asset   string
	Actor   string
	UserID string
	Amount string
}

// AssertResult 复验结果。

type AssertResult struct {
	Allowed bool
	Lease   *MintLease
	Reason  ProofOutcome
	Detail string
	ProofID string
}

// IssueReq 签发租约入参。
type IssueReq struct {
	Kind  string
	Asset string
	Actor string
	UserID string
	Amount string
	TTL    time.Duration
}

// MintWithLeaseReq 持租约铸/赎。

type MintWithLeaseReq struct {
	LeaseID string `json:"mint_lease_id"`
	Kind    string `json:"kind"`
	UserID  string `json:"user_id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Actor          string `json:"actor"`
	IdempotencyKey string `json:"idempotency_key"`
}
