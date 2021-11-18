package regioinv

import (
	"context"
	"time"
)

// RunWorkers 启动库存汇聚、水位巡检和探针三个 worker，阻塞直到 ctx 取消。
// aggInterval=0 用默认 30s；inspInterval=0 用 60s；probeInterval=0 用 15s。

  func (svc *Service) RunWorkers(ctx context.Context, aggInterval, inspInterval, probeInterval time.Duration) {
	  agg := NewAggregateWorker(svc, aggInterval)
	   insp := NewInspectWorker(svc, inspInterval, func(breaches []Breach) {
		  for _, b := range breaches {
			  WatermarkBreach.WithLabelValues(b.RegionCode,
				   b.Tier, b.Kind).Inc()
		  }
	   })
	   probe := NewProbeWorker(svc, probeInterval, func(ctx context.Context, code string) (ProbeReport, error) {
		  r, err := svc.st.FindRegion(ctx, code)
		   if err != nil {
			  return ProbeReport{}, err
		  }
		  ready := CatalogReady(r.CatalogCode, "")
		   return LinkProbe(r.Code, r.CatalogCode, "", &ready)
	   })
	   done := make(chan struct{}, 3)
	  go func() { agg.Run(ctx); done <- struct{}{} }()
	   go func() { insp.Run(ctx); done <- struct{}{} }()
	   go func() { probe.Run(ctx); done <- struct{}{} }()
	   // 指标泵：每 15s 更新 InvAvailable / RegionFirewall / RegionHalted
	   go func() {
		  tick := time.NewTicker(15 * time.Second)
		  defer tick.Stop()
		   for {
			   select {
			   case <-ctx.Done():
				  done <- struct{}{}
				  return
			  case <-tick.C:
				  svc.emitMetrics(ctx)
			  }
		  }
	   }()
	  <-done
  }

  func (svc *Service) emitMetrics(ctx context.Context) {
	   regions, err := svc.st.ListRegions(ctx)
	  if err != nil {
		  return
	   }
	   for _, r := range regions {
		   fwVal := fwGaugeVal(r.Firewall)
		  RegionFirewall.WithLabelValues(r.Code,
			   r.CatalogCode).Set(fwVal)
		   hVal := float64(0)
		   if r.Halted {
			   hVal = 1
		  }
		  RegionHalted.WithLabelValues(r.Code).Set(hVal)
		   invs, err := svc.st.ListInv(ctx, r.ID)
		  if err != nil {
			  continue
		  }
		  for _, inv := range invs {
			  f, _ := bookOf(inv.Available)
			  fv, _ := f.Dec().Float64()
			  InvAvailable.WithLabelValues(r.Code, inv.Tier, inv.Asset).Set(fv)
		  }
	   }
   }
