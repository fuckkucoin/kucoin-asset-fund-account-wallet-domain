package accrual

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/clearing"
)

// EscalateFunc 挂账升级回调。调用方注入实际告警/通知逻辑。
// entry 为触发升级的 SuspenseEntry。
type EscalateFunc func(entry clearing.SuspenseEntry)

// WorkerConfig 日切与应计 Worker 配置。

type WorkerConfig struct {
	// DayCutHour 每天触发日切的小时数（UTC），默认 16（即 UTC 16:00）。
	DayCutHour int
	// DayCutMinute 触发分钟数。
	DayCutMinute int
	// AccrualNotePrefix 应计分录备注前缀。
	AccrualNotePrefix string
	// SuspenseEscalate 升级回调（可为 nil，升级时仅记录 EscalatedAt）。
	SuspenseEscalate EscalateFunc
}

// DefaultWorkerConfig 默认 Worker 配置。

  func DefaultWorkerConfig() WorkerConfig {
	   return WorkerConfig{
		  DayCutHour:   16,
		   DayCutMinute: 0,
		  AccrualNotePrefix: "auto-accrual",
	  }
   }

// Worker 日切与应计调度器。

type Worker struct {
	cfg      WorkerConfig
	engine   *Engine
	dcReg    *clearing.DayCloseRegistry
	suspBook *clearing.SuspenseBook
	clock    func() time.Time
}

// NewWorker 初始化 Worker。
   func NewWorker(cfg WorkerConfig, engine *Engine, dcReg *clearing.DayCloseRegistry, suspBook *clearing.SuspenseBook) *Worker {
	  if engine == nil {
		   engine = NewEngine()
	   }
	  return &Worker{
		   cfg:    cfg,
		  engine: engine,
		  dcReg:    dcReg,
		   suspBook: suspBook,
		   clock:    func() time.Time { return time.Now().UTC() },
	  }
   }

// WithClock 注入时钟（用）。

  func (w *Worker) WithClock(c func() time.Time) *Worker {
	   if w != nil && c != nil {
		  w.clock = c
		  w.engine.WithClock(c)
	   }
	  return w
   }

   // RunDayCut 执行指定营业日的完整日切流程：
   // 1. 检查是否为假日 → 是则 HolidaySkip；
  // 2. 触发封锁（OpenDay + Freeze）；
  // 3. 扫描在途 items 生成应计批次；
   // 4. 过账应计批次；
   // 5. 关闭 DayClose。
  // items 为当日批次下的清算 Item 切片（仅处理跨日未结项）。
   // actor 为触发日切的系统 actor。
  // 返回 DayClose 和 AccrualBatch（假日时 batch 为空值）。
  func (w *Worker) RunDayCut(businessDay, actor string, items []clearing.Item, calendarBatchID string) (clearing.DayClose, AccrualBatch, error) {
	   if w == nil {
		   return clearing.DayClose{}, AccrualBatch{}, ErrEngineNil
	   }
	  now := w.clock()
	   cal := w.dcReg // 假日判断由 DayCloseRegistry 内部 Calendar 处理
	  // 1. 尝试开日 → 若假日则 HolidaySkip
	  dc, err := cal.OpenDay(businessDay, actor)
	  if err == clearing.ErrDayCloseHoliday {
		   nextBiz := nextBizDay(businessDay,
			  now)
		   holidayDC, herr := cal.OpenHolidayCatchUp(businessDay, nextBiz, actor)
		   if herr != nil {
			  return clearing.DayClose{}, AccrualBatch{}, herr
		  }
		  return holidayDC, AccrualBatch{}, nil
	  }
	   if err != nil {
		   return clearing.DayClose{}, AccrualBatch{}, err
	  }
	  // 2. 封锁
	  dc, err = cal.Freeze(dc.ID, actor, dc.Fence.Epoch, dc.Fence.Token)
	   if err != nil {
		   return clearing.DayClose{}, AccrualBatch{}, err
	  }
	   // 3. 扫描在途 items 生成应计批次
	  bat, err := w.generateAccrual(businessDay, dc.ID, actor, items)
	  if err != nil {
		  return dc, AccrualBatch{}, err
	  }
	   // 4. 过账（若有分录）
	   if bat.ID != "" && bat.Status == BatchDraft {
		   bat, err = w.engine.Post(bat.ID, actor, bat.Fence.Epoch, bat.Fence.Token)
		  if err != nil {
			   return dc, bat, err
		   }
	   }
	   // 5. 关闭 DayClose
	  dc, err = cal.Close(dc.ID, actor, dc.Fence.Epoch, dc.Fence.Token)
	  if err != nil {
		   return dc, bat, err
	   }
	  return dc, bat, nil
  }

