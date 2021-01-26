// @FileName: main.go
// @Summary: acct-life-gate：资金账户生命周期冻结关闭与并发 hop 竞态封堵工作进程。
// 职责：
// 1. LeaseExpiryWorker — 定期扫描超期 open 租约，标记为 expired。
// 2. MoveBindTimeout — 定期扫描超时 pending MoveBind，标记为 timed_out。
// 3. HalfOpenPatrol — 定期巡检 open/proven LifeDriftCase，触发告警。
// 此工作进程不接收业务 HTTP 请求；
// ops/API 路由由主账户服务注册（/acct-life-gate/）。
// Prometheus 指标在 -metrics-addr 暴露。
// @Author: Jett
// @Created: 2021.01.20

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
	"account-assets.kucoin.net/funding/internal/acctlifegate"
)

  func main() {
	  metricsAddr := flag.String("metrics-addr", ":9296", "Prometheus 指标 HTTP 地址")
	  flag.Parse()
	  ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	  defer cancel()
	  // ── Prometheus 指标 ───────────────────────────────────────────────────────
	   leaseExpiredTotal := promauto.NewCounter(prometheus.CounterOpts{
		   Name: "alg_lease_expired_total",
		  Help: "已标记为 expired 的生命周期世代租约总数",
	  })
	   bindTimedOutTotal := promauto.NewCounter(prometheus.CounterOpts{
		   Name: "alg_bind_timed_out_total",
		   Help: "已标记为 timed_out 的在途 Move 绑定总数",
	  })
	   driftDetectedTotal := promauto.NewCounter(prometheus.CounterOpts{
		  Name: "alg_drift_detected_total",
		   Help: "巡检发现的未关闭漂移案件数（累计）",
	  })
	   driftPatrolledTotal := promauto.NewCounter(prometheus.CounterOpts{
		  Name: "alg_drift_patrolled_total",
		   Help: "已巡检轮次总数",
	  })
	   // ── 内存 Store（环境应替换为 SQLStore）────────────────────────────────
	  store := acctlifegate.NewMemStore()
	   svc := acctlifegate.New(store)
	   metrics := &promMetrics{
		   leaseExpired: leaseExpiredTotal,
		  bindTimedOut: bindTimedOutTotal,
		   driftDetected:  driftDetectedTotal,
		  driftPatrolled: driftPatrolledTotal,
	  }
	   worker := acctlifegate.NewWorker(svc, store, metrics)
	   // ── 日志注入 ─────────────────────────────────────────────────────────────
	  acctlifegate.SetWorkerLogger(
		  func(tag string, err error) {
			  fmt.Fprintf(os.Stderr, "[acct-life-gate][%s] ERROR: %v\n", tag, err)
		   },
		  func(tag, msg string) {
			  fmt.Printf("[acct-life-gate][%s] %s\n", tag, msg)
		  },
	  )
	   // ── Prometheus HTTP ───────────────────────────────────────────────────────
	   mux := http.NewServeMux()
	   mux.Handle("/metrics", promhttp.Handler())
	  mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		   fmt.Fprintln(w, "ok")
	   })
	   srv := &http.Server{Addr: *metricsAddr, Handler: mux}
	   go func() {
		   if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			  fmt.Fprintf(os.Stderr, "metrics server error: %v\n", err)
		   }
	   }()
	  fmt.Printf("acct-life-gate worker started (metrics=%s)\n", *metricsAddr)
	  worker.RunAll(ctx)
	   fmt.Println("acct-life-gate worker stopped")
	  shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	   defer shutCancel()
	   _ = srv.Shutdown(shutCtx)
   }

// ── Prometheus WorkerMetrics 实现 ────────────────────────────────────────────

type promMetrics struct {
	leaseExpired  prometheus.Counter
	bindTimedOut  prometheus.Counter
	driftDetected prometheus.Counter
	driftPatrolled prometheus.Counter
}

  func (m *promMetrics) IncLeaseExpired(n int) { m.leaseExpired.Add(float64(n)) }

   func (m *promMetrics) IncBindTimedOut(n int)   { m.bindTimedOut.Add(float64(n)) }
   func (m *promMetrics) IncDriftDetected(n int)  { m.driftDetected.Add(float64(n)) }
  func (m *promMetrics) IncDriftPatrolled(n int) { m.driftPatrolled.Add(float64(n)) }
