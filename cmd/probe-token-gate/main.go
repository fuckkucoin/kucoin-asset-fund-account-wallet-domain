// @FileName: main.go
// @Summary: main 模块 main。
// cmd/probe-token-gate：资金写路径探针互斥与令牌校验非原子挡写世代引擎服务入口。
// 该服务封装 probetokengate 模块，提供：
// - 联合 Check API（JointCheck：writegate.CheckGate + 签发联合租约）
// - 写提交绑定 API（BindLease：checked → bound + commit_ref）
// - 写提交最终核验 API（CommitCheck：复验探针+令牌，消费或拒绝）
// - 漂移证明管理 API（ExportProof / ConfirmProof / DismissProof）
// - 事件通知 API（ProbeFlip / PoisonRevoke 批量吊销）
// - 后台 Worker（LeaseExpiry / ProbeFlipRevoke / PoisonRevoke / ProofProve）
// - 不修改 pricepoison.PoisonEngine 内核算法（r10 B）。
// - 不修改 custodyhalt Clear/ClearForced（r10 C）。
// - 不修改 ledger Apply 内核。
// - 不修改 signpipe InjectFault（r15 A）。
// - 不修改 treasury Plan→Exec（r15 B）。
// - 不写签章故障/库务 Exec/偏离闸/法币上分/多科目账单/出箱 Halt。
// 依赖注入：环境需替换 MemStore 为 SQLStore（对接 ptg_probe_token_leases 等三张表）。
// @Author: Orin
// @Created: 2021.10.03

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
	"account-assets.kucoin.net/funding/internal/probetokengate"
	"account-assets.kucoin.net/funding/internal/writegate"
)

  func main() {
	   zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	  log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	   // 应替换为 SQLStore；此处 MemStore 供集成冒烟使用。
	   store := probetokengate.NewMemStore()
	   // 应绑定 pricepoison.ProbeMutex 与 GenToken 实际实现。
	   // 此处使用内存只读实现供冒烟。
	   probeReader := writegate.NewMemProbeMutexReader()
	   tokenReader := writegate.NewMemGenTokenReader()
	  poisonReader := writegate.NewMemPoisonProofReader()
	   checker := writegate.NewChecker(probeReader,
		  tokenReader, poisonReader)
	   cfg := probetokengate.Config{
		  LeaseTTL:        120 * time.Second,
		  RevokeBatchSize: 200,
		   ProofBatchSize:  200,
	  }
	   workerCfg := probetokengate.WorkerConfig{
		   Interval: 30 * time.Second,
		  BatchSize: 200,
	   }
	   router := gin.New()
	   router.Use(gin.Recovery())
	   rg := router.Group("/api/v1")
	  ctx, cancel := context.WithCancel(context.Background())
	  defer cancel()
	   mod := probetokengate.Register(
		   ctx,
		  store,
		  checker,
		   cfg,
		   workerCfg,
		   nil,
		   rg,
	   )
	   addr := ":8090"
	   if v := os.Getenv("LISTEN_ADDR"); v != "" {
		   addr = v
	  }
	   srv := &http.Server{
		   Addr:        addr,
		  Handler:     router,
		   ReadTimeout: 15 * time.Second,
		  WriteTimeout: 15 * time.Second,
	   }
	   log.Info().
		   Str("addr", addr).
		  Str("lease_ttl", cfg.LeaseTTL.String()).
		   Msg("probe-token-gate starting")
	   go func() {
		   if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			   log.Fatal().Err(err).Msg("probe-token-gate listen failed")
		  }
	   }()
	  quit := make(chan os.Signal, 1)
	   signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	   <-quit
	   log.Info().Msg("probe-token-gate shutting down")
	   mod.Stop()
	  shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	  defer shutdownCancel()
	  if err := srv.Shutdown(shutdownCtx); err != nil {
		  log.Error().Err(err).Msg("probe-token-gate shutdown error")
	   }
	   log.Info().Msg("probe-token-gate stopped")
   }
