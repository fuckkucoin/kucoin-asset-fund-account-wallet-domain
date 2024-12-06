// 功能：main 模块 main。
// fund-guard-lease：守恒闸快照世代租约与部分批次失衡封堵工作进程。
// 职责：
// 1. 定期扫描并标记已超期的 open 世代租约为 expired（LeaseExpiryWorker）；
// 2. 定期巡检 open 状态的部分批次金额失衡记录，告警并触发升级（ImbalanceScanWorker）；
// 3. 定期巡检 fail 状态的守恒证明，超过四眼解除时限则升级为 halted，
// 触发全局 SetHalt 保护（ProofEscalateWorker）。
// 此工作进程不接收 HTTP 业务请求；ops API 路由由主账户服务提供（/v1/guardlease/...）。
// Prometheus 指标在 -metrics-addr 暴露。

// 作者：Cason
// 日期：2021.06.02

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
	"account-assets.kucoin.net/funding/internal/guardlease"
)

   func main() {
	  leaseExpiryInterval := flag.Duration("lease-expiry-interval", 15*time.Second, "租约过期扫描间隔")
	  imbalanceScanInterval := flag.Duration("imbalance-scan-interval", 30*time.Second, "失衡巡检间隔")
	   proofEscalateInterval := flag.Duration("proof-escalate-interval", 60*time.Second, "守恒证明升级巡检间隔")
	  metricsAddr := flag.String("metrics-addr", ":9295", "Prometheus 指标 HTTP 地址")
	  leaseScanLimit := flag.Int("lease-scan-limit", 50, "单次租约扫描上限")
	   imbalanceScanLimit := flag.Int("imbalance-scan-limit", 50, "单次失衡扫描上限")
	  proofScanLimit := flag.Int("proof-scan-limit", 50, "单次证明扫描上限")
	  flag.Parse()
	   ctx,
		   cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	   defer cancel()
	   // 使用内存仓储（部署时替换为 guardlease.NewSQLStore(db)）
	  store := guardlease.NewMemStore()
	   svc := guardlease.New(store)
	  // Prometheus 指标
	  leasesExpiredTotal := promauto.NewCounter(prometheus.CounterOpts{
		  Name: "guardlease_leases_expired_total",
		   Help: "累计标记为 expired 的世代租约数量",
	  })
	   imbalancesOpenGauge := promauto.NewGauge(prometheus.GaugeOpts{
		   Name: "guardlease_imbalances_open",
		   Help: "当前 open 状态的部分批次金额失衡数量",
	   })
	  proofFailsGauge := promauto.NewGauge(prometheus.GaugeOpts{
		  Name: "guardlease_proof_fails",
		   Help: "当前 fail/halted 状态的守恒证明数量",
	   })
	   workerErrors := promauto.NewCounterVec(prometheus.CounterOpts{
		  Name: "guardlease_worker_errors_total",
		  Help: "Worker 错误总次数（按类型分组）",
	   }, []string{"kind"})
	   haltedGauge := promauto.NewGauge(prometheus.GaugeOpts{
		   Name: "guardlease_halted",
		  Help: "系统是否处于 halt 状态（1=halted, 0=normal）",
	   })
	   workerCfg := guardlease.WorkerConfig{
		   LeaseExpiryInterval:   *leaseExpiryInterval,
		  ImbalanceScanInterval: *imbalanceScanInterval,
		  ProofEscalateInterval: *proofEscalateInterval,
		   ImbalanceScanLimit:    *imbalanceScanLimit,
		  ProofScanLimit:        *proofScanLimit,
		   AlertFunc: func(kind, detail string) {
			   fmt.Fprintf(os.Stderr, "[ALERT] kind=%s detail=%s\n", kind, detail)
			   switch kind {
			  case "guardlease.leases_expired":
				   leasesExpiredTotal.Add(1)
			  case "guardlease.imbalance_open", "guardlease.imbalance_stale":
				  imbalancesOpenGauge.Inc()
			  case "guardlease.proof_fail_pending_clear", "guardlease.proof_escalated_to_halted":
				   proofFailsGauge.Inc()
			  default:
				  if len(kind) > 6 && kind[len(kind)-6:] == "_error" {
					   workerErrors.WithLabelValues(kind).Inc()
				  }
			   }
			  if svc.Halted() {
				   haltedGauge.Set(1)
			   } else {
				  haltedGauge.Set(0)
			   }
		  },
	   }
	  _ = leaseScanLimit // 保留 flag 供未来 ListActiveLeases 扩展
	   workers := guardlease.NewWorkers(svc, workerCfg)
	   // 启动 Prometheus metrics HTTP 服务
	  go func() {
		  mux := http.NewServeMux()
		   mux.Handle("/metrics", promhttp.Handler())
		   mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			   status := "ok"
			  code := http.StatusOK
			   if svc.Halted() {
				  status = "halted"
				   code = http.StatusServiceUnavailable
			   }
			  w.WriteHeader(code)
			   _, _ = fmt.Fprintf(w, `{"status":"%s"}`, status)
		   })
		  srv := &http.Server{
			   Addr: *metricsAddr,
			   Handler:           mux,
			   ReadHeaderTimeout: 5 * time.Second,
		   }
		  fmt.Fprintf(os.Stderr, "[INFO] fund-guard-lease metrics: %s\n", *metricsAddr)
		  if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			   fmt.Fprintf(os.Stderr, "[ERROR] metrics server: %v\n", err)
		   }
	  }()
	  fmt.Fprintf(os.Stderr,
		   "[INFO] fund-guard-lease started: lease_expiry=%s imbalance_scan=%s proof_escalate=%s\n",
		  *leaseExpiryInterval, *imbalanceScanInterval, *proofEscalateInterval,
	   )
	   workers.RunAll(ctx)
   }
