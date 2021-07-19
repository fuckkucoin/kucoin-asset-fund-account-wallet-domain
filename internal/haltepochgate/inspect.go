package haltepochgate

import (
	"context"
	"fmt"
)

// InspectReport 巡检报告。
type InspectReport struct {
	Epochs int `json:"epochs"`
	Mismatched int `json:"lane_mismatch"`
	OpenWindows int `json:"open_windows"`
	OpenProofs int `json:"open_proofs"`
	AllowedStale int `json:"allowed_stale"`
	Notes []string `json:"notes,omitempty"`
}

   func (s *Service) Inspect(ctx context.Context) (*InspectReport, error) {
	   if s == nil || s.store == nil {
		  return nil, ErrStoreUnavailable
	  }
	  rep := &InspectReport{}
	   epochs, err := s.store.ListEpochs(ctx, 200)
	   if err != nil {
		   return nil, err
	   }
	  rep.Epochs = len(epochs)
	  for _, e := range epochs {
		  c := e.LaneConclusion()
		   if !LaneMatchesEpoch(c, e) {
			   rep.Mismatched++
			  rep.Notes = append(rep.Notes, fmt.Sprintf("lane mismatch %s gen=%d", e.Scope.String(), e.Generation))
		   }
		  if e.State.WindowSealed() {
			   rep.OpenWindows++
		  }
	   }
	  open, err := s.store.ListProofs(ctx, SlipOpen, 200)
	  if err != nil {
		   return nil, err
	   }
	   rep.OpenProofs = len(open)
	  allowed, err := s.store.ListAllowedIntents(ctx, 200)
	   if err != nil {
		  return nil, err
	  }
	  for _, in := range allowed {
		   ep, e2 := s.store.GetEpoch(ctx, in.Scope)
		  if e2 != nil {
			  continue
		  }
		   if in.StaleVersus(ep.Generation) || ep.State.WindowSealed() {
			   rep.AllowedStale++
		   }
	  }
	   return rep, nil
  }
