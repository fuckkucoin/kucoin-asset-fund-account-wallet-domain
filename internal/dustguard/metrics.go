package dustguard

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// DustAttackScoreGauge 按链和等级实时攻击分值（热点告警：level=critical）。
	DustAttackScoreGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_dust_attack_score",
		Help: "Current dust attack score by chain and level",
	}, []string{"chain", "level"})
	// DustQuarantineActive 各链当前活跃隔离案数。
	DustQuarantineActive = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_dust_quarantine_active",
		Help: "Active quarantine cases by chain",
	}, []string{"chain"})
	// DustQuarantineTotal 隔离案状态变更累计（open/holding/released/burned）。
	DustQuarantineTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_dust_quarantine_total",
		Help: "Quarantine state transitions by chain and state",
	}, []string{"chain", "state"})
	// DustCreditBlocked 被防护钩子拦截的观察贷记次数。
	DustCreditBlocked = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_dust_credit_blocked_total",
		Help: "Deposit observe calls blocked by dust guard hook",
	}, []string{"chain"})
	// DustAllocateBlocked 被防护钩子拦截的地址分配次数。
	DustAllocateBlocked = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_dust_allocate_blocked_total",
		Help: "Address allocations blocked by dust guard hook",
	}, []string{"chain"})
	// DustRotateTotal 轮换计划创建与执行累计。
	DustRotateTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_dust_rotate_total",
		Help: "Rotate plan events by chain and outcome (planned/done/aborted/auto_failed/auto_done)",
	}, []string{"chain", "outcome"})
	// DustPoolWaterLevel 地址池空闲水位百分比（低于策略阈值时告警）。
	DustPoolWaterLevel = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_dust_pool_water_level_pct",
		Help: "Address pool free slot percentage by chain; alert when below policy threshold",
	}, []string{"chain"})
	// DustForensicsExpiredBacklog 已过取证窗口但未人工释放的隔离案数。
	DustForensicsExpiredBacklog = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_dust_forensics_expired_backlog",
		Help: "Holding quarantine cases past forensics window awaiting ops release",
	})
	// DustWorkerPanic worker goroutine panic 次数（任何非零值须告警）。
	DustWorkerPanic = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_dust_worker_panic_total",
		Help: "Panics recovered in dust guard worker ticks",
	}, []string{"worker"})
	// DustSignalTotal 记录的攻击信号累计（flood/cluster/surface）。
	DustSignalTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_dust_signal_total",
		Help: "Dust attack signals recorded by kind",
	}, []string{"chain", "kind"})
	// DustTicketBacklog 当前开放防护单数（超过阈值须告警）。
	DustTicketBacklog = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_dust_ticket_backlog",
		Help: "Open guard tickets blocking credit by chain",
	}, []string{"chain"})
)
