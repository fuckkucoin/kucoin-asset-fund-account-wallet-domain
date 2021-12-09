// 重点：
// 本服务独立部署，通过 HTTP 提供密码世代查询、漂移案件双人确认、证明导出三类 ops API。
// Worker 定时任务：在途令牌扫描（30s）、漂移巡检（2min）、冷却绑定确认（60s）。
// 本服务不直接操作账本——仅通过 tradepwgen.Service 管理 PwGeneration / VerifyLease / PwDriftCase 状态机。
// 风险点：
// 部署须配置 LISTEN_ADDR；运营 API 须由 API Gateway 鉴权保护，禁止对外暴露。
// 漂移案件双人确认接口（/drift/:case_id/prove）须绑定操作审计日志，本服务不实现鉴权层。

// Author: Cason
// Date: 2021.12.08

package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"account-assets.kucoin.net/funding/internal/tradepwgen"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

  func main() {
	  zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	  logger := log.Logger.With().Str("service", "trade-pw-gen").Logger()
	   svc := buildService()
	  worker := buildWorker(svc)
	   mux := http.NewServeMux()
	  handler := tradepwgen.NewHandler(svc)
	  handler.RegisterRoutes(mux, "/internal/tradepwgen")
	   addr := envOr("LISTEN_ADDR", ":8102")
	  srv := &http.Server{
		  Addr: addr,
		  Handler: mux,
		   ReadTimeout: 10 * time.Second,
		   WriteTimeout: 10 * time.Second,
	  }
	  ctx,
		   cancel := context.WithCancel(context.Background())
	   defer cancel()
	  go runWorkers(ctx, worker, logger)
	   go func() {
		   if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			  logger.Fatal().Err(err).Msg("HTTP server failed")
		   }
	  }()
	  logger.Info().Str("addr", addr).Msg("trade-pw-gen started")
	   quit := make(chan os.Signal, 1)
	   signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	   <-quit
	   logger.Info().Msg("shutting down")
	  shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	   defer shutCancel()
	   _ = srv.Shutdown(shutCtx)
	  cancel()
	  logger.Info().Msg("trade-pw-gen stopped")
   }

// buildService 构造 tradepwgen.Service。
// 模式须通过 NewServiceWith 注入 SQL 实现；此处使用内存存储作为默认值。
  func buildService() *tradepwgen.Service {
	  leaseTTL := 10 * time.Minute
	   return tradepwgen.NewService(leaseTTL)
   }

// buildWorker 构造 Worker。

  func buildWorker(svc *tradepwgen.Service) *tradepwgen.Worker {
	   return tradepwgen.NewWorker(svc, tradepwgen.DefaultWorkerConfig)
  }

// runWorkers 在独立 goroutine 内运行 Worker（ctx 取消时退出）。

  func runWorkers(ctx context.Context, w *tradepwgen.Worker, logger zerolog.Logger) {
	  if err := w.Run(ctx); err != nil && err != context.Canceled {
		   logger.Error().Err(err).Msg("Worker exited with error")
	  }
   }

  func envOr(key, def string) string {
	   v := os.Getenv(key)
	   if v == "" {
		  return def
	   }
	  return v
   }
