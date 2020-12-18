package wrfreeze

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics 级联冻结协议指标集合。
type Metrics struct {
	PlanCreatedTotal prometheus.Counter
	PlanArmedTotal   prometheus.Counter
	PlanSealedTotal  prometheus.Counter
	PlanAbortedTotal prometheus.Counter
	PlanExpiredTotal prometheus.Counter
	WaveRunTotal     prometheus.Counter
	WaveDoneTotal   prometheus.Counter
	WaveFailedTotal prometheus.Counter
	WaveAdvancedTotal    prometheus.Counter
	WaveFrozenUsersTotal prometheus.Counter
	PhaseInitTotal       *prometheus.CounterVec
	PhasePassedTotal  *prometheus.CounterVec
	PhaseFailedTotal  *prometheus.CounterVec
	PhaseTimeoutTotal prometheus.Counter
	ProofVerdictTotal      *prometheus.CounterVec
	ProofDeltaGauge        prometheus.Gauge
	RollbackRequestedTotal prometheus.Counter
	RollbackRestoredTotal prometheus.Counter
	RollbackLegCreatedTotal prometheus.Counter
	LiveHashSkipTotal prometheus.Counter
}

// NewMetrics 注册并返回指标集合（使用 promauto 自动注册）。

   func NewMetrics() *Metrics {
	   return &Metrics{
		   PlanCreatedTotal: promauto.NewCounter(prometheus.CounterOpts{
			  Name: "wrf_cascade_plan_created_total",
			  Help: "Cascade freeze plans created",
		   }),
		   PlanArmedTotal: promauto.NewCounter(prometheus.CounterOpts{
			  Name: "wrf_cascade_plan_armed_total",
			  Help: "Cascade freeze plans successfully armed (dual-control passed)",
		  }),
		   PlanSealedTotal: promauto.NewCounter(prometheus.CounterOpts{
			  Name: "wrf_cascade_plan_sealed_total",
			  Help: "Cascade freeze plans sealed (100% coverage proof)",
		  }),
		   PlanAbortedTotal: promauto.NewCounter(prometheus.CounterOpts{
			  Name: "wrf_cascade_plan_aborted_total",
			   Help: "Cascade freeze plans aborted (manual or expiry)",
		   }),
		  PlanExpiredTotal: promauto.NewCounter(prometheus.CounterOpts{
			  Name: "wrf_cascade_plan_expired_total",
			   Help: "Cascade freeze plans expired and auto-aborted by worker",
		   }),
		  WaveRunTotal: promauto.NewCounter(prometheus.CounterOpts{
			   Name: "wrf_freeze_wave_run_total",
			   Help: "Freeze wave execution attempts",
		  }),
		  WaveDoneTotal: promauto.NewCounter(prometheus.CounterOpts{
			   Name: "wrf_freeze_wave_done_total",
			   Help: "Freeze waves completed successfully",
		   }),
		  WaveFailedTotal: promauto.NewCounter(prometheus.CounterOpts{
			  Name: "wrf_freeze_wave_failed_total",
			  Help: "Freeze waves failed (all users failed to freeze)",
		  }),
		   WaveAdvancedTotal: promauto.NewCounter(prometheus.CounterOpts{
			  Name: "wrf_freeze_wave_advanced_total",
			  Help: "Wave advancement steps triggered by worker",
		   }),
		   WaveFrozenUsersTotal: promauto.NewCounter(prometheus.CounterOpts{
			  Name: "wrf_freeze_wave_frozen_users_total",
			  Help: "Cumulative count of users frozen by cascade waves",
		  }),
		   PhaseInitTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			  Name: "wrf_unfreeze_phase_init_total",
			  Help: "Unfreeze phase initiations by kind",
		   }, []string{"kind"}),
		  PhasePassedTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			   Name: "wrf_unfreeze_phase_passed_total",
			   Help: "Unfreeze phases passed by kind",
		  }, []string{"kind"}),
		  PhaseFailedTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			  Name: "wrf_unfreeze_phase_failed_total",
			   Help: "Unfreeze phases failed by kind",
		  }, []string{"kind"}),
		  PhaseTimeoutTotal: promauto.NewCounter(prometheus.CounterOpts{
			  Name: "wrf_unfreeze_phase_timeout_total",
			   Help: "Unfreeze phases timed out and marked failed by worker",
		  }),
		  ProofVerdictTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			   Name: "wrf_coverage_proof_verdict_total",
			   Help: "Coverage proof verdicts by result",
		  }, []string{"verdict"}),
		  ProofDeltaGauge: promauto.NewGauge(prometheus.GaugeOpts{
			  Name: "wrf_coverage_proof_delta_users",
			   Help: "Current delta user count from latest proof (0 = fully covered)",
		  }),
		   RollbackRequestedTotal: promauto.NewCounter(prometheus.CounterOpts{
			   Name: "wrf_rollback_leg_requested_total",
			  Help: "Rollback legs requested",
		  }),
		   RollbackRestoredTotal: promauto.NewCounter(prometheus.CounterOpts{
			   Name: "wrf_rollback_leg_restored_total",
			  Help: "Rollback legs successfully restored (availability view refreshed)",
		   }),
		   RollbackLegCreatedTotal: promauto.NewCounter(prometheus.CounterOpts{
			   Name: "wrf_rollback_leg_auto_created_total",
			   Help: "Rollback legs auto-created for expired incidents by worker",
		  }),
		   LiveHashSkipTotal: promauto.NewCounter(prometheus.CounterOpts{
			  Name: "wrf_live_hash_skip_total",
			  Help: "Users skipped from unfreeze due to live-hash withdraw in-flight",
		  }),
	  }
   }
