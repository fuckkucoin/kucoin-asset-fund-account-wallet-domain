package booksnapgate

import (
	"context"
	"time"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/workerloop"
)

// SnapExpiryConfig 过期扫描参数。

type SnapExpiryConfig struct {
	Interval time.Duration
	BatchSize int
	Clock     func() time.Time
}

// NewSnapExpiryJob 创建快照过期 Job。
  func NewSnapExpiryJob(store Store, cfg SnapExpiryConfig, log zerolog.Logger) workerloop.Job {
	   if cfg.BatchSize <= 0 {
		  cfg.BatchSize = DefaultExpiryBatch
	   }
	  clock := cfg.Clock
	   if clock == nil {
		   clock = func() time.Time { return time.Now().UTC() }
	   }
	   return func(ctx context.Context) error {
		  now := clock()
		   expired, err := store.ListExpiredLeases(ctx, now, cfg.BatchSize)
		   if err != nil {
			  return err
		   }
		   fsm := LeaseStateMachine()
		   for _, lease := range expired {
			   if !IsActive(lease.State) {
				  continue
			  }
			   newState, fsmErr := fsm.Apply(string(lease.State), LeaseEvExpire)
			   if fsmErr != nil {
				  log.Warn().Str("lease_id", lease.ID).Str("state", string(lease.State)).Msg("无法过期转换")
				   continue
			   }
			   proof := BookSnapDriftProof{
				  ID:      uuid.NewString(),
				  LeaseID: lease.ID,
				  UserID: lease.UserID,
				  Asset:   lease.Asset,
				  SnapGen: lease.SnapGen,
				   Kind:    DriftKindExpiry,
				   Detail:    "快照租约过期自动清理",
				   CreatedAt: now,
			  }
			  if _,
				   err := store.SaveDriftProof(ctx, proof); err != nil {
				  log.Error().Err(err).Str("lease_id", lease.ID).Msg("保存过期证明失败")
			   }
			   expiredAt := now
			  lease.State = LeaseState(newState)
			   lease.AbortedAt = &expiredAt
			   lease.UpdatedAt = now
			   if _, err := store.SaveLease(ctx, lease); err != nil {
				  log.Error().Err(err).Str("lease_id", lease.ID).Msg("更新过期租约失败")
				  continue
			  }
			  log.Info().Str("lease_id", lease.ID).Str("user", lease.UserID).Msg("快照租约已过期")
		   }
		   return nil
	  }
  }

// AssembleAbortConfig 组装超时回收参数。
type AssembleAbortConfig struct {
	Interval time.Duration
	AssembleTimeout time.Duration
	BatchSize       int
	Clock func() time.Time
}

// NewAssembleAbortJob 创建组装超时回收 Job。
  func NewAssembleAbortJob(store Store, cfg AssembleAbortConfig, log zerolog.Logger) workerloop.Job {
	  if cfg.AssembleTimeout <= 0 {
		  cfg.AssembleTimeout = DefaultAssembleTimeout
	  }
	   if cfg.BatchSize <= 0 {
		  cfg.BatchSize = DefaultAbortBatch
	  }
	  clock := cfg.Clock
	  if clock == nil {
		  clock = func() time.Time { return time.Now().UTC() }
	   }
	  return func(ctx context.Context) error {
		   now := clock()
		  cutoff := now.Add(-cfg.AssembleTimeout)
		   stale,
			  err := store.ListAbortableLeases(ctx, cutoff, cfg.BatchSize)
		  if err != nil {
			   return err
		  }
		  fsm := LeaseStateMachine()
		   for _, lease := range stale {
			  if lease.State != LeaseAssembling {
				  continue
			  }
			   newState, fsmErr := fsm.Apply(string(lease.State), LeaseEvAbort)
			   if fsmErr != nil {
				  log.Warn().Str("lease_id", lease.ID).Str("state", string(lease.State)).Msg("无法中止转换")
				  continue
			   }
			   proof := BookSnapDriftProof{
				  ID:      uuid.NewString(),
				  LeaseID: lease.ID,
				  UserID: lease.UserID,
				  Asset:  lease.Asset,
				  SnapGen: lease.SnapGen,
				  Kind:   DriftKindAbort,
				  Detail: "组装超时自动中止",
				  CreatedAt: now,
			  }
			   if _, err := store.SaveDriftProof(ctx, proof); err != nil {
				   log.Error().Err(err).Str("lease_id", lease.ID).Msg("保存中止证明失败")
			  }
			  abortedAt := now
			  lease.State = LeaseState(newState)
			  lease.AbortedAt = &abortedAt
			  lease.UpdatedAt = now
			   if _, err := store.SaveLease(ctx, lease); err != nil {
				  log.Error().Err(err).Str("lease_id", lease.ID).Msg("更新中止租约失败")
				  continue
			  }
			  log.Info().Str("lease_id", lease.ID).Str("user", lease.UserID).Msg("组装超时已中止")
		   }
		  return nil
	  }
  }

// RunSnapExpiryWorker 启动过期扫描循环。

   func RunSnapExpiryWorker(ctx context.Context, store Store, cfg SnapExpiryConfig, log zerolog.Logger) error {
	  interval := cfg.Interval
	   if interval <= 0 {
		  interval = 30 * time.Second
	   }
	   return workerloop.Run(ctx, workerloop.Config{
		  Name: "book-snap-expiry",
		  Interval: interval,
	   }, log, NewSnapExpiryJob(store, cfg, log))
   }

// RunAssembleAbortWorker 启动组装超时回收循环。

   func RunAssembleAbortWorker(ctx context.Context, store Store, cfg AssembleAbortConfig, log zerolog.Logger) error {
	  interval := cfg.Interval
	   if interval <= 0 {
		  interval = 60 * time.Second
	   }
	   return workerloop.Run(ctx, workerloop.Config{
		   Name:     "book-snap-abort",
		  Interval: interval,
	  }, log, NewAssembleAbortJob(store, cfg, log))
   }
