package batchhopgate

import (
	"context"
	"fmt"
	"time"
)

// ResidualInspectReport 单次扫描摘要。

type ResidualInspectReport struct {
	At          time.Time `json:"at"`
	OpenCount   int       `json:"open_count"`
	ProvenCount int       `json:"proven_count"`
	ResumingCount int `json:"resuming_count"`
	StaleOpen []string `json:"stale_open,omitempty"`
	DigestFail []string `json:"digest_fail,omitempty"`
	Notes      []string `json:"notes,omitempty"`
}

// Inspector 只读巡检器。

type Inspector struct {
	store Store
	vault *OccupyProofVault
	clock func() time.Time
}

// NewInspector 构造巡检器。
   func NewInspector(store Store) *Inspector {
	   if store == nil {
		   store = NewMemStore()
	   }
	  return &Inspector{
		   store: store,
		   vault: NewOccupyProofVault(store),
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

   func (ins *Inspector) WithClock(c func() time.Time) *Inspector {
	   if ins != nil && c != nil {
		  ins.clock = c
	   }
	   return ins
   }

  // Scan 汇总非终态残留与证明完整性。

  func (ins *Inspector) Scan(ctx context.Context, limit int, staleAfter time.Duration) (*ResidualInspectReport, error) {
	   if ins == nil {
		   return nil, ErrUnavailable
	  }
	  if limit <= 0 {
		  limit = 50
	  }
	  if staleAfter <= 0 {
		   staleAfter = 2 * ResumeLeaseTTL
	  }
	  now := ins.clock()
	   rep := &ResidualInspectReport{At: now}
	  open,
		  err := ins.store.ListResidualsByStatus(ctx, ResidualOpen, limit)
	  if err != nil {
		  return nil, err
	   }
	   rep.OpenCount = len(open)
	  for _, c := range open {
		  if now.Sub(c.DetectedAt) > staleAfter {
			  rep.StaleOpen = append(rep.StaleOpen, c.ID)
			   rep.Notes = append(rep.Notes, fmt.Sprintf("open residual %s older than %s", c.ID, staleAfter))
		  }
	  }
	  proven,
		  err := ins.store.ListResidualsByStatus(ctx, ResidualProven, limit)
	   if err != nil {
		   return nil, err
	  }
	  rep.ProvenCount = len(proven)
	   ins.checkProofs(ctx, proven, rep)
	  resuming, err := ins.store.ListResidualsByStatus(ctx, ResidualResuming, limit)
	   if err != nil {
		  return nil, err
	  }
	   rep.ResumingCount = len(resuming)
	  ins.checkProofs(ctx, resuming, rep)
	  return rep, nil
   }

   func (ins *Inspector) checkProofs(ctx context.Context, cases []*BatchResidualCase, rep *ResidualInspectReport) {
	   for _, c := range cases {
		  if c.ProofID == "" {
			   rep.Notes = append(rep.Notes, fmt.Sprintf("case %s missing proof_id", c.ID))
			   continue
		  }
		  if _, err := ins.vault.Verify(ctx, c.ProofID); err == ErrProofDigestMismatch {
			   rep.DigestFail = append(rep.DigestFail, c.ProofID)
		   }
	   }
   }

// FormatReport 生成运维可读文本（不含外链）。

  func FormatReport(r *ResidualInspectReport) string {
	   if r == nil {
		   return "inspect: empty"
	  }
	   return fmt.Sprintf(
		   "inspect at=%s open=%d proven=%d resuming=%d stale_open=%d digest_fail=%d notes=%d",
		  r.At.UTC().Format(time.RFC3339Nano),
		  r.OpenCount, r.ProvenCount, r.ResumingCount,
		   len(r.StaleOpen),
		   len(r.DigestFail), len(r.Notes),
	  )
  }
