package lendoccupygate

import (
	"context"
	"errors"
	"time"
)

const (
	defaultScannerBatchSize = 200
)

// ScannerConfig 扫描器配置。
type ScannerConfig struct {
	// StaleTTL placed 租约的最大容忍时间（默认 5min）；超过则视为半截候选。
	StaleTTL time.Duration
	// BatchSize 单次最大处理数（默认 200）。
	BatchSize int
	// ScannerID 扫描器标识（供日志/审计）。
	ScannerID string
}

  func (c *ScannerConfig) fill() {
	   if c.StaleTTL <= 0 {
		  c.StaleTTL = defaultStaleTTL
	   }
	   if c.BatchSize <= 0 {
		   c.BatchSize = defaultScannerBatchSize
	   }
	   if c.ScannerID == "" {
		  c.ScannerID = "lendoccupygate.scanner"
	   }
   }

// Scanner lendoccupygate 扫描器。
type Scanner struct {
	engine *Engine
	store Store
	config ScannerConfig
	clock  func() time.Time
}

// NewScanner 构造扫描器。

   func NewScanner(engine *Engine, store Store, cfg ScannerConfig) *Scanner {
	  cfg.fill()
	  return &Scanner{
		   engine: engine,
		  store:  store,
		   config: cfg,
		   clock:  func() time.Time { return time.Now().UTC() },
	   }
   }

// WithScannerClock 注入时钟。
   func (s *Scanner) WithScannerClock(fn func() time.Time) *Scanner {
	   if fn != nil {
		  s.clock = fn
	  }
	   return s
   }

   // ScanHalfPlace 扫描悬挂占用租约并触发半截补偿。
  // 查找超过 staleTTL 仍为 placed 的租约，逐条调用 Engine.CompensateResidue。
   func (s *Scanner) ScanHalfPlace(ctx context.Context) (*ScanResult, error) {
	   threshold := s.clock().Add(-s.config.StaleTTL)
	  leases,
		   err := s.store.ListStalePlacedLeases(ctx, threshold, s.config.BatchSize)
	  if err != nil {
		   return nil, err
	  }
	   result := &ScanResult{ScannedLeases: len(leases)}
	   for _, lease := range leases {
		   select {
		  case <-ctx.Done():
			  return result, ctx.Err()
		  default:
		  }
		   _, compErr := s.engine.CompensateResidue(ctx, lease)
		  if compErr == nil {
			  result.CompensatedCount++
			   continue
		   }
		   if errors.Is(compErr, ErrLeaseNotPlaced) {
			  // 并发情况：lease 已被正常 Settle/Cancel 关闭
			  result.SkippedCount++
			  continue
		   }
		  result.ErrorCount++
	  }
	   return result, nil
  }

// ScanDupRelease 检测已消费 gen 但 lease 未关闭的孤立记录（可选，辅助巡检）。
// 此场景在 Settle/Cancel 的 ConsumeGen 成功但 UpdateLeaseState 失败时出现。
  func (s *Scanner) ScanDupRelease(ctx context.Context, orderIDs []string) (*ScanResult, error) {
	  result := &ScanResult{ScannedLeases: len(orderIDs)}
	   for _, orderID := range orderIDs {
		  select {
		  case <-ctx.Done():
			   return result, ctx.Err()
		   default:
		   }
		   gen, err := s.store.GetGenByOrder(ctx, orderID)
		  if err != nil {
			  result.ErrorCount++
			   continue
		   }
		  if !gen.Consumed {
			   result.SkippedCount++
			  continue
		   }
		  lease, err := s.store.GetLeaseByOrder(ctx, orderID)
		   if err != nil {
			   result.ErrorCount++
			   continue
		  }
		   if lease.State.Terminal() {
			  result.SkippedCount++
			  continue
		  }
		   // gen 已消费但 lease 未关闭：尝试推进状态
		  now := s.clock()
		   var patch *LendOccupyLease
		   var targetState LeaseState
		  switch gen.ConsumedBy {
		   case "settle":
			   t := now
			  patch = &LendOccupyLease{SettledAt: &t}
			  targetState = LeaseSettled
		  case "cancel":
			   t := now
			  patch = &LendOccupyLease{CancelledAt: &t}
			  targetState = LeaseCancelled
		  default:
			   result.SkippedCount++
			  continue
		  }
		  if stErr := s.store.UpdateLeaseState(ctx, lease.LeaseID, LeasePlaced, targetState, patch); stErr != nil {
			  result.ErrorCount++
			   continue
		  }
		  result.CompensatedCount++
	  }
	  return result, nil
  }
