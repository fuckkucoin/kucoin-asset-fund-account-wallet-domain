package clearing

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

// DayCloseStatus 日终封锁状态枚举。
type DayCloseStatus string

const (
	// DayCloseOpen 日内正常接单阶段；清算 Item 可自由入队。
	DayCloseOpen DayCloseStatus = "open"
	// DayCloseFreeze 封锁窗已触发；无 CrossDayTag 的跨日 Item 被拒绝。
	DayCloseFreeze DayCloseStatus = "freezing"
	// DayCloseClosed 日切完成；本日批次不再接受新 Item。
	DayCloseClosed DayCloseStatus = "closed"
	// DayCloseReopened 有限再开：特殊运营操作后允许有限入队，须留原因。
	DayCloseReopened DayCloseStatus = "reopened_limited"
)

// ErrDayCloseFenced 封锁窗拒绝无跨日标记的 Item 入队。

var ErrDayCloseFenced = fmt.Errorf("day-close fence active: cross-day item must carry CrossDayTag")

// ErrDayCloseConflict 同一日期已存在未关闭的 DayClose。

var ErrDayCloseConflict = fmt.Errorf("day-close conflict: a non-closed record exists for this business day")

// ErrDayCloseTransition 非法状态跃迁。

var ErrDayCloseTransition = fmt.Errorf("day-close illegal transition")

// ErrDayCloseHoliday 节假日不执行日切。

var ErrDayCloseHoliday = fmt.Errorf("day-close skipped: business holiday")

// ErrDayCloseReopenReason 再开必须填写原因。

var ErrDayCloseReopenReason = fmt.Errorf("day-close reopen: reason required")

