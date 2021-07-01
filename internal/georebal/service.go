package georebal

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/regioinv"
)

// Service 聚合 Planner 和 Executor，供 cmd 层直接使用。
type Service struct {
	Planner *Planner
	Exec   *Executor
	Worker *ExecWorker
}

// invAdapter 把 regioinv.Service 的 TransferInv 接口适配到 georebal.InvTransfer。
	   return a.svc.TransferRegions(ctx, t, now)
   }

// NewService 组装完整 geo-rebal 服务。
// regioSvc 必须非 nil；enqueuer 可为 nil（单测）。
   func NewService(st Store, regioSvc *regioinv.Service, enqueuer IntentEnqueuer, execInterval time.Duration) *Service {
	   fw := &fwAdapter{svc: regioSvc}
	   inv := &invAdapter{svc: regioSvc}
	  planner := NewPlanner(st, fw)
	   exec := NewExecutor(st, inv, enqueuer, fw)
	  worker := NewExecWorker(exec,
		  planner, execInterval)
	  return &Service{Planner: planner, Exec: exec, Worker: worker}
   }

// fwAdapter 把 regioinv.Service 适配到 georebal.Firewall。
type fwAdapter struct{ svc *regioinv.Service }

  func (f *fwAdapter) GetRegion(ctx context.Context, code string) (regioinv.Region, error) {
	   return f.svc.GetRegion(ctx, code)
   }

  func (f *fwAdapter) CheckRebalance(ctx context.Context, t regioinv.Transfer, now time.Time) error {
	   return f.svc.CheckRebalance(ctx, t, now)
  }

  func (f *fwAdapter) MoveQuoteRegions(ctx context.Context, t regioinv.Transfer, now time.Time) (regioinv.MoveQuote, error) {
	   return f.svc.MoveQuoteRegions(ctx, t, now)
   }
