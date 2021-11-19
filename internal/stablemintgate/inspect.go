package stablemintgate

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/stablebr"
)

// Diagnosis 复验诊断。

type Diagnosis struct {
	MintLeaseID string   `json:"mint_lease_id,omitempty"`
	Allowed     bool     `json:"allowed"`
	Reasons     []string `json:"reasons,omitempty"`
	Generation  int64    `json:"mint_gen"`
	CurrentGen  int64    `json:"current_gen"`
	Fingerprint string `json:"fingerprint,omitempty"`
	CurrentFP   string `json:"current_fingerprint,omitempty"`
}

// Diagnose 对租约做只读复验，不改状态。
  func (e *Engine) Diagnose(ctx context.Context, leaseID string) (*Diagnosis, error) {
	   if e == nil {
		   return nil, ErrUnavailable
	  }
	   d := &Diagnosis{MintLeaseID: leaseID}
	   lease,
		   err := e.leases.Get(ctx, leaseID)
	   if err != nil {
		   d.Reasons = append(d.Reasons, "lease_not_found")
		   return d, nil
	  }
	  now := e.now()
	   d.Generation = lease.MintGen
	  d.Fingerprint = lease.Fingerprint.Digest
	   g, _ := e.gens.Get(ctx, ScopeKey(lease.Kind, lease.Asset))
	   if g != nil {
		  d.CurrentGen = g.MintGen
	  }
	   fp,
		  _ := e.br.CurrentFingerprint(string(lease.Kind), lease.Asset)
	   d.CurrentFP = fp.Digest
	   if lease.State == LeaseRevoked {
		   d.Reasons = append(d.Reasons, "revoked")
	  }
	   if lease.State == LeaseExpired || !now.Before(lease.ExpireAt) {
		  d.Reasons = append(d.Reasons, "lease_expired")
	   }
	   if lease.State.Terminal() && lease.State != LeasePosted {
		   if lease.State == LeaseConsumed {
			  d.Reasons = append(d.Reasons, "consumed")
		  }
	  }
	  if g != nil && g.MintGen != lease.MintGen {
		   d.Reasons = append(d.Reasons, "generation_mismatch")
	  }
	  if err := e.br.MatchFingerprint(lease.Kind.BridgeKind(), lease.Asset, lease.Fingerprint.Digest); err != nil {
		  d.Reasons = append(d.Reasons,
			  err.Error())
	   }
	  d.Allowed = len(d.Reasons) == 0 && lease.IsValid(now)
	  return d, nil
   }

// ReconstructHalf 按幂等键复现半截证明；不存在则按 Journal 现场补写只读视图。
  func (e *Engine) ReconstructHalf(ctx context.Context, idem string, j *stablebr.Journal) (*StableHalfProof, error) {
	   if e == nil {
		   return nil, ErrUnavailable
	  }
	  if p, err := e.halves.GetByIdem(ctx, idem); err == nil && p != nil {
		  cp := p.Copy()
		  return &cp, nil
	   }
	   if j == nil || j.ID == "" {
		   return nil, ErrHalfNotFound
	  }
	  if p, err := e.halves.GetByJournal(ctx, j.ID); err == nil && p != nil {
		   cp := p.Copy()
		   return &cp, nil
	  }
	  return nil, ErrHalfNotFound
   }

// FailClosedReasons 规范化 fail-closed 原因码。

  func FailClosedReasons() []string {
	   return []string{
		  string(OutcomeBlockedNoLease),
		   string(OutcomeBlockedClear),
		  string(OutcomeBlockedExpire),
		  string(OutcomeBlockedDrift),
		   string(OutcomeBlockedGen),
		  string(OutcomeBlockedReplay),
		  string(OutcomeBlockedRevoke),
		  string(OutcomeBlockedLeaseExp),
		   string(OutcomeHalfOpen),
	   }
   }

// ScopeOfJournal 从 Journal 推导世代键。
  func ScopeOfJournal(j stablebr.Journal) string {
	  return ScopeKey(LeaseKind(j.Kind), j.Asset)
  }

// DescribeError 将哨兵错误映射为证明 outcome。
  func DescribeError(err error) ProofOutcome {
	   if err == nil {
		   return OutcomePosted
	  }
	  switch {
	  case err == ErrNoLease:
		   return OutcomeBlockedNoLease
	   case err == ErrLeaseExpired:
		  return OutcomeBlockedLeaseExp
	  case err == ErrAttestExpired:
		   return OutcomeBlockedExpire
	   case err == ErrFingerprintDrift:
		  return OutcomeBlockedDrift
	   case err == ErrGenMismatch:
		  return OutcomeBlockedGen
	   case err == ErrLeaseRevoked:
		   return OutcomeBlockedRevoke
	   case err == ErrIdemReplay, err == ErrHalfOpen:
		   return OutcomeBlockedReplay
	  default:
		   if strings.Contains(fmt.Sprint(err), "half") {
			   return OutcomeHalfOpen
		  }
		  return OutcomeBlockedDrift
	   }
  }
