// 功能：经纪子账围栏旁路收口引擎独立进程入口。
// 重点：启动围栏 API 服务与后台 Worker（旁路巡检、守恒抽样、老化升级）；
// 使用 brokerbook.Service 作为账本读取后端，两者通过接口解耦。
// 风险点：Worker goroutine 必须随 context 取消优雅退出；
// 互斥令牌仓库在内存中，重启后失效——须换 Redis 实现。

// 作者：Hale
// 日期：2021.03.07

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/brokerbook"
	"account-assets.kucoin.net/funding/internal/brokerfence"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	   svcboot.Main("broker-fence-gate", config.HostAccountBook, register)
  }

// register 将围栏旁路引擎 API 与后台 Worker 挂入进程运行时。
   func register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   // 构造 brokerbook Service（读账本用）。
	  booksvc := brokerbook.New()
	  // 构造围栏引擎（使用内存持久化；需替换为 SQL 实现）。
	   fenceSvc := brokerfence.NewService(booksvc)
	   // 将围栏引擎注入 brokerbook，建立旁路挂钩。
	  booksvc.SetFenceGuard(fenceSvc)
	   // 注册 HTTP API。
	  brokerfence.RegisterAPI(v1, fenceSvc)
	  // 后台 Worker 随进程生命周期运行。
	   ctx := workerContext()
	   workerCfg := brokerfence.DefaultWorkerConfig()
	   bypassCh := make(chan brokerfence.BypassScanResult,
		  32)
	   conserveCh := make(chan brokerfence.ConserveSampleResult, 32)
	   agingCh := make(chan brokerfence.LeaseAgingResult, 32)
	   go brokerfence.RunBypassScanner(ctx, fenceSvc, workerCfg, bypassCh)
	  go brokerfence.RunConserveSampler(ctx, fenceSvc, workerCfg, conserveCh)
	  go brokerfence.RunLeaseAging(ctx, fenceSvc, workerCfg, agingCh)
	  go brokerfence.RunTokenCleaner(ctx, fenceSvc, 0)
	   // Worker 结果消费（接 metrics/日志平台，此处接收并丢弃）。
	   go drainWorkerResults(ctx, bypassCh, conserveCh, agingCh)
   }

   func drainWorkerResults(
	  ctx context.Context,
	  bypassCh <-chan brokerfence.BypassScanResult,
	  conserveCh <-chan brokerfence.ConserveSampleResult,
	   agingCh <-chan brokerfence.LeaseAgingResult,
   ) {
	  for {
		   select {
		   case <-ctx.Done():
			  return
		  case <-bypassCh:
		   case <-conserveCh:
		   case <-agingCh:
		  }
	   }
  }

// workerContext 构造跟随进程信号取消的 context。

   func workerContext() context.Context {
	   ctx, cancel := context.WithCancel(context.Background())
	  go func() {
		  ch := make(chan os.Signal, 1)
		   signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
		  <-ch
		  cancel()
	  }()
	  return ctx
   }
