package holdwritegate

import (
	"context"
	"fmt"
	"time"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/workerloop"
)

// ──────────────────────────────────────────
// AdviceEscalateScan 建议升级扫描
// ──────────────────────────────────────────

type AdviceEscalateScanConfig struct {
	Engine    *Engine
	BatchSize int
}

   func AdviceEscalateScanJob(cfg AdviceEscalateScanConfig) workerloop.Job {
	  if cfg.BatchSize <= 0 {
		   cfg.BatchSize = 50
	  }
	  return func(ctx context.Context) error {
		   if cfg.Engine == nil {
			   return ErrWorkerDisabled
		  }
		  openEscs, err := cfg.Engine.store.ListEscalationsByState(ctx, EscalationOpen, cfg.BatchSize)
		   if err != nil {
			  return err
		  }
		  now := cfg.Engine.clock()
		   for _, esc := range openEscs {
			  lease, err := cfg.Engine.store.GetBlockLease(ctx, esc.LeaseID)
			  if err != nil {
				   continue
			  }
			   if lease.State == BlockLeaseExpired || lease.State == BlockLeaseCleared {
				  updated := esc
				   updated.State = EscalationClosed
				   clNow := now
				   updated.ClosedAt = &clNow
				  updated.Resolution = "lease_terminated"
				  updated.UpdatedAt = now
				  _, _ = cfg.Engine.store.SaveEscalation(ctx, updated)
			   }
		   }
		  return nil
	  }
  }

   func AdviceEscalateScanLoop(ctx context.Context, log zerolog.Logger, cfg AdviceEscalateScanConfig) error {
	  return workerloop.Run(ctx, workerloop.Config{
		  Name:     "holdwritegate-advice-escalate-scan",
		  Interval: 10 * time.Second,
	  }, log, AdviceEscalateScanJob(cfg))
   }

// ──────────────────────────────────────────
// BlockExpiry 过期 block 扫描
// ──────────────────────────────────────────

type BlockExpiryConfig struct {
	Engine    *Engine
	BatchSize int
}

  func BlockExpiryJob(cfg BlockExpiryConfig) workerloop.Job {
	  if cfg.BatchSize <= 0 {
		  cfg.BatchSize = 50
	  }
	   return func(ctx context.Context) error {
		  if cfg.Engine == nil {
			  return ErrWorkerDisabled
		  }
		   now := cfg.Engine.clock()
		  expired, err := cfg.Engine.store.ListExpiredLeases(ctx, now, cfg.BatchSize)
		  if err != nil {
			   return err
		  }
		  for _, lease := range expired {
			  updated := lease
			   updated.State = BlockLeaseExpired
			   eNow := now
			  updated.ExpiredAt = &eNow
			  updated.UpdatedAt = now
			  _, _ = cfg.Engine.store.SaveBlockLease(ctx, updated)
			   escs, _ := cfg.Engine.store.ListEscalationsByLease(ctx, lease.ID, 100)
			   for _, esc := range escs {
				   if esc.State == EscalationOpen {
					   esc.State = EscalationClosed
					   clNow := now
					   esc.ClosedAt = &clNow
					  esc.Resolution = fmt.Sprintf("block_expired: gen=%d", lease.BlockGen)
					   esc.UpdatedAt = now
					  _, _ = cfg.Engine.store.SaveEscalation(ctx, esc)
					  timeoutEsc := AdviceEscalation{
						  ID:      uuid.NewString(),
						  LeaseID: lease.ID,
						   UserID:  lease.UserID,
						  Asset:    lease.Asset,
						  Book:     lease.Book,
						   AdviceID: esc.AdviceID,
						   Kind:     EscalationTimeoutReopen,
						  State:    EscalationOpen,
						  BlockGen: lease.BlockGen,
						  Detail: fmt.Sprintf("timeout reopen from expired lease %s", lease.ID),
						   CreatedAt: now,
						   UpdatedAt: now,
					  }
					   _,
						   _ = cfg.Engine.store.SaveEscalation(ctx, timeoutEsc)
				   }
			  }
		  }
		  return nil
	  }
   }

   func BlockExpiryLoop(ctx context.Context, log zerolog.Logger, cfg BlockExpiryConfig) error {
	  return workerloop.Run(ctx, workerloop.Config{
		   Name: "holdwritegate-block-expiry",
		  Interval: 15 * time.Second,
	   }, log, BlockExpiryJob(cfg))
   }

