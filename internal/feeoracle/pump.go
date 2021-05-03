package feeoracle

import (
	"context"
	"sort"
	"sync"
	"time"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
)

// PumpConfig 泵配置。

type PumpConfig struct {
	// OraclePollInterval 每个源的默认轮询间隔；0 使用默认 30 秒。
	OraclePollInterval time.Duration
	// ExpiryScanInterval 过期扫描间隔；0 使用默认 5 分钟。
	ExpiryScanInterval time.Duration
	// AlignScanInterval 实耗一致扫描间隔；0 使用默认 60 秒。
	AlignScanInterval time.Duration
	// MedianWindowSize 中位数滑动窗口大小；0 使用默认 20。
	MedianWindowSize int
}

// DefaultPumpConfig 默认配置。
var DefaultPumpConfig = PumpConfig{
	OraclePollInterval: 30 * time.Second,
	ExpiryScanInterval: 5 * time.Minute,
	AlignScanInterval:  60 * time.Second,
	MedianWindowSize:   20,
}

   func (c *PumpConfig) fill() {
	  if c.OraclePollInterval <= 0 {
		   c.OraclePollInterval = DefaultPumpConfig.OraclePollInterval
	   }
	   if c.ExpiryScanInterval <= 0 {
		   c.ExpiryScanInterval = DefaultPumpConfig.ExpiryScanInterval
	  }
	  if c.AlignScanInterval <= 0 {
		  c.AlignScanInterval = DefaultPumpConfig.AlignScanInterval
	   }
	  if c.MedianWindowSize <= 0 {
		   c.MedianWindowSize = DefaultPumpConfig.MedianWindowSize
	   }
   }

// Pump 后台 oracle 泵与维护任务调度器。

type Pump struct {
	svc   *Service
	specs *SpecBook
	cfg PumpConfig
	log zerolog.Logger
	mu sync.Mutex
	recentNative map[string][]float64 // chain → rolling native fee samples
}

// NewPump 创建 Pump 实例。

  func NewPump(svc *Service, specs *SpecBook, cfg PumpConfig, log zerolog.Logger) *Pump {
	  cfg.fill()
	   return &Pump{
		   svc:   svc,
		   specs: specs,
		   cfg: cfg,
		   log: log,
		  recentNative: map[string][]float64{},
	   }
   }

// Start 启动所有后台 goroutine，阻塞直到 ctx 取消。

  func (p *Pump) Start(ctx context.Context) {
	   var wg sync.WaitGroup
	  wg.Add(1)
	  go func() {
		   defer wg.Done()
		   p.runExpiryCron(ctx)
	   }()
	  wg.Add(1)
	   go func() {
		  defer wg.Done()
		   p.runAlignWorker(ctx)
	  }()
	  sources := p.svc.Oracle().Sources()
	  for _, src := range sources {
		  if !src.Enabled {
			   continue
		   }
		  s := src
		   wg.Add(1)
		  go func() {
			  defer wg.Done()
			  p.runSourcePump(ctx, s)
		   }()
	   }
	  wg.Wait()
   }

// runExpiryCron 定期批量过期超时的 QuoteVersion。

   func (p *Pump) runExpiryCron(ctx context.Context) {
	  ticker := time.NewTicker(p.cfg.ExpiryScanInterval)
	   defer ticker.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			   return
		  case <-ticker.C:
			  n, err := p.svc.ExpireOld(ctx)
			   if err != nil {
				   p.log.Debug().Err(err).Msg("expire old versions")
				   continue
			   }
			  if n > 0 {
				   MetricVersionExpiredTotal.Add(float64(n))
				  p.log.Info().Int("expired", n).Msg("quote versions expired by cron")
			   }
		  }
	  }
   }

// runAlignWorker 周期扫描并记录 oracle 健康状态；
// 实际 AlignActual 由出金确认 worker 在广播回调中调用。
   func (p *Pump) runAlignWorker(ctx context.Context) {
	  ticker := time.NewTicker(p.cfg.AlignScanInterval)
	   defer ticker.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			  return
		  case <-ticker.C:
			   h := p.svc.Health()
			  if !h.Healthy {
				   p.log.Warn().Str("reason", h.Reason).Msg("oracle unhealthy during align scan")
			   }
		   }
	   }
  }

// runSourcePump 单个 oracle 源的轮询循环。
// 非 HTTP 源（endpoint 为空）直接跳过，等待外部 PushTick 注入。

   func (p *Pump) runSourcePump(ctx context.Context, src SourceConfig) {
	   if src.Endpoint == "" {
		  <-ctx.Done()
		   return
	  }
	  interval := p.cfg.OraclePollInterval
	  if src.StaleDur > 0 && src.StaleDur/3 < interval {
		   interval = src.StaleDur / 3
	  }
	  ticker := time.NewTicker(interval)
	   defer ticker.Stop()
	   p.log.Info().
		  Str("source", src.ID).Str("chain", src.Chain).
		   Dur("interval", interval).Msg("oracle source pump started")
	  for {
		  select {
		  case <-ctx.Done():
			  return
		   case <-ticker.C:
			  p.sampleAndUpdateMedian(src.Chain)
		   }
	  }
  }

// sampleAndUpdateMedian 从聚合层读最新 tick，追加到样本窗口并更新 SpecBook 中位数。
  func (p *Pump) sampleAndUpdateMedian(chain string) {
	  tick, err := p.svc.oracle.GetAggregated(chain)
	   if err != nil {
		   RecordOracleAggregation(chain, nil, err)
		  p.log.Debug().Err(err).Str("chain", chain).Msg("oracle sample failed")
		   return
	  }
	  RecordOracleAggregation(chain, &tick, nil)
	   if tick.Normalized.NativeAmount.IsZero() {
		  return
	   }
	  f,
		   exact := tick.Normalized.NativeAmount.Float64()
	   if !exact {
		  return
	   }
	   p.mu.Lock()
	   samples := append(p.recentNative[chain], f)
	   if len(samples) > p.cfg.MedianWindowSize {
		   samples = samples[len(samples)-p.cfg.MedianWindowSize:]
	   }
	  p.recentNative[chain] = samples
	   median := computeMedian(samples)
	   p.mu.Unlock()
	   if p.specs != nil && median > 0 {
		   _ = p.specs.UpdateMedian(chain, decimal.NewFromFloat(median))
	  }
  }

// computeMedian 计算 float64 切片的中位数（排序后取中间值）。
  func computeMedian(samples []float64) float64 {
	  if len(samples) == 0 {
		   return 0
	   }
	  cp := make([]float64, len(samples))
	  copy(cp, samples)
	   sort.Float64s(cp)
	   n := len(cp)
	  if n%2 == 0 {
		   return (cp[n/2-1] + cp[n/2]) / 2
	  }
	   return cp[n/2]
   }
