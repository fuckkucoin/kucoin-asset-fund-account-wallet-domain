// dust-leak 进程入口与路由注册。

// Author: Knox
// Date: 2021.04.15

/*
 * 粉尘兑换超额泄漏与牌价漂移封堵引擎进程入口。
 * 职责：
 * - 启动 ScanWorker、DriftPatrolWorker、RollbackPumpWorker、GateAgingWorker。
 * - 挂载 /dustleak/* 运营 HTTP 端点。
 * - 注册 Prometheus 指标。
 * 不涉及 r6 cvfund 路径占用科目；不涉及 r3 地址池粉尘防护。
 */
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/dustconv"
	"account-assets.kucoin.net/funding/internal/dustleak"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  dustleak.RegisterMetrics()
	   svcboot.Main("dust-leak", config.HostAccountBook, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		  dust := dustconv.New()
		   svc := dustleak.New(dust, nil /* store: nil = 纯内存，注入 SQLStore */)
		  driftReg := dustleak.NewDriftCapRegistry(dustleak.DefaultDriftConfig)
		   detector := dustleak.NewDetector(svc, dust, driftReg)
		   dustleak.Register(v1, rt, svc, detector)
		   // 启动后台 workers
		  go func() {
			  ctx, cancel := context.WithCancel(context.Background())
			   defer cancel()
			  sig := make(chan os.Signal, 1)
			   signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
			   fetcher := &noopFetcher{} // 环境注入 SQLConvertFetcher
			  workers := dustleak.NewWorkers(svc, detector, fetcher, dustleak.WorkerConfig{})
			   go workers.RunAll(ctx)
			   <-sig
		   }()
	   })
  }

// noopFetcher 空实现，仅用于进程在无数据库时启动不崩溃。
// 环境须替换为 dustleak.NewSQLConvertFetcher(db)。

type noopFetcher struct{}

  func (n *noopFetcher) RecentConverts(_ context.Context, _ time.Time, _ int) ([]dustleak.ConvertRecord, error) {
	  return nil, nil
   }
