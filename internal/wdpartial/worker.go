package wdpartial

import (
	"context"
	"strings"
	"time"
)

// BatchLister 获取需要巡检的 wdbatch 批次 ID 列表（只读）。

type BatchLister interface {
	// SubmittedBatchIDs 返回 status=submitted 且无收口单的批次 ID 列表。
	SubmittedBatchIDs(limit int) ([]string, error)
}

// ─── ResidueScanner ───────────────────────────────────────────────────────────
// ResidueScanner 后台残留扫描 worker。

type ResidueScanner struct {
	detector BatchLister
	detect   *Detector
	interval time.Duration
	limit int
}

// NewResidueScanner 构造 ResidueScanner。
  func NewResidueScanner(lister BatchLister, detect *Detector, interval time.Duration, limit int) *ResidueScanner {
	  if interval <= 0 {
		   interval = 30 * time.Second
	   }
	  if limit <= 0 {
		   limit = 50
	   }
	   return &ResidueScanner{
		  detector: lister,
		  detect:   detect,
		   interval: interval,
		  limit: limit,
	   }
   }

// RunOnce 执行一次扫描，返回结果摘要。

   func (w *ResidueScanner) RunOnce() WorkerScanResult {
	  if w == nil || w.detector == nil {
		   return WorkerScanResult{Errors: []error{ErrUnavailable}}
	   }
	  ids, err := w.detector.SubmittedBatchIDs(w.limit)
	  if err != nil {
		  return WorkerScanResult{Errors: []error{err}}
	  }
	   return w.detect.ScanBatches(ids)
  }

// Run 循环运行 ResidueScanner，直到 ctx 取消。

  func (w *ResidueScanner) Run(ctx context.Context) {
	   ticker := time.NewTicker(w.interval)
	   defer ticker.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			   return
		   case <-ticker.C:
			   w.RunOnce()
		  }
	   }
  }

// ─── ReleaseExecutor ──────────────────────────────────────────────────────────
// ReleaseExecutor 后台释放执行 worker。
// 对确认无任何占用的 open 残留项，以系统 actor 自动记录 ProofKindNoOccupy 证明。
// 这种情况发生在：账本已取消或未创建，且冻结位已解除，但 ResidueItem 仍为 open。

type ReleaseExecutor struct {
	book ResidueBook
	proofMgr *ProofManager
	mutex    *MutexManager
	detector *Detector
	actor string
	interval time.Duration
	limit int
}

// NewReleaseExecutor 构造 ReleaseExecutor。

  func NewReleaseExecutor(book ResidueBook, proofMgr *ProofManager, mutex *MutexManager, detect *Detector, actor string, interval time.Duration, limit int) *ReleaseExecutor {
	   if interval <= 0 {
		   interval = 60 * time.Second
	   }
	   if limit <= 0 {
		  limit = 100
	  }
	   actor = strings.TrimSpace(actor)
	  if actor == "" {
		  actor = "system-release-executor"
	  }
	   return &ReleaseExecutor{
		   book:     book,
		  proofMgr: proofMgr,
		   mutex: mutex,
		  detector: detect,
		  actor:    actor,
		  interval: interval,
		  limit: limit,
	  }
  }

// RunOnce 执行一次释放扫描。

   func (w *ReleaseExecutor) RunOnce(batchIDs []string) ReleaseExecResult {
	  var result ReleaseExecResult
	  for _, bid := range batchIDs {
		   items, err := w.book.ListByBatch(bid)
		   if err != nil {
			  result.Errors = append(result.Errors, err)
			  continue
		   }
		  // 获取世代锁
		  lease, err := w.mutex.Acquire(bid, w.actor)
		   if err != nil {
			  // 如果锁冲突，跳过此批次（其他 worker 正在处理）
			  result.Skipped++
			   continue
		  }
		   for _, it := range items {
			  if result.Processed >= w.limit {
				   break
			   }
			  if !it.Open() {
				   continue
			  }
			  // 刷新占用状态
			  if rerr := w.detector.RefreshOccupancy(it); rerr != nil {
				  result.Errors = append(result.Errors, rerr)
				  continue
			  }
			   if err := w.book.Update(it); err != nil {
				  result.Errors = append(result.Errors, err)
				  continue
			  }
			  result.Processed++
			  // 若确认无任何占用，自动记录 no_occupy 证明
			  if !it.FreezeOccupied && !it.LedgerOccupied {
				  _, perr := w.proofMgr.RecordProof(RecordProofReq{
					   WDBatchID: bid,
					  ResidueID: it.ID,
					   Kind: ProofKindNoOccupy,
					  Actor:       w.actor,
					  Summary:     "auto: no freeze and no ledger occupancy detected by release executor",
					  LeaseHolder: w.actor,
					   LeaseGen:    lease.Gen,
				   })
				  if perr != nil {
					  result.Errors = append(result.Errors, perr)
					  continue
				  }
				   result.Released++
			  }
		  }
		  // 主动释放世代锁
		   _ = w.mutex.Release(bid, w.actor, lease.Gen)
	  }
	  return result
   }

