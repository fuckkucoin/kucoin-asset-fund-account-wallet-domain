package fiatbreak

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/validate"
)

// SuspenseCreateReq 创建挂账条目请求。
type SuspenseCreateReq struct {
	BreakID   string
	BookingID string
	Reason    SuspenseReason
	Asset     string
	Amount    string
	Currency string
	SLADeadline time.Time
}

// SuspenseService 挂账条目服务。
// 保证每条挂账最终 released 或 consumed；泄漏由巡逻 worker 检出。
type SuspenseService struct {
	mu sync.RWMutex
	// entries 按 ID 存储挂账条目。
	entries map[string]*SuspenseEntry
	// byBreak 按 break_id 索引 suspense_id。
	byBreak map[string]string
	// byBooking 按 booking_id 索引 suspense_id（快速定位）。
	byBooking map[string]string
	clock func() time.Time
}

// NewSuspenseService 创建挂账服务实例。
   func NewSuspenseService() *SuspenseService {
	   return &SuspenseService{
		  entries: map[string]*SuspenseEntry{},
		  byBreak:   map[string]string{},
		  byBooking: map[string]string{},
		   clock:     func() time.Time { return time.Now().UTC() },
	   }
   }

// now 可注入时钟。

   func (s *SuspenseService) now() time.Time {
	  if s == nil || s.clock == nil {
		  return time.Now().UTC()
	   }
	  return s.clock()
  }

// Create 创建挂账条目。
// amount 必须为正；SLADeadline 必须在当前时间之后。

   func (s *SuspenseService) Create(_ context.Context, req SuspenseCreateReq) (SuspenseEntry, error) {
	  if s == nil {
		   return SuspenseEntry{}, ErrUnavailable
	  }
	   amt, err := decimal.NewFromString(strings.TrimSpace(req.Amount))
	  if err != nil || !amt.IsPositive() {
		   return SuspenseEntry{}, fmt.Errorf("%w: suspense amount must be positive", ErrAmount)
	  }
	  now := s.now()
	  if !req.SLADeadline.After(now) {
		   return SuspenseEntry{}, fmt.Errorf("%w: sla_deadline must be in future", ErrPolicy)
	  }
	   e := &SuspenseEntry{
		  ID:        uuid.NewString(),
		   BreakID:   strings.TrimSpace(req.BreakID),
		  BookingID: strings.TrimSpace(req.BookingID),
		  Reason:    req.Reason,
		  State:  SuspenseOpen,
		   Asset:  strings.ToUpper(strings.TrimSpace(req.Asset)),
		  Amount: amt.StringFixed(8),
		   Currency: strings.ToUpper(strings.TrimSpace(req.Currency)),
		  SLADeadline: req.SLADeadline,
		  CreatedAt:   now,
		   UpdatedAt:   now,
	  }
	   s.mu.Lock()
	   s.entries[e.ID] = e
	   if e.BreakID != "" {
		   s.byBreak[e.BreakID] = e.ID
	   }
	  if e.BookingID != "" {
		  s.byBooking[e.BookingID] = e.ID
	  }
	   s.mu.Unlock()
	  metrics.FiatSuspense.WithLabelValues(string(req.Reason), string(SuspenseOpen)).Inc()
	  return e.copy(), nil
  }

// Get 按 ID 获取挂账条目。
   func (s *SuspenseService) Get(id string) (SuspenseEntry, error) {
	   if s == nil {
		   return SuspenseEntry{}, ErrUnavailable
	  }
	  s.mu.RLock()
	   e, ok := s.entries[strings.TrimSpace(id)]
	   s.mu.RUnlock()
	  if !ok {
		  return SuspenseEntry{}, ErrNotFound
	  }
	   return e.copy(), nil
  }

