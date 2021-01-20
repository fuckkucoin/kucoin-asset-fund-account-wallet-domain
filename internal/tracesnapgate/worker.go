package tracesnapgate

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

// ─── DriftPatrolWorker ──────────────────────────────────────────────────────
// PatrolStats 漂移巡检统计（原子计数，并发安全）。

type PatrolStats struct {
	// Rounds 完成巡检轮数。
	Rounds int64
	// Checked 已复验快照总数。
	Checked int64
	// DriftsDetected 发现漂移次数。
	DriftsDetected int64
	// Errors 巡检错误次数（不含漂移错误）。
	Errors int64
}

// DriftPatrolWorker 定期对密封快照执行 Replay 复验。

type DriftPatrolWorker struct {
	engine    *Engine
	userIndex UserIndexer
	interval time.Duration
	batchSize int
	stats     PatrolStats
}

// UserIndexer 提供活跃用户 ID 列表（只读；不改写用户状态）。
// 实现可查 ledger accounts 表；可注入固定列表。

type UserIndexer interface {
	ActiveUserIDs(ctx context.Context, limit int) ([]string, error)
}

// NewDriftPatrolWorker 创建漂移巡检 Worker。
// 参数：
// - engine：Engine 实例。
// - idx：活跃用户索引。
// - interval：巡检间隔（默认 5 分钟）。
   func NewDriftPatrolWorker(engine *Engine, idx UserIndexer, interval time.Duration) *DriftPatrolWorker {
	   if interval <= 0 {
		  interval = 5 * time.Minute
	  }
	   return &DriftPatrolWorker{
		   engine: engine,
		   userIndex: idx,
		   interval:  interval,
		  batchSize: engine.cfg.ReplayBatchSize,
	   }
  }

// Run 启动漂移巡检循环（阻塞；ctx 取消时退出）。
  func (w *DriftPatrolWorker) Run(ctx context.Context) error {
	  for {
		   select {
		   case <-ctx.Done():
			  return ctx.Err()
		  default:
		  }
		  w.runOnce(ctx)
		  atomic.AddInt64(&w.stats.Rounds, 1)
		   select {
		  case <-ctx.Done():
			   return ctx.Err()
		   case <-time.After(w.interval):
		  }
	  }
   }

// runOnce 单轮巡检：获取活跃用户 → 读密封快照 → Replay 复验。
   func (w *DriftPatrolWorker) runOnce(ctx context.Context) {
	   userIDs, err := w.userIndex.ActiveUserIDs(ctx, w.batchSize*10)
	   if err != nil {
		   atomic.AddInt64(&w.stats.Errors, 1)
		   return
	   }
	   checked := 0
	   for _, uid := range userIDs {
		   if checked >= w.batchSize {
			  break
		  }
		  snaps, err := w.engine.store.SnapshotsByUser(ctx, uid, 10)
		  if err != nil {
			  atomic.AddInt64(&w.stats.Errors, 1)
			   continue
		  }
		  for _, s := range snaps {
			  if s.Status != StatusSealed {
				  continue
			  }
			  checked++
			   _, rerr := w.engine.Replay(ctx, s.ID)
			   if rerr != nil {
				  if _, isDrift := IsDriftError(rerr); isDrift {
					  atomic.AddInt64(&w.stats.DriftsDetected, 1)
				  } else if !errors.Is(rerr, ErrNotSealed) && !errors.Is(rerr, ErrSuperseded) {
					   atomic.AddInt64(&w.stats.Errors, 1)
				   }
			  }
			   atomic.AddInt64(&w.stats.Checked,
				   1)
		  }
	  }
  }

// Stats 返回当前巡检统计（快照，并发安全）。

   func (w *DriftPatrolWorker) Stats() PatrolStats {
	  return PatrolStats{
		  Rounds:  atomic.LoadInt64(&w.stats.Rounds),
		   Checked: atomic.LoadInt64(&w.stats.Checked),
		   DriftsDetected: atomic.LoadInt64(&w.stats.DriftsDetected),
		   Errors: atomic.LoadInt64(&w.stats.Errors),
	   }
  }

// ─── SealArchiveWorker ──────────────────────────────────────────────────────
// ArchiveStats 密封归档统计（原子计数）。
type ArchiveStats struct {
	// Rounds 完成归档轮数。
	Rounds int64
	// Archived 已归档（supersede）孤儿 open 快照数。
	Archived int64
	// Errors 归档错误次数。
	Errors int64
}

// SealArchiveWorker 扫描超 TTL open 快照，置 superseded 清理孤儿。
type SealArchiveWorker struct {
	engine *Engine
	idx UserIndexer
	interval time.Duration
	stats ArchiveStats
}

// NewSealArchiveWorker 创建密封归档 Worker。
  func NewSealArchiveWorker(engine *Engine, idx UserIndexer, interval time.Duration) *SealArchiveWorker {
	   if interval <= 0 {
		  interval = 15 * time.Minute
	   }
	  return &SealArchiveWorker{
		   engine: engine,
		  idx: idx,
		  interval: interval,
	  }
   }

// Run 启动归档循环（阻塞；ctx 取消时退出）。
  func (w *SealArchiveWorker) Run(ctx context.Context) error {
	   for {
		  select {
		   case <-ctx.Done():
			  return ctx.Err()
		   default:
		   }
		  w.runOnce(ctx)
		   atomic.AddInt64(&w.stats.Rounds, 1)
		   select {
		   case <-ctx.Done():
			  return ctx.Err()
		   case <-time.After(w.interval):
		  }
	   }
   }

// runOnce 单轮归档：获取活跃用户 → 找超 TTL open 快照 → 置 superseded。

   func (w *SealArchiveWorker) runOnce(ctx context.Context) {
	   userIDs, err := w.idx.ActiveUserIDs(ctx, 200)
	   if err != nil {
		  atomic.AddInt64(&w.stats.Errors, 1)
		   return
	   }
	  now := w.engine.now()
	  for _, uid := range userIDs {
		  snaps, err := w.engine.store.SnapshotsByUser(ctx, uid, 20)
		   if err != nil {
			  atomic.AddInt64(&w.stats.Errors, 1)
			   continue
		   }
		  for _, s := range snaps {
			   if s.Status != StatusOpen {
				  continue
			  }
			   if now.Sub(s.CreatedAt) <= w.engine.cfg.SealTTL {
				   continue
			   }
			  if cerr := CanSupersede(&s); cerr != nil {
				  continue
			   }
			   ApplySupersede(&s, "", now)
			   if perr := w.engine.store.PutSnapshot(ctx, s); perr != nil {
				  atomic.AddInt64(&w.stats.Errors, 1)
			   } else {
				   atomic.AddInt64(&w.stats.Archived, 1)
			  }
		  }
	   }
   }

// Stats 返回当前归档统计。

  func (w *SealArchiveWorker) Stats() ArchiveStats {
	  return ArchiveStats{
		   Rounds: atomic.LoadInt64(&w.stats.Rounds),
		  Archived: atomic.LoadInt64(&w.stats.Archived),
		   Errors:   atomic.LoadInt64(&w.stats.Errors),
	  }
   }

// ─── FixedUserIndexer（辅助）─────────────────────────────────────────────
// FixedUserIndexer 固定用户列表的 UserIndexer 实现（注入用）。

type FixedUserIndexer struct {
	IDs []string
}

  func (f *FixedUserIndexer) ActiveUserIDs(_ context.Context, limit int) ([]string, error) {
	   if limit > 0 && len(f.IDs) > limit {
		  return f.IDs[:limit], nil
	  }
	   return f.IDs, nil
  }
