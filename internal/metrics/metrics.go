// Prometheus 指标：查询、提交、行情滞后、对账和节点。
// 作者: Likeo
// 日期: 2020.08.30
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_http_requests_total",
		Help: "HTTP requests by path and status (alert on 5xx)",
	}, []string{"path", "method", "status"})
	HTTPDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "aa_http_request_seconds",
		Help: "HTTP handler latency; wallet query SLO budget is 500ms p99",
		Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2},
	}, []string{"path", "method"})
	TransferByState = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_transfers_total",
		Help: "Transfers reaching a given state",
	}, []string{"state", "direction"})
	WalletQuerySeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "aa_wallet_query_seconds",
		Help: "Wallet balance/history query latency (design p99 ≤ 0.5s)",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
	}, []string{"op"})
	NodeLatency = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_node_latency_ms",
		Help: "Smoothed mock node latency in milliseconds",
	}, []string{"chain", "endpoint"})
	NodeAvailable = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_node_available",
		Help: "1 if the endpoint is in-circuit healthy, 0 if open/down",
	}, []string{"chain", "endpoint"})
	ChainRPC = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_chain_rpc_total",
		Help: "Chain RPC attempts by operation and result (alert on fail ratio)",
	}, []string{"chain", "op", "result"})
	ChainSubmit = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_chain_submit_total",
		Help: "Chain submit attempts after failover/retry (design success ≥ 99.9%)",
	}, []string{"chain", "result"})
	SwapOrders = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_swap_orders_total",
		Help: "Swap orders by status",
	}, []string{"status"})
	SpotOrders = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_spot_orders_total",
		Help: "Spot orders by status",
	}, []string{"status"})
	ReconcileRuns = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_reconcile_runs_total",
		Help: "Reconcile job outcomes (alert on mismatch)",
	}, []string{"result"})
	ReconcileMismatches = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_reconcile_mismatches",
		Help: "Current mismatch count for a user after the last reconcile",
	}, []string{"user_id"})
	DriftBlocks = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_drift_blocks_total",
		Help: "New movements rejected because reconcile is dirty",
	}, []string{"op"})
	MarketTickLag = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "aa_market_tick_lag_seconds",
		Help: "Produce-to-cache lag (design: sub-second vs 15s polling-style baseline)",
		Buckets: []float64{0.001,
			0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
	})
	MarketStale = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_market_stale_ticks_total",
		Help: "Ticks dropped as stale/out-of-order or over lag warn (design stale rate ≤ 0.5%)",
	}, []string{"reason"})
	MarketApplied = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_market_ticks_applied_total",
		Help: "Ticks applied to cache/WS",
	})
	IdempotencyConflicts = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_idempotency_conflicts_total",
		Help: "Idempotency key reused with a different payload (alert)",
	})
	FrozenStuck = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_frozen_stuck_total",
		Help: "Stuck freeze/lock without submit, or submitted txs past the watch window (alert)",
	}, []string{"state"})
	QuoteRejected = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_quote_rejected_total",
		Help: "Swap quotes/executes rejected for stale or slipped market (alert on rise)",
	}, []string{"reason"})
	SubmitDeduped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_chain_submit_deduped_total",
		Help: "At-least-once submit retries that reused a nonce instead of a second chain tx",
	})
	LockWait = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "aa_lock_wait_seconds",
		Help: "Time to acquire lock:user:{id} (lease, not a DB row lock)",
		Buckets: []float64{0.0001, 0.001, 0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 1, 2},
	})
	WorkerConfirm = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "aa_worker_confirm_seconds",
		Help:    "ConfirmAndCredit latency on the worker path",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
	})
	OutboxEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_outbox_events_total",
		Help: "Outbox rows by action (enqueued vs published)",
	}, []string{"action"})
	SpanSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "aa_span_seconds",
		Help:    "In-process span duration (logs + this histogram; no OTLP exporter)",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 8},
	}, []string{"span"})
	ChainWriteSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aa_chain_write_seconds",
		Help:    "Submit/Observe/Status wall time including failover (SLO-oriented write budget)",
		Buckets: []float64{0.001, 0.01, 0.05, 0.1, 0.18, 0.25, 0.5, 1, 2},
	}, []string{"op"})
	KMSOps = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_kms_ops_total",
		Help: "KMS/HSM operations (encrypt/decrypt/sign)",
	}, []string{"op"})
	KYTDecisions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_kyt_decisions_total",
		Help: "Pre-broadcast KYT outcomes (allow/review/deny/error); error is fail-closed",
	}, []string{"decision"})
	AuthRejected = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_auth_rejected_total",
		Help: "HTTP /v1 requests rejected for missing/invalid bearer or HMAC",
	})
	FenceRejected = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_fence_rejected_total",
		Help: "Row mutations refused because the fencing epoch/token was stale",
	})
	ReorgUnwinds = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_reorg_unwinds_total",
		Help: "Credited on-chain movements unwound after a mock-chain reorg below depth",
	}, []string{"direction"})
	ObservationHolds = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_observation_holds_total",
		Help: "Deposits held because chain observation amount/asset mismatched the request",
	}, []string{"reason"})
	InspectBatchSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "aa_inspect_batch_size",
		Help: "Hashes per InspectBatch call (worker confirm path)",
		Buckets: []float64{1, 2, 4, 8, 16, 32, 64, 128},
	})
	BalanceCoalesce = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_balance_coalesce_total",
		Help: "Balance loads that ran after L1/singleflight (not a cache-hit counter)",
	})
	ComplianceDecisions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_compliance_decisions_total",
		Help: "Compliance gate outcomes (allow/review/deny/error); error is fail-closed",
	}, []string{"decision"})
	CaseExpired = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_kyt_cases_expired_total",
		Help: "Review cases expired by TTL; expired is not allow",
	})
	CustodyApprovals = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_custody_approvals_total",
		Help: "Hot/cold dual-control approvals by state",
	}, []string{"state"})
	CeremonySteps = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_ceremony_steps_total",
		Help: "Key-ceremony phase advances; records only, not a custody certificate",
	}, []string{"phase"})
	HAElections = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_ha_elections_total",
		Help: "Leader elections by service and region",
	}, []string{"service", "region"})
	OpsAdmin = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_ops_admin_total",
		Help: "Admin freeze/reconcile triggers",
	}, []string{"op"})
	ReviewSLABreaches = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_review_sla_breaches_total",
		Help: "Review tickets past SLA; breach is not allow",
	})
	SweepPlanned = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_sweep_planned_total",
		Help: "Hot-to-cold sweep plans emitted by policy",
	}, []string{"asset"})
	SignPipeline = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_sign_pipeline_total",
		Help: "Withdraw sign jobs by state",
	}, []string{"state"})
	RegionFailover = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_region_failover_total",
		Help: "Dual-region failover or split-brain fence events",
	}, []string{"reason"})
	HotInventory = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_hot_inventory",
		Help: "Hot (system.bridge) available by asset; over=1 when above cap",
	}, []string{"asset", "over"})
	PersistFence = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_persist_fence_total",
		Help: "Persist row fence outcomes",
	}, []string{"result"})
	FiatRail = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_fiat_rail_total",
		Help: "Card/fiat rail bookings by kind and state (internal journals only)",
	}, []string{"kind", "state"})
	P2PSend = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_p2p_send_total",
		Help: "Internal user-to-user book moves by result",
	}, []string{"result"})
	AirgapSessions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_airgap_sessions_total",
		Help: "Cold ceremony recorder phase advances (not a custody certificate)",
	}, []string{"phase"})
	SLOBurnAlerts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_slo_burn_alerts_total",
		Help: "In-process SLO burn alerts by name",
	}, []string{"name"})
	PolicyHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_policy_hits_total",
		Help: "Versioned policy pack evaluations by pack, rule and mode",
	}, []string{"pack", "rule", "mode"})
	PolicyShadow = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_policy_shadow_total",
		Help: "Shadow-mode rule hits that would have blocked in enforce",
	}, []string{"pack", "rule"})
	HSMAdapterOps = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_hsm_adapter_ops_total",
		Help: "HSM/KMS adapter operations by op and result",
	}, []string{"op", "result"})
	PGPool = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_pg_pool",
		Help: "Postgres pool settings used by account-assets SQL (pg-0.kucoin.net)",
	}, []string{"param"})
	FundGates = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_fund_gates_total",
		Help: "Fund-path gate outcomes (destallow/amount/velocity/daily); store is fail-closed",
	}, []string{"gate", "result"})
	ReconTickets = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_recon_tickets_total",
		Help: "Reconciliation discrepancy tickets by event (open/acked/resolved/rejected/store)",
	}, []string{"event"})
	TravelEvidence = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_travel_evidence_total",
		Help: "Travel-rule evidence-chain outcomes (link/verify/broken)",
	}, []string{"result"})
	AuditGaps = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_audit_gaps_total",
		Help: "Audit-chain completeness checks by result",
	}, []string{"result"})
	ReconBacklog = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_recon_backlog",
		Help: "Open or acked reconciliation tickets; store faults must not zero this gauge as not-found",
	})
	RegionWriteOK = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_region_write_ok",
		Help: "1 if SG write / TYO standby CIDR probe passed; 0 after a failed probe",
	}, []string{"region"})
	CoverSweep = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_cover_sweep_total",
		Help: "Background stranded-cover sweeper outcomes (undone/fail_closed/young/in_use)",
	}, []string{"result"})
	CoverStranded = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_cover_stranded_total",
		Help: "Cover undo still incomplete after sweeper retries; fail-closed alert hook",
	})
	HopRecover = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_hop_recover_total",
		Help: "RecoverOpenHops outcomes (ok/error/skip)",
	}, []string{"result"})
	XferLoop = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_xfer_loop_total",
		Help: "Transfer close-loop workbench outcomes by kind",
	}, []string{"kind", "result"})
	TripartyRuns = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_triparty_recon_total",
		Help: "Hot inventory, custody books and user totals reconciliation",
	}, []string{"result"})
	TravelFXStale = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_travel_fx_stale",
		Help: "1 if travel-rule USD rate for asset is stale beyond threshold; 0 otherwise",
	}, []string{"asset", "source"})
	TravelFXStaleCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_travel_fx_stale_count",
		Help: "Number of stale travel-rule USD rates after refresh tick",
	})
	TravelFXRefresh = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_travel_fx_refresh_total",
		Help: "Travel FX refresh tick outcomes (refreshed/stale/skip)",
	}, []string{"result"})
	TravelFXVendor = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_travel_fx_vendor_total",
		Help: "Live FX vendor quote outcomes for travel USD (ok/fail/fallback)",
	}, []string{"result"})
	TravelFXVendorLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "aa_travel_fx_vendor_seconds",
		Help: "Live FX vendor quote latency",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5},
	})
	TravelFXBurnRemaining = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_travel_fx_burn_remaining",
		Help: "Remaining Live FX vendor error budget (target 0.995); negative means exhausted",
	})
	TravelFXBurnRate = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_travel_fx_burn_rate",
		Help: "Live FX vendor error-budget burn rate (errRate/budget); primary 1h window",
	})
	TravelFXBurnRateWindow = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_travel_fx_burn_rate_window",
		Help: "Live FX vendor burn rate by rolling window (5m/1h)",
	}, []string{"window"})
	TravelFXBurnRemainingWindow = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_travel_fx_burn_remaining_window",
		Help: "Live FX vendor remaining error budget by rolling window (5m/1h)",
	}, []string{"window"})
	WDTrackOK = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_wd_track_ok",
		Help: "1 if wd-track surface healthy (hydrate/persist); 0 otherwise",
	}, []string{"kind"}) // broadcast|fee_bump
	WDTrackMemoryN = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_wd_track_memory_n",
		Help: "In-memory wd-track row count",
	}, []string{"kind"})
	WDTrackSQLN = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_wd_track_sql_n",
		Help: "SQL wd-track row count",
	}, []string{"kind"})
	WDTrackDrift = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_wd_track_drift",
		Help: "1 if memory vs SQL count drift for wd-track",
	}, []string{"kind"})
	WDTrackHydrateErr = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_wd_track_hydrate_err",
		Help: "1 if last hydrate failed for wd-track",
	}, []string{"kind"})
	WDTrackPersistErr = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_wd_track_persist_err",
		Help: "1 if last persist failed for wd-track",
	}, []string{"kind"})
	WDReorgSaga = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_wd_reorg_saga",
		Help: "Open broadcast reorg→resubmit sagas by phase",
	}, []string{"phase"})
	// WDReorgSagaMaxAge 开放 saga 最大年龄（秒），按相位；告警 ticket/page 调谐用。
	WDReorgSagaMaxAge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_wd_reorg_saga_max_age_seconds",
		Help: "Max age seconds of open reorg→resubmit sagas by phase",
	}, []string{"phase"})
	// WDReorgSagaTimeoutTotal TickSagas 超时失败累计（须人工重开）。
	WDReorgSagaTimeoutTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_wd_reorg_saga_timeout_total",
		Help: "Count of reorg→resubmit sagas failed by TickSagas timeout",
	})
	// DepDesk* 入金异常台 Hydrate 健康（一致 wd-track）。
	DepDeskOK = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_dep_desk_ok",
		Help: "1 if deposit desk hydrate healthy",
	})
	DepDeskHydrateErr = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_dep_desk_hydrate_err",
		Help: "1 if last deposit desk hydrate failed",
	})
	DepDeskMemoryN = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_dep_desk_memory_n",
		Help: "Open deposit exception rows in memory",
	})
	DepDeskHydratedN = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_dep_desk_hydrated_n",
		Help: "Rows loaded by last deposit desk hydrate",
	})
	// EarnProductAudit* 理财目录软删审计运营面（CSV/保留/告警）。
	EarnProductAudit = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_earn_product_audit_total",
		Help: "Earn product soft-delete/undelete/export/purge audit actions",
	}, []string{"action"})
	EarnProductAuditRows = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_earn_product_audit_rows",
		Help: "Earn product audit row count",
	})
	EarnProductAuditMaxAge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_earn_product_audit_max_age_seconds",
		Help: "Age seconds of oldest earn product audit row (retention signal)",
	})
	EarnProductAuditWriteErr = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_earn_product_audit_write_err",
		Help: "1 if last earn product audit metric probe failed",
	})
	EarnProductAuditColdArchives = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_earn_product_audit_cold_archives",
		Help: "Earn product audit cold-archive shard count from last ops archive",
	})
	EarnProductAuditColdRows = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_earn_product_audit_cold_manifest_rows",
		Help: "Earn product audit cold manifest SQL row count",
	})
	// GasTankLevel gas 仓水位（链/原生资产/穿仓等级）；等级标签：ok/warn/crit/halt。
	GasTankLevel = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_gas_tank_level",
		Help: "Gas tank available balance by chain and native asset; label breach_level=ok/warn/crit/halt",
	}, []string{"chain", "native", "breach_level"})
	// GasTankBreachTotal 穿仓事件计数（Crit/Halt 触发补仓或停仓时递增）。
	GasTankBreachTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_gas_tank_breach_total",
		Help: "Gas tank breach events by chain, native and level (crit/halt)",
	}, []string{"chain", "native", "level"})
	// EgressGateBlocked 出金预检门控拒绝计数（链/资产/穿仓等级）。
	EgressGateBlocked = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_egress_gate_blocked_total",
		Help: "Egress precheck gate blocks by chain, asset and breach level",
	}, []string{"chain", "asset", "breach_level"})
	// RefillMutexConflicts 补仓互斥锁冲突计数（同链并发注入被拒时递增）。
	RefillMutexConflicts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_refill_mutex_conflicts_total",
		Help: "Refill mutex conflicts (parallel double-refill attempts) by chain",
	}, []string{"chain"})
	// ─────────────────────────────────────────────────────────
	// 法币断裂引擎指标（fiatbreak）
	// ─────────────────────────────────────────────────────────
	// FiatBreak 断裂记录状态变化计数（kind × state）。
	FiatBreak = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_fiat_break_total",
		Help: "Fiat rail break records by kind and state transition",
	}, []string{"kind", "state"})
	// FiatSuspense 挂账条目状态变化计数（reason × state）。
	FiatSuspense = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_fiat_suspense_total",
		Help: "Fiat suspense entry lifecycle events by reason and state",
	}, []string{"reason", "state"})
	// FiatSuspenseBalance 各资产当前 open 挂账余额（Gauge）。
	FiatSuspenseBalance = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_fiat_suspense_balance",
		Help: "Total open fiat suspense balance by asset (alert if unexpected growth)",
	}, []string{"asset"})
	// FiatAgingEscalate 老化升级事件计数（policy × desk）。
	FiatAgingEscalate = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_fiat_aging_escalate_total",
		Help: "Fiat break/suspense SLA aging escalations by policy and desk",
	}, []string{"policy", "desk"})
	// FiatWorker 后台 worker 操作计数（op）。
	FiatWorker = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_fiat_worker_total",
		Help: "Fiat break worker operations: recon_ok/recon_error/recon_drop/aging_escalate/suspense_leak",
	}, []string{"op"})
	// FiatProofGenerate 双边证明生成计数。
	FiatProofGenerate = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_fiat_proof_generate_total",
		Help: "Bilateral proof generation count (append-only; no overwrite)",
	})
	// ─────────────────────────────────────────────────────────
	// 核验放行租约收口引擎指标（allowleasegate）
	// ─────────────────────────────────────────────────────────
	AllowLeaseDecisions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_allow_lease_decisions_total",
		Help: "Allow-lease gate outcomes: issued/submit_allowed/reject_*/consumed/revoked",
	}, []string{"decision"})
	AllowLeaseActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_allow_lease_active",
		Help: "Currently active (allowed|bound) allow leases",
	})
	AllowLeaseExpired = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_allow_lease_expired_total",
		Help: "Allow leases expired by TTL; expired is fail-closed, not allow",
	})
	AllowDriftProofSealed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_allow_drift_proof_sealed_total",
		Help: "AllowDriftProof sealed (broadcast blocked due to lease gap)",
	})
	AllowLeasePatrolRevoked = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_allow_lease_patrol_revoked_total",
		Help: "Allow leases revoked by case-state patrol worker",
	})
	// BreachCloseGate* — SLA 违约关案挡写引擎指标。
	// breach_gen 签发后在 breachclosegate 落盘；TryAllowCheck 被拒每次计一；
	// 四眼清障 propose/approve/reject 各计一；案件强制 Expire 计一。
	BreachLeaseCreated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_breach_lease_created_total",
		Help: "SLA breach leases created; breach is not allow",
	})
	BreachAllowBlocked = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_breach_allow_blocked_total",
		Help: "Decide(allow)/Submit blocked by active SLA breach lease",
	})
	BreachCaseExpired = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_breach_case_expired_total",
		Help: "Cases force-expired by SLA breach close gate worker",
	})
	BreachClearanceProposed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_breach_clearance_proposed_total",
		Help: "SLA breach clearance proposals submitted (four-eye workflow)",
	})
	BreachClearanceApproved = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_breach_clearance_approved_total",
		Help: "SLA breach clearances approved by second actor; no auto-allow",
	})
	BreachClearanceRejected = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_breach_clearance_rejected_total",
		Help: "SLA breach clearances rejected; breach block remains active",
	})
	BreachActiveLeases = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_breach_active_leases",
		Help: "Count of SLA breach leases in blocking state (alert if > 0 for > 1h)",
	})
	BreachWorkerScanRounds = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_breach_worker_scan_rounds_total",
		Help: "Total scan rounds executed by breach close gate worker",
	})
	// 审查案件围栏世代与双结放行封堵引擎指标（casefencegate）
	CaseFenceDecideTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_case_fence_decide_total",
		Help: "Case fence decide outcomes by decision and mode (fenced/bare)",
	}, []string{"decision", "mode"})
	CaseFenceDualDecideTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_case_fence_dual_decide_total",
		Help: "Dual-reviewer concurrent decide race; sealed CaseDualDecideProof count",
	})
	CaseFenceBareDecideTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_case_fence_bare_decide_total",
		Help: "Bare Decide calls on fund path blocked or alerted",
	})
	CaseFencePolicyDriftTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_case_fence_policy_drift_total",
		Help: "Submit rejected due to policyVer drift between ticket and submit",
	})
	CaseFenceTicketExpiredTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_case_fence_ticket_expired_total",
		Help: "Release tickets expired by TTL or case expire patrol",
	})
	CaseFenceActiveTickets = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_case_fence_active_tickets",
		Help: "Currently active (open) release tickets",
	})
)
