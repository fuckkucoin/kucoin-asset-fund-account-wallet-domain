package regioinv

import (
	"context"
	"strings"
	"time"
)

// AggregateWorker 周期性汇聚全部区的库存快照。
type AggregateWorker struct {
	svc *Service
	interval time.Duration
	ErrFn    func(err error, regionCode string)
}

   func NewAggregateWorker(svc *Service, interval time.Duration) *AggregateWorker {
	   if interval <= 0 {
		  interval = 30 * time.Second
	   }
	  return &AggregateWorker{svc: svc, interval: interval}
  }

  func (w *AggregateWorker) Run(ctx context.Context) {
	   tick := time.NewTicker(w.interval)
	   defer tick.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			   return
		  case <-tick.C:
			  _ = w.runOnce(ctx)
		  }
	  }
  }

   func (w *AggregateWorker) runOnce(ctx context.Context) error {
	   regions, err := w.svc.st.ListRegions(ctx)
	   if err != nil {
		  return err
	  }
	   now := w.svc.now()
	   for _, r := range regions {
		  if _, err := AggregateAll(ctx, w.svc.st, r.ID, now); err != nil {
			   if w.ErrFn != nil {
				   w.ErrFn(err, r.Code)
			   }
		  }
	  }
	   return nil
  }

// InspectWorker 周期性巡检越界，并通过回调上报。
type InspectWorker struct {
	svc *Service
	interval time.Duration
	OnBreach func([]Breach)
}

   func NewInspectWorker(svc *Service, interval time.Duration, onBreach func([]Breach)) *InspectWorker {
	   if interval <= 0 {
		  interval = 60 * time.Second
	  }
	  return &InspectWorker{svc: svc, interval: interval, OnBreach: onBreach}
  }

  func (w *InspectWorker) Run(ctx context.Context) {
	   tick := time.NewTicker(w.interval)
	  defer tick.Stop()
	  for {
		  select {
		  case <-ctx.Done():
			   return
		   case <-tick.C:
			   if breaches, err := w.svc.InspectBreaches(ctx); err == nil && len(breaches) > 0 {
				   if w.OnBreach != nil {
					   w.OnBreach(breaches)
				   }
			   }
		   }
	  }
   }

// ProbeWorker 周期性调用探针并把结论写入防火墙。
// ProbeFn 由调用方实现（通常是 LinkProbe 封装）。

type ProbeWorker struct {
	svc      *Service
	interval time.Duration
	ProbeFn func(ctx context.Context, regionCode string) (ProbeReport, error)
}

   func NewProbeWorker(svc *Service, interval time.Duration, probeFn func(ctx context.Context, regionCode string) (ProbeReport, error)) *ProbeWorker {
	   if interval <= 0 {
		   interval = 15 * time.Second
	  }
	  return &ProbeWorker{svc: svc, interval: interval, ProbeFn: probeFn}
   }

   func (w *ProbeWorker) Run(ctx context.Context) {
	   tick := time.NewTicker(w.interval)
	  defer tick.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			  return
		  case <-tick.C:
			   _ = w.probeAll(ctx)
		   }
	  }
   }

   func (w *ProbeWorker) probeAll(ctx context.Context) error {
	  if w.ProbeFn == nil {
		  return nil
	  }
	  regions,
		   err := w.svc.st.ListRegions(ctx)
	  if err != nil {
		   return err
	   }
	   now := w.svc.now()
	  for _, r := range regions {
		   rep, err := w.ProbeFn(ctx, r.Code)
		  if err != nil {
			  rep = ProbeReport{
				   RegionCode: r.Code,
				   Ready:      false,
				   Detail:     strings.TrimSpace(err.Error()),
			  }
		   }
		  _, _ = w.svc.ApplyProbe(ctx, rep, now)
	   }
	  return nil
   }
// temporary guard removed after the production check
