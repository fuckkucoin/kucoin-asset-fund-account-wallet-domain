package destentrygate

import (
	"context"
	"time"
)

// LifecycleEvent 条目生命周期事件（不改写 destallow FSM，只消费通知）。
type LifecycleEvent struct {
	Kind string // temp_expire | breakglass_reclaim | revoke | cooldown_not_full
	Entry  LiveEntry
	Actor  string
	Reason string
}

// ExpireWorker TTL 扫描泵。
type ExpireWorker struct {
	svc      *Service
	interval time.Duration
}

   func NewExpireWorker(svc *Service, interval time.Duration) *ExpireWorker {
	   if interval <= 0 {
		   interval = 15 * time.Second
	   }
	   return &ExpireWorker{svc: svc,
		   interval: interval}
   }

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

   func (w *ExpireWorker) RunOnce(ctx context.Context) (int, error) {
	   return w.svc.ExpireLeases(ctx)
   }

// TempExpireScanWorker Temp 过期扫描。
type TempExpireScanWorker struct {
	svc *Service
	interval time.Duration
}

  func NewTempExpireScanWorker(svc *Service, interval time.Duration) *TempExpireScanWorker {
	  if interval <= 0 {
		   interval = 10 * time.Second
	   }
	   return &TempExpireScanWorker{svc: svc, interval: interval}
  }

   func (w *TempExpireScanWorker) Run(ctx context.Context) error {
	   ticker := time.NewTicker(w.interval)
	   defer ticker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			   return ctx.Err()
		  case <-ticker.C:
			  _, _ = w.svc.ScanTempExpire(ctx)
		  }
	  }
   }

   func (w *TempExpireScanWorker) RunOnce(ctx context.Context) (int, error) {
	  return w.svc.ScanTempExpire(ctx)
   }

// BreakglassReclaimWorker 破窗收回扫描。
type BreakglassReclaimWorker struct {
	svc      *Service
	interval time.Duration
}

  func NewBreakglassReclaimWorker(svc *Service, interval time.Duration) *BreakglassReclaimWorker {
	  if interval <= 0 {
		  interval = 10 * time.Second
	   }
	  return &BreakglassReclaimWorker{svc: svc,
		   interval: interval}
   }

   func (w *BreakglassReclaimWorker) Run(ctx context.Context) error {
	  ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			  return ctx.Err()
		  case <-ticker.C:
			  _, _ = w.svc.ScanBreakglassReclaim(ctx)
		  }
	   }
  }

   func (w *BreakglassReclaimWorker) RunOnce(ctx context.Context) (int, error) {
	   return w.svc.ScanBreakglassReclaim(ctx)
   }

// RevokePumpWorker 消费显式生命周期事件。
type RevokePumpWorker struct {
	svc *Service
	events <-chan LifecycleEvent
}

   func NewRevokePumpWorker(svc *Service, events <-chan LifecycleEvent) *RevokePumpWorker {
	   return &RevokePumpWorker{svc: svc,
		  events: events}
  }

  func (w *RevokePumpWorker) Run(ctx context.Context) error {
	   for {
		  select {
		  case <-ctx.Done():
			   return ctx.Err()
		  case ev, ok := <-w.events:
			  if !ok {
				   return nil
			  }
			  _, _ = w.Handle(ctx, ev)
		   }
	  }
   }

  func (w *RevokePumpWorker) Handle(ctx context.Context, ev LifecycleEvent) (int, error) {
	   switch ev.Kind {
	  case "temp_expire":
		   return w.svc.OnTempExpired(ctx, ev.Entry, firstNonEmpty(ev.Actor, "revoke-pump"))
	   case "breakglass_reclaim":
		   return w.svc.OnBreakglassReclaim(ctx, ev.Entry, firstNonEmpty(ev.Actor, "revoke-pump"))
	   case "revoke":
		  return w.svc.OnEntryRevoke(ctx, ev.Entry, firstNonEmpty(ev.Actor, "revoke-pump"))
	   case "cooldown_not_full":
		   return w.svc.OnRestoreCooldownNotFull(ctx, ev.Entry, firstNonEmpty(ev.Actor, "revoke-pump"))
	  default:
		  return w.svc.OnEntryRevoke(ctx, ev.Entry, firstNonEmpty(ev.Actor, "revoke-pump"))
	  }
  }

// UnconsumedPatrolWorker 未消费巡检。

type UnconsumedPatrolWorker struct {
	svc *Service
	interval time.Duration
}

   func NewUnconsumedPatrolWorker(svc *Service, interval time.Duration) *UnconsumedPatrolWorker {
	   if interval <= 0 {
		  interval = 30 * time.Second
	  }
	   return &UnconsumedPatrolWorker{svc: svc, interval: interval}
   }

  func (w *UnconsumedPatrolWorker) Run(ctx context.Context) error {
	  ticker := time.NewTicker(w.interval)
	   defer ticker.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			  return ctx.Err()
		  case <-ticker.C:
			   _,
				   _ = w.svc.PatrolUnconsumed(ctx)
		   }
	  }
  }

  func (w *UnconsumedPatrolWorker) RunOnce(ctx context.Context) (int, error) {
	   return w.svc.PatrolUnconsumed(ctx)
   }

// ProofExportWorker 证明导出泵。
type ProofExportWorker struct {
	svc *Service
	interval time.Duration
}

  func NewProofExportWorker(svc *Service, interval time.Duration) *ProofExportWorker {
	   if interval <= 0 {
		  interval = 60 * time.Second
	  }
	   return &ProofExportWorker{svc: svc, interval: interval}
   }

  func (w *ProofExportWorker) Run(ctx context.Context) error {
	   ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			   return ctx.Err()
		  case <-ticker.C:
			  _, _ = w.svc.ExportProofs(ctx)
		   }
	  }
  }

  func (w *ProofExportWorker) RunOnce(ctx context.Context) ([]DestEntryDriftProof, error) {
	  return w.svc.ExportProofs(ctx)
  }
