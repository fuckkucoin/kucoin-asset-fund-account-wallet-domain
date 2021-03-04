// 职责：
// 1. 定期执行老化扫描（AgingScanner.Scan），推进 hold 的 AgingTier。
// 2. 定期执行账本漂移巡检（Book.CheckDrift），发现并记录缺口单。
// 3. SLA 超时不得自动 Resolve Case；Worker 只升级 AgingTier，不操作 Case 状态机。
// 此工作进程不接收 HTTP 业务请求；ops/API 路由由主账户服务提供（/v1/aml/holds/）。
// Prometheus 指标在 -metrics-addr 暴露。

// Author: MilesFord
// Date: 2021.02.02

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
	"account-assets.kucoin.net/funding/internal/amlhold"
)

  func main() {
	   agingInterval := flag.Duration("aging-interval", 5*time.Minute, "老化扫描间隔")
	   driftInterval := flag.Duration("drift-interval", 10*time.Minute, "账本漂移巡检间隔")
	   metricsAddr := flag.String("metrics-addr", ":9292", "Prometheus 指标 HTTP 地址")
	   warnAfter := flag.Int64("warn-after", 86400, "SLA warn 阈值（秒）")
	  forceHoldAfter := flag.Int64("force-hold-after", 129600, "force_hold 阈值（秒）")
	   escalateAfter := flag.Int64("escalate-after", 172800, "escalated 阈值（秒）")
	  compelReviewAfter := flag.Int64("compel-review-after", 259200, "compel_review 阈值（秒）")
	   flag.Parse()
	  ctx,
		  cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	   defer cancel()
	   cfg := amlhold.AgingConfig{
		   WarnAfter:      *warnAfter,
		   ForceHoldAfter: *forceHoldAfter,
		  EscalateAfter:     *escalateAfter,
		  CompelReviewAfter: *compelReviewAfter,
	   }
	  // 服务层（无账本观察者；部署时通过 WithLedger 注入 ledger 适配器）
	  svc := amlhold.NewWithConfig(nil,
		   cfg)
	  // Prometheus 指标
	  scansTotal := promauto.NewCounter(prometheus.CounterOpts{
		   Name: "amlhold_aging_scans_total",
		   Help: "老化扫描执行总次数",
	  })
	  advancedTotal := promauto.NewCounter(prometheus.CounterOpts{
		  Name: "amlhold_aging_advanced_total",
		   Help: "老化升级的 hold 总次数",
	  })
	  scanErrors := promauto.NewCounter(prometheus.CounterOpts{
		  Name: "amlhold_aging_scan_errors_total",
		   Help: "老化扫描错误总次数",
	  })
	  driftGapsTotal := promauto.NewCounter(prometheus.CounterOpts{
		   Name: "amlhold_drift_gaps_total",
		   Help: "账本漂移缺口单累计开启次数",
	  })
	   openHoldsGauge := promauto.NewGauge(prometheus.GaugeOpts{
		   Name: "amlhold_open_holds",
		  Help: "当前活跃（非终态）hold 数量",
	  })
	  workerCfg := amlhold.WorkerConfig{
		   AgingScanInterval: *agingInterval,
		  DriftCheckInterval: *driftInterval,
	  }
	   log := &stderrLogger{}
	   worker := amlhold.NewWorker(svc,
		   workerCfg, log).
		   OnScan(func(res amlhold.ScanResult) {
			  scansTotal.Add(1)
			  advancedTotal.Add(float64(res.Advanced))
			  scanErrors.Add(float64(len(res.Errors)))
			   stats := svc.Stats()
			  openHoldsGauge.Set(float64(stats.Open + stats.Aging + stats.Escalated + stats.CompelReview))
		   }).
		  OnDrift(func(gaps []amlhold.GapTicket) {
			  driftGapsTotal.Add(float64(len(gaps)))
		  })
	   // 启动 Prometheus metrics HTTP 服务
	   go func() {
		   mux := http.NewServeMux()
		   mux.Handle("/metrics", promhttp.Handler())
		   mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			   w.WriteHeader(http.StatusOK)
			  _, _ = w.Write([]byte("ok"))
		   })
		  srv := &http.Server{Addr: *metricsAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
		   log.Infof("aml-hold-aging metrics: %s", *metricsAddr)
		   if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			  log.Errorf("metrics server: %v", err)
		  }
	  }()
	  log.Infof("aml-hold-aging started: aging_interval=%s drift_interval=%s", *agingInterval, *driftInterval)
	   if err := worker.Run(ctx); err != nil && err != context.Canceled {
		   log.Errorf("worker exited: %v", err)
		  os.Exit(1)
	  }
   }

// stderrLogger 简单 stderr 日志输出。
type stderrLogger struct{}

   func (l *stderrLogger) Infof(format string, args ...any) {
	  fmt.Fprintf(os.Stderr, "[INFO]  "+format+"\n", args...)
  }

  func (l *stderrLogger) Errorf(format string, args ...any) {
	   fmt.Fprintf(os.Stderr, "[ERROR] "+format+"\n", args...)
  }
