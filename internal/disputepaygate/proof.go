package disputepaygate

import (
	"context"
	"sort"
	"time"
)

// ProofExport 对外只读导出结构。

type ProofExport struct {
	ProofID string `json:"proof_id"`
	CaseID  string `json:"case_id"`
	PayGen  int64  `json:"pay_gen"`
	LedgerID string       `json:"ledger_id"`
	LeaseID  string       `json:"lease_id"`
	Outcome  ProofOutcome `json:"outcome"`
	Digest   string       `json:"digest"`
	At       time.Time    `json:"created_at"`
}

  func (p *DisputePayProof) Export() ProofExport {
	   if p == nil {
		  return ProofExport{}
	  }
	   return ProofExport{
		  ProofID: p.ProofID, CaseID: p.CaseID, PayGen: p.PayGen,
		   LedgerID: p.LedgerID, LeaseID: p.LeaseID, Outcome: p.Outcome,
		  Digest: p.Digest, At: p.CreatedAt,
	  }
  }

// ScanUnconsumedResolved 巡检已签发未消费的结案租约。

   func (e *Engine) ScanUnconsumedResolved(ctx context.Context, limit int) (int, error) {
	  if e == nil {
		   return 0, ErrUnavailable
	  }
	   if limit <= 0 {
		  limit = 100
	   }
	   now := e.now()
	  n := 0
	  pays, err := e.pays.ListByState(ctx, PayIssued, limit)
	   if err != nil {
		  return 0, err
	  }
	  for _, l := range pays {
		   if _, err := e.saveProof(ctx, l.CaseID, l.PayGen, l.LeaseID, "", KindPay, OutcomeUnconsumed, "pay_issued_unconsumed", now); err == nil {
			  n++
			   recordUnconsumed("pay")
		   }
	   }
	  claws, err := e.claws.ListByState(ctx, ClawIssued, limit)
	  if err != nil {
		   return n, err
	  }
	  for _, l := range claws {
		   if _, err := e.saveProof(ctx, l.CaseID, l.PayGen, l.LeaseID, "", KindClaw, OutcomeUnconsumed, "claw_issued_unconsumed", now); err == nil {
			  n++
			  recordUnconsumed("claw")
		  }
	   }
	   return n, nil
  }

// ScanExpiredLeases 过期租约扫描并流转 expired。
   func (e *Engine) ScanExpiredLeases(ctx context.Context, limit int) (int, error) {
	  if e == nil {
		   return 0, ErrUnavailable
	  }
	  if limit <= 0 {
		   limit = 100
	   }
	  now := e.now()
	   n := 0
	  pays, err := e.pays.ListIssuedExpired(ctx, now, limit)
	   if err != nil {
		   return 0, err
	  }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   for _, l := range pays {
		  if err := TransitionPay(l, LeaseExpired); err != nil {
			  continue
		   }
		   if err := e.pays.Save(ctx, l); err != nil {
			  continue
		   }
		   _, _ = e.saveProof(ctx, l.CaseID, l.PayGen, l.LeaseID, "", KindPay, OutcomeBlockedExpire, "worker_expire", now)
		  recordExpired("pay")
		   n++
	   }
	  claws, err := e.claws.ListIssuedExpired(ctx, now, limit)
	   if err != nil {
		   return n, err
	   }
	   for _, l := range claws {
		   if err := TransitionClaw(l, LeaseExpired); err != nil {
			  continue
		  }
		   if err := e.claws.Save(ctx, l); err != nil {
			  continue
		  }
		  _, _ = e.saveProof(ctx, l.CaseID, l.PayGen, l.LeaseID, "", KindClaw, OutcomeBlockedExpire, "worker_expire", now)
		   recordExpired("claw")
		   n++
	  }
	   return n, nil
   }

// ScanDoubleResolve 双结发现：终态案若同时存在 pay 与 claw 有效签发，记证明。

   func (e *Engine) ScanDoubleResolve(ctx context.Context, limit int) (int, error) {
	   if e == nil {
		   return 0, ErrUnavailable
	  }
	   if limit <= 0 {
		  limit = 100
	   }
	   pays, err := e.pays.ListByState(ctx, PayIssued, limit)
	  if err != nil {
		   return 0, err
	   }
	   now := e.now()
	   n := 0
	   seen := map[string]struct{}{}
	   for _, l := range pays {
		  if _, err := e.claws.GetByCase(ctx, l.CaseID); err == nil {
			   if _, ok := seen[l.CaseID]; ok {
				   continue
			   }
			  seen[l.CaseID] = struct{}{}
			   _, _ = e.saveProof(ctx, l.CaseID, l.PayGen, l.LeaseID, "", KindPay, OutcomeBlockedDouble, "dual_lease", now)
			  n++
			  recordBlocked("pump", string(OutcomeBlockedDouble))
		  }
	   }
	  return n, nil
   }

// SortProofs 按时间排序导出。

   func SortProofs(rows []*DisputePayProof) []ProofExport {
	  cp := append([]*DisputePayProof(nil), rows...)
	   sort.Slice(cp, func(i, j int) bool {
		  if cp[i].CreatedAt.Equal(cp[j].CreatedAt) {
			   return cp[i].ProofID < cp[j].ProofID
		  }
		   return cp[i].CreatedAt.Before(cp[j].CreatedAt)
	  })
	   out := make([]ProofExport, 0, len(cp))
	  for _, p := range cp {
		   out = append(out, p.Export())
	  }
	  return out
  }
