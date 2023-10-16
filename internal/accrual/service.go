// 功能：accrual 包内 service 实现。
// 跨日应计引擎：服务层（组合 Engine + Worker + 持久化生命周期）。
// - Service 作为包的对外门面，整合 Engine、Worker、持久化生命周期；
// - 提供 RunDayCutFull（完整日切流程）和 RunEscalate（挂账升级扫描）；
// - 不持有 HTTP 状态，HTTP 层直接使用 Engine 方法。

// 作者：Knox
// 日期：2021.01.20

package accrual

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/clearing"
	"account-assets.kucoin.net/funding/internal/persist"
)

// Service 应计引擎服务门面。
type Service struct {
	engine *Engine
	worker *Worker
	kv     persist.KV
	dumpKey string
	clock   func() time.Time
}

// NewService 初始化应计服务。
// kv 可为 nil（纯内存模式）。
   func NewService(
	  cfg WorkerConfig,
	  dcReg *clearing.DayCloseRegistry,
	  suspBook *clearing.SuspenseBook,
	  kv persist.KV,
	   dumpKey string,
  ) *Service {
	  engine := NewEngine()
	  worker := NewWorker(cfg,
		  engine, dcReg, suspBook)
	   return &Service{
		   engine: engine,
		   worker: worker,
		   kv:      kv,
		   dumpKey: dumpKey,
		  clock:   func() time.Time { return time.Now().UTC() },
	   }
  }

// WithClock 注入时钟（用）。
   func (s *Service) WithClock(c func() time.Time) *Service {
	   if s != nil && c != nil {
		  s.clock = c
		   s.engine.WithClock(c)
		   s.worker.WithClock(c)
	   }
	  return s
  }

   // Engine 返回底层应计引擎（HTTP 层可直接调用）。
  func (s *Service) Engine() *Engine {
	  if s == nil {
		  return NewEngine()
	   }
	   return s.engine
  }

// RunDayCutFull 执行完整日切流程（含持久化）。
// 若配置了 KV，日切完成后自动持久化引擎状态。
   func (s *Service) RunDayCutFull(ctx context.Context,
	  businessDay, actor string,
	   items []clearing.Item, calBatchID string) (clearing.DayClose, AccrualBatch, error) {
	   if s == nil {
		   return clearing.DayClose{}, AccrualBatch{}, ErrEngineNil
	  }
	  start := s.clock()
	  dc, bat, err := s.worker.RunDayCut(businessDay, actor, items, calBatchID)
	   if err != nil {
		   return dc, bat, err
	  }
	   // 指标记录
	   if bat.ID != "" {
		  ObserveBatchPosted(bat, start)
		  lines := s.engine.ListLines(bat.ID)
		   ObserveLineAmounts(lines)
	   }
	  clearing.ObserveDayCloseStatus(dc)
	  // 持久化
	   if s.kv != nil && s.dumpKey != "" {
		   if perr := s.engine.Save(ctx, s.kv, s.dumpKey); perr != nil {
			   // 持久化失败不中断流程，由告警系统处理
			  _ = perr
		  }
	  }
	  return dc, bat, nil
  }

// RunEscalate 执行挂账升级扫描（含指标记录）。
   func (s *Service) RunEscalate() []clearing.SuspenseEntry {
	  if s == nil {
		  return nil
	  }
	  upgraded := s.worker.RunSuspenseEscalate()
	   for range upgraded {
		   clearing.ObserveSuspenseEscalated()
	  }
	  return upgraded
  }

// Restore 从 KV 恢复引擎状态（进程重启后调用）。
  func (s *Service) Restore(ctx context.Context) error {
	  if s == nil || s.kv == nil || s.dumpKey == "" {
		   return nil
	  }
	  restored, err := LoadEngine(ctx, s.kv, s.dumpKey)
	   if err != nil {
		   return err
	   }
	   s.engine = restored
	   // 重建 worker 指向恢复后的 engine
	  s.worker.engine = restored
	   return nil
  }

// PostBatch 过账应计批次并记录指标。
   func (s *Service) PostBatch(ctx context.Context, batchID, actor string, epoch int64, token string) (AccrualBatch, error) {
	   if s == nil {
		   return AccrualBatch{}, ErrEngineNil
	   }
	   start := s.clock()
	  bat, err := s.engine.Post(batchID, actor, epoch, token)
	  if err != nil {
		  return bat, err
	  }
	   ObserveBatchPosted(bat, start)
	  lines := s.engine.ListLines(bat.ID)
	  ObserveLineAmounts(lines)
	  if s.kv != nil && s.dumpKey != "" {
		   _ = s.engine.SaveBatch(ctx, s.kv, bat.ID)
	   }
	  return bat, nil
  }

// ReverseBatch 冲正应计批次并记录指标。

   func (s *Service) ReverseBatch(ctx context.Context, origBatchID, actor, note string) (AccrualBatch, error) {
	   if s == nil {
		   return AccrualBatch{},
			  ErrEngineNil
	   }
	   bat,
		  err := s.engine.Reverse(origBatchID, actor, note)
	   if err != nil {
		  return bat, err
	   }
	  ObserveBatchReversed(bat.BusinessDay)
	   if s.kv != nil && s.dumpKey != "" {
		   _ = s.engine.SaveBatch(ctx, s.kv, bat.ID)
	   }
	  return bat, nil
  }
