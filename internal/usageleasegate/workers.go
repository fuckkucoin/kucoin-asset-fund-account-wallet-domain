package usageleasegate

import (
	"context"
	"time"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/workerloop"
)

type WorkerConfig struct {
	Interval  time.Duration
	BatchSize int
}

   func NewExpireReleaseJob(eng *Engine, cfg WorkerConfig, log zerolog.Logger) workerloop.Job {
	   if cfg.BatchSize <= 0 {
		   cfg.BatchSize = DefaultExpiryBatch
	  }
	   return func(ctx context.Context) error {
		   if eng == nil {
			  return ErrNilEngine
		   }
		   n, err := eng.ExpireRelease(ctx, cfg.BatchSize)
		   if err != nil {
			  return err
		  }
		   if n > 0 {
			   log.Info().Int("released", n).Msg("usageleasegate 过期预约已释放")
		  }
		   return nil
	   }
  }

  func NewWindowDriftPatrolJob(eng *Engine, cfg WorkerConfig, log zerolog.Logger) workerloop.Job {
	  if cfg.BatchSize <= 0 {
		   cfg.BatchSize = DefaultPatrolBatch
	   }
	  return func(ctx context.Context) error {
		   if eng == nil {
			  return ErrNilEngine
		   }
		   n, err := eng.PatrolWindowDrift(ctx, cfg.BatchSize)
		   if err != nil {
			  return err
		  }
		  if n > 0 {
			  log.Info().Int("revoked", n).Msg("usageleasegate 窗口漂移巡检撤销")
		   }
		  return nil
	  }
  }

   func NewOverCapDiscoveryJob(eng *Engine, cfg WorkerConfig, log zerolog.Logger) workerloop.Job {
	   if cfg.BatchSize <= 0 {
		   cfg.BatchSize = DefaultPatrolBatch
	   }
	  return func(ctx context.Context) error {
		  if eng == nil {
			  return ErrNilEngine
		  }
		  n, err := eng.DiscoverOverCap(ctx, cfg.BatchSize)
		  if err != nil {
			  return err
		   }
		   if n > 0 {
			  log.Info().Int("found", n).Msg("usageleasegate 超限发现")
		   }
		   return nil
	  }
   }

// ExpireRelease 过期租约与预约自动释放，释放后占用不计入。
  func (e *Engine) ExpireRelease(ctx context.Context, batch int) (int, error) {
	   if err := e.require(); err != nil {
		   return 0, err
	  }
	   now := e.now()
	   n := 0
	  held, err := e.store.ListExpiredHeld(ctx, now, batch)
	  if err != nil {
		   return 0, err
	  }
	  for _, r := range held {
		   if _, err := e.releaseReservation(ctx, r, "ttl_expire"); err != nil {
			  continue
		  }
		   if l, err := e.store.GetLease(ctx, r.LeaseID); err == nil && CanExpire(l.State) {
			  _, _ = e.expireOrRevoke(ctx, l, LeaseEvExpire, "reservation_auto_release")
		   }
		   n++
	  }
	   lives, err := e.store.ListExpiredLive(ctx, now, batch)
	   if err != nil {
		  return n, err
	   }
	   for _, l := range lives {
		  if l.ReservationID != "" {
			   if r, err := e.store.GetReservation(ctx, l.ReservationID); err == nil && r.State == ResHeld {
				  _, _ = e.releaseReservation(ctx, r, "lease_ttl")
			   }
		  }
		  if CanExpire(l.State) {
			  if _, err := e.expireOrRevoke(ctx, l, LeaseEvExpire, "lease_ttl"); err == nil {
				  n++
			   }
		   }
	  }
	   return n, nil
   }

// PatrolWindowDrift 未提交租约窗口指纹与实时占用，漂移则撤销。
   func (e *Engine) PatrolWindowDrift(ctx context.Context, batch int) (int, error) {
	  if err := e.require(); err != nil {
		  return 0, err
	   }
	   if batch <= 0 {
		   batch = DefaultPatrolBatch
	   }
	   n := 0
	  for _, st := range []LeaseState{LeaseSnapshotted, LeaseReserved} {
		   list, err := e.store.ListLeasesByState(ctx, st, batch)
		  if err != nil {
			   return n, err
		  }
		  for _, l := range list {
			   live := e.liveFromLease(l)
			  if FingerprintsEqual(l.WindowPrint, ComputeViewPrint(live)) {
				  continue
			   }
			   e.sealProof(ctx, l, live, DriftWindowChanged, l.Amount, "0", false, "patrol window drift")
			   if l.ReservationID != "" {
				   if r, err := e.store.GetReservation(ctx, l.ReservationID); err == nil {
					  _, _ = e.releaseReservation(ctx, r, "patrol_drift")
				  }
			   }
			   if _, err := e.expireOrRevoke(ctx, l, LeaseEvRevoke, "window_patrol"); err == nil {
				  n++
			   }
		  }
	   }
	   return n, nil
  }

// DiscoverOverCap 发现预约合计将超限的活租约并出证明。
  func (e *Engine) DiscoverOverCap(ctx context.Context, batch int) (int, error) {
	   if err := e.require(); err != nil {
		   return 0, err
	   }
	   list, err := e.store.ListLeasesByState(ctx, LeaseReserved, batch)
	   if err != nil {
		  return 0, err
	   }
	  n := 0
	   seen := map[string]struct{}{}
	  for _, l := range list {
		   wk := WindowKey(l.Kind,
			   l.UserID, l.Asset, l.Dest)
		   if _, ok := seen[wk]; ok {
			  continue
		  }
		   seen[wk] = struct{}{}
		  held, err := e.store.ListHeldByWindow(ctx, wk)
		  if err != nil {
			   continue
		  }
		  capAmt, err := ParsePositiveCap(l.DailyCap)
		  if err != nil {
			   continue
		  }
		   today := e.todayAmt(l.Kind, l.UserID, l.Asset)
		   reserved := SumHeld(held)
		   // 已预约合计相对今日占用若超限，封存发现证明（不抹已提交占用）。
		  if MustAdd(today, reserved).Cmp(capAmt) > 0 {
			   live := e.liveFromLease(l)
			  e.sealProof(ctx, l, live, DriftWouldExceedCap, reserved.String(), reserved.String(), true, "over-cap discovery")
			   n++
		  }
	  }
	  return n, nil
   }
