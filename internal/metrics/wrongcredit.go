package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// WrongCreditPhaseTransitions 案件 Phase 变更计数（from/to/asset 三维）。
	// 告警：任意 to=bad_debt 的 asset 分组出现即触发 P2 告警。
	WrongCreditPhaseTransitions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_wrongcredit_phase_transitions_total",
		Help: "WrongCredit case phase transitions by from, to, and asset",
	}, []string{"from", "to", "asset"})
	// WrongCreditCasesTerminated 终态案件计数（phase/asset 两维）。
	WrongCreditCasesTerminated = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_wrongcredit_cases_terminated_total",
		Help: "WrongCredit cases reaching a terminal phase (reversed/partial/waived/bad_debt)",
	}, []string{"phase", "asset"})
	// WrongCreditEvidenceAttached 证据附加计数（按来源）。
	WrongCreditEvidenceAttached = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_wrongcredit_evidence_attached_total",
		Help: "Evidence envelopes attached to WrongCredit cases by source",
	}, []string{"source"})
	// WrongCreditScores 分值结果计数（置信度/风险层级）。
	WrongCreditScores = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_wrongcredit_scores_total",
		Help: "Discovery engine scoring results by confidence and risk_tier",
	}, []string{"confidence", "risk_tier"})
	// WrongCreditOrchSteps 编排器 Step 进入计数；用于追踪 FSM 吞吐和卡步位置。
	WrongCreditOrchSteps = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_wrongcredit_orch_steps_total",
		Help: "WrongCredit orchestrator FSM step entries by step name",
	}, []string{"step"})
	// WrongCreditMutexEvents 互斥锁事件计数（claim/release × reorg/wrong_credit/waive）。
	// 告警：claim:reorg 与 claim:wrong_credit 同时出现在同一 credit_ref → 代码逻辑错误。
	WrongCreditMutexEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_wrongcredit_mutex_events_total",
		Help: "CreditMutex claim/release events by event type and kind",
	}, []string{"event", "kind"})
	// WrongCreditReversals 追回结果计数（full/partial/bad_debt）。
	WrongCreditReversals = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_wrongcredit_reversals_total",
		Help: "WrongCredit reversal outcomes by result type (full/partial/bad_debt)",
	}, []string{"result"})
	// WrongCreditOpenCases 当前 open（非终态）案件数量（Gauge）；
	// 供 SRE 监控积压；不清零因为内存重建后可能暂时偏高。
	WrongCreditOpenCases = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_wrongcredit_open_cases",
		Help: "Current number of non-terminal WrongCredit cases in memory",
	})
	// WrongCreditMutexHeld 当前互斥锁持有数量（Gauge）。
	WrongCreditMutexHeld = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_wrongcredit_mutex_held",
		Help: "Current number of credit_refs held in CreditMutex",
	})
	// WrongCreditBadDebtAmount bad_debt 累计挂账金额（Gauge，按 asset）。
	// 注意：仅统计本进程运行期间产生的坏账金额，不做历史累计持久化。
	WrongCreditBadDebtAmount = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_wrongcredit_baddebt_amount",
		Help: "Cumulative bad-debt amount by asset (in-process session only, not persisted)",
	}, []string{"asset"})
	// WrongCreditJournalErrors 冲销分录提交失败次数；告警阈值：> 0 即 P1。
	WrongCreditJournalErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_wrongcredit_journal_errors_total",
		Help: "WrongCredit reversal journal apply errors by step; > 0 triggers P1 alert",
	}, []string{"step"})
	// WrongCreditAborted 编排器 abort 计数；告警阈值：> 0 需人工核查。
	WrongCreditAborted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_wrongcredit_aborted_total",
		Help: "WrongCredit orchestrator abort count; each abort requires manual review",
	})
	// WrongCreditApprovals 审批操作计数（freeze/revert × first/second）。
	WrongCreditApprovals = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_wrongcredit_approvals_total",
		Help: "WrongCredit approval actions by kind and sequence",
	}, []string{"kind", "seq"})
)
