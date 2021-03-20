package clearing

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// DayCloseService 日切门面服务。

type DayCloseService struct {
	reg      *DayCloseRegistry
	suspBook *SuspenseBook
	elStore  *ExceptLinkStore
	kv       persist.KV
	dumpKey string
	clock   func() time.Time
}

// NewDayCloseService 初始化日切门面服务。

  func NewDayCloseService(cal *Calendar, kv persist.KV, dumpKey string) *DayCloseService {
	   if cal == nil {
		  cal = NewCalendar()
	   }
	   return &DayCloseService{
		   reg:      NewDayCloseRegistry(cal),
		  suspBook: NewSuspenseBook(),
		   elStore:  NewExceptLinkStore(),
		  kv:      kv,
		   dumpKey: dumpKey,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入时钟（用）。
  func (svc *DayCloseService) WithClock(c func() time.Time) *DayCloseService {
	  if svc != nil && c != nil {
		   svc.clock = c
		   svc.reg.WithClock(c)
		   svc.suspBook.WithClock(c)
		   svc.elStore.WithClock(c)
	  }
	   return svc
   }

   // Registry 返回日切注册表（Worker 和 HTTP 层可使用）。
   func (svc *DayCloseService) Registry() *DayCloseRegistry {
	   if svc == nil {
		  return NewDayCloseRegistry(nil)
	  }
	  return svc.reg
  }

// Suspense 返回挂账账册。

  func (svc *DayCloseService) Suspense() *SuspenseBook {
	   if svc == nil {
		   return NewSuspenseBook()
	   }
	   return svc.suspBook
   }

// ExceptLinks 返回异常关联存储。
   func (svc *DayCloseService) ExceptLinks() *ExceptLinkStore {
	   if svc == nil {
		  return NewExceptLinkStore()
	  }
	   return svc.elStore
  }

// OpenDay 开日并持久化。

  func (svc *DayCloseService) OpenDay(ctx context.Context, day, actor string) (DayClose, error) {
	   if svc == nil {
		   return DayClose{}, ErrUnavailable
	  }
	  dc, err := svc.reg.OpenDay(day, actor)
	   if err != nil {
		   return dc, err
	   }
	   ObserveDayCloseStatus(dc)
	  svc.saveAll(ctx)
	   return dc, nil
   }

// Freeze 封锁并持久化。

   func (svc *DayCloseService) Freeze(ctx context.Context, id, actor string, epoch int64, token string) (DayClose, error) {
	  if svc == nil {
		  return DayClose{}, ErrUnavailable
	   }
	   dc, err := svc.reg.Freeze(id, actor, epoch, token)
	   if err != nil {
		  return dc, err
	  }
	   ObserveDayCloseStatus(dc)
	   svc.saveAll(ctx)
	  return dc, nil
   }

// Close 关日并持久化。
// 关日前自动检查是否还有 open 挂账（若有则阻止）。
  func (svc *DayCloseService) Close(ctx context.Context, id, actor string, epoch int64, token string, batchID string) (DayClose, error) {
	   if svc == nil {
		  return DayClose{}, ErrUnavailable
	  }
	   if batchID != "" && svc.suspBook.HasOpenEntries(batchID) {
		  return DayClose{},
			   ErrSuspenseNotOpen
	  }
	   dc, err := svc.reg.Close(id, actor, epoch, token)
	  if err != nil {
		  return dc, err
	  }
	  ObserveDayCloseStatus(dc)
	  openCount := len(svc.suspBook.ListOpen())
	  ObserveSuspenseOpen(openCount)
	   svc.saveAll(ctx)
	   return dc, nil
  }

// Reopen 有限再开并持久化。
   func (svc *DayCloseService) Reopen(ctx context.Context, id, actor, reason string, epoch int64, token string) (DayClose, error) {
	  if svc == nil {
		   return DayClose{},
			   ErrUnavailable
	  }
	   dc, err := svc.reg.Reopen(id, actor, reason, epoch, token)
	   if err != nil {
		   return dc, err
	   }
	  ObserveDayCloseStatus(dc)
	  svc.saveAll(ctx)
	   return dc, nil
   }

// ParkSuspense 新建挂账分录并更新指标。

   func (svc *DayCloseService) ParkSuspense(batchID, itemID, exceptionID, reconDiffID,
	  userID, asset, amount, direction, reason, actor string) (SuspenseEntry, error) {
	  if svc == nil {
		  return SuspenseEntry{}, ErrUnavailable
	   }
	  entry, err := svc.suspBook.Park(batchID, itemID, exceptionID, reconDiffID,
		  userID, asset, amount, direction, reason, actor)
	   if err != nil {
		  return entry, err
	   }
	  openCount := len(svc.suspBook.ListOpen())
	  ObserveSuspenseOpen(openCount)
	   return entry, nil
  }

// ResolveSuspense 处置挂账为 resolved 并更新指标。
  func (svc *DayCloseService) ResolveSuspense(id, actor, note string, epoch int64, token string) (SuspenseEntry, error) {
	  if svc == nil {
		   return SuspenseEntry{},
			   ErrUnavailable
	  }
	  entry, err := svc.suspBook.Resolve(id, actor, note, epoch, token)
	  if err != nil {
		   return entry, err
	  }
	  openCount := len(svc.suspBook.ListOpen())
	  ObserveSuspenseOpen(openCount)
	  return entry, nil
   }

// RejectSuspense 处置挂账为 rejected 并更新指标。
   func (svc *DayCloseService) RejectSuspense(id, actor, note string, epoch int64, token string) (SuspenseEntry, error) {
	  if svc == nil {
		  return SuspenseEntry{}, ErrUnavailable
	   }
	   entry, err := svc.suspBook.Reject(id, actor, note, epoch, token)
	  if err != nil {
		  return entry, err
	   }
	   openCount := len(svc.suspBook.ListOpen())
	  ObserveSuspenseOpen(openCount)
	  return entry, nil
  }

// EscalateSuspense 扫描超 SLA 挂账并触发升级。
// escalate 为升级回调（可为 nil，仅标记 EscalatedAt）。

  func (svc *DayCloseService) EscalateSuspense(escalate func(SuspenseEntry)) []SuspenseEntry {
	   if svc == nil {
		  return nil
	   }
	  now := svc.clock()
	   overdue := svc.suspBook.ScanOverdue(now)
	   for _, e := range overdue {
		   entry := e
		   _,
			  _ = svc.suspBook.MarkEscalated(entry.ID)
		   ObserveSuspenseEscalated()
		   if escalate != nil {
			   escalate(entry)
		  }
	   }
	   return overdue
  }

   // LinkException 建立异常关联并更新指标。

   func (svc *DayCloseService) LinkException(exceptionID, reconDiffID, note, actor string) (ExceptLink, error) {
	  if svc == nil {
		  return ExceptLink{}, ErrUnavailable
	   }
	  lnk, err := svc.elStore.Link(exceptionID, reconDiffID, note, actor)
	  if err != nil {
		   return lnk, err
	  }
	  // 更新关联数指标
	   svc.elStore.mu.Lock()
	  count := len(svc.elStore.links)
	  svc.elStore.mu.Unlock()
	   ObserveExceptLinkCount(count)
	   return lnk, nil
  }

// Restore 从 KV 恢复全部状态（进程重启后调用）。
  func (svc *DayCloseService) Restore(ctx context.Context, cal *Calendar) error {
	  if svc == nil || svc.kv == nil || svc.dumpKey == "" {
		  return nil
	   }
	  var err error
	   reg, err := LoadDayCloseRegistry(ctx, svc.kv, svc.dumpKey, cal)
	   if err != nil {
		   return err
	  }
	   svc.reg = reg
	  sb,
		   err := LoadSuspenseBook(ctx, svc.kv, svc.dumpKey)
	  if err != nil {
		  return err
	   }
	  svc.suspBook = sb
	  els, err := LoadExceptLinkStore(ctx, svc.kv, svc.dumpKey)
	   if err != nil {
		  return err
	  }
	  svc.elStore = els
	   return nil
  }

// saveAll 持久化所有子组件（忽略错误，由告警处理）。

   func (svc *DayCloseService) saveAll(ctx context.Context) {
	   if svc.kv == nil || svc.dumpKey == "" {
		  return
	  }
	   _ = svc.reg.Save(ctx, svc.kv, svc.dumpKey)
	  _ = svc.suspBook.Save(ctx, svc.kv, svc.dumpKey)
	   _ = svc.elStore.Save(ctx, svc.kv, svc.dumpKey)
  }

// CheckCrossDayFence 封锁窗前置检查并记录拒绝指标。

  func (svc *DayCloseService) CheckCrossDayFence(day string, crossDayTagged bool) error {
	  if svc == nil {
		   return ErrUnavailable
	   }
	   err := svc.reg.CheckCrossDayFence(day, crossDayTagged)
	  if err == ErrDayCloseFenced {
		   ObserveFenceBlocked(day)
	  }
	   return err
  }
// temporary guard removed after the production check
