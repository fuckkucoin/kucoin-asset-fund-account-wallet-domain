package accrual

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/clearing"
)

// SchedulerConfig 调度器配置。
type SchedulerConfig struct {
	// DayCutHour 日切触发小时（UTC）。
	DayCutHour int
	// DayCutMinute 日切触发分钟。
	DayCutMinute int
	// EscalateInterval 挂账升级扫描间隔。
	EscalateInterval time.Duration
	// PollInterval 调度器主循环轮询间隔（建议 30s）。
	PollInterval time.Duration
	// ItemsFetcher 获取当日在途 Item 的回调（可为 nil，Worker 直接传空）。
	ItemsFetcher func(businessDay string) []clearing.Item
}

// DefaultSchedulerConfig 默认调度器配置。
   func DefaultSchedulerConfig() SchedulerConfig {
	  return SchedulerConfig{
		  DayCutHour:       16,
		  DayCutMinute:     0,
		   EscalateInterval: time.Hour,
		   PollInterval:     30 * time.Second,
	  }
  }

// Scheduler 应计引擎定时调度器。
type Scheduler struct {
	cfg SchedulerConfig
	svc *Service
	clock   func() time.Time
	lastCut string // 最近一次成功日切的营业日（防重复）
	lastEsc time.Time
}

// NewScheduler 初始化调度器。

  func NewScheduler(cfg SchedulerConfig, svc *Service) *Scheduler {
	  return &Scheduler{
		  cfg: cfg,
		  svc: svc,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
   }

// WithClock 注入时钟（用）。

   func (sc *Scheduler) WithClock(c func() time.Time) *Scheduler {
	  if sc != nil && c != nil {
		   sc.clock = c
	   }
	   return sc
  }

   // Run 启动调度主循环，阻塞直到 ctx 取消。

  func (sc *Scheduler) Run(ctx context.Context, actor string) {
	  ticker := time.NewTicker(sc.cfg.PollInterval)
	   defer ticker.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			  return
		   case <-ticker.C:
			   sc.tick(ctx, actor)
		   }
	  }
  }

// tick 单次调度心跳。

   func (sc *Scheduler) tick(ctx context.Context, actor string) {
	   now := sc.clock()
	   // 检查日切时间窗
	   if sc.shouldCut(now) {
		   businessDay := now.Format("2006-01-02")
		  var items []clearing.Item
		  if sc.cfg.ItemsFetcher != nil {
			  items = sc.cfg.ItemsFetcher(businessDay)
		   }
		  _, _, err := sc.svc.RunDayCutFull(ctx, businessDay, actor, items, "")
		   if err == nil {
			  sc.lastCut = businessDay
		  }
		   // 错误由告警系统处理，调度器不中断
	  }
	   // 检查挂账升级扫描
	  if sc.shouldEscalate(now) {
		  sc.svc.RunEscalate()
		  sc.lastEsc = now
	   }
   }

// shouldCut 判断是否应触发日切。
   func (sc *Scheduler) shouldCut(now time.Time) bool {
	   businessDay := now.Format("2006-01-02")
	  if strings.TrimSpace(sc.lastCut) == businessDay {
		   return false // 今日已执行过日切
	  }
	  if now.Hour() < sc.cfg.DayCutHour {
		   return false
	   }
	   if now.Hour() == sc.cfg.DayCutHour && now.Minute() < sc.cfg.DayCutMinute {
		  return false
	  }
	  return true
   }

// shouldEscalate 判断是否应触发升级扫描。

  func (sc *Scheduler) shouldEscalate(now time.Time) bool {
	   if sc.lastEsc.IsZero() {
		  return true
	  }
	   return now.Sub(sc.lastEsc) >= sc.cfg.EscalateInterval
  }
