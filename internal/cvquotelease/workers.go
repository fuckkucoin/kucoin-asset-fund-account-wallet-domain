package cvquotelease

import (
	"context"
	"time"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/workerloop"
)

type WorkerConfig struct {
	Interval time.Duration
	BatchSize int
	Clock func() time.Time
}

  func NewExpiredQuoteJob(eng *Engine, cfg WorkerConfig, log zerolog.Logger) workerloop.Job {
	  if cfg.BatchSize <= 0 {
		   cfg.BatchSize = DefaultExpiryBatch
	  }
	  return func(ctx context.Context) error {
		  if eng == nil {
			   return ErrNilEngine
		   }
		  n, err := eng.ScanExpiredQuotes(ctx, cfg.BatchSize)
		  if err != nil {
			  return err
		  }
		   if n > 0 {
			   log.Info().Int("expired", n).Msg("cvquotelease 过期报价已封堵")
		   }
		  return nil
	  }
   }

  func NewMarkRevokePumpJob(eng *Engine, cfg WorkerConfig, log zerolog.Logger) workerloop.Job {
	   if cfg.BatchSize <= 0 {
		  cfg.BatchSize = DefaultRevokeBatch
	   }
	  return func(ctx context.Context) error {
		  if eng == nil {
			   return ErrNilEngine
		  }
		   n, err := eng.PumpMarkRevokes(ctx, cfg.BatchSize)
		   if err != nil {
			  return err
		  }
		   if n > 0 {
			  log.Info().Int("revoked", n).Msg("cvquotelease Mark 变更撤销泵")
		  }
		   return nil
	   }
   }

  func NewUnconsumedReconcileJob(eng *Engine, cfg WorkerConfig, log zerolog.Logger) workerloop.Job {
	   return func(ctx context.Context) error {
		   if eng == nil {
			   return ErrNilEngine
		   }
		  n, err := eng.ReconcileUnconsumed(ctx, cfg.BatchSize)
		  if err != nil {
			   return err
		  }
		   if n > 0 {
			   log.Info().Int("reconciled", n).Msg("cvquotelease 未消费租约对账")
		  }
		  return nil
	  }
  }

   func (e *Engine) ScanExpiredQuotes(ctx context.Context, batch int) (int, error) {
	  if err := e.require(); err != nil {
		   return 0, err
	  }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  now := e.now()
	   list, err := e.store.ListExpiredIssued(ctx, now, batch)
	  if err != nil {
		   return 0, err
	  }
	  n := 0
	   for _, lease := range list {
		  if err := e.expireIssuedLocked(ctx, lease, now, "expired_quote_scan"); err != nil {
			  continue
		   }
		  if q, qerr := e.store.GetQuote(ctx, lease.QuoteID); qerr == nil && !q.Used {
			   // 过期后钉死 Used，禁止复活重入
			   q.Used = true
			  _, _ = e.store.SaveQuote(ctx, q)
		  }
		   _ = e.sealProofLocked(ctx, lease.ID, lease.QuoteID, lease.UserID, lease.FromAsset, lease.ToAsset, DriftExpired, lease.MarkGen, 0, lease.Rate, "", lease.Book, "", lease.Fingerprint, "", "expired quote scan")
		   n++
	   }
	   return n, nil
   }

  func (e *Engine) PumpMarkRevokes(ctx context.Context, batch int) (int, error) {
	   if err := e.require(); err != nil {
		  return 0, err
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   marks, err := e.store.ListMarks(ctx)
	   if err != nil {
		  return 0, err
	   }
	   n := 0
	   now := e.now()
	  for _, mk := range marks {
		  issued, _ := e.store.ListIssuedByPair(ctx, mk.PairKey, batch)
		  for _, lease := range issued {
			   if lease.MarkGen >= mk.Gen {
				  continue
			  }
			  if !CanRevoke(lease.State) {
				  continue
			   }
			  next, ferr := e.fsm.Apply(string(lease.State), LeaseEvRevoke)
			   if ferr != nil {
				  continue
			  }
			  ts := now
			   lease.State = LeaseState(next)
			  lease.RevokedAt = &ts
			   lease.UpdatedAt = now
			  lease.RevokeReason = "mark_revoke_pump"
			  proof := e.sealProofLocked(ctx, lease.ID, lease.QuoteID, lease.UserID, lease.FromAsset, lease.ToAsset, DriftRevoked, lease.MarkGen, mk.Gen, lease.Rate, mk.Rate, lease.Book, "", lease.Fingerprint, "", "mark-change revoke pump")
			   lease.ProofID = proof.ID
			  if _, serr := e.store.SaveLease(ctx, lease); serr != nil {
				  continue
			   }
			   n++
		  }
	   }
	  return n, nil
   }

  func (e *Engine) ReconcileUnconsumed(ctx context.Context, batch int) (int, error) {
	   if err := e.require(); err != nil {
		   return 0, err
	  }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   if batch <= 0 {
		   batch = DefaultExpiryBatch
	   }
	   issued,
		  err := e.store.ListIssued(ctx, batch)
	   if err != nil {
		   return 0, err
	   }
	   n := 0
	  now := e.now()
	  for _, lease := range issued {
		   if now.After(lease.ExpiresAt) {
			   if err := e.expireIssuedLocked(ctx, lease, now, "unconsumed_reconcile"); err == nil {
				  n++
			   }
			   continue
		   }
		  mk, err := e.store.GetMark(ctx, PairKey(lease.FromAsset, lease.ToAsset))
		   if err != nil {
			  continue
		   }
		  if mk.Gen != lease.MarkGen && CanRevoke(lease.State) {
			  next, ferr := e.fsm.Apply(string(lease.State), LeaseEvRevoke)
			   if ferr != nil {
				  continue
			   }
			   ts := now
			  lease.State = LeaseState(next)
			  lease.RevokedAt = &ts
			  lease.UpdatedAt = now
			   lease.RevokeReason = "unconsumed_mark_drift"
			  _, _ = e.store.SaveLease(ctx, lease)
			   _ = e.sealProofLocked(ctx, lease.ID, lease.QuoteID, lease.UserID, lease.FromAsset, lease.ToAsset, DriftMarkGen, lease.MarkGen, mk.Gen, lease.Rate, mk.Rate, lease.Book, "", lease.Fingerprint, "", "unconsumed lease reconcile")
			  n++
		  }
	   }
	   return n, nil
   }
