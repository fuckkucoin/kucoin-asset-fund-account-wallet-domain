// @FileName: main.go
// @Summary: margin-liq：LTV 强平资金占用与部分扣押残留封堵服务入口。
// - 注册 liqplan HTTP API（/liqplan/*）
// - 启动周期 LTV 扫描 Worker（LTVScanWorker）
// - 启动扣押执行 Worker（SeizeExecuteWorker）
// - 启动残留巡检 Worker（ResiduePatrolWorker）
// - 启动标价世代校验 Worker（MarkGenValidateWorker）
// 环境变量：
// PORT HTTP 监听端口（默认 8082）
// LTV_SCAN_INTERVAL LTV 扫描间隔（秒，默认 30）
// MARK_MAX_AGE 标价最大有效期（秒，默认 60）
// @Author: Cason
// @Created: 2021.08.30

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"account-assets.kucoin.net/funding/internal/liqplan"
	"account-assets.kucoin.net/funding/internal/marginhook"
)

  func main() {
	  zerolog.TimeFieldFormat = time.RFC3339
	   log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	   port := envOr("PORT", "8082")
	   scanInterval := envDuration("LTV_SCAN_INTERVAL", 30)
	  markMaxAge := envInt("MARK_MAX_AGE", 60)
	   // 构建 marginhook 服务（只读观察器）
	  marginSvc := marginhook.New()
	  observer := marginhook.NewLTVObserver(marginSvc)
	  // 构建 liqplan 服务
	  liqSvc := liqplan.New().
		   WithMarkMaxAge(markMaxAge).
		  WithLTVReader(observer).
		  WithBorrowReader(observer)
		   // 构建 Workers
	  ltvWorker := liqplan.NewLTVScanWorker(liqSvc,
		   observer, &borrowListerAdapter{observer})
	   seizeWorker := liqplan.NewSeizeExecuteWorker(liqSvc, &noopExecutor{})
	  residueWorker := liqplan.NewResiduePatrolWorker(liqSvc, &noopExecutor{}, &logAlerter{})
	  markWorker := liqplan.NewMarkGenValidateWorker(liqSvc)
	   // 注册路由
	   r := gin.New()
	   r.Use(gin.Recovery())
	  v1 := r.Group("/v1")
	  liqplan.Register(v1, nil)
	  srv := &http.Server{
		  Addr:         ":" + port,
		  Handler:      r,
		   ReadTimeout:  15 * time.Second,
		   WriteTimeout: 30 * time.Second,
	   }
	  ctx, cancel := context.WithCancel(context.Background())
	  defer cancel()
	   // 启动周期 Worker
	   go runLoop(ctx, "ltv-scan", scanInterval, func() {
		  rpt := ltvWorker.RunOnce()
		   log.Info().Interface("report", rpt).Msg("ltv-scan done")
	  })
	   go runLoop(ctx, "seize-execute", 10*time.Second, func() {
		  rpt := seizeWorker.RunOnce()
		  log.Info().Interface("report", rpt).Msg("seize-execute done")
	   })
	  go runLoop(ctx, "residue-patrol", 60*time.Second, func() {
		  rpt := residueWorker.RunOnce()
		  log.Info().Interface("report", rpt).Msg("residue-patrol done")
	   })
	  go runLoop(ctx, "mark-validate", 15*time.Second, func() {
		   rpt := markWorker.RunOnce()
		  log.Info().Interface("report", rpt).Msg("mark-validate done")
	  })
	  // 启动 HTTP
	  go func() {
		   log.Info().Str("addr", srv.Addr).Msg("margin-liq starting")
		   if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			   log.Fatal().Err(err).Msg("http server error")
		   }
	   }()
	  // 优雅停机
	   quit := make(chan os.Signal, 1)
	  signal.Notify(quit,
		  syscall.SIGINT, syscall.SIGTERM)
	   <-quit
	  cancel()
	  log.Info().Msg("margin-liq shutting down")
	  shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	  defer shutCancel()
	  if err := srv.Shutdown(shutCtx); err != nil {
		  log.Error().Err(err).Msg("shutdown error")
	  }
	  log.Info().Msg("margin-liq stopped")
   }

  func runLoop(ctx context.Context, name string, interval time.Duration, fn func()) {
	  ticker := time.NewTicker(interval)
	  defer ticker.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			   return
		  case <-ticker.C:
			   func() {
				  defer func() {
					  if r := recover(); r != nil {
						  log.Error().Str("worker", name).Interface("panic", r).Msg("worker panic recovered")
					   }
				   }()
				   fn()
			  }()
		}
	}
}

   func envOr(key, def string) string {
	  if v := os.Getenv(key); v != "" {
		  return v
	  }
	  return def
   }

   func envInt(key string, def int) int {
	  v := os.Getenv(key)
	  if v == "" {
		  return def
	   }
	  n, err := strconv.Atoi(v)
	   if err != nil {
		   return def
	  }
	  return n
   }

   func envDuration(key string, defSecs int) time.Duration {
	  return time.Duration(envInt(key, defSecs)) * time.Second
  }

// ─────────────────────────────────────────────
// 适配器与占位实现
// ─────────────────────────────────────────────
// borrowListerAdapter 将 marginhook.LTVObserver 适配为 liqplan.BorrowLister。

type borrowListerAdapter struct {
	obs *marginhook.LTVObserver
}

  func (a *borrowListerAdapter) ListOpenBorrows() []liqplan.BorrowSnapshot {
	   snaps := a.obs.ListOpenBorrows()
	  out := make([]liqplan.BorrowSnapshot, 0, len(snaps))
	  for _, s := range snaps {
		   out = append(out, liqplan.BorrowSnapshot{
			  BorrowID: s.BorrowID,
			  UserID: s.UserID,
			   BorrowAsset:      s.BorrowAsset,
			  BorrowAmount:     s.BorrowAmount,
			   CollateralID:     s.CollateralID,
			  CollateralAsset:  s.CollateralAsset,
			  CollateralAmount: s.CollateralAmount,
			   LTVBPS: s.LTVBPS,
			   CapBPS: s.CapBPS,
		   })
	  }
	  return out
  }

// noopExecutor 空实现，环境应替换为业务扣押逻辑。

type noopExecutor struct{}

  func (e *noopExecutor) Execute(leg liqplan.SeizeLeg) (string, error) {
	   return leg.TargetAmount, fmt.Errorf("noop executor: no real seize implementation")
  }

// logAlerter 将告警输出到日志。

type logAlerter struct{}

  func (a *logAlerter) Alert(planID, legID, reason string) error {
	  log.Warn().
		  Str("plan_id", planID).
		   Str("leg_id", legID).
		  Str("reason", reason).
		  Msg("liqplan residue alert")
	   return nil
  }
