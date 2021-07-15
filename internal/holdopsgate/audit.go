package holdopsgate

import (
	"context"
	"sync"
	"time"
)

type AuditLog struct {
	mu sync.Mutex
	ring []AuditEvent
	cap  int
}

type AuditEvent struct {
	Kind    string
	LeaseID string
	OpsID   string
	UserID string
	ProofID string
	Note    string
	At time.Time
}

   func NewAuditLog(n int) *AuditLog {
	   if n <= 0 {
		  n = 1024
	   }
	  return &AuditLog{cap: n,
		   ring: make([]AuditEvent, 0, n)}
   }

   func (a *AuditLog) Append(ev AuditEvent) {
	   if a == nil {
		   return
	  }
	   a.mu.Lock()
	  defer a.mu.Unlock()
	   if len(a.ring) >= a.cap {
		   a.ring = a.ring[1:]
	  }
	  a.ring = append(a.ring, ev)
  }

  func (a *AuditLog) Snapshot() []AuditEvent {
	  if a == nil {
		  return nil
	  }
	  a.mu.Lock()
	   defer a.mu.Unlock()
	   out := make([]AuditEvent, len(a.ring))
	  copy(out, a.ring)
	   return out
  }

type PersistAuditor struct {
	store  Store
	secret string
	clock  func() time.Time
}

   func newPersistAuditor(store Store, secret string, clock func() time.Time) *PersistAuditor {
	   return &PersistAuditor{store: store, secret: secret, clock: clock}
   }

  func (a *PersistAuditor) Write(ctx context.Context, lease HoldOpsLease, outcome, proofID, note string) (HoldOpsAudit, error) {
	   now := a.clock()
	  row := HoldOpsAudit{
		  OpsID:   lease.OpsID,
		  LeaseID: lease.ID,
		  TokenID: lease.TokenID,
		  GapID: lease.GapID,
		   Gen:   lease.OpsGen,
		   Actor: lease.Actor,
		   Kind: lease.Kind,
		   Outcome: outcome,
		   ProofID: proofID,
		   Note:    note,
		  At:        now,
		   ReplayKey: replayKey(lease.OpsID, lease.TokenID, genString(lease.OpsGen), lease.Kind, lease.Actor+"|"+outcome),
	  }
	  row.ID = newAuditID(a.secret, row.OpsID, outcome, now.UTC().Format(time.RFC3339Nano))
	   if err := a.store.PutAudit(ctx, row); err != nil {
		   if err == ErrReplayAudit {
			   if prev, ok, e := a.store.GetAuditByReplay(ctx, row.ReplayKey); e == nil && ok {
				   return prev, nil
			   }
		   }
		  return HoldOpsAudit{}, err
	   }
	  return row, nil
   }
