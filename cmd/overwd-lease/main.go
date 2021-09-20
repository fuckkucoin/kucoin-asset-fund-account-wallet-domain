// @FileName: main.go
// @Summary: cmd/overwd-lease：超提屏障预检提交租约漂移与差异证明收口服务入口。
// 该服务封装 overwdlease 模块，提供：
// - 预检绑定租约 API（IssueBind / CommitBind / RevokeBind）
// - 差异证明四眼确认 API（ConfirmProof / DismissProof）
// - 漂移案例管理 API（ConfirmDriftCase / DismissDriftCase）
// - 证明导出 API（ExportProof）
// - 后台 Worker（BindExpiryWorker / ProofChaseWorker / DriftPatrolWorker）
// - 不修改 availcas CAS 内核公式（r6 A）。
// - 不修改 hotbound 热库 cap（r10 F）。
// - 不修改出金广播 FSM 内核。
// - 只读消费 LeaseObservation（availcas 只读快照）。
// 依赖注入：环境需替换 MemStore 为 SQLStore（对接 ow_precheck_binds 等三张表）。
// @Author: Lior
// @Created: 2021.09.19

package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/overwdlease"
)

  func main() {
	  zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	   log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr,
		   TimeFormat: time.RFC3339})
	   // 应替换为 SQLStore；此处 MemStore 供集成冒烟使用
	   store := overwdlease.NewMemStore()
	  cfg := overwdlease.Config{
		  BindTTL:             60 * time.Second,
		   GenDriftTolerance:   0,
		   AvailDriftTolerance: false,
	   }
	  workerCfg := overwdlease.WorkerConfig{
		   Interval: 30 * time.Second,
		   BatchSize: 200,
	   }
	  router := gin.New()
	   router.Use(gin.Recovery())
	  rg := router.Group("/api/v1")
	  notifyFn := func(p *overwdlease.VarianceProof) error {
		  log.Warn().
			   Str("proof_id", p.ID).
			  Str("user_id", p.UserID).
			   Str("asset", p.Asset).
			  Str("trigger", string(p.Trigger)).
			   Int64("drift_gen", p.DriftGen).
			   Msg("overwdlease: open VarianceProof requires four-eye confirmation")
		  return nil
	  }
	   mod := overwdlease.Register(store, cfg, workerCfg, notifyFn, rg)
	   defer mod.Stop()
	   addr := app.HTTPListenAddr("8127")
	   log.Info().Str("addr", addr).Msg("overwdlease: service started")
	  log.Info().Msg("overwdlease: BindExpiryWorker / ProofChaseWorker / DriftPatrolWorker running")
	  srv := &http.Server{
		  Addr:    addr,
		  Handler: router,
		  ReadTimeout: 15 * time.Second,
		   WriteTimeout: 15 * time.Second,
	   }
	   go func() {
		   if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			  log.Fatal().Err(err).Msg("overwdlease: server error")
		  }
	  }()
	  quit := make(chan os.Signal, 1)
	  signal.Notify(quit,
		  syscall.SIGINT, syscall.SIGTERM)
	  <-quit
	  log.Info().Msg("overwdlease: shutting down")
	  ctx,
		   cancel := context.WithTimeout(context.Background(), 10*time.Second)
	   defer cancel()
	   if err := srv.Shutdown(ctx); err != nil {
		  log.Error().Err(err).Msg("overwdlease: forced shutdown")
	   }
	   log.Info().Msg("overwdlease: stopped")
   }
