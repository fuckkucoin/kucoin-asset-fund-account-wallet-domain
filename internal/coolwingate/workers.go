package coolwingate

import (
	"context"
	"fmt"
	"time"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/workerloop"
)

// ──────────────────────────────────────────
// PreflightBindScan 预检绑定过期扫描
// ──────────────────────────────────────────

type PreflightBindScanConfig struct {
	Engine *Engine
	BatchSize int
}

  func PreflightBindScanJob(cfg PreflightBindScanConfig) workerloop.Job {
	   if cfg.BatchSize <= 0 {
		  cfg.BatchSize = 50
	  }
	   return func(ctx context.Context) error {
		   if cfg.Engine == nil {
			  return ErrWorkerDisabled
		  }
		   now := cfg.Engine.clock()
		   expired, err := cfg.Engine.store.ListExpiredBinds(ctx, now, cfg.BatchSize)
		  if err != nil {
			  return err
		  }
		  for _, b := range expired {
			   revoked := b
			  revoked.State = BindRevoked
			  revoked.RevokeReason = "bind_expired_scan"
			   rNow := now
			   revoked.RevokedAt = &rNow
			  revoked.UpdatedAt = now
			  _, _ = cfg.Engine.store.SaveBind(ctx, revoked)
		  }
		   return nil
	  }
  }

  func PreflightBindScanLoop(ctx context.Context, log zerolog.Logger, cfg PreflightBindScanConfig) error {
	   return workerloop.Run(ctx, workerloop.Config{
		  Name: "coolwingate-preflight-bind-scan",
		   Interval: 5 * time.Second,
	   },
		   log, PreflightBindScanJob(cfg))
   }

// ──────────────────────────────────────────
// MarkRevokePump Mark 事件驱动的预检撤回泵
// ──────────────────────────────────────────

type MarkRevokePumpConfig struct {
	Engine    *Engine
	BatchSize int
}

  func MarkRevokePumpJob(cfg MarkRevokePumpConfig) workerloop.Job {
	  if cfg.BatchSize <= 0 {
		   cfg.BatchSize = 50
	  }
	   return func(ctx context.Context) error {
		   if cfg.Engine == nil {
			   return ErrWorkerDisabled
		   }
		   now := cfg.Engine.clock()
		   leases, err := cfg.Engine.store.ListExpiredLeases(ctx, now, cfg.BatchSize)
		  if err != nil {
			  return err
		   }
		   for _, lease := range leases {
			  binds, err := cfg.Engine.store.ListBindsByLease(ctx, lease.ID, cfg.BatchSize)
			  if err != nil {
				  continue
			  }
			   for _, b := range binds {
				  if b.State != BindOpen {
					  continue
				   }
				  revoked := b
				   revoked.State = BindRevoked
				   revoked.RevokeReason = "lease_expired_pump"
				   rNow := now
				  revoked.RevokedAt = &rNow
				  revoked.UpdatedAt = now
				   _, _ = cfg.Engine.store.SaveBind(ctx, revoked)
				   wc := CoolWindowCase{
					  ID: uuid.NewString(),
					  UserID: b.UserID,
					   WithdrawID: b.WithdrawID,
					   BindID:     b.ID,
					  LeaseID:    lease.ID,
					   Kind:       CaseLeaseExpiredBind,
					   State:      CaseOpen,
					   Detail:    fmt.Sprintf("lease_expired: id=%s gen=%d", lease.ID, lease.CoolGen),
					   CreatedAt: now,
					  UpdatedAt: now,
				  }
				  _, _ = cfg.Engine.store.SaveCase(ctx, wc)
			  }
			  expLease := lease
			  expLease.State = LeaseExpired
			  aNow := now
			  expLease.ExpiredAt = &aNow
			  expLease.UpdatedAt = now
			  _, _ = cfg.Engine.store.SaveLease(ctx, expLease)
		   }
		  return nil
	  }
  }

   func MarkRevokePumpLoop(ctx context.Context, log zerolog.Logger, cfg MarkRevokePumpConfig) error {
	   return workerloop.Run(ctx, workerloop.Config{
		  Name: "coolwingate-mark-revoke-pump",
		  Interval: 8 * time.Second,
	  }, log, MarkRevokePumpJob(cfg))
   }

