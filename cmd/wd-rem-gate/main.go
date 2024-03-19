// 重点：
// 本服务独立部署，通过 HTTP 提供以下 ops API：
// - 开立 ShardHold（含 skip 腿守卫世代绑定）
// - 带世代剩余释放（rem_release_gen 互斥）
// - 守恒校验与案件证明导出
// - 双人确认强制释放
// Worker 定时任务：开放 Hold 巡检（2min）、守恒失败升级巡检（1min）、过期租约吊销（30s）。
// 本服务不直接操作账本——仅通过 wdpartial.Service 管理 Hold/Fill/Lease/ConserveCase/GuardGen 状态机。
// 风险点：
// 部署须配置 LISTEN_ADDR；运营 API 须由 API Gateway 鉴权保护，禁止对外暴露。
// force-release 接口须绑定操作审计日志，本服务不实现鉴权层。
// 内存模式（默认）进程重启后数据丢失；须注入 SQL 实现。

// Author: Vance
// Date: 2021.12.20

package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"account-assets.kucoin.net/funding/internal/wdpartial"
	"account-assets.kucoin.net/funding/internal/wdremgate"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

   func main() {
	  zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	  logger := log.Logger.With().Str("service", "wd-rem-gate").Logger()
	  svc := buildService()
	  gate := wdremgate.NewRemReleaseGate(svc)
	   worker := buildWorker(gate, svc)
	   mux := http.NewServeMux()
	  handler := wdremgate.NewHandler(gate)
	  handler.RegisterRoutes(mux, "/internal/wd-rem-gate")
	   addr := envOr("LISTEN_ADDR", ":8103")
	   srv := &http.Server{
		  Addr:         addr,
		  Handler:      mux,
		   ReadTimeout:  10 * time.Second,
		  WriteTimeout: 15 * time.Second,
	  }
	  ctx, cancel := context.WithCancel(context.Background())
	   defer cancel()
	   go runWorker(ctx, worker, logger)
	   go func() {
		  if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			  logger.Fatal().Err(err).Msg("HTTP server failed")
		  }
	   }()
	   logger.Info().Str("addr", addr).Msg("wd-rem-gate started")
	  quit := make(chan os.Signal, 1)
	   signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	   <-quit
	  logger.Info().Msg("shutting down")
	  shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	  defer shutCancel()
	  _ = srv.Shutdown(shutCtx)
	  cancel()
	   logger.Info().Msg("wd-rem-gate stopped")
   }

// buildService 构造 wdpartial.Service。
// 模式须通过 SQL 实现注入；此处使用内存存储作为默认值。
  func buildService() *wdpartial.Service {
	  return wdpartial.New()
   }

// buildWorker 构造后台 Worker。

  func buildWorker(gate *wdremgate.RemReleaseGate, svc *wdpartial.Service) *wdremgate.Worker {
	  return wdremgate.NewWorker(gate, svc, wdremgate.DefaultWorkerConfig)
  }

// runWorker 在独立 goroutine 内运行 Worker（ctx 取消时退出）。
  func runWorker(ctx context.Context, w *wdremgate.Worker, logger zerolog.Logger) {
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
