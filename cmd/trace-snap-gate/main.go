// @FileName: main.go
// @Summary: cmd/trace-snap-gate：资金 hop 溯源活账快照世代与法医重放一致性引擎服务入口。
// 该服务封装 tracesnapgate 模块，提供：
// - 快照创建 API（CreateSnapshot：基于 TraceAsset 瞬时结果建 open 快照）
// - 快照密封 API（Seal：固化 TransfersRoot / BooksRoot / AsOf）
// - 重放复验 API（Replay：摘要根，漂移则落 TraceDriftProof）
// - 法医导出 API（Export：fail-closed，缺 snapshot_gen 则拒绝）
// - 案件绑定 API（BindCase：一案一绑，幂等）
// - 漂移证明查询 API（GetProof / ListProofs）
// - 后台 Worker（DriftPatrol / SealArchive）
// - 不修改 r2 不可篡改审计仓与法医重放仓主链。
// - 不修改 r3 账本事件溯源因果图边公式。
// - 不修改 r6 E leddiff 快照差分内核。
// - 不修改 ledger 记账主路径（Transfer / Credit / Freeze）。
// - 只读消费 ledger.Transfer、ledger.BookView、snap.Position。
// 依赖注入：环境需替换 MemStore 为 SQLStore，并接入业务 TraceProvider。
// 此处 MemStore + StaticTraceProvider 供集成冒烟使用。
// @Author: Blaise
// @Created: 2021.12.08

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
	"account-assets.kucoin.net/funding/internal/investigate"
	"account-assets.kucoin.net/funding/internal/tracesnapgate"
)

  func main() {
	  zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	   log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	  // 应替换为 SQLStore；此处 MemStore 供集成冒烟使用。
	  store := tracesnapgate.NewMemStore()
	   // 应接入 ledger.Service 实例；此处静态 Provider 仅供冒烟。
	   provider := newStaticProvider()
	   cfg := tracesnapgate.Config{
		   SealTTL:         10 * time.Minute,
		  ReplayBatchSize: 50,
		   AllowLiveExport: false,
	  }
	   idx := &tracesnapgate.FixedUserIndexer{}
	   mod := tracesnapgate.New(provider, store, idx, cfg)
	  router := gin.New()
	   router.Use(gin.Recovery())
	   rg := router.Group("/api/v1")
	   mod.Register(rg)
	   ctx,
		   cancel := context.WithCancel(context.Background())
	   defer cancel()
	   mod.StartWorkers(ctx)
	  addr := envOrDefault("LISTEN_ADDR", ":9090")
	  srv := &http.Server{Addr: addr, Handler: router, ReadHeaderTimeout: 10 * time.Second}
	  quit := make(chan os.Signal, 1)
	  signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	  go func() {
		   log.Info().Str("addr", addr).Msg("trace-snap-gate: starting")
		   if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			  log.Fatal().Err(err).Msg("trace-snap-gate: server error")
		   }
	  }()
	  <-quit
	   log.Info().Msg("trace-snap-gate: shutting down")
	  cancel()
	  shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	   defer shutCancel()
	  if err := srv.Shutdown(shutCtx); err != nil {
		  log.Error().Err(err).Msg("trace-snap-gate: shutdown error")
	  }
	   log.Info().Msg("trace-snap-gate: stopped")
   }

   func envOrDefault(key, def string) string {
	  if v := os.Getenv(key); v != "" {
		  return v
	   }
	   return def
  }

// ─── staticProvider（冒烟用）──────────────────────────────────────────────────
// staticProvider 静态 TraceProvider：返回空列表（冒烟用；须接入 ledger.Service）。
type staticProvider struct{}

  func newStaticProvider() tracesnapgate.TraceProvider { return &staticProvider{} }

  func (p *staticProvider) TraceHops(_ context.Context, _, _ string) ([]investigate.Hop, error) {
	   return []investigate.Hop{}, nil
  }

  func (p *staticProvider) TraceBooks(_ context.Context, _, _ string) ([]tracesnapgate.SnapPosition, error) {
	  return []tracesnapgate.SnapPosition{},
		  nil
  }