// GetByBreak 按断裂 ID 获取挂账条目。

   func (s *SuspenseService) GetByBreak(breakID string) (SuspenseEntry, error) {
	  if s == nil {
		   return SuspenseEntry{}, ErrUnavailable
	  }
	  s.mu.RLock()
	   id, ok := s.byBreak[strings.TrimSpace(breakID)]
	   if !ok {
		  s.mu.RUnlock()
		  return SuspenseEntry{}, ErrNotFound
	  }
	  e := s.entries[id]
	   s.mu.RUnlock()
	  if e == nil {
		  return SuspenseEntry{}, ErrNotFound
	  }
	  return e.copy(), nil
  }

// List 列举挂账条目（按状态过滤；limit≤0 默认 50）。

  func (s *SuspenseService) List(filterState SuspenseState, limit int) []SuspenseEntry {
	  if s == nil {
		  return nil
	  }
	  if limit <= 0 {
		  limit = 50
	  }
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  out := make([]SuspenseEntry,
		   0, limit)
	   for _, e := range s.entries {
		  if filterState != "" && e.State != filterState {
			  continue
		  }
		  out = append(out, e.copy())
		  if len(out) >= limit {
			   break
		  }
	   }
	  return out
   }

// Release 挂账释放：差额归还（无损场景，如写核销后 PSP 已打回款项）。
   func (s *SuspenseService) Release(_ context.Context, id, actor, note string) (SuspenseEntry, error) {
	   return s.dispose(id, actor, note, SuspenseReleased)
   }

// Consume 挂账消费：差额已入账/已核销（断裂 resolved 时调用）。

  func (s *SuspenseService) Consume(_ context.Context, id, actor, note string) (SuspenseEntry, error) {
	   return s.dispose(id, actor, note, SuspenseConsumed)
   }

// dispose 统一处置逻辑（release / consume）。

  func (s *SuspenseService) dispose(id, actor, note string, target SuspenseState) (SuspenseEntry, error) {
	  if s == nil {
		  return SuspenseEntry{}, ErrUnavailable
	   }
	   if _, err := validate.Actor(actor); err != nil {
		   return SuspenseEntry{}, ErrActor
	   }
	  note = strings.TrimSpace(note)
	   id = strings.TrimSpace(id)
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  e, ok := s.entries[id]
	   if !ok {
		   return SuspenseEntry{}, ErrNotFound
	  }
	   if err := CanAdvanceSuspense(e.State, target); err != nil {
		   return e.copy(), err
	   }
	   now := s.now()
	   e.State = target
	  e.DisposedBy = actor
	   e.DisposalNote = note
	   e.DisposedAt = &now
	  e.UpdatedAt = now
	   metrics.FiatSuspense.WithLabelValues(string(e.Reason), string(target)).Inc()
	   return e.copy(), nil
   }

// LeakedEntries 返回所有超过 SLADeadline 且仍为 open 的挂账条目（泄漏巡逻用）。
  func (s *SuspenseService) LeakedEntries(now time.Time) []SuspenseEntry {
	   if s == nil {
		   return nil
	  }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  var out []SuspenseEntry
	  for _, e := range s.entries {
		  if e.State == SuspenseOpen && now.After(e.SLADeadline) {
			   out = append(out, e.copy())
		  }
	   }
	  return out
  }

// TotalOpen 返回所有 open 挂账的总金额（按资产分组）。
// 供对账校验使用。

   func (s *SuspenseService) TotalOpen() map[string]decimal.Decimal {
	   if s == nil {
		  return nil
	  }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  totals := map[string]decimal.Decimal{}
	  for _, e := range s.entries {
		   if e.State != SuspenseOpen {
			   continue
		   }
		  amt, err := decimal.NewFromString(e.Amount)
		  if err != nil {
			   continue
		   }
		   totals[e.Asset] = totals[e.Asset].Add(amt)
	  }
	   return totals
   }

// copy 深拷贝（SuspenseEntry 含指针字段 DisposedAt，需特殊处理）。
   func (e *SuspenseEntry) copy() SuspenseEntry {
	  if e == nil {
		   return SuspenseEntry{}
	  }
	  cp := *e
	  if e.DisposedAt != nil {
		   t := *e.DisposedAt
		   cp.DisposedAt = &t
	   }
	  return cp
   }
