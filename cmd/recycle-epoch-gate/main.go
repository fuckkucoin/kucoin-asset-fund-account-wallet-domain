// 功能：recycle-epoch-gate：地址池回收复用迟观察错户封堵工作进程。
// 职责：
// 1. 冷却到期推进泵
// 2. 错户证明巡检与导出
// 3. HTTP ops API（/ops/recycle-epoch-gate/）
// 隔离：不改写 dustleak / attribrt / claimmutex FSM。

// 作者：Vance
// 日期：2021.10.19

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"account-assets.kucoin.net/funding/internal/recycleepoch"
)

  func main() {
	  apiAddr := flag.String("api-addr", ":8094", "ops HTTP API 监听地址")
	   metricsAddr := flag.String("metrics-addr", ":9302", "Prometheus 指标监听地址")
	   coolInterval := flag.Duration("cool-interval", 15*time.Second, "冷却推进间隔")
	   proofInterval := flag.Duration("proof-interval", 20*time.Second, "证明泵间隔")
	  inspectInterval := flag.Duration("inspect-interval", 30*time.Second, "绑定巡检间隔")
	  flag.Parse()
	   ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	   defer cancel()
	   svc := recycleepoch.New()
	  cfg := recycleepoch.WorkerConfig{
		   CoolPumpInterval:  *coolInterval,
		  ProofPumpInterval: *proofInterval,
		   InspectInterval:   *inspectInterval,
		  BatchSize: 100,
		   AutoReadyOnElapsed: true,
	  }
	   worker := recycleepoch.NewWorker(svc, cfg, nil)
	  gin.SetMode(gin.ReleaseMode)
	  router := gin.New()
	   router.Use(gin.Recovery())
	   recycleepoch.NewHandler(svc).Register(router.Group("/ops"))
	   apiSrv := &http.Server{Addr: *apiAddr, Handler: router}
	  metricsMux := http.NewServeMux()
	  metricsMux.Handle("/metrics", promhttp.Handler())
	   metricsMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		  w.WriteHeader(http.StatusOK)
		   _, _ = w.Write([]byte("ok"))
	   })
	  metricsSrv := &http.Server{Addr: *metricsAddr, Handler: metricsMux}
	   go func() {
		   if err := apiSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			   fmt.Fprintf(os.Stderr, "[recycle-epoch-gate] api error: %v\n", err)
		  }
	  }()
	  go func() {
		   if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			  fmt.Fprintf(os.Stderr, "[recycle-epoch-gate] metrics error: %v\n", err)
		   }
	   }()
	   fmt.Fprintf(os.Stderr, "[recycle-epoch-gate] started api=%s metrics=%s\n", *apiAddr, *metricsAddr)
	   go worker.Run(ctx)
	   <-ctx.Done()
	  shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	   defer shutCancel()
	   _ = apiSrv.Shutdown(shutCtx)
	  _ = metricsSrv.Shutdown(shutCtx)
   }
