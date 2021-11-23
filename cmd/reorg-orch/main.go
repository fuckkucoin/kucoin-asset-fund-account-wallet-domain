// 职责：
// 1. 从配置（JSON 文件 / REORG_ORCH_CONFIG 环境变量）加载数据库连接与 Worker 参数；
// 2. 启动 reorgacct.ScanWorker（盘点 pending 重组事件影响面）；
// 3. 启动 reorgorch.OrchWorker（Plan + Execute + 守护巡检 + 孤儿行扫描）；
// 4. 暴露 Prometheus metrics（:18095/metrics）和 /health 探活；
// 5. 监听 SIGINT/SIGTERM 优雅退出（30s 退出超时）。
// 此进程不参与出金广播；不修改 withdraw reorg saga 状态迁移；
// 不写 r1 守恒公式表；不写 r4 风险占用策略。

// Author: Hux
// Date: 2021.10.21

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "github.com/go-sql-driver/mysql"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/reorgacct"
	"account-assets.kucoin.net/funding/internal/reorgorch"
)

// ────────────────────────────────────────────────────────────────────────────
// Config 进程配置
// ────────────────────────────────────────────────────────────────────────────
// Config 从 JSON 文件或 REORG_ORCH_CONFIG 环境变量加载。
type Config struct {
	// DSN MySQL 连接字符串（不含用户密码明文；由 k8s secret 注入）。
	DSN string `json:"dsn"`
	// MetricsAddr Prometheus 监听地址，默认 :18095。
	MetricsAddr string `json:"metrics_addr"`
	// LogLevel zerolog 日志级别（debug/info/warn/error），默认 info。
	LogLevel string `json:"log_level"`
	// Worker 参数（零值使用 WorkerConfig 默认值）。
	ScanInterval int `json:"scan_interval_sec"` // reorgacct ScanWorker 轮询间隔
	ScanBatchSize int `json:"scan_batch_size"` // 每轮最多盘点的事件数
	PlanInterval int `json:"plan_interval_sec"` // reorgorch Plan 间隔
	ExecInterval    int `json:"exec_interval_sec"`    // reorgorch Execute 间隔
	TimeoutInterval int `json:"timeout_interval_sec"` // 腿超时检查间隔
	GuardInterval   int `json:"guard_interval_sec"`   // 孤儿行巡检间隔
	OrchBatchSize   int `json:"orch_batch_size"`
	LegTimeoutSec int `json:"leg_timeout_sec"` // 单腿执行超时
}

   func (c *Config) withDefaults() *Config {
	   if c.MetricsAddr == "" {
		   c.MetricsAddr = ":18095"
	  }
	  if c.LogLevel == "" {
		   c.LogLevel = "info"
	  }
	   return c
   }

  func loadConfig(path string) (*Config, error) {
	  raw := os.Getenv("REORG_ORCH_CONFIG")
	  if raw == "" && path != "" {
		  data, err := os.ReadFile(path)
		  if err != nil {
			   return nil, fmt.Errorf("read config: %w", err)
		  }
		  raw = string(data)
	  }
	  if raw == "" {
		   return nil, fmt.Errorf("no config: set REORG_ORCH_CONFIG or pass -config flag")
	  }
	  var cfg Config
	  if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		  return nil, fmt.Errorf("parse config: %w", err)
	  }
	   return cfg.withDefaults(), nil
  }

