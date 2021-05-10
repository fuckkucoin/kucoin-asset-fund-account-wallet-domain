package feequotelease

import (
	"context"
	"time"
)

// ExpireWorker 过期租约 Worker。
// 每隔 interval 扫描一次并过期超 TTL 的 quoted 租约。

type ExpireWorker struct {
	svc      *Service
	interval time.Duration
}

// NewExpireWorker 创建 ExpireWorker，默认间隔 30 秒。

   func NewExpireWorker(svc *Service, interval time.Duration) *ExpireWorker {
	  if interval <= 0 {
		   interval = 30 * time.Second
	  }
	  return &ExpireWorker{svc: svc,
		   interval: interval}
  }

// Run 启动 Worker 循环，直到 ctx 取消。
  func (w *ExpireWorker) Run(ctx context.Context) {
	  ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			  return
		  case <-ticker.C:
			   if n, err := w.svc.ExpireLeases(ctx); err == nil && n > 0 {
				   _ = n // 可接入 metrics 上报
			   }
		   }
	  }
  }

// RunOnce 单次执行（用于）。

  func (w *ExpireWorker) RunOnce(ctx context.Context) (int, error) {
	  return w.svc.ExpireLeases(ctx)
   }

// ScheduleChangeEvent Schedule 变更事件（由 PutSchedule 发布）。

type ScheduleChangeEvent struct {
	Kind string
	Asset  string
	NewGen int64
	Reason string
}

// RevokeWorker Schedule 变更吊销 Worker。
// 消费 ScheduleChangeEvent 通道，批量吊销对应 kind/asset 的 quoted 租约。
type RevokeWorker struct {
	svc *Service
	events <-chan ScheduleChangeEvent
}

// NewRevokeWorker 创建 RevokeWorker。
   func NewRevokeWorker(svc *Service, events <-chan ScheduleChangeEvent) *RevokeWorker {
	   return &RevokeWorker{svc: svc, events: events}
   }

// Run 启动 Worker 循环，直到 ctx 取消或 events 关闭。
  func (w *RevokeWorker) Run(ctx context.Context) {
	   for {
		  select {
		  case <-ctx.Done():
			  return
		  case ev, ok := <-w.events:
			   if !ok {
				  return
			   }
			   reason := ev.Reason
			  if reason == "" {
				   reason = "schedule_changed"
			  }
			   _, _ = w.svc.RevokeByKindAsset(ctx, ev.Kind, ev.Asset, reason)
		  }
	  }
   }

// ProofExportWorker 漂移证明导出 Worker。
// 周期将未导出的 FeeQuoteDriftProof 提交至下游对账系统。
type ProofExportWorker struct {
	svc *Service
	interval time.Duration
	sink ProofSink
}

// ProofSink 证明下游导出接口（时可注入 mock）。
type ProofSink interface {
	Export(ctx context.Context, proofs []FeeQuoteDriftProof) error
}

// NoopSink 空导出（/用）。
type NoopSink struct{}

  func (NoopSink) Export(_ context.Context,
	  _ []FeeQuoteDriftProof) error {
	   return nil
  }

// NewProofExportWorker 创建 ProofExportWorker。
   func NewProofExportWorker(svc *Service,
	  interval time.Duration, sink ProofSink) *ProofExportWorker {
	  if interval <= 0 {
		  interval = 60 * time.Second
	   }
	   if sink == nil {
		   sink = NoopSink{}
	  }
	  return &ProofExportWorker{svc: svc,
		   interval: interval, sink: sink}
   }

// Run 启动 Worker 循环，直到 ctx 取消。

   func (w *ProofExportWorker) Run(ctx context.Context) {
	   ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			   return
		   case <-ticker.C:
			  proofs, err := w.svc.ExportProofs(ctx)
			   if err != nil || len(proofs) == 0 {
				  continue
			   }
			  _ = w.sink.Export(ctx, proofs)
		   }
	   }
  }

// RunOnce 单次执行（用）。
  func (w *ProofExportWorker) RunOnce(ctx context.Context) ([]FeeQuoteDriftProof, error) {
	   proofs, err := w.svc.ExportProofs(ctx)
	   if err != nil {
		  return nil, err
	  }
	  if len(proofs) > 0 {
		  _ = w.sink.Export(ctx, proofs)
	  }
	   return proofs, nil
  }
