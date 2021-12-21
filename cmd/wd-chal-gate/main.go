// 该服务封装 wdchalgate 模块，提供：
// - 挑战租约签发 API（IssueLease / PresentLease / ConsumeLease / RevokeLease）
// - 旧令牌拒放行证明管理 API（ConfirmProof / DismissProof / ExportProof）
// - 因子变更通知 API（NotifyFactorChange / NotifyFailLockUnlock）
// - 后台 Worker（LeaseExpiryWorker / StaleChalWorker / FactorWatchWorker / FailLockWatchWorker）
// - 不修改 tradepw Change/Reset 哈希主路径（r13 B）。
// - 不修改 seclock Mark/RequireWithdraw 冷却窗口（r13 E）。
// - 不修改 wdguard CheckTriple 三联闸（r12 C）。
// - 不修改 dualctl 资金放行票证（r10 G）。
// - 不写余额证明 / 分片剩余 / 气隙 / 热库存 / 巡检挡写 / hop 补偿。
// 依赖注入：环境需替换 MemStore 为 SQLStore（对接 wdvc_chal_leases 等三张表）。

// Author: Orin
// Date: 2021.12.20

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
	"account-assets.kucoin.net/funding/internal/wdchalgate"
)

  func main() {
	  zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	  log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	   // 应替换为 SQLStore；此处 MemStore 供集成冒烟使用。
	   store := wdchalgate.NewMemStore()
	   cfg := wdchalgate.Config{
		  LeaseTTL: 300 * time.Second,
		   AllowGenDrift:  false,
		   StaleBatchSize: 200,
	   }
	   workerCfg := wdchalgate.WorkerConfig{
		  Interval: 30 * time.Second,
		   BatchSize: 200,
	   }
	   router := gin.New()
	  router.Use(gin.Recovery())
	  rg := router.Group("/api/v1")
	   ctx, cancel := context.WithCancel(context.Background())
	   defer cancel()
	   notifyFn := wdchalgate.ProofProvenNotifier(func(notifyCtx context.Context, proof *wdchalgate.StaleChalProof) {
		   log.Warn().
			   Str("proof_id", proof.ID).
			   Str("lease_id", proof.LeaseID).
			   Str("user_id", proof.UserID).
			  Str("factor", proof.Factor).
			  Int64("old_combo_gen", proof.OldComboGen).
			  Int64("new_combo_gen", proof.NewComboGen).
			   Str("drift_reason", string(proof.DriftReason)).
			   Msg("wdchalgate: StaleChalProof open → proven，需运营四眼确认")
	   })
	   mod := wdchalgate.Register(ctx,
		   store, cfg, workerCfg, notifyFn, rg)
	   defer mod.Stop()
	  addr := app.HTTPListenAddr("8128")
	  log.Info().Str("addr", addr).Msg("wdchalgate: service started")
	  log.Info().Msg("wdchalgate: LeaseExpiryWorker / StaleChalWorker / FactorWatchWorker / FailLockWatchWorker running")
	  srv := &http.Server{
		   Addr: addr,
		  Handler: router,
		   ReadTimeout:  15 * time.Second,
		   WriteTimeout: 15 * time.Second,
	  }
	  go func() {
		  if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			   log.Fatal().Err(err).Msg("wdchalgate: server error")
		   }
	   }()
	  quit := make(chan os.Signal, 1)
	   signal.Notify(quit,
		   syscall.SIGINT, syscall.SIGTERM)
	   <-quit
	  log.Info().Msg("wdchalgate: shutting down")
	   shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	   defer shutCancel()
	   if err := srv.Shutdown(shutCtx); err != nil {
		   log.Error().Err(err).Msg("wdchalgate: forced shutdown")
	   }
	   log.Info().Msg("wdchalgate: stopped")
   }
