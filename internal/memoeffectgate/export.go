package memoeffectgate

import (
	"context"
	"sort"
	"time"
)

type ProofExportRow struct {
	ID string `json:"id"`
	TicketID string `json:"ticket_id"`
	Kind ProofKind `json:"kind"`
	State HalfState `json:"state"`
	SilentKeep bool `json:"silent_keep"`
	Blocked bool `json:"blocked"`
	ObservedAmt string    `json:"observed_amount"`
	Print       string    `json:"fingerprint"`
	CreatedAt   time.Time `json:"created_at"`
}

   func (e *Engine) ExportProofView(ctx context.Context, kind ProofKind, limit int) ([]ProofExportRow, error) {
	  if err := e.require(); err != nil {
		   return nil, err
	   }
	  rows, err := e.store.ListProofs(ctx, kind, limit)
	  if err != nil {
		  return nil, err
	   }
	   out := make([]ProofExportRow, 0, len(rows))
	  for _, p := range rows {
		  out = append(out, ProofExportRow{
			  ID: p.ID, TicketID: p.TicketID, Kind: p.Kind, State: p.State,
			  SilentKeep: p.SilentKeep, Blocked: p.Blocked, ObservedAmt: p.ObservedAmt,
			   Print:     ProofFingerprint(p),
			   CreatedAt: p.CreatedAt,
		   })
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	   return out, nil
  }