// ──────────────────────────────────────────
// GapCloseVerify Gap 关闭校验
// ──────────────────────────────────────────

type GapCloseVerifyConfig struct {
	Engine *Engine
	BatchSize int
}

   func GapCloseVerifyJob(cfg GapCloseVerifyConfig) workerloop.Job {
	   if cfg.BatchSize <= 0 {
		  cfg.BatchSize = 50
	  }
	   return func(ctx context.Context) error {
		   if cfg.Engine == nil || cfg.Engine.gapQuerier == nil {
			   return ErrWorkerDisabled
		  }
		   now := cfg.Engine.clock()
		  blockingLeases, err := cfg.Engine.store.ListBlockingLeases(ctx, cfg.BatchSize)
		  if err != nil {
			   return err
		  }
		   for _, lease := range blockingLeases {
			  gapAmount, gapOpen, err := cfg.Engine.gapQuerier.QueryGap(ctx, lease.UserID, lease.Asset, lease.Book)
			   if err != nil {
				  continue
			  }
			   if gapOpen {
				   continue
			  }
			  proofs, _ := cfg.Engine.store.ListProofsByLease(ctx, lease.ID, 10)
			   for _, p := range proofs {
				  if !p.Validated && p.GapClosed {
					   p.Validated = true
					  vNow := now
					   p.ValidatedAt = &vNow
					   p.ValidatedBy = "gap_close_verify_worker"
					  _, _ = cfg.Engine.store.SaveCloseProof(ctx, p)
				  }
			  }
			  _ = gapAmount
			   escs, _ := cfg.Engine.store.ListEscalationsByLease(ctx, lease.ID, 100)
			  for _, esc := range escs {
				  if esc.State == EscalationOpen {
					  esc.State = EscalationProven
					  pNow := now
					  esc.ProvenAt = &pNow
					   esc.UpdatedAt = now
					   _, _ = cfg.Engine.store.SaveEscalation(ctx, esc)
				  }
			   }
		   }
		   return nil
	  }
  }

  func GapCloseVerifyLoop(ctx context.Context, log zerolog.Logger, cfg GapCloseVerifyConfig) error {
	   return workerloop.Run(ctx, workerloop.Config{
		   Name: "holdwritegate-gap-close-verify",
		  Interval: 20 * time.Second,
	  }, log, GapCloseVerifyJob(cfg))
   }

// ──────────────────────────────────────────
// EscalationProofPatrol 升级证明巡检
// ──────────────────────────────────────────

type EscalationProofPatrolConfig struct {
	Engine *Engine
	BatchSize int
}

  func EscalationProofPatrolJob(cfg EscalationProofPatrolConfig) workerloop.Job {
	   if cfg.BatchSize <= 0 {
		  cfg.BatchSize = 50
	  }
	   return func(ctx context.Context) error {
		  if cfg.Engine == nil {
			   return ErrWorkerDisabled
		  }
		   now := cfg.Engine.clock()
		   provenEscs, err := cfg.Engine.store.ListEscalationsByState(ctx, EscalationProven, cfg.BatchSize)
		  if err != nil {
			   return err
		  }
		   for _, esc := range provenEscs {
			  allSealed := true
			  for _, pid := range esc.ProofIDs {
				  p, err := cfg.Engine.store.GetCloseProof(ctx, pid)
				  if err != nil {
					   allSealed = false
					   break
				  }
				  if !p.Sealed {
					  p.Sealed = true
					   sNow := now
					   p.SealedAt = &sNow
					   _, _ = cfg.Engine.store.SaveCloseProof(ctx, p)
				  }
			   }
			  _ = allSealed
		   }
		   return nil
	  }
   }

  func EscalationProofPatrolLoop(ctx context.Context, log zerolog.Logger, cfg EscalationProofPatrolConfig) error {
	   return workerloop.Run(ctx, workerloop.Config{
		  Name:     "holdwritegate-escalation-proof-patrol",
		  Interval: 30 * time.Second,
	  }, log, EscalationProofPatrolJob(cfg))
  }
