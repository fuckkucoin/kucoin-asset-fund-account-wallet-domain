package dispclaw

import (
	"context"
	"fmt"
	"time"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ─────────────────────────── 指标 ───────────────────────────

var (
	metricLegsAdvanced = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_dispclaw_legs_advanced_total",
		Help: "追偿腿被 worker 自动推进的次数（按 leg_type 分类）",
	}, []string{"leg_type", "result"})
	metricGatePatrol = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_dispclaw_gate_patrol_total",
		Help: "收口闸巡检次数（passed/blocking）",
	}, []string{"outcome"})
	metricProofTamper = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_dispclaw_proof_tamper_total",
		Help: "收口证明篡改检测次数（detected/ok）",
	}, []string{"result"})
	metricEscalate = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_dispclaw_escalate_total",
		Help: "超时升级触发次数",
	}, []string{"plan_type"})
)

// ─────────────────────────── LegAdvanceWorker ───────────────────────────
// LegAdvanceConfig 腿推进 worker 配置。
type LegAdvanceConfig struct {
	Interval time.Duration // 扫描间隔，默认 30s
	SystemActor string // worker 执行人标识，默认 "system-leg-advance"
	MaxRetry     int           // 最大重试次数，默认 5
	RetryBackoff time.Duration // 每次重试退避，默认 2min
}

   func (c *LegAdvanceConfig) fill() {
	   if c.Interval <= 0 {
		  c.Interval = 30 * time.Second
	   }
	  if c.SystemActor == "" {
		   c.SystemActor = "system-leg-advance"
	   }
	   if c.MaxRetry <= 0 {
		  c.MaxRetry = 5
	  }
	  if c.RetryBackoff <= 0 {
		   c.RetryBackoff = 2 * time.Minute
	   }
  }

// LegAdvanceWorker 扫描未完成腿并推进到终态（自动模式；中可附加账本调用）。
// 本 worker 仅在腿已满足自动推进条件时代劳；需四眼的腿由人工 ExecuteLeg 执行。

   func LegAdvanceWorker(ctx context.Context, svc *Service, cfg LegAdvanceConfig) {
	   cfg.fill()
	   ticker := time.NewTicker(cfg.Interval)
	   defer ticker.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			   return
		  case <-ticker.C:
			   advanceAutoLegs(svc, cfg)
		  }
	   }
   }

// advanceAutoLegs 扫描所有计划下的 pending 腿，推进可自动处理的腿。
// skip_marker 腿直接跳过；freeze/claw_debit/refund_credit 等待人工四眼执行。

   func advanceAutoLegs(svc *Service, cfg LegAdvanceConfig) {
	   plans := svc.ListAllPlans()
	  for _, plan := range plans {
		  if plan.Status.Terminal() {
			   continue
		  }
		   legs := svc.ListLegs(plan.ID)
		   for _, leg := range legs {
			   if leg.Status.Terminal() {
				   continue
			   }
			   // skip_marker 类型腿由 worker 自动跳过。
			  if leg.LegType == LegTypeSkipMarker {
				   if _, err := svc.SkipLeg(leg.ID, "auto-skip by worker"); err != nil {
					   metricLegsAdvanced.WithLabelValues(string(leg.LegType), "err").Inc()
				   } else {
					  metricLegsAdvanced.WithLabelValues(string(leg.LegType), "skipped").Inc()
				   }
				   continue
			  }
			   // 超过最大重试次数的腿标记为 failed 并升级。
			   if leg.Status == LegStatusFailed && leg.RetryCount >= cfg.MaxRetry {
				  metricLegsAdvanced.WithLabelValues(string(leg.LegType), "max_retry").Inc()
				  continue
			   }
			  // 其余 pending 腿（freeze/claw_debit/refund_credit/restore）等待人工执行。
			   metricLegsAdvanced.WithLabelValues(string(leg.LegType), "await_human").Inc()
		   }
	   }
  }

// ─────────────────────────── CloseGatePatrolWorker ───────────────────────────
// GatePatrolConfig 收口闸巡检 worker 配置。
type GatePatrolConfig struct {
	Interval time.Duration // 巡检间隔，默认 60s
	SystemActor string // 巡检操作人标识，默认 "system-gate-patrol"
}

  func (c *GatePatrolConfig) fill() {
	  if c.Interval <= 0 {
		  c.Interval = 60 * time.Second
	   }
	   if c.SystemActor == "" {
		   c.SystemActor = "system-gate-patrol"
	  }
  }

