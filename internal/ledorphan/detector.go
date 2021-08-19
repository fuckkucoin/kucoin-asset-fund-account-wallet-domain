package ledorphan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
	"github.com/google/uuid"
)

// BatchObserver 只读接口：从 ledbatch 读取意图集合。
// 实现者由 ledbatch.Service 通过 ObserverAdapter 提供；
// 不回写 ledbatch 内部状态，仅观察。
type BatchObserver interface {
	// GetBatch 按 batch_id 读取批次快照（只读，不得修改）。
	GetBatch(ctx context.Context,
		batchID string) (BatchSnapshot, error)
	// ListBatches 按状态分页列出批次（用于周期扫描）。
	// statuses 为空则返回全部；after 为游标（上次最后一条 ID）。
	ListBatches(ctx context.Context, statuses []string, after string, limit int) ([]BatchSnapshot, error)
}

// LedgerObserver 只读接口：从 ledger 查询已落账 hop 集合。
// 实现者通过 ledger.Service 的 LookupByIdem 方法提供；
// 只做点查和集合查询，不触发新的分录。

type LedgerObserver interface {
	// LookupHopByIdem 按幂等键查找已落账 ledger entry ID。
	// 返回 ("", nil) 表示未落账；返回 (entryID, nil) 表示已落账。
	LookupHopByIdem(ctx context.Context, idemKey string) (entryID string, err error)
	// BatchLookupHopsByIdem 批量查询，减少 RTT；idemKeys 为输入，返回 idem→entryID 映射（缺失则无 key）。
	BatchLookupHopsByIdem(ctx context.Context, idemKeys []string) (map[string]string, error)
}

// BatchSnapshot ledbatch 批次的只读快照（避免直接依赖 ledbatch 包，打破循环）。

type BatchSnapshot struct {
	ID     string
	Status string // "open" | "submitted" | "halted" | "canceled"
	Actor string
	Requested string
	Submitted string
	Skipped   string
	Failed    string
	Items     []BatchItemSnapshot
	CreatedAt time.Time
	UpdatedAt time.Time
}

// BatchItemSnapshot 批次 item 的只读快照。

type BatchItemSnapshot struct {
	Seq    int
	UserID string
	Asset  string
	Amount string
	Direction      string
	IdempotencyKey string
	Status         string // "pending" | "submitted" | "skipped" | "failed"
	LedgerID string // 非空表示 batch 记录已 submitted 并有 ledger entry
	Reason   string
}

// ── 检测器 ───────────────────────────────────────────────────────────────────
// Detector 孤儿检测器。持有对 BatchObserver 和 LedgerObserver 的只读引用。
type Detector struct {
	batch  BatchObserver
	ledger LedgerObserver
	nowFn  func() time.Time
}

// NewDetector 构造检测器。必须同时注入 batch 和 ledger observer。

   func NewDetector(batch BatchObserver, ledger LedgerObserver) *Detector {
	  return &Detector{
		  batch:  batch,
		  ledger: ledger,
		  nowFn: func() time.Time { return time.Now().UTC() },
	   }
   }

  func (d *Detector) now() time.Time {
	   if d.nowFn != nil {
		  return d.nowFn()
	  }
	   return time.Now().UTC()
   }

