// 功能：main 模块 main。
// oracle-tick-gate：估值 Tick 与资金写路径租约工作进程。
// 职责：
// 1. 过期 Tick / 租约 TTL 吊销
// 2. 价格漂移与世代翻转吊销泵
// 3. HTTP ops API（/ops/oracle-tick-gate/）
// 隔离：不改写 pricepoison 毒价公式、feeoracle 链上费报价、market/listing、fxmark 牌价。

// 作者：Cason
// 日期：2021.09.11

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
	"account-assets.kucoin.net/funding/internal/oracle"
	"account-assets.kucoin.net/funding/internal/oracletickgate"
)

  func main() {
	  apiAddr := flag.String("api-addr", ":8096", "ops HTTP API 监听地址")
	  metricsAddr := flag.String("metrics-addr", ":9304", "Prometheus 指标监听地址")
	   expireInterval := flag.Duration("expire-interval", 5*time.Second, "过期吊销间隔")
	  staleInterval := flag.Duration("stale-interval", 5*time.Second, "过期 Tick 扫描间隔")
	   flag.Parse()
	   ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	  defer cancel()
	   feed := oracle.Mock(time.Now().UTC())
	   svc := oracletickgate.New(feed)
	   cfg := oracletickgate.WorkerConfig{
		   ExpireInterval: *expireInterval,
		   StaleInterval: *staleInterval,
		   BatchSize: 100,
	   }
	  worker := oracletickgate.NewWorker(svc,
		   cfg)
	   gin.SetMode(gin.ReleaseMode)
	  router := gin.New()
	  router.Use(gin.Recovery())
	  oracletickgate.NewHandler(svc).Register(router.Group("/ops"))
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
			   fmt.Fprintf(os.Stderr, "[oracle-tick-gate] api error: %v\n", err)
		  }
	   }()
	   go func() {
		  if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			  fmt.Fprintf(os.Stderr, "[oracle-tick-gate] metrics error: %v\n", err)
		   }
	  }()
	  fmt.Fprintf(os.Stderr, "[oracle-tick-gate] started api=%s metrics=%s\n", *apiAddr, *metricsAddr)
	  go worker.Run(ctx)
	  <-ctx.Done()
	  shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	   defer shutCancel()
	   _ = apiSrv.Shutdown(shutCtx)
	  _ = metricsSrv.Shutdown(shutCtx)
   }
