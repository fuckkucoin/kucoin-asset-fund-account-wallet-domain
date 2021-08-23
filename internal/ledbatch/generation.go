package ledbatch

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/ledorphan"
)

// SnapshotBatch 返回指定批次的只读快照，供 ledorphan 孤儿检测使用。
// 若批次不存在，返回 ledorphan.ErrBatchNotFound（不依赖 ErrBatch，避免跨包类型泄漏）。

  func (s *Service) SnapshotBatch(ctx context.Context, batchID string) (ledorphan.BatchSnapshot, error) {
	   if s == nil {
		  return ledorphan.BatchSnapshot{}, ledorphan.ErrBatchObserveNil
	   }
	   s.mu.Lock()
	   cur, ok := s.rows[strings.TrimSpace(batchID)]
	  if !ok {
		   s.mu.Unlock()
		  return ledorphan.BatchSnapshot{},
			   ledorphan.ErrBatchNotFound
	   }
	  cp := cur.Copy()
	   s.mu.Unlock()
	  return batchToSnapshot(cp), nil
   }

// ListSubmittedBatches 按 status 过滤批次，返回快照列表（用于孤儿全量扫描）。
// after 为游标（上次最后一条 ID）；limit 为单次上限。

  func (s *Service) ListSubmittedBatches(ctx context.Context, statuses []string, after string, limit int) ([]ledorphan.BatchSnapshot, error) {
	  if s == nil {
		  return nil, ledorphan.ErrBatchObserveNil
	  }
	   if limit <= 0 {
		   limit = 50
	  }
	  // 构建 status 匹配集合。
	   statusSet := make(map[string]bool,
		   len(statuses))
	   for _, st := range statuses {
		   statusSet[strings.ToLower(strings.TrimSpace(st))] = true
	  }
	   s.mu.Lock()
	  snaps := make([]ledorphan.BatchSnapshot, 0, limit)
	  afterPassed := after == ""
	  for _, r := range s.rows {
		  if !afterPassed {
			   if r.ID == strings.TrimSpace(after) {
				   afterPassed = true
			  }
			   continue
		  }
		  if len(statusSet) > 0 && !statusSet[strings.ToLower(string(r.Status))] {
			   continue
		   }
		  snaps = append(snaps, batchToSnapshot(r.Copy()))
		   if len(snaps) >= limit {
			  break
		   }
	  }
	  s.mu.Unlock()
	  return snaps, nil
  }

// CheckIdemBlocked 检查指定 idem key 是否已被孤儿封堵引擎拦截。
// 若被封堵，Submit 应拒绝该 hop 并返回 ErrIdem。
// idemChecker 由调用方注入（ledorphan.Service.IsIdemBlocked 的函数化形式），
// 不在 ledbatch 包内直接引用 ledorphan，避免循环依赖。
  func (s *Service) CheckIdemBlocked(ctx context.Context, idemKey string, checker func(ctx context.Context, key string) (bool, error)) (bool, error) {
	  if checker == nil {
		  return false, nil
	  }
	   return checker(ctx, strings.TrimSpace(idemKey))
  }

  // LedbatchObserverAdapter 将 ledbatch.Service 适配为 ledorphan.BatchObserver。
  // 通过此适配器，ledorphan 包不需要直接 import ledbatch，避免循环依赖。
   type LedbatchObserverAdapter struct {
	   svc *Service
  }

  // NewLedbatchObserverAdapter 构造适配器。

  func NewLedbatchObserverAdapter(svc *Service) *LedbatchObserverAdapter {
	   return &LedbatchObserverAdapter{svc: svc}
   }

// GetBatch 实现 ledorphan.BatchObserver.GetBatch。
  func (a *LedbatchObserverAdapter) GetBatch(ctx context.Context, batchID string) (ledorphan.BatchSnapshot, error) {
	  if a == nil || a.svc == nil {
		  return ledorphan.BatchSnapshot{}, ledorphan.ErrBatchObserveNil
	   }
	  return a.svc.SnapshotBatch(ctx, batchID)
   }

// ListBatches 实现 ledorphan.BatchObserver.ListBatches。
  func (a *LedbatchObserverAdapter) ListBatches(ctx context.Context, statuses []string, after string, limit int) ([]ledorphan.BatchSnapshot, error) {
	  if a == nil || a.svc == nil {
		   return nil, ledorphan.ErrBatchObserveNil
	   }
	   return a.svc.ListSubmittedBatches(ctx, statuses, after, limit)
   }

// ── 类型转换辅助 ──────────────────────────────────────────────────────────────
// batchToSnapshot 将 ledbatch.Batch 转换为 ledorphan.BatchSnapshot。
   func batchToSnapshot(b Batch) ledorphan.BatchSnapshot {
	   snap := ledorphan.BatchSnapshot{
		   ID: b.ID,
		   Status:    string(b.Status),
		   Actor:     b.Actor,
		   Requested: b.Requested,
		  Submitted: b.Submitted,
		   Skipped:   b.Skipped,
		   Failed: b.Failed,
		   CreatedAt: b.CreatedAt,
		   UpdatedAt: b.UpdatedAt,
		  Items:     make([]ledorphan.BatchItemSnapshot, len(b.Items)),
	   }
	  for i, it := range b.Items {
		   snap.Items[i] = ledorphan.BatchItemSnapshot{
			  Seq:    it.Seq,
			  UserID: it.UserID,
			  Asset:  it.Asset,
			  Amount: it.Amount,
			   Direction:      it.Direction,
			   IdempotencyKey: it.IdempotencyKey,
			  Status:         string(it.Status),
			  LedgerID:       it.LedgerID,
			  Reason: it.Reason,
		  }
	  }
	   return snap
   }

// ── 孤儿发现辅助：批次终态统计 ────────────────────────────────────────────────
// BatchOrphanStat 批次内 item 落账状态统计（供孤儿检测摘要使用）。

type BatchOrphanStat struct {
	BatchID        string
	TotalItems     int
	SubmittedItems int // 有 LedgerID 的 item 数
	PendingItems   int // 仍为 pending 的 item 数
	SkippedItems int
	FailedItems  int
	LastUpdated  time.Time
}

// StatForOrphan 读取指定批次的落账状态统计，供孤儿检测前的快速预判使用。
// 若 SubmittedItems < TotalItems - SkippedItems - FailedItems，可能有孤儿。
   func (s *Service) StatForOrphan(batchID string) (BatchOrphanStat, error) {
	  if s == nil {
		   return BatchOrphanStat{}, ErrUnavailable
	   }
	  s.mu.Lock()
	  cur, ok := s.rows[strings.TrimSpace(batchID)]
	  if !ok {
		   s.mu.Unlock()
		   return BatchOrphanStat{}, ErrBatch
	  }
	   cp := cur.Copy()
	   s.mu.Unlock()
	   stat := BatchOrphanStat{
		   BatchID:     cp.ID,
		  TotalItems:  len(cp.Items),
		  LastUpdated: cp.UpdatedAt,
	   }
	   for _, it := range cp.Items {
		   switch it.Status {
		   case ItemSubmitted:
			   stat.SubmittedItems++
		  case ItemPending:
			  stat.PendingItems++
		  case ItemSkipped:
			   stat.SkippedItems++
		   case ItemFailed:
			  stat.FailedItems++
		  }
	   }
	   return stat, nil
   }
