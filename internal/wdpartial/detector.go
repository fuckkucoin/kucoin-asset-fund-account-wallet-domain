package wdpartial

import (
	"strings"
	"time"
	"github.com/google/uuid"
)

// BatchReader wdbatch 只读视图接口，供 Detector 消费。
type BatchReader interface {
	// BatchItems 返回批次中所有 item（含 failed/skipped）。
	// 批次不存在时返回 ErrBatchNotFound。
	BatchItems(batchID string) (items []BatchItemView, requested, submitted, skipped, failed string, err error)
}

// BatchItemView wdbatch item 的只读视图。
type BatchItemView struct {
	Seq int
	UserID string
	Asset string
	Amount    string
	Chain     string
	ToAddress string
	IdempotencyKey string
	Status         string // "pending"/"submitted"/"skipped"/"failed"
	Reason         string
	LedgerID string
}

// ResidueBook 残留项存储接口（内存或持久化实现均可）。
type ResidueBook interface {
	// ExistsByBatchSeq 检查 (wdbatchID, seq, idemKey) 是否已有残留记录。
	ExistsByBatchSeq(wdbatchID string, seq int, idemKey string) bool
	// Save 保存一批新残留项（幂等追加）。
	Save(items []*ResidueItem) error
	// ListByBatch 返回指定批次的所有残留项。
	ListByBatch(wdbatchID string) ([]*ResidueItem, error)
	// Get 按 ID 获取单个残留项。
	Get(id string) (*ResidueItem, error)
	// Update 更新单个残留项（状态流转）。
	Update(item *ResidueItem) error
}

// Detector Partial Residue Detector。
type Detector struct {
	reader   BatchReader
	observer OccupyObserver
	book     ResidueBook
	clock    func() time.Time
}

// NewDetector 构造 Detector。

  func NewDetector(reader BatchReader, observer OccupyObserver, book ResidueBook) *Detector {
	  return &Detector{
		  reader:   reader,
		   observer: safeObserver(observer),
		   book:     book,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入时钟。

  func (d *Detector) WithClock(c func() time.Time) *Detector {
	  if d != nil && c != nil {
		   d.clock = c
	  }
	  return d
  }

   // Detect 对单个 wdbatch 批次执行残留检测，返回 DetectResult。
  // 调用者无需持有 ResumeLease；检测本身是只读轻量操作。
   // 若批次不存在或已无 failed/skipped 项，返回空 DetectResult（非错误）。

  func (d *Detector) Detect(batchID string) (*DetectResult, error) {
	  if d == nil {
		   return nil, ErrUnavailable
	  }
	   batchID = strings.TrimSpace(batchID)
	  if batchID == "" {
		  return nil, ErrBatchID
	   }
	   items, _, _, _, _, err := d.reader.BatchItems(batchID)
	  if err != nil {
		   return nil, err
	  }
	   result := &DetectResult{
		  WDBatchID: batchID,
		   TotalItems: len(items),
	  }
	   now := d.clock()
	   var newResidues []*ResidueItem
	  for _, it := range items {
		   switch it.Status {
		   case "failed":
			   result.FailedItems++
		   case "skipped":
			   result.SkippedItems++
		  default:
			  continue
		  }
		   // 幂等：已存在则跳过
		  if d.book.ExistsByBatchSeq(batchID, it.Seq, it.IdempotencyKey) {
			   result.ResidueCount++
			   continue
		   }
		  // 只读占用探测
		   frozen,
			   ferr := d.observer.ObserveFreeze(it.UserID, it.Asset, it.LedgerID)
		   if ferr != nil {
			   result.Errors = append(result.Errors, ferr)
			   frozen = false
		   }
		  occupied, occupyLedgerID, oerr := d.observer.ObserveLedger(it.UserID, it.Asset, it.IdempotencyKey)
		  if oerr != nil {
			  result.Errors = append(result.Errors, oerr)
			   occupied = false
		  }
		   // 若确认无任何占用，记录 no_occupy 快照供后续收口
		   origin := ResidueOriginFailed
		   if it.Status == "skipped" {
			  origin = ResidueOriginSkipped
		   }
		   residue := &ResidueItem{
			  ID:        uuid.NewString(),
			   WDBatchID: batchID,
			   Seq: it.Seq,
			   UserID: it.UserID,
			   Asset:  it.Asset,
			   Amount: it.Amount,
			  Chain:  it.Chain,
			  ToAddress: it.ToAddress,
			   IdemKey:    it.IdempotencyKey,
			  Origin:     origin,
			  FailReason: it.Reason,
			  Status:         ResidueOpen,
			   FreezeOccupied: frozen,
			  LedgerOccupied: occupied,
			  OccupyLedgerID: occupyLedgerID,
			  DetectedAt:     now,
			  UpdatedAt:      now,
		   }
		  newResidues = append(newResidues, residue)
		   result.ResidueCount++
	  }
	   if len(newResidues) > 0 {
		   if err := d.book.Save(newResidues); err != nil {
			   return result, err
		   }
		  result.NewResidues = newResidues
	   }
	   return result, nil
   }

// ScanBatches 批量扫描一组批次，产出 WorkerScanResult。
// batchIDs 为空时不执行任何操作。

   func (d *Detector) ScanBatches(batchIDs []string) WorkerScanResult {
	  var out WorkerScanResult
	   for _, bid := range batchIDs {
		  dr, err := d.Detect(bid)
		   if err != nil {
			  out.Errors = append(out.Errors, err)
			   continue
		  }
		  out.ScannedBatches++
		  out.NewResidues += len(dr.NewResidues)
		   out.Errors = append(out.Errors,
			   dr.Errors...)
	   }
	   return out
   }

// RefreshOccupancy 重新探测指定残留项的占用状态（不改变 Status）。
// 供巡检 worker 刷新快照，避免占用状态已解除而 ResidueItem.FreezeOccupied 仍为 true。
   func (d *Detector) RefreshOccupancy(item *ResidueItem) error {
	  if d == nil || item == nil {
		  return ErrUnavailable
	  }
	  frozen,
		   ferr := d.observer.ObserveFreeze(item.UserID, item.Asset, item.OccupyLedgerID)
	  if ferr != nil {
		  return ferr
	   }
	  occupied, occupyLedgerID, oerr := d.observer.ObserveLedger(item.UserID, item.Asset, item.IdemKey)
	  if oerr != nil {
		   return oerr
	   }
	   item.FreezeOccupied = frozen
	  if !occupied {
		   item.LedgerOccupied = false
	  } else {
		  item.LedgerOccupied = occupied
		  if occupyLedgerID != "" {
			  item.OccupyLedgerID = occupyLedgerID
		   }
	   }
	  item.UpdatedAt = d.clock()
	  return nil
   }
