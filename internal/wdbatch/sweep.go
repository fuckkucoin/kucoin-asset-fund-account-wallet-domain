package wdbatch

import (
	"context"
	"errors"
	"time"
	"account-assets.kucoin.net/funding/internal/withdraw"
)

const sweepPath = "wdbatch"

// ReleaseRow 一批清扫里单笔的结果。

type ReleaseRow struct {
	BatchID  string `json:"batch_id"`
	LedgerID string `json:"ledger_id"`
	UserID   string `json:"user_id"`
	Asset string `json:"asset"`
	Action string `json:"action"`
	Alert  string `json:"alert,omitempty"`
}

// SweepRelease 扫描已提交项。无出金服务时整批拒绝，避免绕过闸门解冻。

   func (s *Service) SweepRelease(ctx context.Context, actor string, limit int) ([]ReleaseRow, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	   if _, err := requireActor(actor); err != nil {
		   return nil, err
	   }
	   if s.wd == nil {
		  return nil, ErrUnavailable
	  }
	   if limit <= 0 {
		   limit = 200
	   }
	  if limit > 500 {
		   limit = 500
	   }
	   s.mu.Lock()
	  batches := make([]Batch, 0, len(s.rows))
	   for _, r := range s.rows {
		  batches = append(batches, r.Copy())
	  }
	  s.mu.Unlock()
	  now := time.Now().UTC()
	  out := make([]ReleaseRow, 0)
	  for _, b := range batches {
		  if b.Status != StatusSubmitted {
			   continue
		  }
		   for _, it := range b.Items {
			   if len(out) >= limit {
				   return out, nil
			   }
			  if it.Status != ItemSubmitted || it.LedgerID == "" {
				  continue
			  }
			   row := ReleaseRow{BatchID: b.ID,
				  LedgerID: it.LedgerID, UserID: it.UserID, Asset: it.Asset}
			   if err := s.wd.ProbeAutoUnfreeze(it.LedgerID, sweepPath); err != nil {
				   row.Action = releaseAction(err)
				   if errors.Is(err, withdraw.ErrLiveHashUnfreeze) {
					   row.Alert = withdraw.AlertLiveHashDenied
				  }
				   out = append(out, row)
				   continue
			  }
			  st, err := s.wd.RecoverStuck(it.LedgerID, now)
			   if err != nil {
				  row.Action = releaseAction(err)
				   if errors.Is(err,
					   withdraw.ErrLiveHashUnfreeze) {
					  row.Alert = withdraw.AlertLiveHashDenied
				   }
				  out = append(out, row)
				   continue
			  }
			   if st.Action == withdraw.StuckRecover {
				  row.Action = "released"
			  } else {
				  row.Action = string(st.Action)
			   }
			   out = append(out, row)
		   }
	   }
	   return out, nil
  }

   func releaseAction(err error) string {
	  switch {
	   case errors.Is(err, withdraw.ErrLiveHashUnfreeze):
		   return "refused_live_hash"
	  case errors.Is(err, withdraw.ErrAbuseInFlight):
		  return "refused_abuse"
	  default:
		   if err == nil {
			  return "skipped"
		  }
		  return "refused"
	  }
   }
