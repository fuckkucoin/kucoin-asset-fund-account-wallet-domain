package hmacwritelease

import (
	"context"
	"time"
)

// WorkerConfig Worker 通用配置。
type WorkerConfig struct {
	// ExpiryInterval 过期扫描间隔，默认 30s。
	ExpiryInterval time.Duration
	// HaltRevokeInterval Halt 吊销扫描间隔，默认 15s。
	HaltRevokeInterval time.Duration
	// NonceCleanInterval nonce 清理间隔，默认 5min。
	NonceCleanInterval time.Duration
	// BatchSize 每批处理条数，默认 100。
	BatchSize int
}

  func (c *WorkerConfig) defaults() {
	  if c.ExpiryInterval <= 0 {
		   c.ExpiryInterval = 30 * time.Second
	  }
	   if c.HaltRevokeInterval <= 0 {
		  c.HaltRevokeInterval = 15 * time.Second
	  }
	  if c.NonceCleanInterval <= 0 {
		   c.NonceCleanInterval = 5 * time.Minute
	   }
	  if c.BatchSize <= 0 {
		   c.BatchSize = 100
	  }
  }

// ──────────────────────────────────────────────────────────────────────────────
// LeaseExpiryWorker
// ──────────────────────────────────────────────────────────────────────────────
// LeaseExpiryWorker 租约过期扫描 Worker。
// 功能：周期性调用 Engine.ExpireStaleLeases，将 ExpireAt 已过的 issued/bound 租约
// 批量流转到 expired 状态，防止过期租约在数据库中持续占用非终态位置。

type LeaseExpiryWorker struct {
	eng *Engine
	cfg WorkerConfig
}

// NewLeaseExpiryWorker 构造租约过期扫描 Worker。

  func NewLeaseExpiryWorker(eng *Engine, cfg WorkerConfig) *LeaseExpiryWorker {
	   cfg.defaults()
	   return &LeaseExpiryWorker{eng: eng,
		   cfg: cfg}
   }

// Run 启动过期扫描循环；context 取消时退出。

   func (w *LeaseExpiryWorker) Run(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.ExpiryInterval)
	   defer ticker.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			  return
		  case <-ticker.C:
			  n, _ := w.eng.ExpireStaleLeases(ctx, w.cfg.BatchSize)
			  if n > 0 {
				   recordNonceCleanedMetric(0) // 仅触发 scrape，过期数由 metricLeaseExpiredTotal 记录
			  }
		  }
	  }
  }

// ──────────────────────────────────────────────────────────────────────────────
// HaltRevokeWorker
// ──────────────────────────────────────────────────────────────────────────────
// HaltRevokeWorker Halt 翻转吊销 Worker。
// 功能：周期性调用 Engine.RevokeByHaltGen，将 HaltGen < currentGen 的非终态租约
// 批量吊销，确保 Halt 翻转后旧租约在下次 WriteCheck 前已持久标记失效。
type HaltRevokeWorker struct {
	eng *Engine
	cfg WorkerConfig
}

// NewHaltRevokeWorker 构造 Halt 吊销 Worker。

  func NewHaltRevokeWorker(eng *Engine, cfg WorkerConfig) *HaltRevokeWorker {
	   cfg.defaults()
	   return &HaltRevokeWorker{eng: eng, cfg: cfg}
  }

// Run 启动 Halt 吊销循环；context 取消时退出。

   func (w *HaltRevokeWorker) Run(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.HaltRevokeInterval)
	   defer ticker.Stop()
	  for {
		  select {
		  case <-ctx.Done():
			   return
		  case <-ticker.C:
			   _, _ = w.eng.RevokeByHaltGen(ctx, w.cfg.BatchSize)
		  }
	   }
  }

// ──────────────────────────────────────────────────────────────────────────────
// NonceCleanWorker
// ──────────────────────────────────────────────────────────────────────────────
// NonceCleanWorker nonce 窗清理 Worker。
// 功能：周期性清理 hmac_nonce_seen 表中 ExpireAt 已过的记录，
// 防止持久 nonce 防重放表无限膨胀。

type NonceCleanWorker struct {
	nonces NonceSeenRepo
	cfg    WorkerConfig
}

// NewNonceCleanWorker 构造 nonce 清理 Worker。
   func NewNonceCleanWorker(nonces NonceSeenRepo, cfg WorkerConfig) *NonceCleanWorker {
	   cfg.defaults()
	   return &NonceCleanWorker{nonces: nonces, cfg: cfg}
  }

// Run 启动 nonce 清理循环；context 取消时退出。

  func (w *NonceCleanWorker) Run(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.NonceCleanInterval)
	   defer ticker.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			  return
		  case <-ticker.C:
			  n, _ := w.nonces.DeleteExpiredNonces(ctx, w.cfg.BatchSize)
			  if n > 0 {
				  recordNonceCleanedMetric(n)
			   }
		  }
	   }
  }
