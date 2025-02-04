package shardholdgen

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/wdshard"
)

// ExpireWorker TTL 扫描。
type ExpireWorker struct {
	svc *Service
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
	  t := time.NewTicker(w.interval)
	  defer t.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			  return ctx.Err()
		  case <-t.C:
			  _, _ = w.svc.ExpireGens(ctx)
		  }
	  }
  }

   func (w *ExpireWorker) RunOnce(ctx context.Context) (int, error) {
	   return w.svc.ExpireGens(ctx)
  }

// HoldPatrolWorker 腿占巡检：failed 未释与 held 过期。
type HoldPatrolWorker struct {
	svc *Service
	interval time.Duration
}

  func NewHoldPatrolWorker(svc *Service, interval time.Duration) *HoldPatrolWorker {
	  if interval <= 0 {
		  interval = 20 * time.Second
	  }
	   return &HoldPatrolWorker{svc: svc,
		  interval: interval}
   }

  func (w *HoldPatrolWorker) Run(ctx context.Context) error {
	   t := time.NewTicker(w.interval)
	   defer t.Stop()
	  for {
		   select {
		  case <-ctx.Done():
			  return ctx.Err()
		   case <-t.C:
			  _, _ = w.svc.PatrolHalf(ctx)
			   _, _ = w.svc.ExpireGens(ctx)
		  }
	   }
   }

   func (w *HoldPatrolWorker) RunOnce(ctx context.Context) (int, error) {
	   n, err := w.svc.PatrolHalf(ctx)
	  if err != nil {
		  return n, err
	  }
	   m, err := w.svc.ExpireGens(ctx)
	   return n + m, err
  }

// ReleaseGateWorker 将 wdshard.ReleaseWorker 扫到的 failed 腿送入 ReleaseAssert。
type ReleaseGateWorker struct {
	svc *Service
	src      *wdshard.ReleaseWorker
	interval time.Duration
}

   func NewReleaseGateWorker(svc *Service, src *wdshard.ReleaseWorker, interval time.Duration) *ReleaseGateWorker {
	  if interval <= 0 {
		  interval = 20 * time.Second
	   }
	   return &ReleaseGateWorker{svc: svc, src: src, interval: interval}
   }

   func (w *ReleaseGateWorker) Run(ctx context.Context) error {
	  t := time.NewTicker(w.interval)
	  defer t.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			   return ctx.Err()
		  case <-t.C:
			   _, _ = w.RunOnce(ctx)
		  }
	   }
  }

  func (w *ReleaseGateWorker) RunOnce(ctx context.Context) (int, error) {
	  if w == nil || w.svc == nil || w.src == nil {
		   return 0, ErrUnavailable
	   }
	   n := 0
	   for _, snap := range w.src.Failed(ctx) {
		   _, err := w.svc.ReleaseAssert(ctx, ReleaseReq{
			  PlanID: snap.Plan.PlanID, LegID: snap.Leg.LegID, Actor: "release-gate",
		  })
		  if err == nil {
			   n++
		   }
	  }
	   return n, nil
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
	  return &ProofExportWorker{svc: svc,
		  interval: interval}
   }

  func (w *ProofExportWorker) Run(ctx context.Context) error {
	  t := time.NewTicker(w.interval)
	   defer t.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			   return ctx.Err()
		  case <-t.C:
			  _, _ = w.svc.ExportProofs(ctx, ExportFilter{UnexportedOnly: true, Limit: ProofExportBatch})
		  }
	   }
  }

  func (w *ProofExportWorker) RunOnce(ctx context.Context) (int, error) {
	   list, err := w.svc.ExportProofs(ctx, ExportFilter{UnexportedOnly: true, Limit: ProofExportBatch})
	   return len(list), err
  }

// AlignPump AlignWorker 挂钩。
type AlignPump struct {
	svc *Service
	src      *wdshard.AlignWorker
	interval time.Duration
}

  func NewAlignPump(svc *Service, src *wdshard.AlignWorker, interval time.Duration) *AlignPump {
	  if interval <= 0 {
		  interval = 25 * time.Second
	   }
	  return &AlignPump{svc: svc, src: src, interval: interval}
   }

  func (w *AlignPump) Run(ctx context.Context) error {
	   t := time.NewTicker(w.interval)
	  defer t.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			  return ctx.Err()
		   case <-t.C:
			   _, _ = w.RunOnce(ctx)
		  }
	  }
   }

   func (w *AlignPump) RunOnce(ctx context.Context) (int, error) {
	  if w == nil || w.svc == nil || w.src == nil {
		  return 0, ErrUnavailable
	  }
	   n := 0
	   for _, snap := range w.src.Broadcasting(ctx) {
		  _, err := w.svc.AlignWithGen(ctx, wdshard.AlignHint{
			   PlanID: snap.Plan.PlanID, LegID: snap.Leg.LegID, FillDelta: 0,
		   })
		  if err == nil {
			   n++
		  }
	   }
	  return n, nil
   }
