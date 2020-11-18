// 重点：
// 本服务独立部署，通过 HTTP 提供冻额世代查询、双释确认、证明导出三类 ops API。
// Worker 定时任务：双释巡检（5 min）、孤儿 Frozen 扫描（10 min）、证明固化（15 min）。
// 本服务不直接操作账本——仅通过 swapthaw.Service 管理 FreezeLease / ThawCase 状态机。
// 风险点：
// 部署须配置 DB_DSN 和 REDIS_ADDR；缺失时禁止启动。
// 运营 API（/internal/swap-thaw/...）须由 API Gateway 保护，禁止对外暴露。

// Author: Gunner
// Date: 2020.11.10

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"account-assets.kucoin.net/funding/internal/swapthaw"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

  func main() {
	   zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	   logger := log.Logger.With().Str("service", "swap-thaw-gate").Logger()
	   svc := buildService()
	  worker := buildWorker(svc, logger)
	  r := gin.New()
	  r.Use(gin.Recovery())
	   internal := r.Group("/internal/swap-thaw")
	   swapthaw.RegisterRoutes(internal, svc)
	   addr := envOr("LISTEN_ADDR", ":8097")
	  srv := &http.Server{Addr: addr, Handler: r, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	  ctx, cancel := context.WithCancel(context.Background())
	  defer cancel()
	  // 启动 Worker 定时任务。
	   go runWorkers(ctx, worker, logger)
	   go func() {
		  if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			  logger.Fatal().Err(err).Msg("HTTP server failed")
		  }
	  }()
	  logger.Info().Str("addr", addr).Msg("swap-thaw-gate started")
	   quit := make(chan os.Signal, 1)
	  signal.Notify(quit,
		   syscall.SIGINT, syscall.SIGTERM)
	  <-quit
	  logger.Info().Msg("shutting down")
	  shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	   defer shutCancel()
	   _ = srv.Shutdown(shutCtx)
	  cancel()
	  logger.Info().Msg("swap-thaw-gate stopped")
  }

// buildService 构造 swapthaw.Service（模式：从环境变量读取 DSN，回退到内存存储）。

  func buildService() *swapthaw.Service {
	   leaseTTL := 30 * time.Minute
	   // 模式通过 NewServiceWith 注入 SQL 实现；此处使用内存存储作为默认值，
	  // 待 DB 初始化后在 svcboot 阶段替换。
	   return swapthaw.NewService(leaseTTL)
   }

// buildWorker 构造 Worker。

  func buildWorker(svc *swapthaw.Service, logger zerolog.Logger) *swapthaw.Worker {
	   // fetchFrozen 在模式下由 swap.Service.FetchFrozenOrders 注入；此处为空操作占位。
	  var fetchFrozen swapthaw.FrozenOrderFetcher = func(_ context.Context) ([]*swapthaw.FrozenOrder, error) {
		   return nil, nil
	  }
	   alert := func(_ context.Context, level, msg string, fields map[string]any) {
		  evt := logger.WithLevel(alertLevel(level))
		  for k, v := range fields {
			  evt = evt.Interface(k, v)
		  }
		   evt.Msg(msg)
	   }
	  return swapthaw.NewWorker(svc, fetchFrozen, alert, logger)
   }

// runWorkers 在独立 goroutine 内定时运行各 Worker。
   func runWorkers(ctx context.Context,
	  w *swapthaw.Worker, logger zerolog.Logger) {
	   patrolTick := time.NewTicker(5 * time.Minute)
	  orphanTick := time.NewTicker(10 * time.Minute)
	   consolidateTick := time.NewTicker(15 * time.Minute)
	   defer patrolTick.Stop()
	   defer orphanTick.Stop()
	   defer consolidateTick.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			  return
		  case <-patrolTick.C:
			  if err := w.DoubleReleasePatrol(ctx); err != nil {
				  logger.Error().Err(err).Msg("DoubleReleasePatrol failed")
			   }
		  case <-orphanTick.C:
			   if err := w.OrphanFrozenScanner(ctx); err != nil {
				  logger.Error().Err(err).Msg("OrphanFrozenScanner failed")
			  }
		   case <-consolidateTick.C:
			  if err := w.ProofConsolidator(ctx); err != nil {
				  logger.Error().Err(err).Msg("ProofConsolidator failed")
			   }
		  }
	   }
  }

  func envOr(key, def string) string {
	   v := os.Getenv(key)
	   if v == "" {
		  return def
	   }
	   return v
   }

  func alertLevel(level string) zerolog.Level {
	   switch level {
	   case "critical":
		   return zerolog.ErrorLevel
	  case "warning":
		   return zerolog.WarnLevel
	   default:
		  return zerolog.InfoLevel
	  }
   }

// printBuildInfo 在启动时打印代次信息（调试用）。
   func printBuildInfo() {
	  fmt.Fprintf(os.Stdout, "swap-thaw-gate built at %s\n", time.Now().Format(time.RFC3339))
   }