// Run 循环运行 ReleaseExecutor。
   func (w *ReleaseExecutor) Run(ctx context.Context, lister BatchLister) {
	   ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			   return
		  case <-ticker.C:
			   ids, err := lister.SubmittedBatchIDs(w.limit)
			   if err != nil {
				   continue
			   }
			  w.RunOnce(ids)
		   }
	   }
   }

// ─── LeaseReaper ──────────────────────────────────────────────────────────────
// LeaseReaper 过期租约清理 worker（辅助 GC，不影响主流程）。
type LeaseReaper struct {
	leaseBook LeaseBook
	lister    BatchLister
	interval  time.Duration
	clock     func() time.Time
}

// NewLeaseReaper 构造 LeaseReaper。

   func NewLeaseReaper(leaseBook LeaseBook, lister BatchLister, interval time.Duration) *LeaseReaper {
	  if interval <= 0 {
		   interval = 10 * time.Minute
	   }
	  return &LeaseReaper{
		  leaseBook: leaseBook,
		   lister: lister,
		  interval: interval,
		   clock:    func() time.Time { return time.Now().UTC() },
	   }
  }

// WithClock 注入时钟。

   func (r *LeaseReaper) WithClock(c func() time.Time) *LeaseReaper {
	  if r != nil && c != nil {
		   r.clock = c
	   }
	  return r
   }

   // Run 循环运行 LeaseReaper。
  func (r *LeaseReaper) Run(ctx context.Context) {
	  ticker := time.NewTicker(r.interval)
	  defer ticker.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			   return
		  case <-ticker.C:
			  // LeaseBook 实现可在此扫描过期记录进行 GC；
			   // 内存实现不需要主动 GC（Acquire 时懒惰检查过期）。
		   }
	   }
  }

// ─── MockBatchLister ─────────────────────────────────────────────────────────
// MockBatchLister 用内存 BatchLister。
type MockBatchLister struct {
	IDs []string
}

// SubmittedBatchIDs 返回预设的批次 ID 列表。

   func (m *MockBatchLister) SubmittedBatchIDs(limit int) ([]string, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	  }
	  out := m.IDs
	   if limit > 0 && len(out) > limit {
		  out = out[:limit]
	  }
	   return out, nil
  }

// ─── MockBatchReader ──────────────────────────────────────────────────────────
// MockBatchReader 用内存 BatchReader。
type MockBatchReader struct {
	Batches map[string]mockBatch
}

type mockBatch struct {
	Items []BatchItemView
	Requested string
	Submitted string
	Skipped   string
	Failed    string
}

// NewMockBatchReader 构造 MockBatchReader。
  func NewMockBatchReader() *MockBatchReader {
	  return &MockBatchReader{Batches: make(map[string]mockBatch)}
  }

// AddBatch 添加批次数据。

   func (r *MockBatchReader) AddBatch(id string, items []BatchItemView, requested, submitted, skipped, failed string) {
	  r.Batches[id] = mockBatch{
		  Items:     items,
		  Requested: requested,
		  Submitted: submitted,
		  Skipped:   skipped,
		   Failed:    failed,
	  }
  }

// BatchItems 返回批次的 item 列表。

  func (r *MockBatchReader) BatchItems(batchID string) ([]BatchItemView, string, string, string, string, error) {
	  if r == nil {
		   return nil, "", "", "", "", ErrUnavailable
	   }
	  b, ok := r.Batches[batchID]
	   if !ok {
		  return nil, "", "", "", "", ErrBatchNotFound
	   }
	  items := make([]BatchItemView, len(b.Items))
	   copy(items, b.Items)
	  return items, b.Requested, b.Submitted, b.Skipped, b.Failed, nil
   }
