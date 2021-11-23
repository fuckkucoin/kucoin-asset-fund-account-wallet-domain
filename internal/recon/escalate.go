package recon

import (
	"fmt"
	"strings"
	"time"
)

type Escalation struct {
	TicketID string `json:"ticket_id"`
	OpenedBy string `json:"opened_by"`
	Age time.Duration `json:"age"`
}

var ErrEscalateActor = fmt.Errorf("reconcile escalate actor invalid")

// Escalate 列出超 SLA 仍未关的单。只记升级，不得自动结案。
   func (w *Workflow) Escalate(now time.Time, actor string) ([]Escalation, error) {
	   if w == nil {
		  return nil, ErrNotFound
	   }
	   if strings.TrimSpace(actor) == "" || strings.EqualFold(actor, "system") {
		  return nil, ErrEscalateActor
	  }
	   breached := w.Breach(now)
	   out := make([]Escalation, 0, len(breached))
	  for _, t := range breached {
		  out = append(out, Escalation{TicketID: t.ID, OpenedBy: t.OpenedBy, Age: now.Sub(t.CreatedAt)})
	  }
	  return out, nil
   }

// FreezeGate 未关差异挡新冻账，避免在漂移未解时改冻结位。
  func (w *Workflow) FreezeGate(userID string) error {
	  if w != nil && w.BlocksUser(userID) {
		  return fmt.Errorf("reconcile open mismatch blocks freeze")
	  }
	   return nil
  }
