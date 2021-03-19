// @FileName: main.go
// @Summary: 清算释放贷记租约夹缝封堵引擎独立服务入口。
// - 启动 clrcreditgate Worker（过期租约扫描、未贷记巡检、批次排空泵）。
// - 暴露 /v1/clrcreditgate/ HTTP API（ReleaseWithLease、ApplyWithLease、
// CloseBatchGuard、VoidLease、证明导出、Lease 查询）。
// - 不写日终清算应计链路；不操作法币轧差编排；不含 portfolio 科目树逻辑。
// - 未配置 DB 时使用内存 Store（单节点模式，不具备持久化能力）。
// 不变量：
// - SIGTERM/SIGINT 触发 graceful shutdown；Worker goroutine 通过 context cancel 退出。
// - 环境须通过环境变量注入 DB_DSN；未注入时服务以内存模式启动并打印警告。
// @Author: Likeo
// @Created: 2021.03.18

package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"account-assets.kucoin.net/funding/internal/clrcreditgate"
)

  func main() {
	   ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	  defer cancel()
	   nodeID := os.Getenv("NODE_ID")
	   if nodeID == "" {
		  nodeID = "clearing-credit-gate-default"
	   }
	   addr := os.Getenv("ADDR")
	  if addr == "" {
		  addr = ":8120"
	  }
	  metricsAddr := os.Getenv("METRICS_ADDR")
	   if metricsAddr == "" {
		  metricsAddr = ":9120"
	   }
	   // ── Store 构造 ────────────────────────────────────────────────────────────
	  var store clrcreditgate.Store
	   if dsn := os.Getenv("DB_DSN"); dsn != "" {
		  db, err := sql.Open("mysql", dsn)
		  if err != nil {
			  panic("clearing-credit-gate: failed to open DB: " + err.Error())
		  }
		   db.SetMaxOpenConns(20)
		   db.SetMaxIdleConns(5)
		  db.SetConnMaxLifetime(5 * time.Minute)
		  store = clrcreditgate.NewSQLStore(db)
	   } else {
		  _, _ = os.Stderr.WriteString(
			  "clearing-credit-gate: DB_DSN not set, running in memory-store mode\n",
		  )
		  store = clrcreditgate.NewMemStore()
	  }
	  // ── Engine ────────────────────────────────────────────────────────────────
	   // ClearingService 由部署通过 DI 框架注入（含 clearing.Service + portfolio.BookLedger）。
	   // 此处以 nil 注入（nilClearing stub 保证 Engine 在无外部依赖时仍可启动用于健康检查）。
	   leaseCfg := clrcreditgate.LeaseConfig{
		   DefaultTTL:   parseDuration(os.Getenv("LEASE_TTL"), 10*time.Minute),
		  DrainTimeout: parseDuration(os.Getenv("DRAIN_TIMEOUT"), 5*time.Minute),
	  }
	  leaseCfg.Fill()
	  reg := prometheus.NewRegistry()
	   metrics := clrcreditgate.NewMetrics(reg)
	  engine := clrcreditgate.NewEngine(
		  store,
		  nil, // 环境注入 clrcreditgate.ClearingService 实现
		  clrcreditgate.WithLeaseConfig(leaseCfg),
	   )
	   // ── Worker ────────────────────────────────────────────────────────────────
	  workerCfg := clrcreditgate.WorkerConfig{
		   ExpiryScanInterval: parseDuration(os.Getenv("EXPIRY_SCAN_INTERVAL"), 60*time.Second),
		  UnredeemedScanInterval: parseDuration(os.Getenv("UNREDEEMED_SCAN_INTERVAL"), 5*time.Minute),
		  DrainPumpInterval: parseDuration(os.Getenv("DRAIN_PUMP_INTERVAL"), 30*time.Second),
		   BatchSize:         200,
		   WorkerID:          nodeID + ".worker",
	   }
	  worker := clrcreditgate.NewWorker(engine, store, metrics, nil, workerCfg)
	   go worker.Run(ctx)
	  // ── HTTP ──────────────────────────────────────────────────────────────────
	  r := gin.New()
	   r.Use(gin.Recovery())
	   v1 := r.Group("/v1")
	  handler := clrcreditgate.NewHandler(engine, worker, store, metrics)
	  handler.RegisterRoutes(v1)
	   // 健康检查
	  r.GET("/healthz", func(c *gin.Context) {
		   c.JSON(http.StatusOK, gin.H{"status": "ok", "node": nodeID})
	  })
	  // ── Metrics 服务器 ────────────────────────────────────────────────────────
	   metricsMux := http.NewServeMux()
	   metricsMux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	  metricsSrv := &http.Server{Addr: metricsAddr, Handler: metricsMux}
	  go func() {
		   if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			   _, _ = os.Stderr.WriteString("clearing-credit-gate: metrics server error: " + err.Error() + "\n")
		   }
	  }()
	   // ── 主服务器 ──────────────────────────────────────────────────────────────
	   srv := &http.Server{
		  Addr: addr,
		  Handler: r,
		  ReadTimeout: 10 * time.Second,
		  WriteTimeout: 30 * time.Second,
	   }
	   go func() {
		  <-ctx.Done()
		   shutCtx,
			  shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
		   defer shutCancel()
		  _ = srv.Shutdown(shutCtx)
		   _ = metricsSrv.Shutdown(shutCtx)
	   }()
	  if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		  panic("clearing-credit-gate: server error: " + err.Error())
	  }
   }

// parseDuration 解析环境变量时间形态（fallback 默认值）。

  func parseDuration(s string, def time.Duration) time.Duration {
	  if s == "" {
		   return def
	   }
	  d, err := time.ParseDuration(s)
	   if err != nil || d <= 0 {
		  return def
	  }
	  return d
   }
