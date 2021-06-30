package fundmesh

import (
	"sync"
	"time"
)

type MemOutboundProofRepo struct {
	mu sync.Mutex
	rows []OutboundProof
}

   func NewMemOutboundProofRepo() *MemOutboundProofRepo {
	  return &MemOutboundProofRepo{}
  }

   func (r *MemOutboundProofRepo) Put(p OutboundProof) {
	   if r == nil {
		  return
	   }
	  if p.At.IsZero() {
		  p.At = time.Now().UTC()
	  }
	  if p.ProofID == "" {
		   p.ProofID = "op-" + p.Kind + "-" + p.IdempotencyKey
		   if len(p.ProofID) > 80 {
			  p.ProofID = p.ProofID[:80]
		   }
	  }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	   r.rows = append(r.rows, p)
  }

  func (r *MemOutboundProofRepo) List() []OutboundProof {
	   if r == nil {
		  return nil
	  }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  out := make([]OutboundProof, len(r.rows))
	   copy(out, r.rows)
	   return out
  }

   func (e *Engine) noteOutboundProof(kind, wd, key, fp, phase, code, detail string, attempt int, blocked bool) {
	   if e == nil || e.outProofs == nil {
		   return
	  }
	   e.outProofs.Put(OutboundProof{
		  Kind:       kind,
		  WithdrawID: wd,
		  IdempotencyKey: key,
		  Fingerprint:    fp,
		   PriorPhase: phase,
		  PriorCode:  code,
		   DebitAttempt: attempt,
		   Blocked:      blocked,
		  Detail:       detail,
		  At:           e.nowUTC(),
	  })
   }

  func (e *Engine) ExportOutboundProofs() map[string]any {
	   if e == nil || e.outProofs == nil {
		   return map[string]any{"proofs": []OutboundProof{}, "sum": map[string]int{}}
	   }
	  rows := e.outProofs.List()
	  return map[string]any{
		   "proofs": rows,
		   "sum": SummarizeOutboundProofs(rows),
	   }
   }
