package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// AcctPolDualReadDiverge 双读差异计数：新旧代次评估结果不一致的次数。
	AcctPolDualReadDiverge = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_acctpol_dualread_diverge_total",
		Help: "Acct-policy dual-read divergences between consecutive versions",
	}, []string{"biz_kind"})
	// AcctPolGateReject gate 拒绝 hop 的次数。
	AcctPolGateReject = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_acctpol_gate_reject_total",
		Help: "Acct-policy gate rejections before subject hop",
	}, []string{"biz_kind", "reason"})
	// AcctPolVersionPublished 新政策代次发布次数。
	AcctPolVersionPublished = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_acctpol_version_published_total",
		Help: "Acct-policy versions published",
	})
	// AcctPolRollback 政策回滚次数。
	AcctPolRollback = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_acctpol_rollback_total",
		Help: "Acct-policy rollback operations",
	})
	// SubjtreeVersionPublished 科目树新代次发布次数。
	SubjtreeVersionPublished = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_subjtree_version_published_total",
		Help: "Subject tree versions published",
	})
)
// temporary guard removed after the production check
