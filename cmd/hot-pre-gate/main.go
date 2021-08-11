// @FileName: main.go
// @Summary: main 模块 main。
// hot-pre-gate：热库存预检读闸与活库存漂移租约封堵工作进程。
// 职责：
// 1. ExpiryWorker — 定期扫描超期 issued/bound 预检租约，标记为 expired。
// 2. DriftPatrolWorker — 定期对活跃预检租约主动 Observe，发现漂移后升级并落 InvDriftProof。
// 3. HTTP ops API — 预检签发、绑定、复核、证明导出接口（/hotpregate/...）。
// 此工作进程只读消费 ledger.Store（Observe 路径）；
// 绝不触发 Sweep / Transfer / CapChange —— 只读契约由 guard.go 的 panic 强制保证。
// Prometheus 指标在 -metrics-addr 暴露。
// ops HTTP 在 -addr 监听。
// @Author: Cason
// @Created: 2021.07.20

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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/hotpregate"
)

   func main() {
	   addr := flag.String("addr", ":8097", "ops HTTP 监听地址")
	  metricsAddr := flag.String("metrics-addr", ":9297", "Prometheus 指标 HTTP 地址")
	   expiryInterval := flag.Duration("expiry-interval", 30*time.Second, "过期租约扫描间隔")
	  patrolInterval := flag.Duration("patrol-interval", 60*time.Second, "漂移巡检间隔")
	  expiryBatch := flag.Int("expiry-batch", 100, "过期扫描单次批量上限")
	   patrolBatch := flag.Int("patrol-batch", 50, "漂移巡检单次批量上限")
	  flag.Parse()
	   ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	   defer cancel()
	   // ── Prometheus 指标 ───────────────────────────────────────────────────────
	   leaseExpiredTotal := promauto.NewCounter(prometheus.CounterOpts{
		   Name: "hpg_worker_lease_expired_total",
		  Help: "Worker 标记为 expired 的租约总数",
	   })
	   driftDetectedTotal := promauto.NewCounter(prometheus.CounterOpts{
		  Name: "hpg_worker_drift_detected_total",
		  Help: "Worker 巡检发现的漂移总数",
	   })
	  patrolRoundTotal := promauto.NewCounter(prometheus.CounterOpts{
		  Name: "hpg_worker_patrol_round_total",
		  Help: "漂移巡检 Worker 轮次总数",
	   })
	  expiryRoundTotal := promauto.NewCounter(prometheus.CounterOpts{
		   Name: "hpg_worker_expiry_round_total",
		  Help: "过期扫描 Worker 轮次总数",
	   })
	  wm := &hotpregate.WorkerMetrics{
		  LeaseExpiredTotal: leaseExpiredTotal.Inc,
		   DriftDetectedTotal: driftDetectedTotal.Inc,
		   PatrolledTotal: patrolRoundTotal.Inc,
		  ExpiryRunTotal: expiryRoundTotal.Inc,
	   }
	  // ── 存储与服务初始化 ──────────────────────────────────────────────────────
	  // 环境应将 MemStore 替换为 SQL 实现（接入 persist.Pool）。
	  preStore := hotpregate.NewMemStore()
	  hotBook := hotinv.New(map[string]hotinv.Cap{})
	  // ledger.Store 环境由 app.Runtime.Store 注入；此处 nil 触发 guard observe 失败保护。
	   observer := hotpregate.NewGuardedObserver(hotBook, nil)
	  svc := hotpregate.New(preStore,
		   hotBook, observer)
	  // ── Worker 启动 ───────────────────────────────────────────────────────────
	   expiryWorker := hotpregate.NewExpiryWorker(svc, *expiryInterval, *expiryBatch, wm)
	  patrolWorker := hotpregate.NewDriftPatrolWorker(svc, *patrolInterval, *patrolBatch, nil, wm)
	   go expiryWorker.Run(ctx)
	  go patrolWorker.Run(ctx)
	  // ── HTTP ops API ──────────────────────────────────────────────────────────
	   gin.SetMode(gin.ReleaseMode)
	  r := gin.New()
	   r.Use(gin.Recovery())
	  v1 := r.Group("/v1")
	   hotpregate.RegisterWithService(v1, svc)
	   srv := &http.Server{Addr: *addr, Handler: r}
	  go func() {
		  if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			   fmt.Fprintf(os.Stderr, "hot-pre-gate http: %v\n", err)
		  }
	   }()
	   // ── Prometheus ────────────────────────────────────────────────────────────
	   metricsMux := http.NewServeMux()
	   metricsMux.Handle("/metrics", promhttp.Handler())
	  metricsSrv := &http.Server{Addr: *metricsAddr,
		   Handler: metricsMux}
	  go func() {
		  if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			   fmt.Fprintf(os.Stderr, "hot-pre-gate metrics: %v\n", err)
		   }
	  }()
	  <-ctx.Done()
	  shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	  defer shutCancel()
	  _ = srv.Shutdown(shutCtx)
	   _ = metricsSrv.Shutdown(shutCtx)
   }
