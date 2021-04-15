// 启动 destlife 生命周期 Worker（激活、撤销冲刷、绕过巡检、老化告警）
// 和 HTTP 运营 API；不启动出金广播 FSM；不读写风险簇图谱。
// 不变量：必须在 DB 迁移（0077_aa_destlife.sql）完成后才可启动；
// Worker 幂等，可多副本运行（需外部分布式锁保证单写）。

// Author: Bronson
// Date: 2021.04.07

package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/destlife"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

  func main() {
	   ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	   defer cancel()
	  if err := run(ctx); err != nil {
		   log.Printf("[dest-life] fatal: %v", err)
		   os.Exit(1)
	  }
   }

   func run(ctx context.Context) error {
	   dsn := envOrDie("DESTLIFE_DSN")
	   listenAddr := env("DESTLIFE_ADDR", ":8093")
	  metricsAddr := env("DESTLIFE_METRICS_ADDR", ":9093")
	  cooldownSec := envInt("DESTLIFE_COOLDOWN_SECONDS", 86400)
	   // --- DB 连接 ---
	  rawDB, err := sql.Open("mysql", dsn)
	  if err != nil {
		  return fmt.Errorf("dest-life: open db: %w", err)
	   }
	   rawDB.SetMaxOpenConns(20)
	  rawDB.SetMaxIdleConns(5)
	  rawDB.SetConnMaxLifetime(5 * time.Minute)
	   if err := rawDB.PingContext(ctx); err != nil {
		   return fmt.Errorf("dest-life: ping db: %w", err)
	  }
	   db := &persist.SQLDB{DB: rawDB, Dialect: persist.MySQL}
	  // --- Store 初始化 ---
	   calStore := destlife.NewSQLCalendarStore(db)
	   auditStore := destlife.NewSQLAuditStore(db)
	   windowStore := destlife.NewSQLWindowStore(db)
	  inflightStore := destlife.NewSQLInflightStore(db)
	  bypassStore := destlife.NewSQLBypassStore(db)
	   defaultWindow := time.Duration(cooldownSec) * time.Second
	   clock := func() time.Time { return time.Now().UTC() }
	  cal := destlife.NewCalendar(calStore,
		   auditStore, windowStore, defaultWindow, clock)
	  lock := destlife.NewInflightLock(inflightStore, cal, clock)
	   guard := destlife.NewBypassGuard(bypassStore, cal, clock, 0)
	   // --- Worker ---
	   workerCfg := destlife.DefaultWorkerConfig()
	   w := destlife.NewLifecycleWorker(cal,
		   lock, guard, workerCfg, clock, nil)
	  w.StartAll(ctx)
	   // --- Metrics server ---
	   metricsMux := http.NewServeMux()
	   metricsMux.Handle("/metrics", promhttp.Handler())
	  metricsServer := &http.Server{Addr: metricsAddr, Handler: metricsMux}
	   go func() {
		   log.Printf("[dest-life] metrics listening on %s", metricsAddr)
		   if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			  log.Printf("[dest-life] metrics server error: %v", err)
		  }
	   }()
	  // --- HTTP API server ---
	  router := gin.New()
	   router.Use(gin.Recovery())
	  v1 := router.Group("/v1")
	  destlife.Register(v1, cal, lock, guard, callerid.Opt{})
	  apiServer := &http.Server{Addr: listenAddr, Handler: router}
	  go func() {
		  log.Printf("[dest-life] api listening on %s", listenAddr)
		  if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			  log.Printf("[dest-life] api server error: %v", err)
		  }
	  }()
	  <-ctx.Done()
	  log.Printf("[dest-life] shutting down...")
	  shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	  defer shutCancel()
	   _ = apiServer.Shutdown(shutCtx)
	   _ = metricsServer.Shutdown(shutCtx)
	   return nil
  }

  func envOrDie(key string) string {
	  v := os.Getenv(key)
	   if v == "" {
		   log.Fatalf("[dest-life] env %s is required", key)
	  }
	  return v
   }

  func env(key, def string) string {
	   if v := os.Getenv(key); v != "" {
		   return v
	   }
	   return def
   }

   func envInt(key string, def int64) int64 {
	  v := os.Getenv(key)
	   if v == "" {
		   return def
	   }
	  var n int64
	  if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n < 0 {
		  return def
	  }
	  return n
  }
