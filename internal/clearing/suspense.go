package clearing

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
)

// SuspenseStatus 挂账状态枚举。

type SuspenseStatus string

const (
	SuspenseOpen     SuspenseStatus = "open"
	SuspenseResolved SuspenseStatus = "resolved"
	SuspenseRejected SuspenseStatus = "rejected"
)

// DefaultSuspenseSLA 挂账超时升级 SLA（默认 48 小时）。
const DefaultSuspenseSLA = 48 * time.Hour

// ErrSuspenseNotOpen 挂账已非 open 状态，不可再跃迁。

var ErrSuspenseNotOpen = fmt.Errorf("suspense entry is not in open status")

// ErrSuspenseAmount 挂账金额非正。
var ErrSuspenseAmount = fmt.Errorf("suspense amount must be positive")

// ErrSuspenseNote 处置备注不可为空。
var ErrSuspenseNote = fmt.Errorf("suspense disposition note required")

// SuspenseEntry 挂账科目分录。
type SuspenseEntry struct {
	ID string `json:"id"`
	BatchID string `json:"batch_id"` // 来源清算批次
	ItemID string `json:"item_id,omitempty"` // 来源清算 Item（可选）
	ExceptionID string `json:"exception_id,omitempty"` // 关联的 Exception ID（可选）
	ReconDiffID string `json:"recon_diff_id,omitempty"` // recon 差异单只读引用（不调用其分类器）
	UserID      string `json:"user_id"`
	Asset       string `json:"asset"`
	Amount string `json:"amount"` // 挂账金额（正数）
	Direction string         `json:"direction"` // "debit" | "credit"
	Reason    string         `json:"reason"`
	Status    SuspenseStatus `json:"status"`
	Note        string     `json:"note,omitempty"` // 处置备注
	EscalateAt  time.Time  `json:"escalate_at"`    // SLA 升级触发时间
	EscalatedAt *time.Time `json:"escalated_at,omitempty"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	RejectedAt *time.Time `json:"rejected_at,omitempty"`
	Actor      string `json:"actor"`
	DisposedBy string `json:"disposed_by,omitempty"`
	Fence     persist.Fence `json:"fence"`
	CreatedAt time.Time     `json:"created_at"`
}

// SuspenseBook 挂账账册，单进程内存实现。
type SuspenseBook struct {
	mu sync.Mutex
	entries map[string]*SuspenseEntry
	byBatch map[string][]string
	fence   *persist.Table
	clock func() time.Time
	sla time.Duration
}

// NewSuspenseBook 初始化挂账账册。

  func NewSuspenseBook() *SuspenseBook {
	  return &SuspenseBook{
		  entries: map[string]*SuspenseEntry{},
		   byBatch: map[string][]string{},
		   fence:   persist.NewTable(),
		  clock: func() time.Time { return time.Now().UTC() },
		  sla:   DefaultSuspenseSLA,
	   }
   }

// WithClock 注入时钟（用）。

   func (sb *SuspenseBook) WithClock(c func() time.Time) *SuspenseBook {
	  if sb != nil && c != nil {
		   sb.clock = c
	  }
	  return sb
  }

  // WithSLA 设置 SLA 超时时长。
   func (sb *SuspenseBook) WithSLA(d time.Duration) *SuspenseBook {
	  if sb != nil && d > 0 {
		   sb.sla = d
	  }
	  return sb
  }

// Park 新建挂账分录（open 状态）。
// direction 取 "debit" 或 "credit"。

  func (sb *SuspenseBook) Park(batchID, itemID, exceptionID, reconDiffID,
	   userID, asset, amount, direction, reason, actor string) (SuspenseEntry, error) {
	  if sb == nil {
		   return SuspenseEntry{}, ErrUnavailable
	   }
	  act, err := actorOK(actor)
	   if err != nil {
		  return SuspenseEntry{}, err
	   }
	  amt, err := money.Parse(amount)
	  if err != nil || !amt.IsPositive() {
		  return SuspenseEntry{},
			   ErrSuspenseAmount
	  }
	  direction = strings.TrimSpace(direction)
	   if direction != "debit" && direction != "credit" {
		   return SuspenseEntry{}, ErrAmount
	   }
	   reason = strings.TrimSpace(reason)
	   if reason == "" {
		   return SuspenseEntry{}, ErrHoliday // 复用 ErrHoliday 为"参数缺失"；实际可扩展
	  }
	   now := sb.clock()
	   entry := SuspenseEntry{
		  ID: uuid.NewString(),
		  BatchID: strings.TrimSpace(batchID),
		  ItemID:      strings.TrimSpace(itemID),
		   ExceptionID: strings.TrimSpace(exceptionID),
		  ReconDiffID: strings.TrimSpace(reconDiffID),
		  UserID:    strings.TrimSpace(userID),
		   Asset:     strings.TrimSpace(asset),
		  Amount:    amount,
		  Direction: direction,
		   Reason:    reason,
		  Status: SuspenseOpen,
		   Actor:      act,
		  EscalateAt: now.Add(sb.sla),
		   CreatedAt: now,
	   }
	  sb.mu.Lock()
	  defer sb.mu.Unlock()
	  f, err := sb.fence.Bump(entry.ID, persist.NewToken())
	  if err != nil {
		  return SuspenseEntry{}, err
	  }
	  entry.Fence = f
	   cp := entry
	   sb.entries[entry.ID] = &cp
	   sb.byBatch[entry.BatchID] = append(sb.byBatch[entry.BatchID], entry.ID)
	   return cp, nil
  }

// Resolve 将 open 的挂账标为 resolved。须填处置备注。
  func (sb *SuspenseBook) Resolve(id, actor, note string, epoch int64, token string) (SuspenseEntry, error) {
	  return sb.dispose(id, actor, note, epoch, token, SuspenseResolved)
   }

// Reject 将 open 的挂账标为 rejected。须填处置备注。
  func (sb *SuspenseBook) Reject(id, actor, note string, epoch int64, token string) (SuspenseEntry, error) {
	   return sb.dispose(id, actor, note, epoch, token, SuspenseRejected)
  }

// MarkEscalated 记录已升级时间戳（由 Worker 调用）。

   func (sb *SuspenseBook) MarkEscalated(id string) (SuspenseEntry, error) {
	   if sb == nil {
		   return SuspenseEntry{}, ErrUnavailable
	  }
	  sb.mu.Lock()
	   defer sb.mu.Unlock()
	  entry, ok := sb.entries[strings.TrimSpace(id)]
	   if !ok {
		  return SuspenseEntry{}, ErrNotFound
	   }
	   if entry.Status != SuspenseOpen {
		   return *entry, nil
	   }
	   now := sb.clock()
	  entry.EscalatedAt = &now
	  return *entry, nil
   }

// ScanOverdue 扫描 SLA 已到期但未升级的 open 挂账。
  func (sb *SuspenseBook) ScanOverdue(now time.Time) []SuspenseEntry {
	  if sb == nil {
		  return nil
	  }
	   if now.IsZero() {
		   now = sb.clock()
	   }
	   sb.mu.Lock()
	  defer sb.mu.Unlock()
	  out := make([]SuspenseEntry, 0)
	   for _, e := range sb.entries {
		   if e.Status == SuspenseOpen && e.EscalatedAt == nil && !now.Before(e.EscalateAt) {
			  out = append(out, *e)
		   }
	   }
	   return out
  }

// HasOpenEntries 检查指定批次是否还有 open 的挂账（关账前的前置检查）。

   func (sb *SuspenseBook) HasOpenEntries(batchID string) bool {
	   if sb == nil {
		   return false
	  }
	  sb.mu.Lock()
	   defer sb.mu.Unlock()
	   for _, id := range sb.byBatch[strings.TrimSpace(batchID)] {
		   if e := sb.entries[id]; e != nil && e.Status == SuspenseOpen {
			  return true
		  }
	  }
	  return false
  }

// Get 按 ID 查挂账分录。
  func (sb *SuspenseBook) Get(id string) (SuspenseEntry, error) {
	   if sb == nil {
		   return SuspenseEntry{}, ErrUnavailable
	  }
	  sb.mu.Lock()
	   defer sb.mu.Unlock()
	   e, ok := sb.entries[strings.TrimSpace(id)]
	   if !ok {
		  return SuspenseEntry{}, ErrNotFound
	  }
	   return *e, nil
  }

// ListByBatch 按批次列出挂账分录。
   func (sb *SuspenseBook) ListByBatch(batchID string) []SuspenseEntry {
	  if sb == nil {
		   return nil
	  }
	  sb.mu.Lock()
	   defer sb.mu.Unlock()
	   ids := sb.byBatch[strings.TrimSpace(batchID)]
	   out := make([]SuspenseEntry, 0, len(ids))
	  for _, id := range ids {
		  if e := sb.entries[id]; e != nil {
			   out = append(out, *e)
		  }
	   }
	   return out
   }

// ListOpen 列出所有 open 的挂账分录。
  func (sb *SuspenseBook) ListOpen() []SuspenseEntry {
	   if sb == nil {
		   return nil
	   }
	   sb.mu.Lock()
	  defer sb.mu.Unlock()
	  out := make([]SuspenseEntry, 0)
	   for _, e := range sb.entries {
		   if e.Status == SuspenseOpen {
			   out = append(out, *e)
		   }
	  }
	  return out
  }

// dispose 内部处置状态跃迁。
   func (sb *SuspenseBook) dispose(id, actor, note string, epoch int64, token string, to SuspenseStatus) (SuspenseEntry, error) {
	   if sb == nil {
		  return SuspenseEntry{}, ErrUnavailable
	   }
	   act, err := actorOK(actor)
	   if err != nil {
		  return SuspenseEntry{}, err
	   }
	  note = strings.TrimSpace(note)
	   if note == "" {
		   return SuspenseEntry{}, ErrSuspenseNote
	   }
	   if err := sb.fence.Check(id, epoch, token); err != nil {
		  return SuspenseEntry{}, err
	  }
	   sb.mu.Lock()
	   defer sb.mu.Unlock()
	  entry, ok := sb.entries[strings.TrimSpace(id)]
	   if !ok {
		  return SuspenseEntry{}, ErrNotFound
	   }
	  if entry.Status != SuspenseOpen {
		   return *entry, ErrSuspenseNotOpen
	  }
	   f, err := sb.fence.Bump(id, persist.NewToken())
	   if err != nil {
		   return SuspenseEntry{}, err
	   }
	  now := sb.clock()
	   entry.Status = to
	  entry.DisposedBy = act
	  entry.Note = note
	   entry.Fence = f
	   switch to {
	  case SuspenseResolved:
		   entry.ResolvedAt = &now
	   case SuspenseRejected:
		   entry.RejectedAt = &now
	  }
	   return *entry, nil
  }
