package memoeffectgate

import (
	"context"
	"time"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/workerloop"
)

type WorkerConfig struct {
	Interval time.Duration
	BatchSize int
}

  func NewHalfCutPatrolJob(eng *Engine, cfg WorkerConfig, log zerolog.Logger) workerloop.Job {
	  if cfg.BatchSize <= 0 {
		   cfg.BatchSize = DefaultPatrolBatch
	  }
	  return func(ctx context.Context) error {
		   if eng == nil {
			  return ErrNilEngine
		   }
		   n,
			  err := eng.PatrolHalfCut(ctx, cfg.BatchSize)
		   if err != nil {
			   return err
		   }
		  if n > 0 {
			  log.Info().Int("resumed", n).Msg("memoeffectgate 半切巡检续跑")
		  }
		  return nil
	   }
  }

  func NewRejectAfterCreditPatrolJob(eng *Engine, cfg WorkerConfig, log zerolog.Logger) workerloop.Job {
	  if cfg.BatchSize <= 0 {
		  cfg.BatchSize = DefaultPatrolBatch
	   }
	   return func(ctx context.Context) error {
		   if eng == nil {
			   return ErrNilEngine
		  }
		  n, err := eng.PatrolRejectAfterCredit(ctx, cfg.BatchSize)
		  if err != nil {
			   return err
		   }
		  if n > 0 {
			  log.Info().Int("proofs", n).Msg("memoeffectgate 拒批后贷记巡检")
		   }
		   return nil
	   }
  }

  func NewProofExportJob(eng *Engine, cfg WorkerConfig, log zerolog.Logger) workerloop.Job {
	   if cfg.BatchSize <= 0 {
		  cfg.BatchSize = DefaultPatrolBatch
	   }
	   return func(ctx context.Context) error {
		   if eng == nil {
			   return ErrNilEngine
		  }
		  n, err := eng.ExportOpenProofs(ctx, cfg.BatchSize)
		  if err != nil {
			   return err
		  }
		  if n > 0 {
			  log.Info().Int("exported", n).Msg("memoeffectgate 半证明导出")
		   }
		   return nil
	   }
   }

  func NewExpireBoundJob(eng *Engine, cfg WorkerConfig, log zerolog.Logger) workerloop.Job {
	   if cfg.BatchSize <= 0 {
		  cfg.BatchSize = DefaultExpiryBatch
	   }
	   return func(ctx context.Context) error {
		  if eng == nil {
			   return ErrNilEngine
		  }
		   n, err := eng.ExpireBound(ctx, cfg.BatchSize)
		   if err != nil {
			   return err
		  }
		   if n > 0 {
			  log.Info().Int("expired", n).Msg("memoeffectgate 过期绑定封堵")
		  }
		   return nil
	   }
  }

   func (e *Engine) PatrolHalfCut(ctx context.Context, batch int) (int, error) {
	   if err := e.require(); err != nil {
		  return 0, err
	   }
	  rows, err := e.store.ListHalfCutFences(ctx, batch)
	  if err != nil {
		  return 0, err
	  }
	  n := 0
	  for _, f := range rows {
		   eff, err := e.store.GetEffect(ctx, f.EffectID)
		   if err != nil {
			  continue
		  }
		  if _, err := e.resumeApprove(ctx, eff, "half-cut-patrol"); err == nil {
			   n++
		   }
	   }
	   return n, nil
   }

  func (e *Engine) PatrolRejectAfterCredit(ctx context.Context, batch int) (int, error) {
	   if err := e.require(); err != nil {
		  return 0, err
	   }
	   rows, err := e.store.ListProofs(ctx, ProofRejectAfterCredit, batch)
	  if err != nil {
		   return 0, err
	   }
	  n := 0
	   for _, p := range rows {
		  if p.SilentKeep {
			   p.SilentKeep = false
			  p.Blocked = true
			   p.Sealed = true
			   _, _ = e.store.SaveProof(ctx, p)
		  }
		   n++
	   }
	   return n, nil
   }

  func (e *Engine) ExportOpenProofs(ctx context.Context, batch int) (int, error) {
	   if err := e.require(); err != nil {
		  return 0, err
	  }
	   rows, err := e.store.ListProofsByState(ctx, HalfOpen, batch)
	   if err != nil {
		   return 0, err
	  }
	   return len(rows), nil
   }

  func (e *Engine) ExpireBound(ctx context.Context, batch int) (int, error) {
	  if err := e.require(); err != nil {
		   return 0, err
	   }
	  now := e.now()
	  rows, err := e.store.ListExpiredBound(ctx, now, batch)
	   if err != nil {
		  return 0, err
	  }
	  n := 0
	  for _, eff := range rows {
		   next, err := MustApplyEffect(eff.State, EffectEvExpire)
		  if err != nil {
			  continue
		  }
		   from := eff.State
		  eff.State = next
		   t := now
		   eff.ExpiredAt = &t
		  eff.UpdatedAt = now
		   if _, err := e.store.SaveEffect(ctx, eff); err != nil {
			  continue
		  }
		   e.audit(ctx, eff.ID, EffectEvExpire, from, next, "expire-patrol", "bound expired")
		   _ = e.sealProof(ctx, MemoHalfCreditProof{
			   Kind: ProofExpireBound, EffectID: eff.ID, TicketID: eff.TicketID,
			   Chain: eff.Chain, TxHash: eff.TxHash, OutputIndex: eff.OutputIndex,
			  Blocked: true, Sealed: true, Detail: "bound expired",
		   })
		  n++
	  }
	   return n, nil
   }
