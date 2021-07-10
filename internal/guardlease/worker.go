package guardlease

import (
	"context"
	"time"
)

// WorkerConfig Worker 配置参数。

type WorkerConfig struct {
	// LeaseExpiryInterval 租约过期扫描间隔。
	LeaseExpiryInterval time.Duration
	// ImbalanceScanInterval 失衡巡检间隔。
	ImbalanceScanInterval time.Duration
	// ProofEscalateInterval 守恒证明升级巡检间隔。
	ProofEscalateInterval time.Duration
	// ImbalanceScanLimit 单次巡检最大记录数。
	ImbalanceScanLimit int
	// ProofScanLimit 单次证明巡检最大记录数。
	ProofScanLimit int
	// AlertFunc 告警回调（用于接入监控告警总线）。
	AlertFunc func(kind, detail string)
}

// DefaultWorkerConfig 默认配置。

  func DefaultWorkerConfig() WorkerConfig {
	   return WorkerConfig{
		   LeaseExpiryInterval:   15 * time.Second,
		   ImbalanceScanInterval: 30 * time.Second,
		   ProofEscalateInterval: 60 * time.Second,
		  ImbalanceScanLimit:    50,
		  ProofScanLimit:        50,
		  AlertFunc: func(kind, detail string) {},
	  }
   }

// Workers 封装所有守恒闸世代 Worker。

type Workers struct {
	svc *Service
	cfg WorkerConfig
}

// NewWorkers 创建 Worker 集合。

  func NewWorkers(svc *Service, cfg WorkerConfig) *Workers {
	  if cfg.AlertFunc == nil {
		   cfg.AlertFunc = func(kind, detail string) {}
	  }
	  if cfg.LeaseExpiryInterval <= 0 {
		  cfg.LeaseExpiryInterval = 15 * time.Second
	   }
	   if cfg.ImbalanceScanInterval <= 0 {
		  cfg.ImbalanceScanInterval = 30 * time.Second
	   }
	   if cfg.ProofEscalateInterval <= 0 {
		   cfg.ProofEscalateInterval = 60 * time.Second
	  }
	  if cfg.ImbalanceScanLimit <= 0 {
		   cfg.ImbalanceScanLimit = 50
	   }
	  if cfg.ProofScanLimit <= 0 {
		   cfg.ProofScanLimit = 50
	  }
	   return &Workers{svc: svc, cfg: cfg}
   }

// RunAll 启动所有 Worker（阻塞直到 ctx 取消）。
			  return
		  case <-ticker.C:
			  w.sweepExpiredLeases(ctx)
		   }
	   }
   }

  func (w *Workers) sweepExpiredLeases(ctx context.Context) {
	  if w.svc == nil || w.svc.store == nil {
		  return
	   }
	   n, err := w.svc.store.MarkExpiredLeases(ctx)
	  if err != nil {
		  w.cfg.AlertFunc("guardlease.lease_expiry_error", err.Error())
		   return
	   }
	  if n > 0 {
		  w.cfg.AlertFunc("guardlease.leases_expired",
			   "marked "+formatInt(n)+" leases as expired")
	   }
   }

// ImbalanceScanWorker 定期巡检 open 失衡记录，告警未确认的失衡。
   func (w *Workers) ImbalanceScanWorker(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.ImbalanceScanInterval)
	   defer ticker.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			  return
		  case <-ticker.C:
			   w.scanOpenImbalances(ctx)
		  }
	   }
   }

  func (w *Workers) scanOpenImbalances(ctx context.Context) {
	  if w.svc == nil || w.svc.store == nil {
		  return
	  }
	  ims, err := w.svc.store.ListOpenImbalances(ctx, w.cfg.ImbalanceScanLimit)
	  if err != nil {
		   w.cfg.AlertFunc("guardlease.imbalance_scan_error", err.Error())
		   return
	  }
	  for _, im := range ims {
		  age := time.Since(im.DetectedAt)
		   detail := "imbalance_id=" + im.ImbalanceID +
			  " user=" + im.UserID + " asset=" + im.Asset +
			   " delta=" + im.Delta + " age=" + age.String()
		  if age > FourEyesClearDeadline {
			   w.cfg.AlertFunc("guardlease.imbalance_stale", detail)
		   } else {
			   w.cfg.AlertFunc("guardlease.imbalance_open", detail)
		  }
	  }
  }

// ProofEscalateWorker 定期巡检 fail 守恒证明，超过四眼解除时限则升级为 halted。

  func (w *Workers) ProofEscalateWorker(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.ProofEscalateInterval)
	   defer ticker.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			  return
		   case <-ticker.C:
			   w.escalateFailedProofs(ctx)
		   }
	  }
  }

   func (w *Workers) escalateFailedProofs(ctx context.Context) {
	  if w.svc == nil || w.svc.store == nil {
		   return
	  }
	  proofs, err := w.svc.store.ListFailedProofs(ctx, w.cfg.ProofScanLimit)
	   if err != nil {
		  w.cfg.AlertFunc("guardlease.proof_escalate_error", err.Error())
		  return
	   }
	  for _, p := range proofs {
		  if p.State != ProofFail {
			  continue
		  }
		   if p.EscalatedAt == nil {
			  continue
		   }
		  if time.Since(*p.EscalatedAt) < FourEyesClearDeadline {
			   w.cfg.AlertFunc("guardlease.proof_fail_pending_clear",
				  "proof_id="+p.ProofID+" user="+p.UserID+" asset="+p.Asset)
			  continue
		   }
		  // 超过四眼时限：升级为 halted，触发全局 halt
		   extra := map[string]any{}
		   if err := w.svc.store.UpdateProofState(ctx, p.ProofID, ProofFail, ProofHalted, extra); err != nil {
			   w.cfg.AlertFunc("guardlease.proof_escalate_db_error", err.Error())
			  continue
		   }
		  w.svc.SetHalt(true)
		   w.cfg.AlertFunc("guardlease.proof_escalated_to_halted",
			  "proof_id="+p.ProofID+" user="+p.UserID+" asset="+p.Asset+
				   " — system halted, four-eyes clearance required")
	   }
  }

  func formatInt(n int64) string {
	   if n == 0 {
		   return "0"
	  }
	  s := ""
	  neg := n < 0
	  if neg {
		  n = -n
	   }
	  for n > 0 {
		   s = string(rune('0'+n%10)) + s
		   n /= 10
	  }
	  if neg {
		   s = "-" + s
	   }
	  return s
   }
