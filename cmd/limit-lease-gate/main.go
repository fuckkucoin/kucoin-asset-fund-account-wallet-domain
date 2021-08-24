// @FileName: main.go
// @Summary: cmd/limit-lease-gate：限额预检与占用提交租约漂移封堵引擎服务入口。
// 该服务封装 limitleasegate 模块，提供：
// - 预检租约签发 API（IssueCheckLease：limitgate.Check + 签发 LimitCheckLease）
// - 占用提交绑定 API（BindLease：checked → bound + commit_ref）
// - 占用提交复验 API（CommitRecheck：复验 PolicyVer+TodaySum，消费或拒绝）
// - 漂移证明管理 API（ExportProof / ConfirmProof / DismissProof）
// - 策略代次通知 API（PolicyVersionNotify：批量吊销旧代次活跃租约）
// - 后台 Worker（LeaseExpiry / PolicyVersionRevoke / ProofProve）
// - 不修改 availcas 可用余额 CAS 内核（r6 A）。
// - 不修改目的地风险图谱动态限额公式（r4）。
// - 不修改 usage.Store 累计内核算法（只读消费）。
// - 不修改 policy.Registry 文档结构主链（只读消费 Version）。
// - 不写 Screen/冷却/法币凭证/Inflight/核验/贷记出箱。
// 依赖注入：环境需替换 MemStore 为 SQLStore（对接 llg_limit_check_leases 等两张表）。
// @Author: ZaneCruz
// @Created: 2021.08.23

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
	"account-assets.kucoin.net/funding/internal/limitleasegate"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/internal/usage"
)

  func main() {
	  zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	  log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	  // 应替换为 SQLStore；此处 MemStore 供集成冒烟使用。
	   store := limitleasegate.NewMemStore()
	   // 应绑定实际 policy.Registry 与 usage.Store 实现。
	  // 此处使用内存实现供冒烟。
	   reg := policy.NewRegistry(policy.DefaultV1())
	   use := usage.New()
	   cfg := limitleasegate.Config{
		   LeaseTTL:        120 * time.Second,
		  RevokeBatchSize: 200,
		   ProofBatchSize:  200,
	   }
	  workerCfg := limitleasegate.WorkerConfig{
		  Interval: 30 * time.Second,
		  BatchSize: 200,
	   }
	   router := gin.New()
	  router.Use(gin.Recovery())
	  rg := router.Group("/api/v1")
	   ctx, cancel := context.WithCancel(context.Background())
	   defer cancel()
	  mod := limitleasegate.Register(
		  ctx,
		   store,
		  reg,
		   use,
		  cfg,
		   workerCfg,
		   rg,
	   )
	   addr := app.HTTPListenAddr("8124")
	  srv := &http.Server{
		   Addr:              addr,
		  Handler:           router,
		   ReadHeaderTimeout: 10 * time.Second,
	   }
	   go func() {
		   log.Info().Str("addr", addr).Msg("limit-lease-gate listening")
		   if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			  log.Fatal().Err(err).Msg("server error")
		   }
	  }()
	  quit := make(chan os.Signal, 1)
	  signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	  <-quit
	   log.Info().Msg("shutting down limit-lease-gate")
	  mod.Stop()
	  cancel()
	  shutCtx,
		  shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	  defer shutCancel()
	   if err := srv.Shutdown(shutCtx); err != nil {
		  log.Error().Err(err).Msg("server shutdown error")
	   }
	  log.Info().Msg("limit-lease-gate stopped")
  }
