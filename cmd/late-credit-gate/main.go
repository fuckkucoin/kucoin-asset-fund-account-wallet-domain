// @FileName: main.go
// @Summary: late-credit-gate：迟贷记持久互斥与强制围栏工作进程。
// 职责：
// 1. 竞态事件持久化泵
// 2. 强制 fence 巡检
// 3. 僵尸占用收口
// 4. HTTP ops API（/ops/late-credit-gate/）
// 隔离：不改写错链回收编排 FSM，不改 deposit 自动贷记公式。
// @Author: Cason
// @Created: 2021.08.07

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
	"account-assets.kucoin.net/funding/internal/latecreditgate"
)

   func main() {
	  apiAddr := flag.String("api-addr", ":8091", "ops HTTP API 监听地址")
	   metricsAddr := flag.String("metrics-addr", ":9299", "Prometheus 指标监听地址")
	  raceInterval := flag.Duration("race-interval", 15*time.Second, "竞态泵间隔")
	  fenceInterval := flag.Duration("fence-interval", 30*time.Second, "围栏巡检间隔")
	  zombieInterval := flag.Duration("zombie-interval", time.Minute, "僵尸占用扫描间隔")
	  flag.Parse()
	   ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	  defer cancel()
	  svc := latecreditgate.New()
	   cfg := latecreditgate.WorkerConfig{
		  RacePumpInterval:  *raceInterval,
		   FenceScanInterval: *fenceInterval,
		  ZombieInterval:    *zombieInterval,
		  BatchSize: 100,
	  }
	   worker := latecreditgate.NewWorker(svc, cfg, nil)
	   gin.SetMode(gin.ReleaseMode)
	  router := gin.New()
	  router.Use(gin.Recovery())
	  latecreditgate.NewHandler(svc).Register(router.Group("/ops"))
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
			   fmt.Fprintf(os.Stderr, "[late-credit-gate] api error: %v\n", err)
		  }
	   }()
	  go func() {
		  if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			  fmt.Fprintf(os.Stderr, "[late-credit-gate] metrics error: %v\n", err)
		   }
	   }()
	   fmt.Fprintf(os.Stderr, "[late-credit-gate] started api=%s metrics=%s\n", *apiAddr, *metricsAddr)
	  go worker.Run(ctx)
	   <-ctx.Done()
	  shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	  defer shutCancel()
	  _ = apiSrv.Shutdown(shutCtx)
	   _ = metricsSrv.Shutdown(shutCtx)
   }