// generateAccrual 扫描在途 Item，生成应计分录批次（草稿）。
// 对每个跨日未结 Item（ValueDate > businessDay 且 Status 为 pending/excepted）生成一对应计分录。

   func (w *Worker) generateAccrual(businessDay, dayCloseID, actor string, items []clearing.Item) (AccrualBatch, error) {
	   crossDay := filterCrossDay(businessDay, items)
	  if len(crossDay) == 0 {
		   // 无跨日在途，不生成批次
		  return AccrualBatch{}, nil
	  }
	   note := w.cfg.AccrualNotePrefix + " day=" + businessDay
	   bat, err := w.engine.CreateBatch(businessDay, dayCloseID, actor, note)
	  if err != nil {
		  return AccrualBatch{}, err
	  }
	   for _, it := range crossDay {
		   lineNote := "cross-day item=" + it.ID
		   _, _, err := w.engine.AddLinePair(
			  bat.ID,
			  string(LineAccruedInterest),
			   "system_accrual",
			   it.UserID,
			  it.Asset,
			   it.Amount,
			  it.MovementID,
			  lineNote,
		  )
		   if err != nil {
			  return bat, err
		   }
	   }
	  // 刷新 bat（AddLinePair 更新了 TotalDebit/TotalCredit）
	   refreshed, err := w.engine.GetBatch(bat.ID)
	  if err != nil {
		  return bat, err
	   }
	  return refreshed, nil
  }

// RunSuspenseEscalate 扫描 SLA 超时的 open 挂账并执行升级。

  func (w *Worker) RunSuspenseEscalate() []clearing.SuspenseEntry {
	  if w == nil || w.suspBook == nil {
		  return nil
	  }
	  now := w.clock()
	  overdue := w.suspBook.ScanOverdue(now)
	   for _, entry := range overdue {
		  e := entry
		   _,
			  _ = w.suspBook.MarkEscalated(e.ID)
		   if w.cfg.SuspenseEscalate != nil {
			   w.cfg.SuspenseEscalate(e)
		   }
	  }
	  return overdue
   }

// filterCrossDay 过滤出跨日在途 Item（ValueDate > businessDay）。

  func filterCrossDay(businessDay string, items []clearing.Item) []clearing.Item {
	   cutoff, err := time.Parse("2006-01-02", strings.TrimSpace(businessDay))
	   if err != nil {
		  return nil
	  }
	   // 以营业日结束（23:59:59.999）为界
	   cutoffEnd := cutoff.Add(24*time.Hour - time.Millisecond)
	  out := make([]clearing.Item, 0)
	   for _, it := range items {
		  if it.Status == clearing.StatusPending || it.Status == clearing.StatusExcepted {
			   if it.ValueDate.After(cutoffEnd) {
				  out = append(out, it)
			  }
		  }
	  }
	   return out
   }

// nextBizDay 假日后的下一工作日（简单顺延，不依赖 Calendar 以避免循环引用）。

   func nextBizDay(holidayDay string, now time.Time) string {
	  t, err := time.Parse("2006-01-02", strings.TrimSpace(holidayDay))
	   if err != nil {
		  return now.Add(24 * time.Hour).Format("2006-01-02")
	   }
	  next := t.Add(24 * time.Hour)
	  for next.Weekday() == time.Saturday || next.Weekday() == time.Sunday {
		   next = next.Add(24 * time.Hour)
	  }
	  return next.Format("2006-01-02")
   }
