package cfgdual

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// DualMetrics 灰度双评与脑裂封堵核心指标。

type DualMetrics struct {
	// DecisionsPumped 已落仓的决策数量。
	DecisionsPumped prometheus.Counter
	// DecisionDropped 因缓冲区满丢弃的决策数。
	DecisionDropped prometheus.Counter
	// BufLen 当前缓冲区积压量。
	BufLen prometheus.Gauge
	// DiffRatio 各发布当前差分比例（百分比）。
	DiffRatio *prometheus.GaugeVec
	// AllowDenyTotal 各发布累计 allow→deny 脑裂次数。
	AllowDenyTotal *prometheus.CounterVec
	// SealEnforced 各发布封堵触发次数。
	SealEnforced *prometheus.CounterVec
	// RollbackTotal 各发布自动回滚次数。
	RollbackTotal *prometheus.CounterVec
	// EvalTotal 双评调用总数（按 applied 和 has_diff 分）。
	EvalTotal *prometheus.CounterVec
	// EvalSealedTotal 因封堵失败关闭的请求数。
	EvalSealedTotal prometheus.Counter
	// EvalKillSwitchTotal kill switch 触发总数。
	EvalKillSwitchTotal prometheus.Counter
}

// NewDualMetrics 注册并返回 DualMetrics 实例。
// 多次调用幂等（promauto 在 DefaultRegisterer 上注册，重复注册时 panic）；
// 代码应全局单例；用 NewDualMetricsWithRegistry。
  func NewDualMetrics() *DualMetrics {
	   return newDualMetrics(promauto.With(prometheus.DefaultRegisterer))
   }

// NewDualMetricsWithRegistry 使用指定注册表（隔离用）。
  func NewDualMetricsWithRegistry(reg prometheus.Registerer) *DualMetrics {
	   return newDualMetrics(promauto.With(reg))
  }

  func newDualMetrics(f promauto.Factory) *DualMetrics {
	   return &DualMetrics{
		   DecisionsPumped: f.NewCounter(prometheus.CounterOpts{
			  Name: "aa_cfgdual_decisions_pumped_total",
			  Help: "双评决策已落仓数",
		  }),
		  DecisionDropped: f.NewCounter(prometheus.CounterOpts{
			  Name: "aa_cfgdual_decisions_dropped_total",
			  Help: "因缓冲区满丢弃的决策数；> 0 表示消费滞后",
		  }),
		   BufLen: f.NewGauge(prometheus.GaugeOpts{
			  Name: "aa_cfgdual_buf_len",
			  Help: "双评决策缓冲区当前积压量",
		  }),
		  DiffRatio: f.NewGaugeVec(prometheus.GaugeOpts{
			   Name: "aa_cfgdual_diff_ratio",
			  Help: "各发布滑动窗口内差分比例（百分比）",
		  }, []string{"release_id"}),
		  AllowDenyTotal: f.NewCounterVec(prometheus.CounterOpts{
			   Name: "aa_cfgdual_allow_deny_total",
			   Help: "各发布 allow→deny 脑裂累计次数；任意发布 > 0 触发 P1 告警",
		   }, []string{"release_id"}),
		   SealEnforced: f.NewCounterVec(prometheus.CounterOpts{
			   Name: "aa_cfgdual_seal_enforced_total",
			  Help: "各发布封堵触发次数；自增触发 P1 告警",
		   }, []string{"release_id"}),
		   RollbackTotal: f.NewCounterVec(prometheus.CounterOpts{
			  Name: "aa_cfgdual_rollback_total",
			  Help: "各发布自动回滚次数；自增触发 P2 告警",
		  }, []string{"release_id"}),
		   EvalTotal: f.NewCounterVec(prometheus.CounterOpts{
			   Name: "aa_cfgdual_eval_total",
			   Help: "双评调用总数",
		  }, []string{"applied", "has_diff"}),
		  EvalSealedTotal: f.NewCounter(prometheus.CounterOpts{
			  Name: "aa_cfgdual_eval_sealed_total",
			  Help: "因封堵失败关闭的请求数",
		   }),
		   EvalKillSwitchTotal: f.NewCounter(prometheus.CounterOpts{
			  Name: "aa_cfgdual_eval_kill_switch_total",
			   Help: "kill switch 触发总数",
		  }),
	   }
  }