// Detect 对指定批次执行孤儿发现，返回 OrphanReport。
// 若意图集合与落账集合完全一致（无孤儿），返回空 Items 列表（TotalOrphans==0）
// 以及 nil 错误，**不**返回 ErrNoOrphans——调用方通过 TotalOrphans==0 判断。
// 若批次处于 open/halted/canceled 状态，认为尚未进入半提交状态，
// 返回 ErrBatchNotFound（不适合运行孤儿检测）。
  func (d *Detector) Detect(ctx context.Context, batchID string, generation int64) (*OrphanReport, error) {
	  if d.batch == nil {
		   return nil, ErrBatchObserveNil
	   }
	  if d.ledger == nil {
		   return nil, ErrLedgerObserveNil
	  }
	   snap, err := d.batch.GetBatch(ctx, strings.TrimSpace(batchID))
	   if err != nil {
		  return nil, fmt.Errorf("%w: %v", ErrBatchNotFound, err)
	   }
	   // 只有 submitted 批次才有孤儿语义；其他状态不需要孤儿检测。
	   if snap.Status != "submitted" {
		   return nil, fmt.Errorf("%w: batch status is %q, not submitted", ErrBatchNotFound, snap.Status)
	   }
	  // 收集全部 item 的 idem keys（用于批量 ledger 查询，降低 RTT）。
	  idemKeys := make([]string,
		   0, len(snap.Items))
	   for _, it := range snap.Items {
		   if strings.TrimSpace(it.IdempotencyKey) != "" {
			   idemKeys = append(idemKeys, it.IdempotencyKey)
		   }
	  }
	  // 批量查询 ledger 落账集合。
	   ledgerMap,
		   err := d.ledger.BatchLookupHopsByIdem(ctx, idemKeys)
	  if err != nil {
		  return nil, fmt.Errorf("ledorphan detect: ledger batch lookup: %w", err)
	   }
	  now := d.now()
	  report := &OrphanReport{
		   ReportID: uuid.NewString(),
		   BatchID:  snap.ID,
		  Generation: generation,
		  Status:     ReportOpen,
		   CreatedAt:  now,
		   UpdatedAt:  now,
	   }
	  intentIdemKeys := make([]string,
		  0, len(snap.Items))
	   ledgerIdemKeys := make([]string, 0)
	   orphans := make([]OrphanItem, 0)
	  for _, it := range snap.Items {
		  idem := strings.TrimSpace(it.IdempotencyKey)
		  if idem == "" {
			   // idem key 为空的 item 无法进行孤儿关联，跳过（不能定性为孤儿）。
			  continue
		  }
		  intentIdemKeys = append(intentIdemKeys, idem)
		  entryID, inLedger := ledgerMap[idem]
		   batchSubmitted := it.Status == "submitted"
		   batchPending := it.Status == "pending"
		   switch {
		   case batchSubmitted && !inLedger:
			  // Hanging：batch 认为已 submitted（有 LedgerID 标记），但 ledger 中找不到。
			  // 最常见于：ledger.Create 成功后进程崩溃，in-memory batch state 随进程丢失，
			   // 重启后从持久化 dump 恢复的 batch 仍记为 submitted 但实际 ledger 无记录。
			   o := OrphanItem{
				   OrphanID: uuid.NewString(),
				  BatchID:  snap.ID,
				   Seq: it.Seq,
				  UserID: it.UserID,
				   Asset:  it.Asset,
				   Amount: it.Amount,
				   Direction: it.Direction,
				   IdempotencyKey: idem,
				   Kind:   OrphanHanging,
				  Status: OrphanDetected,
				  BatchLedgerID: it.LedgerID,
				  DetectedAt: now,
			  }
			  orphans = append(orphans, o)
		   case batchPending && inLedger:
			   // Missing：ledger 已落账，但 batch 状态仍为 pending。
			   // 场景：ledger.Create 成功后，更新 in-memory item.Status = submitted
			   // 之前进程崩溃；重启恢复的 dump 里该 item 仍是 pending。
			   o := OrphanItem{
				   OrphanID: uuid.NewString(),
				   BatchID:  snap.ID,
				   Seq: it.Seq,
				   UserID: it.UserID,
				  Asset: it.Asset,
				  Amount: it.Amount,
				   Direction: it.Direction,
				  IdempotencyKey: idem,
				  Kind:     OrphanMissing,
				  Status:   OrphanDetected,
				  LedgerID: entryID,
				  DetectedAt: now,
			   }
			   orphans = append(orphans, o)
		  }
		  if inLedger {
			  ledgerIdemKeys = append(ledgerIdemKeys, idem)
		   }
	  }
	  // 对 idem key 列表排序，保证哈希的确定性（幂等检测前提）。
	  sort.Strings(intentIdemKeys)
	  sort.Strings(ledgerIdemKeys)
	   report.Items = orphans
	  report.TotalOrphans = len(orphans)
	   for _, o := range orphans {
		   switch o.Kind {
		  case OrphanHanging:
			   report.HangingCount++
		  case OrphanMissing:
			  report.MissingCount++
		  }
	   }
	   report.IntentHash = hashIdemKeys(intentIdemKeys)
	  report.LedgerHash = hashIdemKeys(ledgerIdemKeys)
	   return report, nil
  }

// ScanAll 对符合条件的全部批次执行孤儿扫描。
// 扫描目标：status == "submitted" 的批次。
// after 为游标（上次最后一条 batch ID）；limit 控制单次扫描批次数量。
// 每条批次独立检测，单条失败不影响其余。

   func (d *Detector) ScanAll(ctx context.Context, generation int64, after string, limit int) ([]*OrphanReport, error) {
	  if d.batch == nil {
		  return nil, ErrBatchObserveNil
	  }
	  if limit <= 0 {
		  limit = 50
	  }
	   batches, err := d.batch.ListBatches(ctx, []string{"submitted"}, after, limit)
	   if err != nil {
		  return nil, fmt.Errorf("ledorphan scan all: list batches: %w", err)
	   }
	  reports := make([]*OrphanReport, 0, len(batches))
	  for _, b := range batches {
		  rep, err := d.Detect(ctx, b.ID, generation)
		   if err != nil {
			   // 单批次失败不中断整体扫描；记录但接着。
			  // 中此处应写入告警日志，此处仅跳过。
			  continue
		  }
		   reports = append(reports, rep)
	   }
	   return reports, nil
   }

