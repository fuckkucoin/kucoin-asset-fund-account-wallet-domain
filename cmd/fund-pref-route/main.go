// @FileName: main.go
// @Summary: main 模块 main。
// cmd/fund-pref-route/main.go 默认资金账户偏好路由漂移封堵守护进程入口。
// 启动三类 Worker：
// 1. InflightCreditScanWorker — 在途贷记扫描与超时处理
// 2. RouteDriftPatrolWorker — 路由漂移巡检
// 3. PendingSwitchAdvanceWorker — 待切换队列推进
// 同时暴露 HTTP ops API（端口 FUND_PREF_ROUTE_PORT，默认 8096）。
// 环境变量：
// FUND_PREF_ROUTE_PORT HTTP 监听端口（默认 8096）
// FUND_PREF_ROUTE_LEASE_TTL 偏好租约有效期（默认 30s）
// FUND_PREF_DRIFT_PATROL_INT 漂移巡检周期（默认 15s）
// FUND_PREF_INFLIGHT_SCAN_INT 在途扫描周期（默认 10s）
// FUND_PREF_PENDING_ADV_INT 待切换推进周期（默认 5s）
// @Author: JaxRoe
// @Created: 2021.06.02

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/prefroute"
)

   func main() {
	  if err := run(); err != nil {
		   fmt.Fprintf(os.Stderr, "fund-pref-route: fatal: %v\n", err)
		   os.Exit(1)
	   }
   }

	  // 构造 Worker
	   worker := prefroute.NewWorker(svc, cfg)
	  // 启动 Worker 协程
	  workerDone := make(chan struct{})
	   go func() {
		  defer close(workerDone)
		   worker.Run(ctx)
	  }()
	   // 注册 HTTP 路由
	  gin.SetMode(gin.ReleaseMode)
	   r := gin.New()
	   r.Use(gin.Recovery())
	   h := prefroute.NewHandler(svc)
	  apiGroup := r.Group("/api/v1/fundpref")
	   h.Register(apiGroup)
	   r.GET("/healthz", func(c *gin.Context) {
		   c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "fund-pref-route"})
	  })
	   srv := &http.Server{
		   Addr:    ":" + port,
		  Handler: r,
		   ReadTimeout: 15 * time.Second,
		  WriteTimeout: 15 * time.Second,
	   }
	   srvErr := make(chan error, 1)
	   go func() {
		  fmt.Fprintf(os.Stdout, "fund-pref-route: listening on :%s\n", port)
		  srvErr <- srv.ListenAndServe()
	   }()
	  select {
	   case <-ctx.Done():
		  shutCtx,
			   shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
		  defer shutCancel()
		  _ = srv.Shutdown(shutCtx)
	   case err := <-srvErr:
		  if err != nil && err != http.ErrServerClosed {
			  return fmt.Errorf("http server: %w", err)
		  }
	   }
	  <-workerDone
	   return nil
   }

  func loadConfig() prefroute.Config {
	  return prefroute.Config{
		   LeaseTTL:             parseDuration(getEnv("FUND_PREF_ROUTE_LEASE_TTL", "30s"), 30*time.Second),
		  DriftPatrolInterval:  parseDuration(getEnv("FUND_PREF_DRIFT_PATROL_INT", "15s"), 15*time.Second),
		  InflightScanInterval: parseDuration(getEnv("FUND_PREF_INFLIGHT_SCAN_INT", "10s"), 10*time.Second),
		   PendingSwitchAdvanceInterval: parseDuration(getEnv("FUND_PREF_PENDING_ADV_INT", "5s"), 5*time.Second),
	   }
  }

   func getEnv(key, defaultVal string) string {
	   if v := os.Getenv(key); v != "" {
		   return v
	   }
	  return defaultVal
   }

  func parseDuration(s string, def time.Duration) time.Duration {
	   if s == "" {
		  return def
	  }
	   d, err := time.ParseDuration(s)
	  if err != nil {
		  return def
	  }
	   return d
  }
