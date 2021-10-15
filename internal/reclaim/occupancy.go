package reclaim

import (
	"sync"
	"time"
)

// occupyRecord 单条占用记录。
type occupyRecord struct {
	OutputKey string `json:"output_key"` // TxHash:Vout
	CaseID string `json:"case_id"`
	Chain      string    `json:"chain"`
	Asset      string    `json:"asset"`
	Amount     string    `json:"amount"`
	OccupiedAt time.Time `json:"occupied_at"`
}

// ReclaimOccupancy 管理所有活跃链上输出的预占记录；进程内单例。
type ReclaimOccupancy struct {
	mu     sync.Mutex
	holds  map[string]*occupyRecord // key = OutputKey
	byCase map[string]string        // caseID → OutputKey
}

// NewReclaimOccupancy 构造新的占用管理器。

  func NewReclaimOccupancy() *ReclaimOccupancy {
	  return &ReclaimOccupancy{
		   holds:  make(map[string]*occupyRecord),
		  byCase: make(map[string]string),
	   }
  }

// Occupy 为指定 caseID 预占 OutputKey。
// 若 OutputKey 已被不同 caseID 占用，返回 ErrOccupancyLocked。
// 若已被相同 caseID 占用（幂等），返回 nil。
   func (o *ReclaimOccupancy) Occupy(caseID, outputKey, chain, asset, amount string) error {
	   if o == nil {
		   return ErrUnavailable
	   }
	   o.mu.Lock()
	  defer o.mu.Unlock()
	  if rec, ok := o.holds[outputKey]; ok {
		   if rec.CaseID == caseID {
			   return nil // 幂等
		  }
		   return ErrOccupancyLocked
	   }
	  now := time.Now().UTC()
	  o.holds[outputKey] = &occupyRecord{
		  OutputKey: outputKey,
		   CaseID: caseID,
		   Chain:  chain,
		   Asset:  asset,
		  Amount: amount,
		  OccupiedAt: now,
	  }
	   o.byCase[caseID] = outputKey
	   return nil
   }

// Release 释放 OutputKey 的预占；owner（caseID）不匹配时静默忽略。
   func (o *ReclaimOccupancy) Release(caseID, outputKey string) {
	   if o == nil {
		   return
	  }
	   o.mu.Lock()
	  defer o.mu.Unlock()
	  rec, ok := o.holds[outputKey]
	   if !ok {
		   return
	   }
	   if rec.CaseID != caseID {
		  return
	   }
	   delete(o.holds, outputKey)
	  delete(o.byCase, caseID)
   }

// HolderOf 返回占用 OutputKey 的 caseID；不存在时返回空字符串。
   func (o *ReclaimOccupancy) HolderOf(outputKey string) string {
	   if o == nil {
		  return ""
	   }
	  o.mu.Lock()
	  defer o.mu.Unlock()
	   rec, ok := o.holds[outputKey]
// 若是，返回 (caseID, true)；调用方应接着贷记并发起 EventAutoCredit。
// 若否（未占用或占用方非 reserved 状态），返回 ("", false)。
// states 参数接受一个"状态查找函数"，避免包内循环依赖（由 Service 提供）。
   func (o *ReclaimOccupancy) TryAutoCredit(outputKey string, isReserved func(caseID string) bool) (caseID string, ok bool) {
	  if o == nil {
		   return "", false
	  }
	  o.mu.Lock()
	  rec, exists := o.holds[outputKey]
	   o.mu.Unlock()
	   if !exists {
		   return "", false
	   }
	  if isReserved != nil && isReserved(rec.CaseID) {
		   return rec.CaseID, true
	   }
	  return "", false
  }

   // Snapshot 返回所有活跃占用记录的快照（只读副本）。
  func (o *ReclaimOccupancy) Snapshot() []occupyRecord {
	  if o == nil {
		  return nil
	   }
	  o.mu.Lock()
	   defer o.mu.Unlock()
	  out := make([]occupyRecord, 0, len(o.holds))
	   for _, rec := range o.holds {
		   out = append(out, *rec)
	   }
	  return out
   }

// Len 返回当前活跃占用数量。
  func (o *ReclaimOccupancy) Len() int {
	  if o == nil {
		  return 0
	  }
	  o.mu.Lock()
	   defer o.mu.Unlock()
	  return len(o.holds)
  }

// OccupancyBook 占用只读账本（挂钩 latecreditgate，不改编排主链）。
type OccupancyBook struct {
	mu sync.Mutex
	byID map[string]*OccupancyView
	byOut map[string]string
}

// NewOccupancyBook 创建账本。

   func NewOccupancyBook() *OccupancyBook {
	  return &OccupancyBook{
		   byID:  make(map[string]*OccupancyView),
		   byOut: make(map[string]string),
	   }
  }

// Put 写入或覆盖占用（夹具 / 适配层）。

   func (b *OccupancyBook) Put(o OccupancyView) {
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   cp := o
	   b.byID[o.OccupancyID] = &cp
	   b.byOut[o.OutputKey.String()] = o.OccupancyID
  }

// GetActiveOccupancy 实现 ReclaimOccupancyReader。
  func (b *OccupancyBook) GetActiveOccupancy(key OutputKey) (*OccupancyView, error) {
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   id, ok := b.byOut[key.String()]
	  if !ok {
		   return nil, nil
	  }
	  o := b.byID[id]
	   if o == nil {
		   return nil, nil
	  }
	  cp := *o
	   return &cp, nil
  }

// CaseBook 工单只读账本。
type CaseBook struct {
	mu sync.Mutex
	byID map[string]*CaseView
	byOut map[string]string
}

// NewCaseBook 创建工单账本。
   func NewCaseBook() *CaseBook {
	   return &CaseBook{
		  byID:  make(map[string]*CaseView),
		  byOut: make(map[string]string),
	  }
   }

// Put 写入工单视图。
  func (b *CaseBook) Put(c CaseView) {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  cp := c
	  b.byID[c.CaseID] = &cp
	  b.byOut[c.OutputKey.String()] = c.CaseID
  }

// GetActiveCase 实现 ReclaimCaseReader。
   func (b *CaseBook) GetActiveCase(key OutputKey) (*CaseView, error) {
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   id, ok := b.byOut[key.String()]
	   if !ok {
		   return nil, nil
	  }
	   c := b.byID[id]
	   if c == nil || !c.IsActive() {
		   return nil, nil
	  }
	  cp := *c
	  return &cp, nil
   }

// MemoryTimeline 内存时间线。

type MemoryTimeline struct {
	mu     sync.Mutex
	events []ViewTimelineEvent
}

// AppendEvent 追加事件。
  func (t *MemoryTimeline) AppendEvent(e ViewTimelineEvent) error {
	   t.mu.Lock()
	  defer t.mu.Unlock()
	  t.events = append(t.events, e)
	   return nil
   }

// Snapshot 按 output_key 过滤。
  func (t *MemoryTimeline) Snapshot(key OutputKey) ViewTimeline {
	  t.mu.Lock()
	  defer t.mu.Unlock()
	  out := ViewTimeline{OutputKey: key}
	  ks := key.String()
	   for _, e := range t.events {
		  if e.OutputKey.String() == ks {
			  out.Events = append(out.Events, e)
		   }
	   }
	  return out
  }

// NowUTC 统一时钟辅助。
   func NowUTC() time.Time { return time.Now().UTC() }
