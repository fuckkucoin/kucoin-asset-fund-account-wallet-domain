package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// PricePoisonDetected 毒价检测计数（交易对/等级）。
	PricePoisonDetected = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_price_poison_detected_total",
		Help: "Price poison detection events by pair and grade (spike/invert/cross_dev/stale_mask)",
	}, []string{"pair", "grade"})
	// PricePoisonTokenIssued 代次令牌签发计数（交易对/作用域）。
	PricePoisonTokenIssued = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_price_poison_token_issued_total",
		Help: "Generation tokens issued (clean tick) by pair and scope",
	}, []string{"pair", "scope"})
	// PricePoisonTokenRevoked 令牌撤销计数（交易对）。
	PricePoisonTokenRevoked = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_price_poison_token_revoked_total",
		Help: "Generation tokens revoked (poison detected) by pair",
	}, []string{"pair"})
	// PricePoisonEventStoreLen 事件仓库当前条目数。
	PricePoisonEventStoreLen = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_price_poison_event_store_len",
		Help: "Current number of entries in the in-process poison event store",
	})
	// WriteGateBlocked 写门拒绝计数（作用域/拒绝原因）。
	WriteGateBlocked = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_write_gate_blocked_total",
		Help: "Write gate check failures by scope and reason (token_invalid/probe_locked/scope_mismatch/unknown_scope)",
	}, []string{"scope", "reason"})
	// WriteGateAllowed 写门放行计数（作用域）。
	WriteGateAllowed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_write_gate_allowed_total",
		Help: "Write gate check passes by scope",
	}, []string{"scope"})
	// ProbeMutexMarked 探针互斥锁标记计数（交易对）。
	ProbeMutexMarked = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_probe_mutex_marked_total",
		Help: "Half-open probe ticks marked by pair (oracle circuit half-open)",
	}, []string{"pair"})
	// ProbeMutexCleared 探针互斥锁清除计数（交易对/结果）。
	ProbeMutexCleared = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_probe_mutex_cleared_total",
		Help: "Probe mutex cleared after probe tick completion by pair and result (ok/fail)",
	}, []string{"pair", "result"})
)
