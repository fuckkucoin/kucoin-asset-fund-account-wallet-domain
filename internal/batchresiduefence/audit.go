package batchresiduefence

import "context"

   func (e *Engine) ListAudits(ctx context.Context, batchID string, limit int) ([]AuditEvent, error) {
	   if err := e.require(); err != nil {
		  return nil, err
	  }
	   id, err := requireBatchID(batchID)
	   if err != nil {
		  return nil, err
	  }
	   return e.store.ListAudits(ctx, id, limit)
  }

   func FilterAudits(rows []AuditEvent, ev string) []AuditEvent {
	   if ev == "" {
		   return rows
	   }
	  out := make([]AuditEvent, 0)
	  for _, a := range rows {
		   if a.Event == ev {
			   out = append(out, a)
		   }
	   }
	   return out
   }

  func CountEvent(rows []AuditEvent, ev string) int {
	  n := 0
	  for _, a := range rows {
		   if a.Event == ev {
			   n++
		  }
	  }
	  return n
   }

  func HasDupAudit(rows []AuditEvent) bool {
	   return CountEvent(rows, AuditDupProof) > 0
  }

  func LastAudit(rows []AuditEvent) (AuditEvent, bool) {
	  if len(rows) == 0 {
		  return AuditEvent{}, false
	   }
	  return rows[0], true
  }