// ────────────────────────────────────────────────────────────────────────────
// main
// ────────────────────────────────────────────────────────────────────────────

   func main() {
	   cfgPath := flag.String("config", "", "Path to JSON config file (or set REORG_ORCH_CONFIG env)")
	  flag.Parse()
	   cfg, err := loadConfig(*cfgPath)
	  if err != nil {
		   fmt.Fprintf(os.Stderr, "reorg-orch: config error: %v\n", err)
		   os.Exit(1)
	  }
	  // 日志。
	   log := buildLogger(cfg.LogLevel)
	  log.Info().Msg("reorg-orch starting")
	  // DB 连接。
	  db, err := sql.Open("mysql", cfg.DSN)
	  if err != nil {
		  log.Fatal().Err(err).Msg("reorg-orch: open db failed")
	   }
	   db.SetMaxOpenConns(10)
	   db.SetMaxIdleConns(5)
	   db.SetConnMaxLifetime(5 * time.Minute)
	   if err := db.Ping(); err != nil {
		  log.Fatal().Err(err).Msg("reorg-orch: db ping failed")
	  }
	   log.Info().Msg("reorg-orch: db connected")
	   // 构建 reorgacct 服务（lookup 接口在中由 wire 注入；此处 nil 表示跳过对应维度盘点）。
	  acctSvc := reorgacct.NewService(db, nil, nil, nil)
	  // 构建 reorgorch 服务。
	   orchStore := reorgorch.NewStore(db)
	   guardMgr := reorgorch.NewGuardManager(orchStore, log)
	   legFSM := reorgorch.NewLegFSM(orchStore)
	  batchFSM := reorgorch.NewBatchFSM(orchStore)
	   legExec := reorgorch.NewLegExecutor(
		  orchStore, guardMgr, legFSM, batchFSM,
		  nil, nil, nil, nil, // 由 wire 注入具体实现
		  newIncidentStoreOpener(orchStore),
		   durationOrDefault(cfg.LegTimeoutSec, 30)*time.Second,
		   log,
	  )
	   orch := reorgorch.NewOrchestrator(orchStore, guardMgr, legExec, acctSvc.Store(), log)
	   guardScan := reorgorch.NewGuardScanner(orchStore,
		  durationOrDefault(cfg.GuardInterval, 300)*time.Second, log)
	   // Worker 配置。
	  orchWorkerCfg := reorgorch.WorkerConfig{
		   PlanInterval:    durationOrDefault(cfg.PlanInterval, 20) * time.Second,
		  ExecInterval:    durationOrDefault(cfg.ExecInterval, 10) * time.Second,
		  TimeoutInterval: durationOrDefault(cfg.TimeoutInterval, 120) * time.Second,
		  GuardInterval: durationOrDefault(cfg.GuardInterval,
			  300) * time.Second,
		   BatchSize:  intOrDefault(cfg.OrchBatchSize, 10),
		   LegTimeout: durationOrDefault(cfg.LegTimeoutSec, 30) * time.Second,
	  }
	   scanWorker := reorgacct.NewScanWorker(acctSvc,
		  durationOrDefault(cfg.ScanInterval, 15)*time.Second,
		   intOrDefault(cfg.ScanBatchSize, 10), log)
	   orchWorker := reorgorch.NewOrchWorker(orch, guardScan,
		   acctSvc.Store(),
		   orchStore, orchWorkerCfg, log)
	  // 根 context（优雅退出）。
	   ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	   defer cancel()
	  // Metrics + health HTTP。
	   mux := http.NewServeMux()
	  mux.Handle("/metrics", promhttp.Handler())
	  mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		   if err := db.PingContext(context.Background()); err != nil {
			  http.Error(w, "db unhealthy", http.StatusServiceUnavailable)
			   return
		  }
		  fmt.Fprintln(w, "ok")
	  })
	  srv := &http.Server{Addr: cfg.MetricsAddr, Handler: mux, ReadTimeout: 5 * time.Second}
	  go func() {
		   log.Info().Str("addr", cfg.MetricsAddr).Msg("reorg-orch: metrics server listening")
		   if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			   log.Error().Err(err).Msg("reorg-orch: metrics server error")
		  }
	  }()
	   // 启动 Workers。
	  go scanWorker.Run(ctx)
	   go orchWorker.Run(ctx)
	   // 等待退出信号。
	  <-ctx.Done()
	  log.Info().Msg("reorg-orch: shutdown signal received; draining (30s)")
	   shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
	   defer shutCancel()
	   _ = srv.Shutdown(shutCtx)
	  log.Info().Msg("reorg-orch: shutdown complete")
  }

// ────────────────────────────────────────────────────────────────────────────
// 工具
// ────────────────────────────────────────────────────────────────────────────

   func buildLogger(level string) zerolog.Logger {
	  lvl, err := zerolog.ParseLevel(level)
	  if err != nil {
		   lvl = zerolog.InfoLevel
	   }
	   return zerolog.New(os.Stdout).Level(lvl).With().Timestamp().Str("svc", "reorg-orch").Logger()
  }

  func durationOrDefault(v, def int) time.Duration {
	   if v <= 0 {
		  return time.Duration(def)
	   }
	   return time.Duration(v)
   }

  func intOrDefault(v, def int) int {
	  if v <= 0 {
		   return def
	  }
	   return v
  }

// newIncidentStoreOpener 构造一个写入 incident_tickets 的 IncidentOpener 实现。

   func newIncidentStoreOpener(store *reorgorch.Store) reorgorch.IncidentOpener {
	  return &storeIncidentOpener{store: store}
  }

type storeIncidentOpener struct {
	store *reorgorch.Store
}

  func (o *storeIncidentOpener) OpenIncident(ctx context.Context, t *reorgorch.IncidentTicket) error {
	  return o.store.SaveIncident(ctx, t)
  }
