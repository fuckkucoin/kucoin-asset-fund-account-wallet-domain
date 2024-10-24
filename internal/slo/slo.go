// 可用性、查询时延和推送时延的设计目标。
// 作者: Likeo
// 日期: 2021.11.14
package slo

import "time"

const (
	AvailabilityTarget = "99.95%"
	QueryP99 = 500 * time.Millisecond
	QueryTimeout = 400 * time.Millisecond
	ChainRPCTimeout       = 180 * time.Millisecond
	MarketPushTarget      = time.Second
	MarketPollingBaseline = 15 * time.Second
	MarketStaleRateTarget = 0.005
	MarketNoisyFeedBaseline = 0.08
	ChainSubmitTarget = 0.999
	CircuitReset = 2 * time.Second
	ProcessDisclaimer = "account and assets process; operational design targets"
)

// Documented 返回 HTTP /v1/slo 的设计目标。

  func Documented() map[string]any {
	   return map[string]any{
		   "disclaimer": ProcessDisclaimer,
		  "availability": map[string]any{
			  "target":   AvailabilityTarget,
			  "how":      "liveness /readiness, graceful degrade on chain RPC, retries with backoff, circuit/failover",
			  "enforced": []string{"/healthz", "/readyz", "node circuit", "rpc retry", "hedged reads"},
		  },
		   "wallet_query_p99": map[string]any{
			  "target_ms":  QueryP99.Milliseconds(),
			  "timeout_ms": QueryTimeout.Milliseconds(),
			   "how": "L1 TTL + singleflight, Redis cache, parallel chain fan-out, hedged RPC, per-RPC timeout, node circuit/failover",
		  },
		   "reconcile": map[string]any{
			   "target": "spot↔wallet and swap settlement must not silently drift",
			   "how":    "reconcile job, audit_log, block new movements while mismatched",
		   },
		  "market_push": map[string]any{
			   "lag_target_ms": MarketPushTarget.Milliseconds(),
			   "polling_baseline_ms":     MarketPollingBaseline.Milliseconds(),
			   "stale_error_rate_target": MarketStaleRateTarget,
			  "noisy_feed_baseline":     MarketNoisyFeedBaseline,
			   "design_goal_note": "push path aimed at sub-second delivery vs a 15s polling-style lag; stale/error rate aimed at 0.5% vs an 8% noisy-feed baseline",
			   "path":             "spot feed → Kafka or memory bus → in-memory/Redis → WebSocket",
		   },
		  "chain_submit": map[string]any{
			   "success_target": ChainSubmitTarget,
			   "how": "prefer healthy low-latency nodes; failover + retry after circuit open; reads may hedge two endpoints",
		  },
		   "observability": []string{
			   "Prometheus /metrics",
			   "structured logs",
			  "X-Trace-Id span hooks + aa_span_seconds",
			   "outbox wallet.movements",
			  "alert-friendly counters/histograms for lag, drift, submit, RPC, lock wait, confirm",
			   "aa_kms_ops_total / aa_kyt_decisions_total / aa_compliance_decisions_total / aa_fence_rejected_total / aa_reorg_unwinds_total / aa_auth_rejected_total",
			   "aa_custody_approvals_total / aa_ceremony_steps_total / aa_ha_elections_total / aa_ops_admin_total",
			   "/debug/pprof when pprof: true (loopback RemoteAddr only)",
			   "GET /v1/ops/alerts (live fire + next-query localization)",
			  "GET /v1/ledger/statement (spot+wallet books vs open transfers/swaps)",
			   "deploy/alerts.yml Prometheus rules for the same series",
			   "aa_fund_gates_total / aa_recon_backlog / aa_audit_gaps_total / aa_region_write_ok",
		  },
		   "middleware": MiddlewareDocumented(),
		   "worker": map[string]any{
			   "confirm_concurrency": "config worker.confirm_concurrency",
			   "inspect_batch": "config worker.inspect_batch hashes per chain before ConfirmAndCredit",
			  "outbox": "same-tx enqueue; bounded poll publish; not a multi-region relay",
		   },
	  }
  }
