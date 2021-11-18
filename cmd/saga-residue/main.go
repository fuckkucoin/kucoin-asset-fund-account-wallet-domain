// @FileName: main.go
// @Summary: 启动模式：
// 启动模式：
// 直接运行： ./saga-residue [flags]
// --mode scan 仅运行 ResidueScanner（单次或持续）
// --mode execute 仅运行 CloseoutExecutor
// --mode workers 启动所有四类 worker（默认）
// 依赖注入顺序：
// 1. 读取配置（环境变量 / YAML）；
// 2. 连接 MySQL / Postgres；
// 3. 初始化 safesaga.Service 和 safesaga.Compensator（只读消费）；
// 4. 初始化 ledger.Service（只读 GetByIdem 路径）；
// 5. 构造 sagaresidue.Service（Detector + Orchestrator + MutexManager + Prover）；
// 6. 启动 HTTP 指标端点（:9190/metrics）；
// 7. 启动 Workers。
// @Author: Jett
// @Created: 2021.10.27

package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
	"os/signal"
	"syscall"
	"time"
	_ "github.com/go-sql-driver/mysql"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sagaresidue"
	"account-assets.kucoin.net/funding/internal/safesaga"
)

var (
	flagMode    = flag.String("mode", "workers", "运行模式：scan|execute|workers")
	flagDSN     = flag.String("dsn", "", "MySQL DSN（默认 ACCOUNT_ASSETS_DSN 或 mysql-0.kucoin.net）")
	flagMetrics = flag.String("metrics", ":9190", "Prometheus 指标监听地址")
	flagWorkerID = flag.String("worker-id", envOrDefault("SR_WORKER_ID", "saga-residue-worker"), "Worker 标识（hostname:pid）")
)

   func main() {
	  flag.Parse()
	  ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	   defer cancel()
	  // ── 1. 连接数据库 ─────────────────────────────────────────────────────────
	  dsn := strings.TrimSpace(*flagDSN)
	  if dsn == "" {
		  dsn = persist.FundingMySQLDSN()
	  }
	  db, err := sql.Open("mysql", dsn)
	   if err != nil {
		  fatalf("open db: %v", err)
	  }
	   defer db.Close()
	  db.SetMaxOpenConns(20)
	  db.SetMaxIdleConns(5)
	  db.SetConnMaxLifetime(5 * time.Minute)
	  if err := db.PingContext(ctx); err != nil {
		   fatalf("ping db: %v", err)
	   }
	   sqlDB := persist.OpenSQL(db)
	   store := sagaresidue.NewSQLStore(sqlDB)
	   // ── 2. 初始化 safesaga 只读依赖 ──────────────────────────────────────────
	  sagaSvc := safesaga.New()
	   sagaComp := safesaga.NewCompensator()
	   partialReader := sagaresidue.NewPartialReaderAdapter(sagaComp)
	  runReader := sagaresidue.NewRunReaderAdapter(sagaSvc)
	   // ── 3. 构造 LedgerReader（只读）───────────────────────────────────────────
	   // 此处为占位实现；环境须注入 *ledger.Service + SQLStore。
	   var ledgerReader sagaresidue.LedgerReader
	  ledgerReader = sagaresidue.NewNopLedgerReader()
	   // ── 4. 构造 Detector ─────────────────────────────────────────────────────
	   detCfg := sagaresidue.DefaultDetectorConfig()
	   detector := sagaresidue.NewDetector(detCfg, ledgerReader, partialReader, runReader)
	   // ── 5. 构造 MutexManager ─────────────────────────────────────────────────
	   mutexMgr := sagaresidue.NewMutexManager(store, sagaresidue.DefaultMutexTTL)
	   // ── 6. 构造 CloseoutOrchestrator ─────────────────────────────────────────
	  orch := sagaresidue.NewCloseoutOrchestrator(
		   sagaresidue.NewNopLedgerWriter(),
		  store, // SQLStore 同时实现 IdemBlockStore
		   mutexMgr,
		  store,
	  )
	  // ── 7. 构造 Prover ────────────────────────────────────────────────────────
	  prover := sagaresidue.NewProver(store)
	  // ── 8. 构造 Service ───────────────────────────────────────────────────────
	  svc := sagaresidue.NewService(detector,
		   orch, mutexMgr, prover, store)
	  // ── 9. 启动 Prometheus 指标端点 ───────────────────────────────────────────
	  go func() {
		  mux := http.NewServeMux()
		   mux.Handle("/metrics", promhttp.Handler())
		   srv := &http.Server{Addr: *flagMetrics, Handler: mux, ReadTimeout: 5 * time.Second}
		   if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			  fmt.Fprintf(os.Stderr, "metrics server: %v\n", err)
		   }
	  }()
	  // ── 10. 启动 Workers ──────────────────────────────────────────────────────
	   cfg := sagaresidue.DefaultWorkerConfig()
	   cfg.WorkerID = *flagWorkerID
	  var sagaRunLister sagaresidue.SagaRunLister
	   sagaRunLister = sagaresidue.NewNopSagaRunLister()
	   workers := sagaresidue.NewWorkers(svc, cfg, &stderrLogger{}, sagaRunLister)
	   switch *flagMode {
	   case "scan":
		  fmt.Fprintln(os.Stdout, "saga-residue: scan mode")
		  go workers.StartAll(ctx)
	   case "execute":
		  fmt.Fprintln(os.Stdout, "saga-residue: execute mode")
		   go workers.StartAll(ctx)
	  default:
		  fmt.Fprintln(os.Stdout, "saga-residue: all workers mode")
		  workers.StartAll(ctx)
	  }
	  <-ctx.Done()
	  fmt.Fprintln(os.Stdout, "saga-residue: shutting down")
  }

// ── 辅助函数与最小实现 ────────────────────────────────────────────────────────

   func envOrDefault(key, def string) string {
	   if v := os.Getenv(key); v != "" {
		   return v
	  }
	  return def
  }

   func fatalf(format string, args ...any) {
	   fmt.Fprintf(os.Stderr, "saga-residue fatal: "+format+"\n", args...)
	  os.Exit(1)
  }

// stderrLogger 将日志输出到 stderr（环境替换为 zerolog）。

type stderrLogger struct{}

  func (l *stderrLogger) Info(msg string, fields map[string]any) {
	   fmt.Fprintf(os.Stdout, "[INFO] %s %v\n", msg, fields)
  }

   func (l *stderrLogger) Error(msg string, err error, fields map[string]any) {
	  fmt.Fprintf(os.Stderr, "[ERROR] %s: %v %v\n", msg, err, fields)
   }
