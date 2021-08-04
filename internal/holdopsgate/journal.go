package holdopsgate

import (
	"context"
	"sort"
	"time"
)

type JournalEntry struct {
	At      time.Time `json:"at"`
	Source  string    `json:"source"`
	OpsID   string    `json:"ops_id"`
	Outcome string    `json:"outcome"`
	ProofID string `json:"proof_id,omitempty"`
	Note    string `json:"note,omitempty"`
}

   func BuildJournal(ctx context.Context, store Store, opsID string) ([]JournalEntry, error) {
	  if store == nil {
		  return nil, ErrUnavailable
	   }
	  aud, err := store.ListAuditByOps(ctx, opsID)
	   if err != nil {
		  return nil, err
	   }
	   out := make([]JournalEntry, 0, len(aud)+4)
	  for _, a := range aud {
		   out = append(out, JournalEntry{
			  At: a.At, Source: "audit", OpsID: a.OpsID, Outcome: a.Outcome, ProofID: a.ProofID, Note: a.Note,
		   })
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	  return out, nil
  }
