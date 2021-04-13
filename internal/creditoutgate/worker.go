package creditoutgate

import (
	"context"
	"log"
	"time"
)

// FenceRevokeWorker 护栏翻转吊销 Worker。
// 从 flipCh 接收 FenceFlipReq，调用 Service.FenceFlip 执行吊销。

type FenceRevokeWorker struct {
	svc    *Service
	flipCh <-chan FenceFlipReq
	logger *log.Logger
}

// NewFenceRevokeWorker 创建护栏翻转 Worker。
// flipCh 由调用方提供，关闭后 Worker 退出。
  func NewFenceRevokeWorker(svc *Service, flipCh <-chan FenceFlipReq, logger *log.Logger) *FenceRevokeWorker {
	  return &FenceRevokeWorker{svc: svc, flipCh: flipCh, logger: logger}
   }

// Run 阻塞运行，直到 ctx 取消或 flipCh 关闭。

   func (w *FenceRevokeWorker) Run(ctx context.Context) {
	  if w == nil || w.svc == nil {
		  return
	  }
	   for {
		  select {
		  case <-ctx.Done():
			   return
		  case req, ok := <-w.flipCh:
			  if !ok {
				  return
			  }
			   res, err := w.svc.FenceFlip(req)
			   if err != nil {
				  w.log("FenceFlip error board=%s err=%v", req.BoardKey, err)
				  continue
			  }
			  w.log("FenceFlip board=%s new_world_gen=%d revoked=%d cases=%d",
				   req.BoardKey, res.NewGen.WorldGen, res.RevokedCount, len(res.Cases))
		   }
	   }
  }

   func (w *FenceRevokeWorker) log(format string, args ...interface{}) {
	  if w.logger != nil {
		  w.logger.Printf("[creditoutgate/fence-revoke] "+format, args...)
	   }
  }

// PatrolWorker 双贷记巡检 Worker。
// 按 interval 周期执行 Patrol + PatrolAllBoards。

type PatrolWorker struct {
	svc      *Service
	interval time.Duration
	limit    int
	logger   *log.Logger
}

// NewPatrolWorker 创建巡检 Worker。
   func NewPatrolWorker(svc *Service, interval time.Duration, limit int, logger *log.Logger) *PatrolWorker {
	   if interval <= 0 {
		  interval = 30 * time.Second
	   }
	   if limit <= 0 {
		  limit = 64
	   }
	  return &PatrolWorker{svc: svc, interval: interval, limit: limit, logger: logger}
  }

// Run 阻塞运行，直到 ctx 取消。

  func (w *PatrolWorker) Run(ctx context.Context) {
	   if w == nil || w.svc == nil {
		  return
	   }
	  ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			  return
		  case <-ticker.C:
			   w.runOnce()
		   }
	  }
   }

   func (w *PatrolWorker) runOnce() {
	   res := w.svc.Patrol(w.limit)
	  w.log("Patrol scanned=%d detected=%d cases=%d", res.Scanned, res.Detected, len(res.Cases))
	   nets := w.svc.PatrolAllBoards()
	   if len(nets) > 0 {
		  w.log("PatrolAllBoards safety_net_revoked=%d", len(nets))
	   }
  }

  func (w *PatrolWorker) log(format string, args ...interface{}) {
	   if w.logger != nil {
		   w.logger.Printf("[creditoutgate/patrol] "+format, args...)
	   }
  }

// ExportWorker 周期性导出证明 Worker。
// 按 interval 周期执行 ExportProofs，将未导出证明标记为已导出。
type ExportWorker struct {
	svc      *Service
	interval time.Duration
	limit    int
	onExport func(ProofExport)
	logger *log.Logger
}

// NewExportWorker 创建导出 Worker。onExport 回调可用于推送到下游对账系统。

  func NewExportWorker(svc *Service, interval time.Duration, limit int, onExport func(ProofExport), logger *log.Logger) *ExportWorker {
	   if interval <= 0 {
		   interval = 60 * time.Second
	   }
	  if limit <= 0 {
		   limit = 128
	   }
	   return &ExportWorker{svc: svc, interval: interval, limit: limit, onExport: onExport, logger: logger}
   }

   // Run 阻塞运行，直到 ctx 取消。

  func (e *ExportWorker) Run(ctx context.Context) {
	  if e == nil || e.svc == nil {
		  return
	   }
	  ticker := time.NewTicker(e.interval)
	   defer ticker.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			   return
		  case <-ticker.C:
			   e.runOnce()
		   }
	   }
   }

   func (e *ExportWorker) runOnce() {
	  exp := e.svc.ExportProofs(e.limit)
	   if len(exp.Proofs) == 0 && len(exp.Cases) == 0 {
		   return
	  }
	   e.log("Export proofs=%d cases=%d total_proofs=%d total_cases=%d",
		  len(exp.Proofs),
		  len(exp.Cases), exp.TotalProofs, exp.TotalCases)
	  if e.onExport != nil {
		   e.onExport(exp)
	  }
  }

   func (e *ExportWorker) log(format string, args ...interface{}) {
	  if e.logger != nil {
		   e.logger.Printf("[creditoutgate/export] "+format, args...)
	   }
   }