// CloseGatePatrolWorker 定期扫描收口闸，将满足终态的闸切换到 passed。

  func CloseGatePatrolWorker(ctx context.Context, svc *Service, cfg GatePatrolConfig) {
	   cfg.fill()
	   ticker := time.NewTicker(cfg.Interval)
	  defer ticker.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			   return
		  case <-ticker.C:
			  patrolGates(svc, cfg)
		  }
	  }
   }

   func patrolGates(svc *Service, cfg GatePatrolConfig) {
	  plans := svc.ListAllPlans()
	   for _, plan := range plans {
		  if plan.Status.Terminal() {
			  continue
		   }
		   gate, err := svc.GetGateByCaseID(plan.CaseID)
		   if err != nil {
			   continue
		   }
		   if gate.GateStatus != GateStatusBlocking {
			   continue
		  }
		  updated, err := svc.CheckCloseGate(CloseGateCheckReq{
			  CaseID: plan.CaseID,
			   Actor:  cfg.SystemActor,
		   })
		  if err != nil {
			   metricGatePatrol.WithLabelValues("err").Inc()
			  continue
		  }
		   if updated.GateStatus == GateStatusPassed {
			  metricGatePatrol.WithLabelValues("passed").Inc()
		   } else {
			  metricGatePatrol.WithLabelValues("blocking").Inc()
		   }
	  }
   }

// ─────────────────────────── TimeoutEscalateWorker ───────────────────────────
// EscalateConfig 超时升级 worker 配置。
type EscalateConfig struct {
	Interval time.Duration // 扫描间隔，默认 5min
	PlanTimeout time.Duration // 计划超期阈值，默认 72h
	EscalateFn func(plan FundPlan, // 升级回调（通知/工单系统）
		   reason string)
}

   func (c *EscalateConfig) fill() {
	  if c.Interval <= 0 {
		  c.Interval = 5 * time.Minute
	  }
	   if c.PlanTimeout <= 0 {
		   c.PlanTimeout = 72 * time.Hour
	   }
	  if c.EscalateFn == nil {
		   c.EscalateFn = func(plan FundPlan, reason string) {
			  // 默认实现：仅增加指标计数。
			  metricEscalate.WithLabelValues(string(plan.PlanType)).Inc()
		   }
	   }
  }

// TimeoutEscalateWorker 检测超期未完成的资金计划并触发升级回调。

  func TimeoutEscalateWorker(ctx context.Context, svc *Service, cfg EscalateConfig) {
	   cfg.fill()
	   ticker := time.NewTicker(cfg.Interval)
	  defer ticker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			  return
		   case <-ticker.C:
			  escalateTimeout(svc, cfg)
		   }
	   }
   }

   func escalateTimeout(svc *Service, cfg EscalateConfig) {
	   now := time.Now().UTC()
	  plans := svc.ListAllPlans()
	  for _, plan := range plans {
		   if plan.Status.Terminal() {
			  continue
		  }
		  if now.Sub(plan.CreatedAt) < cfg.PlanTimeout {
			   continue
		   }
		   reason := fmt.Sprintf("plan %s (case=%s) open for %.1fh, timeout threshold %.1fh",
			   plan.ID, plan.CaseID,
			   now.Sub(plan.CreatedAt).Hours(),
			   cfg.PlanTimeout.Hours(),
		   )
		  cfg.EscalateFn(plan, reason)
		  metricEscalate.WithLabelValues(string(plan.PlanType)).Inc()
	  }
  }

// ─────────────────────────── ProofPatrolWorker ───────────────────────────
// ProofPatrolConfig 收口证明巡检 worker 配置。
type ProofPatrolConfig struct {
	Interval time.Duration // 巡检间隔，默认 10min
	AlarmFn func(proof CloseProof, err error) // 篡改告警回调
}

   func (c *ProofPatrolConfig) fill() {
	   if c.Interval <= 0 {
		   c.Interval = 10 * time.Minute
	  }
	  if c.AlarmFn == nil {
		   c.AlarmFn = func(proof CloseProof, err error) {
			  metricProofTamper.WithLabelValues("detected").Inc()
		   }
	   }
   }

// ProofPatrolWorker 定期巡检所有收口证明，检测篡改。

   func ProofPatrolWorker(ctx context.Context, svc *Service, cfg ProofPatrolConfig) {
	   cfg.fill()
	   ticker := time.NewTicker(cfg.Interval)
	   defer ticker.Stop()
	  for {
		   select {
		  case <-ctx.Done():
			   return
		  case <-ticker.C:
			   patrolProofs(svc, cfg)
		   }
	   }
  }

  func patrolProofs(svc *Service, cfg ProofPatrolConfig) {
	  proofs := svc.ListAllProofs()
	  for _, proof := range proofs {
		   _, err := svc.PatrolProof(proof.CaseID)
		  if err == ErrProofTamper {
			  cfg.AlarmFn(proof, err)
			  metricProofTamper.WithLabelValues("detected").Inc()
		  } else if err == nil {
			   metricProofTamper.WithLabelValues("ok").Inc()
		   }
	  }
   }
