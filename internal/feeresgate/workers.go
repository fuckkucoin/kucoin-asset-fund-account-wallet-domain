package feeresgate

import (
	"context"
	"time"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/feereserve"
	"account-assets.kucoin.net/funding/internal/workerloop"
)

// ──────────────────────────────────────────
// TerminalGapPatrol 巡检逾期未结清的预留租约
// ──────────────────────────────────────────

type TerminalGapPatrolConfig struct {
	Engine *Engine
	GapThreshold time.Duration
	BatchSize int
}

  func TerminalGapPatrolJob(cfg TerminalGapPatrolConfig) workerloop.Job {
	   if cfg.GapThreshold <= 0 {
		  cfg.GapThreshold = 30 * time.Minute
	   }
	   if cfg.BatchSize <= 0 {
		  cfg.BatchSize = 50
	  }
	  return func(ctx context.Context) error {
		  if cfg.Engine == nil {
			  return ErrWorkerDisabled
		   }
		   cutoff := cfg.Engine.clock().Add(-cfg.GapThreshold)
		  expired, err := cfg.Engine.store.ListExpiredLeases(ctx, cutoff, cfg.BatchSize)
		   if err != nil {
			   return err
		  }
		  for _, lease := range expired {
			   now := cfg.Engine.clock()
			  rc := feereserve.ReserveCase{
				   ID: uuid.NewString(),
				  WithdrawID: lease.WithdrawID,
				  LeaseID:    lease.ID,
				   Kind:       feereserve.CaseTerminalGap,
				   State:  feereserve.CaseOpen,
				   Detail: "lease_expired: expiry=" + lease.LeaseExpiry.Format(time.RFC3339) + " gap_threshold=" + cfg.GapThreshold.String(),
				  CreatedAt: now,
				   UpdatedAt: now,
			  }
			   _,
				   _ = cfg.Engine.store.SaveCase(ctx, rc)
			  _, _ = cfg.Engine.Block(ctx, feereserve.BlockReq{
				   WithdrawID: lease.WithdrawID,
				  LeaseID: lease.ID,
				  Reason:  "terminal_gap_expired",
			  })
		  }
		   return nil
	   }
   }

// TerminalGapPatrolLoop 以 workerloop 方式运行巡检。
   func TerminalGapPatrolLoop(ctx context.Context, log zerolog.Logger, cfg TerminalGapPatrolConfig) error {
	  return workerloop.Run(ctx, workerloop.Config{
		  Name: "feeresgate-terminal-gap",
		  Interval: 10 * time.Second,
	   }, log, TerminalGapPatrolJob(cfg))
   }

// ──────────────────────────────────────────
// UnderchargeScan 欠费扫描：结清后 actual < reserved 但 case 仍 open
// ──────────────────────────────────────────

type UnderchargeScanConfig struct {
	Engine *Engine
	BatchSize int
}

  func UnderchargeScanJob(cfg UnderchargeScanConfig) workerloop.Job {
	   if cfg.BatchSize <= 0 {
		   cfg.BatchSize = 50
	  }
	  return func(ctx context.Context) error {
		  if cfg.Engine == nil {
			   return ErrWorkerDisabled
		  }
		   cases, err := cfg.Engine.store.ListCasesByState(ctx, feereserve.CaseOpen, cfg.BatchSize)
		   if err != nil {
			   return err
		  }
		   for _, rc := range cases {
			   if rc.Kind != feereserve.CaseUndercharge {
				  continue
			  }
			   proofs, err := cfg.Engine.store.ListProofsByLease(ctx, rc.LeaseID, 10)
			   if err != nil {
				  continue
			   }
			  allSealed := len(proofs) > 0
			  for _, p := range proofs {
				  if !p.Sealed {
					  allSealed = false
					   break
				   }
			   }
			  if allSealed {
				  now := cfg.Engine.clock()
				  rc.State = feereserve.CaseProven
				  rc.ProvenAt = &now
				   rc.UpdatedAt = now
				  ids := make([]string, 0, len(proofs))
				   for _, p := range proofs {
					   ids = append(ids, p.ID)
				   }
				  rc.ProofIDs = ids
				  _, _ = cfg.Engine.store.SaveCase(ctx, rc)
			   }
		   }
		   return nil
	  }
  }

  func UnderchargeScanLoop(ctx context.Context, log zerolog.Logger, cfg UnderchargeScanConfig) error {
	  return workerloop.Run(ctx, workerloop.Config{
		   Name: "feeresgate-undercharge-scan",
		   Interval: 15 * time.Second,
	  }, log, UnderchargeScanJob(cfg))
  }

// ──────────────────────────────────────────
// ProofSealWorker 自动封口：已 proven 的 case 关联的所有 proof 都封口
// ──────────────────────────────────────────

type ProofSealConfig struct {
	Engine *Engine
	BatchSize int
}

   func ProofSealJob(cfg ProofSealConfig) workerloop.Job {
	  if cfg.BatchSize <= 0 {
		  cfg.BatchSize = 50
	  }
	   return func(ctx context.Context) error {
		   if cfg.Engine == nil {
			  return ErrWorkerDisabled
		  }
		   cases, err := cfg.Engine.store.ListCasesByState(ctx, feereserve.CaseProven, cfg.BatchSize)
		  if err != nil {
			  return err
		  }
		   for _, rc := range cases {
			  for _, pid := range rc.ProofIDs {
				   _, _ = cfg.Engine.SealProof(ctx, pid)
			   }
		   }
		   return nil
	   }
  }

  func ProofSealLoop(ctx context.Context, log zerolog.Logger, cfg ProofSealConfig) error {
	   return workerloop.Run(ctx, workerloop.Config{
		   Name: "feeresgate-proof-seal",
		  Interval: 20 * time.Second,
	   }, log, ProofSealJob(cfg))
  }
