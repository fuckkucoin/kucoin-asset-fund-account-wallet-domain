package disputepaygate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// LeaseKind 租约种类：放款或扣回。
type LeaseKind string

const (
	KindPay LeaseKind = "pay"
	KindClaw LeaseKind = "claw"
)

// LeaseState 结案租约 FSM。
type LeaseState string

const (
	CaseResolved LeaseState = "resolved"
	PayIssued    LeaseState = "pay_issued"
	PayConsumed  LeaseState = "pay_consumed"
	ClawIssued   LeaseState = "claw_issued"
	ClawConsumed LeaseState = "claw_consumed"
	LeaseRevoked LeaseState = "revoked"
	LeaseExpired LeaseState = "expired"
)

  func (s LeaseState) IsTerminal() bool {
	   switch s {
	  case PayConsumed, ClawConsumed, LeaseRevoked, LeaseExpired:
		   return true
	  }
	   return false
  }

  func (s LeaseState) Issued() bool {
	  return s == PayIssued || s == ClawIssued
   }

// DefaultPayLeaseTTL 放款租约默认窗口。
const DefaultPayLeaseTTL = 10 * time.Minute

// DefaultClawLeaseTTL 扣回租约默认窗口。

const DefaultClawLeaseTTL = 15 * time.Minute

// DisputePayLease 用户放款租约。
// 功能：Resolve(uphold/resolve) 触发放款时签发；账本贷记必须携带未过期租约。

type DisputePayLease struct {
	LeaseID     string `json:"dispute_pay_lease_id"`
	CaseID      string `json:"case_id"`
	Idempotency string `json:"idempotency_key"`
	UserID      string `json:"user_id"`
	Amount      string `json:"amount"`
	Asset        string `json:"asset"`
	EvidenceRoot string `json:"evidence_root"`
	Decision     string `json:"decision"`
	PayGen       int64  `json:"pay_gen"`
	State LeaseState `json:"state"`
	IssuedBy string    `json:"issued_by"`
	IssuedAt time.Time `json:"issued_at"`
	ExpireAt    time.Time  `json:"expire_at"`
	BoundLedger string     `json:"bound_ledger_id,omitempty"`
	ConsumedAt  *time.Time `json:"consumed_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	RevokeNote  string     `json:"revoke_note,omitempty"`
}

// IsValid 判断放款租约是否可被账本复验。
   func (l *DisputePayLease) IsValid(now time.Time) bool {
	  if l == nil {
		  return false
	  }
	  if l.State != PayIssued && l.State != CaseResolved {
		  return false
	   }
	   return now.Before(l.ExpireAt)
  }

// ClawLease 占用释放或扣回租约。
type ClawLease struct {
	LeaseID string `json:"claw_lease_id"`
	CaseID string `json:"case_id"`
	Idempotency string `json:"idempotency_key"`
	UserID string `json:"user_id"`
	Amount       string `json:"amount"`
	Asset        string `json:"asset"`
	EvidenceRoot string `json:"evidence_root"`
	Decision   string     `json:"decision"`
	PayGen     int64      `json:"pay_gen"`
	State      LeaseState `json:"state"`
	ReleaseRef string     `json:"release_ref,omitempty"`
	IssuedBy string    `json:"issued_by"`
	IssuedAt time.Time `json:"issued_at"`
	ExpireAt time.Time `json:"expire_at"`
	BoundLedger string `json:"bound_ledger_id,omitempty"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	RevokeNote string     `json:"revoke_note,omitempty"`
}

  func (l *ClawLease) IsValid(now time.Time) bool {
	   if l == nil {
		   return false
	   }
	   if l.State != ClawIssued && l.State != CaseResolved {
		  return false
	  }
	   return now.Before(l.ExpireAt)
  }

// ProofOutcome 证明结果。
type ProofOutcome string

const (
	OutcomePaid ProofOutcome = "paid"
	OutcomeClawed ProofOutcome = "clawed"
	OutcomeBlockedNoLease ProofOutcome = "blocked_no_lease"
	OutcomeBlockedDrift   ProofOutcome = "blocked_evidence_drift"
	OutcomeBlockedGen ProofOutcome = "blocked_gen_mismatch"
	OutcomeBlockedReplay ProofOutcome = "blocked_replay"
	OutcomeBlockedDouble ProofOutcome = "blocked_double_resolve"
	OutcomeBlockedExpire ProofOutcome = "blocked_expired"
	OutcomeBlockedRevoke ProofOutcome = "blocked_revoked"
	OutcomeUnconsumed ProofOutcome = "unconsumed_resolved"
)

// DisputePayProof 结案放款/拒放不可变证明。

type DisputePayProof struct {
	ProofID  string `json:"proof_id"`
	CaseID   string `json:"case_id"`
	PayGen   int64  `json:"pay_gen"`
	LedgerID string `json:"ledger_id,omitempty"`
	LeaseID  string `json:"lease_id,omitempty"`
	Kind      LeaseKind    `json:"kind"`
	Outcome   ProofOutcome `json:"outcome"`
	Detail    string       `json:"detail"`
	Digest    string       `json:"digest"`
	CreatedAt time.Time    `json:"created_at"`
}

  func (p *DisputePayProof) ComputeDigest() string {
	  if p == nil {
		  return ""
	   }
	   raw := fmt.Sprintf("dpp:%s:%s:%d:%s:%s:%s",
		  p.ProofID, p.CaseID, p.PayGen, p.LedgerID, string(p.Outcome),
		  p.CreatedAt.UTC().Format(time.RFC3339Nano))
	   sum := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(sum[:16])
  }

// CaseGeneration 与 desk 一致的世代镜像。
type CaseGeneration struct {
	CaseID string `json:"case_id"`
	PayGen int64  `json:"pay_gen"`
	EvidenceRoot string `json:"evidence_root"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AuditRow 运维审计行。
type AuditRow struct {
	AuditID string `json:"audit_id"`
	CaseID  string `json:"case_id"`
	LeaseID string `json:"lease_id"`
	Actor   string `json:"actor"`
	Action string    `json:"action"`
	Detail string    `json:"detail"`
	At     time.Time `json:"at"`
}

  func NormalizeDecision(raw string) (string, LeaseKind, error) {
	   d := strings.ToLower(strings.TrimSpace(raw))
	  switch d {
	  case "resolve", "close", "uphold", "":
		  if d == "" {
			   d = "resolve"
		  }
		  return d, KindPay, nil
	  case "reject", "deny":
		   return d, KindClaw, nil
	  default:
		   return "", "", fmt.Errorf("%w: decision", ErrInvalidInput)
	  }
   }

   func ProofIDFor(caseID string, payGen int64, outcome ProofOutcome, extra string) string {
	  raw := fmt.Sprintf("disputepaygate:proof:%s:%d:%s:%s",
		  strings.TrimSpace(caseID), payGen, outcome, strings.TrimSpace(extra))
	  sum := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(sum[:16])
  }
