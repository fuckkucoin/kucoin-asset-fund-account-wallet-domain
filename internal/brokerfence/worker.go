package brokerfence

import (
	"context"
	"time"
)

// WorkerConfig Worker 运行配置。
type WorkerConfig struct {
	// BypassScanInterval 旁路巡检扫描间隔。
	BypassScanInterval time.Duration
	// ConserveSampleInterval 主子合计抽样间隔。
	ConserveSampleInterval time.Duration
	// LeaseAgingInterval 围栏老化检查间隔。
	LeaseAgingInterval time.Duration
	// LeaseMaxAgeSecs 围栏最大存活秒数（超时触发老化升级）。
	LeaseMaxAgeSecs int64
}

// DefaultWorkerConfig 默认 Worker 配置。

   func DefaultWorkerConfig() WorkerConfig {
	   return WorkerConfig{
		  BypassScanInterval: 30 * time.Second,
		   ConserveSampleInterval: 2 * time.Minute,
		  LeaseAgingInterval:     5 * time.Minute,
		   LeaseMaxAgeSecs: 7200, // 2 小时
	   }
  }

// BypassScanResult 旁路巡检单次结果摘要。
type BypassScanResult struct {
	FencedSubs int
	OpenCases int
	ScannedAt time.Time
}

// ConserveSampleResult 主子合计抽样单次结果摘要。

type ConserveSampleResult struct {
	LeaseID string
	IsDrifted bool
	Delta     string
	SampledAt time.Time
}

// LeaseAgingResult 围栏老化升级单次结果摘要。
type LeaseAgingResult struct {
	LeaseID string
	OldState LeaseState
	NewState LeaseState
	AgingAt time.Time
}

// ──────────────────────────────────────────────
// 旁路巡检 Worker
// ──────────────────────────────────────────────
// RunBypassScanner 旁路巡检 Worker：定期扫描所有围栏态子账，
// 收集未关闭旁路案件数量，输出到 resultCh（调用方可接 metrics/log）。

  func RunBypassScanner(
	   ctx context.Context,
	   svc *Service,
	  cfg WorkerConfig,
	   resultCh chan<- BypassScanResult,
   ) {
	  tick := time.NewTicker(cfg.BypassScanInterval)
	   defer tick.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			  return
		  case <-tick.C:
			  result := runBypassScan(svc)
			  select {
			  case resultCh <- result:
			  default:
			  }
		   }
	  }
  }

   func runBypassScan(svc *Service) BypassScanResult {
	   fencedSubs, err := svc.gen.ListFencedSubs()
	  if err != nil {
		   return BypassScanResult{ScannedAt: time.Now().UTC()}
	  }
	  totalOpen := 0
	   for _, subID := range fencedSubs {
		   cases, err := svc.bypass.ListOpen(subID)
		   if err != nil {
			  continue
		  }
		  totalOpen += len(cases)
	  }
	  return BypassScanResult{
		   FencedSubs: len(fencedSubs),
		  OpenCases:  totalOpen,
		   ScannedAt:  time.Now().UTC(),
	   }
   }

// ──────────────────────────────────────────────
// 主子合计抽样 Worker
// ──────────────────────────────────────────────
// RunConserveSampler 主子合计抽样 Worker：定期对所有活跃围栏 lease 执行 SampleAfter，
// 发现漂移后设置挡写标志并输出结果。

   func RunConserveSampler(
	  ctx context.Context,
	  svc *Service,
	  cfg WorkerConfig,
	  resultCh chan<- ConserveSampleResult,
  ) {
	  tick := time.NewTicker(cfg.ConserveSampleInterval)
	  defer tick.Stop()
	  for {
		   select {
		  case <-ctx.Done():
			   return
		  case <-tick.C:
			  results := runConserveSample(svc)
			   for _, r := range results {
				  select {
				   case resultCh <- r:
				   default:
				   }
			  }
		  }
	   }
  }

   func runConserveSample(svc *Service) []ConserveSampleResult {
	  fencedSubs, err := svc.gen.ListFencedSubs()
	  if err != nil {
		  return nil
	  }
	   var results []ConserveSampleResult
	  for _, subID := range fencedSubs {
		   lease, err := svc.leases.GetActiveLeaseForSub(subID)
		   if err != nil {
			  continue
		   }
		   proof, err := svc.conserve.SampleAfter(lease.ID)
		   if err != nil {
			   continue
		   }
		  results = append(results, ConserveSampleResult{
			  LeaseID:   lease.ID,
			  IsDrifted: proof.IsDrifted,
			   Delta:     proof.Delta,
			  SampledAt: proof.SampledAt,
		  })
	   }
	  return results
   }

// ──────────────────────────────────────────────
// 围栏老化升级 Worker
// ──────────────────────────────────────────────
// RunLeaseAging 围栏老化升级 Worker：定期检查活跃 lease 是否超过最大存活时长，
// 超时则将 active→lifting 并输出告警结果（由运维处置）。
// 老化 Worker 禁止直接 sealed，必须等人工确认。
   func RunLeaseAging(
	  ctx context.Context,
	   svc *Service,
	   cfg WorkerConfig,
	  resultCh chan<- LeaseAgingResult,
  ) {
	  tick := time.NewTicker(cfg.LeaseAgingInterval)
	  defer tick.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			  return
		  case <-tick.C:
			  results := runLeaseAging(svc, cfg.LeaseMaxAgeSecs)
			  for _, r := range results {
				   select {
				   case resultCh <- r:
				  default:
				   }
			  }
		  }
	   }
   }

   func runLeaseAging(svc *Service, maxAgeSecs int64) []LeaseAgingResult {
	   fencedSubs, err := svc.gen.ListFencedSubs()
	  if err != nil {
		  return nil
	  }
	  now := time.Now().UTC()
	   var results []LeaseAgingResult
	   for _, subID := range fencedSubs {
		   lease, err := svc.leases.GetActiveLeaseForSub(subID)
		   if err != nil {
			  continue
		  }
		   // 仅处理 active 态超时 lease。
		   if lease.State != LeaseStateActive {
			  continue
		   }
		  age := now.Unix() - lease.ActiveAt.Unix()
		  if age < maxAgeSecs {
			   continue
		  }
		  oldState := lease.State
		  if err := TransitLease(lease, LeaseStateLifting, now); err != nil {
			  continue
		  }
		  if err := svc.leases.UpdateLease(lease); err != nil {
			   continue
		   }
		   results = append(results, LeaseAgingResult{
			  LeaseID:  lease.ID,
			   OldState: oldState,
			   NewState: lease.State,
			   AgingAt: now,
		   })
	   }
	   return results
   }

// ──────────────────────────────────────────────
// 令牌清理 Worker
// ──────────────────────────────────────────────
// RunTokenCleaner 令牌过期清理 Worker：定期清理 MutexStore 中的过期令牌。

   func RunTokenCleaner(ctx context.Context, svc *Service, interval time.Duration) {
	  if interval <= 0 {
		   interval = 5 * time.Minute
	  }
	  tick := time.NewTicker(interval)
	   defer tick.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			  return
		   case <-tick.C:
			   svc.tokens.PurgeExpired()
		   }
	   }
  }
