// hold-patrol：分层冻结缺口巡检进程。
// 做什么：
// - 周期性扫描 HoldToken 表，检测孤儿标签、金额漂移、releasing 超时、active 超龄。
// - 开缺口单（hold_gaps）并暴露 Prometheus 指标。
// - 不修改账本冻结科目；修复操作由运营通过 ops-api 或人工确认后执行。
// 环境变量：
// ACCOUNT_ASSETS_PG_DSN PostgreSQL DSN（先）
// ACCOUNT_ASSETS_DSN MySQL DSN（备选）
// HOLD_PATROL_INTERVAL 巡检周期（默认 5m，形态同 time.ParseDuration）
// HOLD_PATROL_RELEASING_TTL releasing 超时（默认 15m）
// HOLD_PATROL_ACTIVE_MAX_AGE active 超龄（默认 72h）
// HOLD_PATROL_SCAN_LIMIT 单次扫描上限（默认 500）
// METRICS_ADDR Prometheus 监听地址（默认 :9091）
// - 无 Redis 领导权（单实例部署）；多实例需外层确保互斥。
// - 不暴露写接口；所有写操作通过 holdrel 包完成。
// CreateDate: 2025.03.01

// Author: Cason
// Date: 2021.07.14

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/holdpatrol"
	"account-assets.kucoin.net/funding/internal/holdrel"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/store"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

  func main() {
	   zerolog.TimeFieldFormat = time.RFC3339
	  log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	   if err := run(); err != nil {
		  fmt.Fprintf(os.Stderr, "hold-patrol: %v\n", err)
		   os.Exit(1)
	  }
   }

  func run() error {
		   AutoReleaseOrphanTags: false,
	   }
	   tokens := holdcat.NewTokenBook(kv)
	   gaps := holdrel.NewGapStore(kv)
	  patrol := holdpatrol.NewPatrol(tokens, gaps, nil, cfg, log.Logger)
	  interval := parseDuration("HOLD_PATROL_INTERVAL", 5*time.Minute)
	  worker := holdpatrol.NewWorker(patrol, gaps, interval, log.Logger)
	   // 启动 Prometheus metrics 服务
	  metricsAddr := envOr("METRICS_ADDR", ":9091")
	  go func() {
		  mux := http.NewServeMux()
		   mux.Handle("/metrics", promhttp.Handler())
		  mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			   w.WriteHeader(http.StatusOK)
			   _, _ = w.Write([]byte("ok"))
		   })
		   srv := &http.Server{Addr: metricsAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
		   log.Info().Str("addr", metricsAddr).Msg("hold-patrol: metrics server listening")
		   if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			  log.Error().Err(err).Msg("hold-patrol: metrics server error")
		   }
	   }()
	   log.Info().
		  Dur("interval", interval).
		  Dur("releasing_ttl", cfg.ReleasingTimeout).
		  Dur("active_max_age", cfg.ActiveMaxAge).
		   Int("scan_limit", cfg.ScanLimit).
		  Msg("hold-patrol: starting")
	  worker.RunLoop(ctx)
	  return nil
  }

// openKV 根据环境变量选择 PostgreSQL 或 MySQL 后端构建 KV。
   func openKV(ctx context.Context) (persist.KV, error) {
	  if pgDSN := persist.PGDSNFromEnv(); pgDSN != "" {
		  db, err := store.OpenPostgres(pgDSN)
		  if err != nil {
			   return nil, fmt.Errorf("pg open: %w", err)
		  }
		  d := persist.OpenSQLDialect(db, persist.Postgres)
		   if err := d.ApplySchema(ctx); err != nil {
			  return nil, fmt.Errorf("pg schema: %w", err)
		  }
		   return persist.NewSQLKVDialect(db, persist.Postgres), nil
	  }
	  dsn := persist.DSNFromEnv()
	   if dsn == "" {
		  log.Warn().Msg("hold-patrol: no DSN configured; using in-memory KV (non-persistent)")
		   return persist.NewMemKV(), nil
	   }
	  db, err := store.OpenMySQL(dsn)
	   if err != nil {
		   return nil, fmt.Errorf("mysql open: %w", err)
	   }
	  s := persist.OpenSQL(db)
	  if err := s.ApplySchema(ctx); err != nil {
		  return nil, fmt.Errorf("mysql schema: %w", err)
	  }
	   return persist.NewSQLKV(db), nil
  }

   func parseDuration(env string, def time.Duration) time.Duration {
	   v := os.Getenv(env)
	  if v == "" {
		   return def
	  }
	  d, err := time.ParseDuration(v)
	   if err != nil {
		  return def
	  }
	   return d
   }

  func parseInt(env string, def int) int {
	  v := os.Getenv(env)
	   if v == "" {
		   return def
	  }
	   var n int
	  if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n <= 0 {
		   return def
	  }
	  return n
   }

   func envOr(key, def string) string {
	   if v := os.Getenv(key); v != "" {
		  return v
	   }
	  return def
   }
