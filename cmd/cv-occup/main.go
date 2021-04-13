// 功能：cv-occup 进程：稳定币闪兑路径占用会计与失败回滚服务。
// 承载 Integrity Gate 巡检、过期回滚、穿仓扫描三个后台 Worker；
// 以及 ops HTTP API（占用查询/强制回滚/违约板）。

// 作者：Cason
// 日期：2021.03.27

package main

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/cvoccup"
	"account-assets.kucoin.net/funding/internal/cvrollback"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

   func main() {
	   svcboot.Main("cv-occup", config.HostFundSurface, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		  // 构造存储层
		  occStore := cvoccup.NewKVStore(rt.KV)
		  rbStore := cvrollback.NewKVStore(rt.KV)
		   // 构造适配器
		  holdAPI := newHoldAdapter(rt)
		  availReader := newAvailLedgerAdapter(rt)
		   // 构造占用服务
		   occSvc := cvoccup.NewService(occStore, holdAPI, availReader).
			  WithMetrics(cvoccup.NewPrometheusMetrics())
			  // 构造回滚编排器
		  orch := cvrollback.NewOrchestrator(
			   rbStore,
			   &occReleaserAdapter{svc: occSvc},
			  nil, // LedgerReverser：侧注入
			   nil, // HoldcatAligner：侧注入
		  )
		   rollbackFn := func(ctx context.Context, occID, orderID, userID, trigger, remain string) error {
			  req := cvrollback.PlanReq{
				  OccID:   occID,
				  OrderID: orderID,
				  UserID: userID,
				  Trigger: cvrollback.RollbackTrigger(trigger),
				   RemainAmount: remain,
			  }
			   _, err := orch.PlanAndExecute(ctx, req)
			  return err
		  }
		  // 注册 HTTP API
		  ops := v1.Group("/cv-occup")
		   cvoccup.NewHandler(occSvc, rollbackFn).Register(ops)
		  rbOps := v1.Group("/cv-rollback")
		  cvrollback.NewHandler(orch).Register(rbOps)
		  // 启动后台 Worker
		   go func() { _ = cvoccup.NewIntegrityPatrol(occSvc, time.Minute).Run(context.Background()) }()
		  go func() {
			  _ = cvoccup.NewExpireRollbackWorker(occSvc, rollbackFn, 2*time.Minute).Run(context.Background())
		   }()
		   go func() { _ = cvoccup.NewBreachScanWorker(occSvc, 5*time.Minute).Run(context.Background()) }()
	   })
   }

// occReleaserAdapter 把 cvoccup.Service 适配为 cvrollback.OccReleaser。
type occReleaserAdapter struct {
	svc *cvoccup.Service
}

  func (a *occReleaserAdapter) Release(ctx context.Context, occID, reason string) error {
	  _, err := a.svc.Release(ctx, occID, reason)
	  return err
   }

  func (a *occReleaserAdapter) CheckIntegrity(ctx context.Context, occID string, openBreach bool) error {
	   return a.svc.CheckIntegrity(ctx, occID, openBreach)
  }