// hashIdemKeys 对已排序的 idem key 列表计算 SHA-256 哈希，用于意图集合/落账集合指纹。
// 输入列表必须已排序；空列表返回固定的 "e3b0c44..." 空哈希。
  func hashIdemKeys(keys []string) string {
	   h := sha256.New()
	   for _, k := range keys {
		   // 以换行符分隔，防止拼接歧义（如 "ab"+"c" == "a"+"bc"）。
		  h.Write([]byte(k))
		   h.Write([]byte{'\n'})
	  }
	   return hex.EncodeToString(h.Sum(nil))
  }

// ── BatchObserver 的 ledbatch 适配器 ─────────────────────────────────────────
// LedbatchAdapter 将 ledbatch.Service 的公共接口适配为 BatchObserver。
// 调用者需确保 ledbatch.Service 已初始化；适配器本身只做类型转换，不持有 mu。

type LedbatchAdapter struct {
	// ListBatchesFn 从 ledbatch 获取批次列表的函数（由 ledbatch 注册时注入）。
	ListBatchesFn func(ctx context.Context,
		   statuses []string, after string, limit int) ([]BatchSnapshot, error)
	// GetBatchFn 从 ledbatch 获取单个批次快照的函数。
	GetBatchFn func(ctx context.Context, batchID string) (BatchSnapshot, error)
}

   func (a *LedbatchAdapter) GetBatch(ctx context.Context, batchID string) (BatchSnapshot, error) {
	   if a == nil || a.GetBatchFn == nil {
		  return BatchSnapshot{}, ErrBatchObserveNil
	   }
	  return a.GetBatchFn(ctx, batchID)
  }

  func (a *LedbatchAdapter) ListBatches(ctx context.Context, statuses []string, after string, limit int) ([]BatchSnapshot, error) {
	   if a == nil || a.ListBatchesFn == nil {
		  return nil, ErrBatchObserveNil
	   }
	  return a.ListBatchesFn(ctx, statuses, after, limit)
  }

// ── LedgerObserver 的 ledger 适配器 ──────────────────────────────────────────
// LedgerObserveAdapter 将 ledger 的 idem 查询接口适配为 LedgerObserver。

type LedgerObserveAdapter struct {
	// LookupFn 单条 idem 查询函数。
	LookupFn func(ctx context.Context, idemKey string) (entryID string, err error)
}

  func (a *LedgerObserveAdapter) LookupHopByIdem(ctx context.Context, idemKey string) (string, error) {
	   if a == nil || a.LookupFn == nil {
		   return "", ErrLedgerObserveNil
	   }
	  return a.LookupFn(ctx, idemKey)
  }

// BatchLookupHopsByIdem 批量点查：循环调用单条查询（可被替换为真正的 IN 查询实现）。

   func (a *LedgerObserveAdapter) BatchLookupHopsByIdem(ctx context.Context, idemKeys []string) (map[string]string, error) {
	   if a == nil || a.LookupFn == nil {
		  return nil, ErrLedgerObserveNil
	  }
	  result := make(map[string]string,
		   len(idemKeys))
	  for _, k := range idemKeys {
		  entryID, err := a.LookupFn(ctx, k)
		  if err != nil {
			   // 单条查询失败：本 idem key 视为未落账，不中断批量查询。
			  // 中应记录警告，本处保守返回接着。
			   continue
		  }
		  if entryID != "" {
			   result[k] = entryID
		  }
	  }
	   return result, nil
  }

// ── 检测结果摘要 ──────────────────────────────────────────────────────────────
// DetectSummary 孤儿检测摘要，供 worker 日志和 metrics 使用。
type DetectSummary struct {
	BatchID    string
	Generation int64
	TotalItems int
	OrphanCount  int
	HangingCount int
	MissingCount int
	IntentHash string
	LedgerHash string
	DetectedAt time.Time
}

// Summarize 从 OrphanReport 提取摘要。

   func Summarize(rep *OrphanReport, totalItems int) DetectSummary {
	  if rep == nil {
		  return DetectSummary{}
	   }
	  return DetectSummary{
		   BatchID:    rep.BatchID,
		  Generation: rep.Generation,
		   TotalItems: totalItems,
		  OrphanCount: rep.TotalOrphans,
		  HangingCount: rep.HangingCount,
		   MissingCount: rep.MissingCount,
		   IntentHash: rep.IntentHash,
		   LedgerHash: rep.LedgerHash,
		   DetectedAt: rep.CreatedAt,
	   }
  }
