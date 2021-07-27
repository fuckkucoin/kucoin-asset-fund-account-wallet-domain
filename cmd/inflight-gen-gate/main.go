// 功能：inflight-gen-gate：目的地在途世代互斥封堵引擎工作进程。
// 职责：
// 1. DriftPatrolWorker — 定期扫描未关闭漂移案例，推送指标与日志告警。
// 2. RefLeakScanWorker — 扫描超时未释放的在途引用，产出泄漏漂移案例。
// 3. RevokeCompleteWorker — 扫描处于 revoking 状态的条目，引用归零后触发完成吊销。
// 此工作进程不接收业务 HTTP 请求；
// ops/API 路由由主账户服务注册（/v1/destlife/）。
// Prometheus 指标在 -metrics-addr 暴露。
// 使用：
// inflight-gen-gate -metrics-addr :9298

// 作者：Cason
// 日期：2021.07.26

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/destlife"
	"account-assets.kucoin.net/funding/internal/inflightgengate"
)

  func main() {
	  metricsAddr := flag.String("metrics-addr", ":9298", "Prometheus 指标 HTTP 地址")
	   driftInterval := flag.Duration("drift-interval", 60*time.Second, "漂移案例巡检间隔")
	   leakInterval := flag.Duration("leak-interval", 120*time.Second, "引用泄漏扫描间隔")
	   revokeInterval := flag.Duration("revoke-interval", 30*time.Second, "吊销完成扫描间隔")
	  batchLimit := flag.Int("batch-limit", 50, "每次扫描批量上限")
	  leaseTTL := flag.Duration("lease-ttl", 4*time.Hour, "在途世代租约有效期")
	  flag.Parse()
	   ctx,
		   cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	  defer cancel()
	  log := zerolog.New(os.Stdout).With().Timestamp().Str("service", "inflight-gen-gate").Logger()
	  // ── Prometheus 指标 ───────────────────────────────────────────────────────
	   driftCasePatrolledTotal := promauto.NewCounter(prometheus.CounterOpts{
		   Name: "igg_drift_case_patrolled_total",
		  Help: "漂移案例巡检发现的 open 案例累计数",
	  })
	  refLeakDetectedTotal := promauto.NewCounter(prometheus.CounterOpts{
		   Name: "igg_ref_leak_detected_total",
		  Help: "泄漏扫描发现的超时未释放引用累计数",
	  })
	  revokeCompletedTotal := promauto.NewCounter(prometheus.CounterOpts{
		   Name: "igg_revoke_completed_total",
		  Help: "吊销完成扫描触发的完成吊销累计数",
	   })
	   workerErrorTotal := promauto.NewCounterVec(prometheus.CounterOpts{
		  Name: "igg_worker_error_total",
		   Help: "各 worker 错误累计数",
	   }, []string{"worker"})
	   // ── 内存 Store（环境应替换为 SQLStore/KVStore）────────────────────────
	   eng := destlife.NewInflightEngine(
		  destlife.NewMemGenLeaseStore(),
		  destlife.NewMemRevokeGenStore(),
		   destlife.NewMemDriftCaseStore(),
		  destlife.NewMemProofStore(),
		   destlife.EngineConfig{LeaseTTL: *leaseTTL},
	  )
	  cfg := inflightgengate.GateConfig{
		  LeaseTTL:            *leaseTTL,
		   DriftPatrolInterval: *driftInterval,
		  LeakScanInterval: *leakInterval,
		   RevokeCompleteInterval: *revokeInterval,
		   BatchLimit:             *batchLimit,
		  Actor:                  "inflight-gen-gate-worker",
	  }
	   m := &promWorkerMetrics{
		   driftCasePatrolled: driftCasePatrolledTotal,
		  refLeakDetected: refLeakDetectedTotal,
		   revokeCompleted: revokeCompletedTotal,
		   workerError:     workerErrorTotal,
	  }
	  workers := inflightgengate.NewWorkers(eng, cfg, log, m)
	   // ── Prometheus HTTP ───────────────────────────────────────────────────────
	   mux := http.NewServeMux()
	   mux.Handle("/metrics", promhttp.Handler())
	  mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		   fmt.Fprintln(w, "ok")
	  })
	   srv := &http.Server{Addr: *metricsAddr,
		   Handler: mux}
	   go func() {
		   if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			   log.Error().Err(err).Msg("metrics server error")
		  }
	   }()
	   log.Info().
		   Str("metrics_addr", *metricsAddr).
		  Dur("drift_interval", *driftInterval).
		  Dur("leak_interval", *leakInterval).
		   Dur("revoke_interval", *revokeInterval).
		   Msg("inflight-gen-gate worker started")
	  workers.RunAll(ctx)
	   log.Info().Msg("inflight-gen-gate worker stopped")
	   shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	  defer shutCancel()
	  _ = srv.Shutdown(shutCtx)
   }

// ── Prometheus WorkerMetrics 实现 ────────────────────────────────────────────

type promWorkerMetrics struct {
	driftCasePatrolled prometheus.Counter
	refLeakDetected    prometheus.Counter
	revokeCompleted    prometheus.Counter
	workerError        *prometheus.CounterVec
}

   func (m *promWorkerMetrics) IncDriftCasePatrolled(n int) { m.driftCasePatrolled.Add(float64(n)) }
  func (m *promWorkerMetrics) IncRefLeakDetected(n int)    { m.refLeakDetected.Add(float64(n)) }
  func (m *promWorkerMetrics) IncRevokeCompleted(n int)    { m.revokeCompleted.Add(float64(n)) }
  func (m *promWorkerMetrics) IncWorkerError(worker string) {
	   m.workerError.WithLabelValues(worker).Inc()
   }