// DayClose 日终封锁记录。
type DayClose struct {
	ID string `json:"id"`
	BusinessDay string         `json:"business_day"` // YYYY-MM-DD，清算日历意义上的营业日
	Status      DayCloseStatus `json:"status"`
	FreezeAt *time.Time `json:"freeze_at,omitempty"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
	ReopenedAt *time.Time `json:"reopened_at,omitempty"`
	OpenedBy string `json:"opened_by"`
	FrozenBy string `json:"frozen_by,omitempty"`
	ClosedBy     string `json:"closed_by,omitempty"`
	ReopenedBy   string `json:"reopened_by,omitempty"`
	ReopenReason string `json:"reopen_reason,omitempty"`
	IsHoliday  bool   `json:"is_holiday"`
	CatchUpDay string `json:"catch_up_day,omitempty"` // 假日时记录补跑对应的工作日
	Fence     persist.Fence `json:"fence"`
	CreatedAt time.Time     `json:"created_at"`
}

// DayCloseRegistry 日终封锁注册表，单进程内存实现。
type DayCloseRegistry struct {
	mu      sync.Mutex
	records map[string]*DayClose // key: ID
	byDay   map[string]string    // key: business_day → ID（取最新活跃记录）
	fence *persist.Table
	clock func() time.Time
	calendar *Calendar
}

// NewDayCloseRegistry 初始化日终封锁注册表。
  func NewDayCloseRegistry(cal *Calendar) *DayCloseRegistry {
	  if cal == nil {
		  cal = NewCalendar()
	  }
	   return &DayCloseRegistry{
		   records: map[string]*DayClose{},
		   byDay:   map[string]string{},
		   fence: persist.NewTable(),
		   clock: func() time.Time { return time.Now().UTC() },
		   calendar: cal,
	  }
  }

// WithClock 注入时钟（用）。
   func (r *DayCloseRegistry) WithClock(c func() time.Time) *DayCloseRegistry {
	   if r != nil && c != nil {
		   r.clock = c
	  }
	  return r
   }

   // OpenDay 为指定营业日建立 DayClose（open 状态）。
   // 若当日为假日则拒绝，返回 ErrDayCloseHoliday。

  func (r *DayCloseRegistry) OpenDay(day, actor string) (DayClose, error) {
	  if r == nil {
		  return DayClose{}, ErrUnavailable
	  }
	  act, err := actorOK(actor)
	   if err != nil {
		  return DayClose{}, err
	   }
	  day = strings.TrimSpace(day)
	   if day == "" {
		   day = r.clock().Format("2006-01-02")
	   }
	   t, err := time.Parse("2006-01-02", day)
	  if err != nil {
		  return DayClose{}, ErrBatch
	  }
	  if r.calendar.Has(t) {
		   return DayClose{}, ErrDayCloseHoliday
	   }
	   r.mu.Lock()
	   defer r.mu.Unlock()
	  // 幂等：同一日期 open 状态已存在则直接返回。
	   if id, ok := r.byDay[day]; ok {
		  if dc := r.records[id]; dc != nil && dc.Status != DayCloseClosed {
			   return *dc, nil
		  }
	   }
	  now := r.clock()
	  dc := DayClose{
		  ID: uuid.NewString(),
		   BusinessDay: day,
		   Status:      DayCloseOpen,
		   OpenedBy: act,
		   CreatedAt: now,
	  }
	   f, err := r.fence.Bump(dc.ID, persist.NewToken())
	   if err != nil {
		  return DayClose{}, err
	   }
	   dc.Fence = f
	  cp := dc
	   r.records[dc.ID] = &cp
	  r.byDay[day] = dc.ID
	   return cp, nil
  }

// OpenHolidayCatchUp 假日跳过：为原假日登记 catchup，关联下一工作日。
// 不新建日切，仅标记假日 + 记录补跑工作日。

  func (r *DayCloseRegistry) OpenHolidayCatchUp(holidayDay, nextBizDay, actor string) (DayClose, error) {
	  if r == nil {
		   return DayClose{}, ErrUnavailable
	  }
	   act, err := actorOK(actor)
	   if err != nil {
		  return DayClose{}, err
	  }
	  holidayDay = strings.TrimSpace(holidayDay)
	  nextBizDay = strings.TrimSpace(nextBizDay)
	   if _, err := time.Parse("2006-01-02", holidayDay); err != nil {
		  return DayClose{}, ErrBatch
	  }
	   if _, err := time.Parse("2006-01-02", nextBizDay); err != nil {
		  return DayClose{}, ErrBatch
	   }
	  r.mu.Lock()
	   defer r.mu.Unlock()
	   now := r.clock()
	  closed := now
	  dc := DayClose{
		   ID:          uuid.NewString(),
		   BusinessDay: holidayDay,
		   Status: DayCloseClosed,
		   OpenedBy: act,
		  ClosedBy: act,
		   ClosedAt: &closed,
		   IsHoliday: true,
		  CatchUpDay: nextBizDay,
		  CreatedAt: now,
	  }
	   f, err := r.fence.Bump(dc.ID, persist.NewToken())
	   if err != nil {
		   return DayClose{}, err
	  }
	  dc.Fence = f
	  cp := dc
	  r.records[dc.ID] = &cp
	   r.byDay[holidayDay] = dc.ID
	  return cp, nil
   }

// Freeze 将 open 状态切为 freezing，开始封锁窗。

  func (r *DayCloseRegistry) Freeze(id, actor string, epoch int64, token string) (DayClose, error) {
	   return r.transition(id, actor, epoch, token, DayCloseOpen, DayCloseFreeze, func(dc *DayClose) {
		   now := r.clock()
		   dc.FreezeAt = &now
		  dc.FrozenBy = actor
	  })
   }

// Close 将 freezing 切为 closed。调用前须确认所有挂账已处置。
  func (r *DayCloseRegistry) Close(id, actor string, epoch int64, token string) (DayClose, error) {
	  return r.transition(id, actor, epoch, token, DayCloseFreeze, DayCloseClosed, func(dc *DayClose) {
		  now := r.clock()
		   dc.ClosedAt = &now
		  dc.ClosedBy = actor
	  })
  }

// Reopen 将 closed 切为 reopened_limited，须填 reason。

   func (r *DayCloseRegistry) Reopen(id, actor, reason string, epoch int64, token string) (DayClose, error) {
	  reason = strings.TrimSpace(reason)
	  if reason == "" {
		   return DayClose{}, ErrDayCloseReopenReason
	  }
	   return r.transition(id, actor, epoch, token, DayCloseClosed, DayCloseReopened, func(dc *DayClose) {
		   now := r.clock()
		  dc.ReopenedAt = &now
		  dc.ReopenedBy = actor
		   dc.ReopenReason = reason
	  })
   }

// Get 按 ID 查 DayClose。

   func (r *DayCloseRegistry) Get(id string) (DayClose, error) {
	  if r == nil {
		   return DayClose{}, ErrUnavailable
	   }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  dc, ok := r.records[strings.TrimSpace(id)]
	   if !ok {
		  return DayClose{}, ErrNotFound
	   }
	  return *dc, nil
   }

// GetByDay 按营业日查询最近一条 DayClose。
   func (r *DayCloseRegistry) GetByDay(day string) (DayClose, error) {
	  if r == nil {
		   return DayClose{}, ErrUnavailable
	   }
	  r.mu.Lock()
	   defer r.mu.Unlock()
	   id, ok := r.byDay[strings.TrimSpace(day)]
	  if !ok {
		   return DayClose{}, ErrNotFound
	   }
	   dc := r.records[id]
	   if dc == nil {
		   return DayClose{}, ErrNotFound
	   }
	  return *dc, nil
   }

// List 返回所有 DayClose 列表（副本）。

   func (r *DayCloseRegistry) List() []DayClose {
	   if r == nil {
		   return nil
	   }
	  r.mu.Lock()
	   defer r.mu.Unlock()
	   out := make([]DayClose,
		  0, len(r.records))
	  for _, dc := range r.records {
		   out = append(out, *dc)
	  }
	  return out
  }

// IsFenced 检查当前时刻指定营业日是否处于封锁状态。
   func (r *DayCloseRegistry) IsFenced(day string) bool {
	  if r == nil {
		   return false
	  }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  id, ok := r.byDay[strings.TrimSpace(day)]
	  if !ok {
		  return false
	  }
	  dc := r.records[id]
	   if dc == nil {
		   return false
	  }
	   return dc.Status == DayCloseFreeze || dc.Status == DayCloseClosed
   }

// CheckCrossDayFence 校验 Item 是否允许在当前封锁状态下入队。
// 若处于 freezing/closed，且 Item 未携带 CrossDayTag，则返回 ErrDayCloseFenced。
  func (r *DayCloseRegistry) CheckCrossDayFence(day string, crossDayTagged bool) error {
	   if !r.IsFenced(day) {
		   return nil
	   }
	   if crossDayTagged {
		  return nil
	   }
	   return ErrDayCloseFenced
   }

// transition 通用状态跃迁助手。
   func (r *DayCloseRegistry) transition(id, actor string, epoch int64, token string,
	  from, to DayCloseStatus, mutate func(*DayClose)) (DayClose, error) {
	  if r == nil {
		  return DayClose{}, ErrUnavailable
	  }
	   act, err := actorOK(actor)
	  if err != nil {
		  return DayClose{}, err
	  }
	   if err := r.fence.Check(id, epoch, token); err != nil {
		   return DayClose{}, err
	   }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  dc, ok := r.records[strings.TrimSpace(id)]
	  if !ok {
		   return DayClose{}, ErrNotFound
	   }
	   if dc.Status != from {
		  return *dc, ErrDayCloseTransition
	   }
	   _ = act
	  f, err := r.fence.Bump(id, persist.NewToken())
	   if err != nil {
		   return DayClose{}, err
	  }
	  dc.Status = to
	   dc.Fence = f
	   mutate(dc)
	  return *dc, nil
   }