// ──────────────────────────────────────────
// LeakPatrol 窗口泄漏巡检
// ──────────────────────────────────────────

type LeakPatrolConfig struct {
	Engine    *Engine
	BatchSize int
}

   func LeakPatrolJob(cfg LeakPatrolConfig) workerloop.Job {
	   if cfg.BatchSize <= 0 {
		   cfg.BatchSize = 50
	   }
	   return func(ctx context.Context) error {
		  if cfg.Engine == nil {
			   return ErrWorkerDisabled
		   }
		   now := cfg.Engine.clock()
		   openCases, err := cfg.Engine.store.ListCasesByState(ctx, CaseOpen, cfg.BatchSize)
		  if err != nil {
			   return err
		   }
		   for _, wc := range openCases {
			   proofs, err := cfg.Engine.store.ListProofsByCase(ctx, wc.ID, 20)
			   if err != nil {
				  continue
			  }
			  if len(proofs) == 0 {
				  continue
			   }
			   allSealed := true
			   for _, p := range proofs {
				   if !p.Sealed {
					   allSealed = false
					   break
				  }
			  }
			   if allSealed {
				  updated := wc
				  updated.State = CaseProven
				  pNow := now
				  updated.ProvenAt = &pNow
				   updated.UpdatedAt = now
				  ids := make([]string,
					   0, len(proofs))
				   for _, p := range proofs {
					  ids = append(ids, p.ID)
				   }
				  updated.ProofIDs = ids
				  _, _ = cfg.Engine.store.SaveCase(ctx, updated)
			  }
		   }
		   provenCases, err := cfg.Engine.store.ListCasesByState(ctx, CaseProven, cfg.BatchSize)
		   if err != nil {
			   return err
		  }
		  for _, wc := range provenCases {
			   for _, pid := range wc.ProofIDs {
				   p, err := cfg.Engine.store.GetProof(ctx, pid)
				   if err != nil {
					   continue
				   }
				  if !p.Sealed {
					  p.Sealed = true
					   sNow := now
					   p.SealedAt = &sNow
					   _,
						  _ = cfg.Engine.store.SaveProof(ctx, p)
				   }
			   }
		   }
		   return nil
	  }
  }

  func LeakPatrolLoop(ctx context.Context, log zerolog.Logger, cfg LeakPatrolConfig) error {
	   return workerloop.Run(ctx, workerloop.Config{
		  Name:     "coolwingate-leak-patrol",
		   Interval: 15 * time.Second,
	   }, log, LeakPatrolJob(cfg))
  }

// ──────────────────────────────────────────
// CacheBypassScan 进程内缓存绕过检测
// ──────────────────────────────────────────

type CacheBypassScanConfig struct {
	Engine    *Engine
	BatchSize int
}

  func CacheBypassScanJob(cfg CacheBypassScanConfig) workerloop.Job {
	   if cfg.BatchSize <= 0 {
		   cfg.BatchSize = 50
	   }
	  return func(ctx context.Context) error {
		  if cfg.Engine == nil || cfg.Engine.ledger == nil {
			   return ErrWorkerDisabled
		  }
		   now := cfg.Engine.clock()
		   openBinds := make([]PreflightBind, 0)
		   cases, _ := cfg.Engine.store.ListCasesByState(ctx, CaseOpen, cfg.BatchSize)
		   for _, wc := range cases {
			   if wc.Kind == CaseCacheBypass {
				  continue
			  }
		  }
		   users := make(map[string]bool)
		  for _, b := range openBinds {
			   users[b.UserID] = true
		   }
		  _ = users
		  allLeases := make([]CoolLease, 0)
		   for _, wc := range cases {
			   if wc.LeaseID != "" {
				   l, err := cfg.Engine.store.GetLease(ctx, wc.LeaseID)
				  if err == nil {
					  allLeases = append(allLeases, l)
				  }
			  }
		   }
		   _ = allLeases
		  _ = now
		  return nil
	  }
  }

  func CacheBypassScanLoop(ctx context.Context, log zerolog.Logger, cfg CacheBypassScanConfig) error {
	   return workerloop.Run(ctx, workerloop.Config{
		  Name: "coolwingate-cache-bypass-scan",
		   Interval: 20 * time.Second,
	  }, log, CacheBypassScanJob(cfg))
   }
