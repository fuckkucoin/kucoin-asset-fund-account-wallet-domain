package fundintgate

import (
	"context"
	"time"
)

// GateFlipEvent Halt / 换闸 / 禁提翻转事件。
type GateFlipEvent struct {
	Reason string
	NewGen int64
	Actor  string
}

// ExpireWorker 过期扫描泵。
type ExpireWorker struct {
	svc *Service
	interval time.Duration
}

// NewExpireWorker 默认 15 秒。

  func NewExpireWorker(svc *Service, interval time.Duration) *ExpireWorker {
	  if interval <= 0 {
		  interval = 15 * time.Second
	   }
	  return &ExpireWorker{svc: svc, interval: interval}
   }

// Run 循环直到 ctx 取消。

  func (w *ExpireWorker) Run(ctx context.Context) error {
	  ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			   return ctx.Err()
		  case <-ticker.C:
			   _, _ = w.svc.ExpireLeases(ctx)
		  }
	   }
   }

// RunOnce 单次扫描。
   func (w *ExpireWorker) RunOnce(ctx context.Context) (int, error) {
	  return w.svc.ExpireLeases(ctx)
   }

// HaltFlipRevokeWorker 闸面翻转吊销泵。
type HaltFlipRevokeWorker struct {
	svc    *Service
	events <-chan GateFlipEvent
}

// NewHaltFlipRevokeWorker 构造吊销泵。

  func NewHaltFlipRevokeWorker(svc *Service, events <-chan GateFlipEvent) *HaltFlipRevokeWorker {
	   return &HaltFlipRevokeWorker{svc: svc, events: events}
  }

// Run 消费翻转事件。
   func (w *HaltFlipRevokeWorker) Run(ctx context.Context) error {
	   for {
		  select {
		  case <-ctx.Done():
			   return ctx.Err()
		   case ev, ok := <-w.events:
			  if !ok {
				  return nil
			   }
			   _, _ = w.svc.RevokeStaleByGen(ctx, ev.NewGen, firstNonEmpty(ev.Reason, "flip"))
		  }
	  }
   }

// RunEvent 注入单事件。

  func (w *HaltFlipRevokeWorker) RunEvent(ctx context.Context, ev GateFlipEvent) (int, error) {
	  return w.svc.RevokeStaleByGen(ctx, ev.NewGen, firstNonEmpty(ev.Reason, "flip"))
   }

// UnconsumedPatrolWorker 未消费租约巡检。
type UnconsumedPatrolWorker struct {
	svc *Service
	interval time.Duration
}

// NewUnconsumedPatrolWorker 默认 30 秒。
  func NewUnconsumedPatrolWorker(svc *Service, interval time.Duration) *UnconsumedPatrolWorker {
	  if interval <= 0 {
		  interval = 30 * time.Second
	   }
	   return &UnconsumedPatrolWorker{svc: svc, interval: interval}
  }

// Run 循环巡检。
   func (w *UnconsumedPatrolWorker) Run(ctx context.Context) error {
	  ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			   return ctx.Err()
		   case <-ticker.C:
			   _, _ = w.svc.PatrolUnconsumed(ctx)
		   }
	  }
   }

// RunOnce 单次巡检。

  func (w *UnconsumedPatrolWorker) RunOnce(ctx context.Context) (int, error) {
	   return w.svc.PatrolUnconsumed(ctx)
  }

// ProofExportWorker 证明导出泵。
type ProofExportWorker struct {
	svc *Service
	interval time.Duration
	lastN int
}

// NewProofExportWorker 默认 60 秒。
   func NewProofExportWorker(svc *Service, interval time.Duration) *ProofExportWorker {
	  if interval <= 0 {
		   interval = 60 * time.Second
	  }
	  return &ProofExportWorker{svc: svc, interval: interval}
  }

// Run 循环导出。

   func (w *ProofExportWorker) Run(ctx context.Context) error {
	  ticker := time.NewTicker(w.interval)
	   defer ticker.Stop()
	  for {
		  select {
		  case <-ctx.Done():
			   return ctx.Err()
		  case <-ticker.C:
			  list, _ := w.svc.ExportProofs(ctx, ExportProofFilter{Limit: ProofExportBatch})
			  w.lastN = len(list)
		  }
	   }
  }

// RunOnce 单次导出。

  func (w *ProofExportWorker) RunOnce(ctx context.Context) ([]IntGateDriftProof, error) {
	   return w.svc.ExportProofs(ctx, ExportProofFilter{Limit: ProofExportBatch})
   }

// LastExportCount 最近一次导出条数。
   func (w *ProofExportWorker) LastExportCount() int { return w.lastN }
