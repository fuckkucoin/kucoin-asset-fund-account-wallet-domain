// breach-close-gate：SLA 违约关案与人工放行夹缝封堵工作进程。
// 职责：
// 1. ScanWorker — 周期扫描 reviewsla.Book，检测新违约，签发 BreachLease，
// 驱动案件强制 Expire（不 Allow）。
// 2. WindowPatrol — 周期巡检活跃 BreachLease，确认挡写窗口生效。
// 3. HTTP ops API — 暴露 /ops/breach-close-gate/ 路由（挂载于 -api-addr）。
// 4. Prometheus — 在 -metrics-addr 暴露指标。
// 隔离说明：
// 此进程不持有 compliance allow_lease；
// 不改写 cases FSM 内核（cases 包负责）；
// 通过 -cases-expire-url 调用 cases 服务强制 Expire（out-of-process HTTP）。
// 使用方式：
// breach-close-gate \
// -api-addr :8089 \
// -metrics-addr :9297 \
// -scan-interval 30s \
// -patrol-interval 60s

// Author: Kael
// Date: 2021.03.04

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
	"account-assets.kucoin.net/funding/internal/breachclosegate"
	"account-assets.kucoin.net/funding/internal/reviewsla"
)

   func main() {
	   apiAddr := flag.String("api-addr", ":8089", "ops HTTP API 监听地址")
	  metricsAddr := flag.String("metrics-addr", ":9297", "Prometheus 指标监听地址")
	  scanInterval := flag.Duration("scan-interval", 30*time.Second, "SLA 违约扫描间隔")
	  patrolInterval := flag.Duration("patrol-interval", 60*time.Second, "阻挡窗口巡检间隔")
	  maxBatch := flag.Int("max-batch", 50, "每轮最多处理的新违约数")
	   slaWindowHrs := flag.Float64("sla-window-hours", 4.0, "SLA 截止窗口（小时）")
	   flag.Parse()
	   ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	   defer cancel()
	   // ── 内存存储（环境应替换为 SQLStore）────────────────────────────────
	   store := breachclosegate.NewMemStore()
	   svc := breachclosegate.New(store)
	  // ── reviewsla.Book（环境应共享进程内单例或从 SQL 恢复）──────────────
	   slaBook := reviewsla.New(time.Duration(float64(time.Hour) * *slaWindowHrs))
	   // ── CaseExpirer（环境应连接 cases HTTP 服务或共享内存 Service）───────
	   // 此处使用 no-op expirer（进程内默认）；应注入 expirer。
	  expirer := breachclosegate.NewFuncExpirer(func(caseID string) error {
		  fmt.Fprintf(os.Stderr, "[breach-close-gate] would expire case %s\n", caseID)
		   return nil
	   })
	   // ── Worker ─────────────────────────────────────────────────────────────
	  cfg := breachclosegate.ScanWorkerConfig{
		   ScanInterval:   *scanInterval,
		   PatrolInterval: *patrolInterval,
		  MaxBatchSize:   *maxBatch,
	  }
	  worker := breachclosegate.NewScanWorker(slaBook, svc, expirer, cfg)
	  // ── ops API ────────────────────────────────────────────────────────────
	  gin.SetMode(gin.ReleaseMode)
	  router := gin.New()
	   router.Use(gin.Recovery())
	   opsGroup := router.Group("/ops")
	   handler := breachclosegate.NewHandler(svc, slaBook)
	  handler.Register(opsGroup)
	  apiSrv := &http.Server{Addr: *apiAddr,
		  Handler: router}
	  // ── Prometheus ─────────────────────────────────────────────────────────
	  metricsMux := http.NewServeMux()
	   metricsMux.Handle("/metrics", promhttp.Handler())
	   metricsMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		   w.WriteHeader(http.StatusOK)
		  _, _ = w.Write([]byte("ok"))
	   })
	   metricsSrv := &http.Server{Addr: *metricsAddr, Handler: metricsMux}
	   go func() {
		   if err := apiSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			   fmt.Fprintf(os.Stderr, "[breach-close-gate] api server error: %v\n", err)
		  }
	   }()
	  go func() {
		   if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			   fmt.Fprintf(os.Stderr, "[breach-close-gate] metrics server error: %v\n", err)
		   }
	  }()
	  // ── Worker 主循环 ──────────────────────────────────────────────────────
	  fmt.Fprintf(os.Stderr, "[breach-close-gate] started api=%s metrics=%s scan=%s\n",
		   *apiAddr, *metricsAddr, *scanInterval)
	  go worker.Run(ctx)
	  <-ctx.Done()
	   fmt.Fprintln(os.Stderr, "[breach-close-gate] shutting down")
	   shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	   defer shutCancel()
	  _ = apiSrv.Shutdown(shutCtx)
	   _ = metricsSrv.Shutdown(shutCtx)
   }
