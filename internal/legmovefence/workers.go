package legmovefence

import (
	"context"
	"time"
)

// HaltEvent 区域 Halted 通知（不改写区域防火墙公式）。

type HaltEvent struct {
	RegionID string
	Actor string
	Reason string
}

// ExpireWorker TTL 扫描。

type ExpireWorker struct {
	svc      *Service
	interval time.Duration
}

   func NewExpireWorker(svc *Service, interval time.Duration) *ExpireWorker {
	  if interval <= 0 {
		  interval = 15 * time.Second
	  }
	  return &ExpireWorker{svc: svc, interval: interval}
   }

   func (w *ExpireWorker) Run(ctx context.Context) error {
	  ticker := time.NewTicker(w.interval)
	   defer ticker.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			  return ctx.Err()
		  case <-ticker.C:
			   _, _ = w.svc.ExpireFences(ctx)
		   }
	   }
  }

  func (w *ExpireWorker) RunOnce(ctx context.Context) (int, error) {
	  return w.svc.ExpireFences(ctx)
   }

// HalfMovePatrol 半截巡检泵。

type HalfMovePatrol struct {
	svc *Service
	interval time.Duration
}

   func NewHalfMovePatrol(svc *Service, interval time.Duration) *HalfMovePatrol {
	   if interval <= 0 {
		   interval = 20 * time.Second
	   }
	  return &HalfMovePatrol{svc: svc, interval: interval}
  }

  func (w *HalfMovePatrol) Run(ctx context.Context) error {
	  ticker := time.NewTicker(w.interval)
	   defer ticker.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			  return ctx.Err()
		   case <-ticker.C:
			   _, _ = w.svc.PatrolHalf(ctx)
		  }
	   }
   }

   func (w *HalfMovePatrol) RunOnce(ctx context.Context) (int, error) {
	   return w.svc.PatrolHalf(ctx)
  }

// DoubleMoveDiscover 双移发现泵。

type DoubleMoveDiscover struct {
	svc      *Service
	interval time.Duration
}

  func NewDoubleMoveDiscover(svc *Service, interval time.Duration) *DoubleMoveDiscover {
	   if interval <= 0 {
		   interval = 30 * time.Second
	   }
	  return &DoubleMoveDiscover{svc: svc, interval: interval}
  }

  func (w *DoubleMoveDiscover) Run(ctx context.Context) error {
	   ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	  for {
		   select {
		  case <-ctx.Done():
			   return ctx.Err()
		   case <-ticker.C:
			  _,
				   _ = w.svc.DiscoverDoubleMove(ctx)
		   }
	   }
  }

   func (w *DoubleMoveDiscover) RunOnce(ctx context.Context) (int, error) {
	  return w.svc.DiscoverDoubleMove(ctx)
  }

// HaltRevokePump Halted 吊销泵。

type HaltRevokePump struct {
	svc    *Service
	events <-chan HaltEvent
}

   func NewHaltRevokePump(svc *Service, events <-chan HaltEvent) *HaltRevokePump {
	   return &HaltRevokePump{svc: svc, events: events}
   }

   func (w *HaltRevokePump) Run(ctx context.Context) error {
	  for {
		   select {
		   case <-ctx.Done():
			   return ctx.Err()
		   case ev, ok := <-w.events:
			  if !ok {
				   return nil
			  }
			   _,
				  _ = w.svc.RevokeHalted(ctx, ev.RegionID)
		  }
	  }
  }

  func (w *HaltRevokePump) RunOnce(ctx context.Context, regionID string) (int, error) {
	   return w.svc.RevokeHalted(ctx, regionID)
   }

// ProofExportWorker 证明导出泵。
type ProofExportWorker struct {
	svc      *Service
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

  func (w *ProofExportWorker) RunOnce(ctx context.Context) (int, error) {
	   return w.svc.ExportProofs(ctx)
  }
