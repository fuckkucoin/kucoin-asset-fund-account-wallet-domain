package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// FundCircuitSignals 进入总线的滥用信号数（按来源种类）。
	FundCircuitSignals = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_fund_circuit_signals_total",
		Help: "Fund abuse signals ingested by kind (dep/wd/hop/http/backpress)",
	}, []string{"kind"})
	// FundCircuitTrips 跳闸次数（按作用域与分档）。
	FundCircuitTrips = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_fund_circuit_trips_total",
		Help: "Fund abuse circuit trips by scope and severity (alert on global/halt)",
	}, []string{"scope", "severity"})
	// FundCircuitBlocks 被熔断挡住的新写数（按入口与作用域）。
	FundCircuitBlocks = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_fund_circuit_blocks_total",
		Help: "New fund writes rejected by the abuse circuit bus, by path and scope",
	}, []string{"path", "scope"})
	// FundCircuitProbes 半开探针结论分布。
	FundCircuitProbes = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_fund_circuit_probes_total",
		Help: "Half-open probe outcomes (passed/failed/expired)",
	}, []string{"result"})
	// FundCircuitCircuits 各状态熔断器数量。
	FundCircuitCircuits = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_fund_circuit_state",
		Help: "Fund abuse circuits currently in a given state",
	}, []string{"state"})
	// FundCircuitCases 各状态案件数量。
	FundCircuitCases = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_fund_circuit_cases",
		Help: "Fund abuse cases by state (open/review/cleared)",
	}, []string{"state"})
	// FundCircuitScore 各作用域当前最高分，供阈值调参。
	FundCircuitScore = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_fund_circuit_score_max",
		Help: "Highest fund abuse score per scope in the current window",
	}, []string{"scope"})
	// FundCircuitClearDenied 被拒绝的解熔断尝试（单人 Clear 等）。
	FundCircuitClearDenied = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_fund_circuit_clear_denied_total",
		Help: "Circuit clear attempts rejected (single-desk clear or missing quorum)",
	}, []string{"reason"})
	// FundCircuitPersistErr 熔断行落库失败标志；1 表示最近一次写失败。
	FundCircuitPersistErr = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_fund_circuit_persist_err",
		Help: "1 if the last fund circuit row persist failed, by table",
	}, []string{"table"})
)
