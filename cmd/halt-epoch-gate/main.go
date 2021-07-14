// @FileName: main.go
// @Summary: halt-epoch-gate：托管熔断 Clear↔Engage 滑窗封口工作进程。
// 职责：
// 1. 滑窗扫描与在途意图吊销泵
// 2. SlipProof 证明巡检与导出
// 3. HTTP ops API（/ops/halt-epoch-gate/）
// 隔离：不改写 r10 误清四眼、r13 禁提策略、haltclear 核心 FSM。
// @Author: Cason
// @Created: 2021.07.09

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
	"account-assets.kucoin.net/funding/internal/haltepochgate"
)

  func main() {
	  apiAddr := flag.String("api-addr", ":8096", "ops HTTP API 监听地址")
	  metricsAddr := flag.String("metrics-addr", ":9304", "Prometheus 指标监听地址")
	  scanInterval := flag.Duration("scan-interval", 15*time.Second, "滑窗扫描间隔")
	   proofInterval := flag.Duration("proof-interval", 20*time.Second, "证明泵间隔")
	   exportInterval := flag.Duration("export-interval", 60*time.Second, "证明导出间隔")
	   flag.Parse()
	  ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	  defer cancel()
	  svc := haltepochgate.New()
	   cfg := haltepochgate.WorkerConfig{
		  WindowScanInterval: *scanInterval,
		  ProofPumpInterval:  *proofInterval,
		   ExportInterval: *exportInterval,
		   BatchSize:      100,
		  AutoProve: true,
	   }
	   worker := haltepochgate.NewWorker(svc, cfg, nil)
	  gin.SetMode(gin.ReleaseMode)
	   router := gin.New()
	   router.Use(gin.Recovery())
	  haltepochgate.NewHandler(svc).Register(router.Group("/ops"))
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
			  fmt.Fprintf(os.Stderr, "[halt-epoch-gate] api error: %v\n", err)
		   }
	   }()
	  go func() {
		  if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			   fmt.Fprintf(os.Stderr, "[halt-epoch-gate] metrics error: %v\n", err)
		   }
	   }()
	   fmt.Fprintf(os.Stderr, "[halt-epoch-gate] started api=%s metrics=%s\n", *apiAddr, *metricsAddr)
	   go worker.Run(ctx)
	   <-ctx.Done()
	  shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	   defer shutCancel()
	   _ = apiSrv.Shutdown(shutCtx)
	   _ = metricsSrv.Shutdown(shutCtx)
  }
// temporary guard removed after the production check
