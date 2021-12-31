package xfermirror

import (
	"context"
	"runtime/debug"
	"time"
	"account-assets.kucoin.net/funding/internal/xferclose"
)

// Logger 泵日志接口。
type Logger interface {
	Info(msg string, fields ...any)
	Error(msg string, err error, fields ...any)
}

type noopLogger struct{}

  func (noopLogger) Info(string, ...any) {}

  func (noopLogger) Error(string, error, ...any) {}

// Pump 镜像物化泵。

type Pump struct {
	machine *Machine
	closeStore xferclose.Store
	interval   time.Duration
	batchSize  int
	log Logger
}

// NewPump 构造镜像物化泵。

  func NewPump(machine *Machine, closeStore xferclose.Store, interval time.Duration) *Pump {
	   return &Pump{
		   machine:    machine,
		  closeStore: closeStore,
		   interval:  interval,
		   batchSize: 50,
		   log:       noopLogger{},
	   }
  }

// WithLogger 注入日志。
   func (p *Pump) WithLogger(l Logger) *Pump { p.log = l; return p }

// Run 启动物化泵，直到 ctx 取消。
  func (p *Pump) Run(ctx context.Context) {
	   ticker := time.NewTicker(p.interval)
	   defer ticker.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			   return
		  case <-ticker.C:
			  p.runOnce(ctx)
		   }
	   }
   }

  func (p *Pump) runOnce(ctx context.Context) {
	  defer func() {
		  if r := recover(); r != nil {
			  p.log.Error("mirror pump panic", nil, "stack", string(debug.Stack()))
		  }
	   }()
	   // 物化已完成计划。
	   done, _ := p.closeStore.ListPlans(ctx, xferclose.PlanDone, 0, p.batchSize)
	  materialized := 0
	   for _, plan := range done {
		  if err := p.materializePlan(ctx, plan); err != nil {
			  p.log.Error("materialize plan error", err, "plan_id", plan.ID)
			  continue
		  }
		  materialized++
	   }
	   // 物化已中止计划。
	   aborted, _ := p.closeStore.ListPlans(ctx, xferclose.PlanAborted, 0, p.batchSize)
	   for _, plan := range aborted {
		   if err := p.materializePlan(ctx, plan); err != nil {
			  p.log.Error("materialize aborted plan error", err, "plan_id", plan.ID)
			   continue
		  }
		  materialized++
	   }
	  if materialized > 0 {
		  p.log.Info("mirror pump", "materialized", materialized)
	  }
  }

// materializePlan 根据计划的收口动作更新镜像状态。
  func (p *Pump) materializePlan(ctx context.Context, plan xferclose.CloseoutPlan) error {
	   // 检查镜像是否已处于终态（幂等）。
	  existing, err := p.machine.store.GetByHopID(ctx, plan.HopID)
	   if err == nil && existing.Terminal() {
		  return nil
	   }
	   // 读取计划腿确认实际执行动作和结果。
	   legs,
		  err := p.closeStore.ListLegs(ctx, plan.ID)
	  if err != nil {
		  return err
	  }
	  var doneLeg *xferclose.CloseoutLeg
	  for i := range legs {
		  if legs[i].State == xferclose.LegDone {
			  doneLeg = &legs[i]
		   }
	  }
	  switch {
	   case plan.State == xferclose.PlanAborted:
		  _, err = p.machine.MarkManual(ctx, plan.HopID, "system", "closeout plan aborted: "+plan.AbortReason)
	   case doneLeg != nil && doneLeg.Action == xferclose.ActionBackfillIngress:
		   entryRef := doneLeg.EntryRef
		   _, err = p.machine.MarkCredited(ctx, plan.HopID, entryRef, plan.ID)
	   case doneLeg != nil && doneLeg.Action == xferclose.ActionRollbackEgress:
		  entryRef := doneLeg.EntryRef
		   _, err = p.machine.MarkRolledBack(ctx, plan.HopID, entryRef, plan.ID)
	  case doneLeg != nil && doneLeg.Action == xferclose.ActionEscalateDesk:
		   _, err = p.machine.MarkManual(ctx, plan.HopID, "system", "escalated to desk")
	   case doneLeg != nil && doneLeg.Action == xferclose.ActionOpenIncident:
		   _, err = p.machine.MarkManual(ctx, plan.HopID, "system", "incident opened: "+doneLeg.EntryRef)
	   case doneLeg != nil && doneLeg.Action == xferclose.ActionRetryAdvance:
		  _, err = p.machine.MarkCredited(ctx, plan.HopID, doneLeg.EntryRef, plan.ID)
	  }
	  if err != nil && !isAlreadyInState(err) {
		  return err
	  }
	  return nil
   }

   func isAlreadyInState(err error) bool {
	  if err == nil {
		  return false
	  }
	  s := err.Error()
	   return contains(s, "already rolled_back") ||
		  contains(s, "already credited") ||
		  contains(s, "already in manual")
   }

   func contains(s, sub string) bool {
	   return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		  func() bool {
			  for i := 0; i <= len(s)-len(sub); i++ {
				   if s[i:i+len(sub)] == sub {
					   return true
				   }
			   }
			   return false
		   }())
  }

// MaterializeOne 对单个 hopID 立即物化（供 API 触发）。

   func (p *Pump) MaterializeOne(ctx context.Context, hopID string) error {
	   plan, found, err := p.closeStore.GetPlanByHopID(ctx, hopID)
	   if err != nil {
		   return err
	   }
	  if !found {
		  return nil
	   }
	   return p.materializePlan(ctx,
		   plan)
  }
