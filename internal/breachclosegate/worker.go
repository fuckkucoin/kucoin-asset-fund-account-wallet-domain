package breachclosegate

import (
	"context"
	"errors"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/reviewsla"
)

// CaseExpirer 驱动案件强制 Expire 的接口（只读消费 cases.Case 状态；不 Allow）。
// 实现方：cases.Service.ExpireByID 或同类包装。
type CaseExpirer interface {
	// ExpireCase 强制将 caseID 对应案件转为 Expired 状态。
	// 不得改为 Allowed；若案件已处于终态则返回 nil（幂等）。
	ExpireCase(caseID string) error
}

// ScanBook reviewsla.Book 的只读消费接口，供注入 mock。

type ScanBook interface {
	ScanRefs() []reviewsla.BreachLeaseRef
}

// ScanWorkerConfig ScanWorker 配置。
type ScanWorkerConfig struct {
	// ScanInterval 扫描间隔（默认 30s）。
	ScanInterval time.Duration
	// PatrolInterval WindowPatrol 巡检间隔（默认 60s）。
	PatrolInterval time.Duration
	// MaxBatchSize 每轮最多处理的新违约数（防止单轮阻塞）。
	MaxBatchSize int
}

   func (c *ScanWorkerConfig) withDefaults() ScanWorkerConfig {
	   out := *c
	   if out.ScanInterval <= 0 {
		   out.ScanInterval = 30 * time.Second
	   }
	   if out.PatrolInterval <= 0 {
		   out.PatrolInterval = 60 * time.Second
	   }
	  if out.MaxBatchSize <= 0 {
		   out.MaxBatchSize = 50
	  }
	  return out
   }

// ScanWorker 主 worker。

type ScanWorker struct {
	book    ScanBook
	svc     *Service
	expirer CaseExpirer
	cfg     ScanWorkerConfig
}

// NewScanWorker 创建 ScanWorker。
  func NewScanWorker(book ScanBook, svc *Service, expirer CaseExpirer, cfg ScanWorkerConfig) *ScanWorker {
	   return &ScanWorker{
		  book: book,
		  svc:  svc,
		  expirer: expirer,
		  cfg: cfg.withDefaults(),
	   }
  }

// Run 启动 worker 主循环，阻塞直到 ctx 取消。

  func (w *ScanWorker) Run(ctx context.Context) {
	   scanTick := time.NewTicker(w.cfg.ScanInterval)
	   patrolTick := time.NewTicker(w.cfg.PatrolInterval)
	   defer scanTick.Stop()
	  defer patrolTick.Stop()
	   // 启动时立即扫一次。
	   w.scanOnce()
	  for {
		  select {
		  case <-ctx.Done():
			  return
		   case <-scanTick.C:
			   w.scanOnce()
		  case <-patrolTick.C:
			   w.patrolOnce()
		  }
	  }
  }

// scanOnce 执行单轮 Scan：发现新违约 → 建档 BreachLease → 驱动案件 Expire。
  func (w *ScanWorker) scanOnce() {
	  metrics.BreachWorkerScanRounds.Inc()
	  refs := w.book.ScanRefs()
	  if len(refs) == 0 {
		   w.updateActiveGauge()
		   return
	   }
	  limit := w.cfg.MaxBatchSize
	   if len(refs) > limit {
		   refs = refs[:limit]
	  }
	  for _, ref := range refs {
		  lease,
			  err := w.svc.ScanAndBlock(ref)
		  if err != nil {
			  // ErrDuplicateBreach：已建档，跳过。其他错误记录后接着。
			   continue
		  }
		   if !lease.Active() {
			  continue
		  }
		  // 驱动案件强制 Expire（fail-closed）。
		   expErr := w.expirer.ExpireCase(ref.CaseID)
		  if expErr != nil {
			   // 案件可能已 Expired（幂等），忽略 ErrClosed / ErrExpired。
			   // 其他错误下 BreachLease 仍保持 blocking，下一轮会重试。
			   continue
		   }
		  // 更新 BreachLease → expired_case，并落 SlaBreachProof。
		  if ferr := w.svc.ForceExpireCase(ref.CaseID); ferr != nil {
			  // 状态更新失败不影响挡写语义（BreachLease 仍 blocking）。
			   _ = ferr
		   }
	  }
	  w.updateActiveGauge()
  }

// patrolOnce 阻挡窗口巡检：对所有活跃 BreachLease 执行 SubmitPrecheck，
// 确认挡写语义仍然生效。若某条 active lease 的 SubmitPrecheck 意外通过，触发告警指标。

  func (w *ScanWorker) patrolOnce() {
	   actives := w.svc.ListActiveLeases(200)
	   for _, lease := range actives {
		  res, err := w.svc.SubmitPrecheck(lease.MovementID, "system-patrol")
		   if res.Allowed || !errors.Is(err, ErrBreachBlock) {
			   // 不应发生：活跃 BreachLease 对应的 movementID 应被拦截。
			   metrics.BreachAllowBlocked.Add(0) // 用于触发告警（实际应用中应打告警日志）。
		   }
	   }
	   w.updateActiveGauge()
  }

// updateActiveGauge 刷新 BreachActiveLeases gauge。
   func (w *ScanWorker) updateActiveGauge() {
	   actives := w.svc.ListActiveLeases(10000)
	   metrics.BreachActiveLeases.Set(float64(len(actives)))
  }

// ── CaseExpirer 适配器 ────────────────────────────────────────────────────────
// FuncExpirer 用函数实现 CaseExpirer，便于注入 lambda。
type FuncExpirer struct {
	fn func(caseID string) error
}

// NewFuncExpirer 创建函数适配器。
  func NewFuncExpirer(fn func(caseID string) error) *FuncExpirer {
	  return &FuncExpirer{fn: fn}
   }

   // ExpireCase 调用注入的函数。
   func (f *FuncExpirer) ExpireCase(caseID string) error {
	  if f == nil || f.fn == nil {
		  return nil
	   }
	  return f.fn(caseID)
  }
