// 功能：main 模块 main。
// cmd/iso-debt-gate/main.go 逐仓借贷比闸守护进程入口。
// 启动三类 Worker：
// 1. RatioRecomputePump — 周期重算 DebtRatio 快照
// 2. DriftPatrolWorker — 漂移案例巡检
// 3. TransferGateBlockWorker — pending gate 阻断扫描
// 同时暴露 HTTP ops API（端口 ISO_DEBT_GATE_PORT，默认 8095）。
// 环境变量：
// ISO_DEBT_GATE_PORT HTTP 监听端口（默认 8095）
// ISO_DEBT_MAX_RATIO 最大借贷比（默认 0.97）
// ISO_DEBT_LEASE_TTL 租约有效期（默认 30s）
// ISO_DEBT_RECOMPUTE_INT 重算周期（默认 10s）
// ISO_DEBT_PATROL_INT 漂移巡检周期（默认 15s）
// ISO_DEBT_DRIFT_THRESHOLD 漂移阈值（默认 0.001）

// 作者：Rook
// 日期：2021.07.28

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
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/isobook"
	"account-assets.kucoin.net/funding/internal/isodebt"
)

  func main() {
	  if err := run(); err != nil {
		   fmt.Fprintf(os.Stderr, "iso-debt-gate: fatal: %v\n", err)
		   os.Exit(1)
	   }
  }

   func run() error {
	   ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	   defer cancel()
	  cfg := loadConfig()
	  // 构造依赖（内存模式；注入Store 与 SQL Book）
	  store := isodebt.NewMemStore()
	   book := isobook.New()
	  svc := isodebt.NewService(store, book, cfg)
	   // 注册世代钩子（book → svc）
	   book.BindGenerationBumper(newBumperAdapter(svc))
	   // 启动 Worker Group
	   wg := isodebt.NewWorkerGroup(svc, book, cfg)
	  workerErr := make(chan error, 1)
	  go func() {
		  workerErr <- wg.Run(ctx)
	   }()
	  // 启动 HTTP API
	  port := envStr("ISO_DEBT_GATE_PORT", "8095")
	   router := gin.New()
	  router.Use(gin.Recovery())
	   isodebt.RegisterRoutes(&router.RouterGroup,
		  svc)
	   srv := &http.Server{
		   Addr:    ":" + port,
		  Handler: router,
	   }
	   httpErr := make(chan error, 1)
	  go func() {
		   fmt.Fprintf(os.Stderr, "iso-debt-gate: listening on :%s\n", port)
		  if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			   httpErr <- err
		  }
		   close(httpErr)
	   }()
	   // 等待信号或错误
	   select {
	  case <-ctx.Done():
		  shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
		   defer shutCancel()
		   _ = srv.Shutdown(shutCtx)
		  return ctx.Err()
	  case err := <-workerErr:
		  return fmt.Errorf("worker: %w", err)
	   case err := <-httpErr:
		   if err != nil {
			  return fmt.Errorf("http: %w", err)
		  }
		   return nil
	   }
   }

// ─────────────────────────────────────────────
// 配置加载
// ─────────────────────────────────────────────

   func loadConfig() isodebt.Config {
	  cfg := isodebt.DefaultConfig()
	   if v := os.Getenv("ISO_DEBT_MAX_RATIO"); v != "" {
		  if d, err := decimal.NewFromString(v); err == nil && d.IsPositive() {
			  cfg.MaxDebtRatio = d
		   }
	  }
	  cfg.LeaseTTL = parseDuration(os.Getenv("ISO_DEBT_LEASE_TTL"), cfg.LeaseTTL)
	  cfg.RecomputeInterval = parseDuration(os.Getenv("ISO_DEBT_RECOMPUTE_INT"), cfg.RecomputeInterval)
	  cfg.DriftPatrolInterval = parseDuration(os.Getenv("ISO_DEBT_PATROL_INT"), cfg.DriftPatrolInterval)
	  if v := os.Getenv("ISO_DEBT_DRIFT_THRESHOLD"); v != "" {
		  if d, err := decimal.NewFromString(v); err == nil && d.IsPositive() {
			  cfg.DriftThreshold = d
		  }
	   }
	   return cfg
   }

  func parseDuration(s string, def time.Duration) time.Duration {
	  if s == "" {
		  return def
	  }
	   d, err := time.ParseDuration(s)
	  if err != nil || d <= 0 {
		   return def
	   }
	  return d
   }

  func envStr(key, def string) string {
	   if v := os.Getenv(key); v != "" {
		   return v
	  }
	   return def
   }

// ─────────────────────────────────────────────
// bumperAdapter 将 isodebt.Service 适配为 isobook.GenerationBumper
// ─────────────────────────────────────────────

type bumperAdapter struct {
	svc *isodebt.Service
}

   func newBumperAdapter(svc *isodebt.Service) *bumperAdapter {
	   return &bumperAdapter{svc: svc}
   }

   func (a *bumperAdapter) BumpCredit(ctx context.Context, userID, symbol, sourceTxnID string) error {
	   _, err := a.svc.CreditAndBump(ctx, userID, symbol, sourceTxnID)
	  return err
   }

  func (a *bumperAdapter) BumpDebit(ctx context.Context, userID, symbol, sourceTxnID string) error {
	  _,
		   err := a.svc.DebitAndBump(ctx, userID, symbol, sourceTxnID)
	   return err
  }
