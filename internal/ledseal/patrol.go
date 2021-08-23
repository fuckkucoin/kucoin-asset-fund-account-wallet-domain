package ledseal

import (
	"context"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/workerloop"
	"github.com/rs/zerolog"
)

// GapPatrol 快照序号间隙巡逻。

type GapPatrol struct {
	svc *Service
	alerter *Alerter
	log     zerolog.Logger
	clock func() time.Time
}

// NewGapPatrol 构造 GapPatrol。
   func NewGapPatrol(svc *Service, alerter *Alerter, log zerolog.Logger) *GapPatrol {
	  return &GapPatrol{
		  svc:     svc,
		   alerter: alerter,
		   log:   log.With().Str("patrol", "gap").Logger(),
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入时钟。
  func (gp *GapPatrol) WithClock(c func() time.Time) *GapPatrol {
	   if gp != nil && c != nil {
		  gp.clock = c
	  }
	   return gp
  }

  // Run 阻塞运行；ctx 取消时退出。

  func (gp *GapPatrol) Run(ctx context.Context) error {
	   cfg := workerloop.Config{
		   Name:     "ledseal-gap-patrol",
		  Interval: 2 * time.Minute,
		  MaxBackoff: 10 * time.Minute,
		   FailThresh: 3,
		   CircuitPause: 5 * time.Minute,
	   }
	  return workerloop.Run(ctx, cfg, gp.log, func(ctx context.Context) error {
		   return gp.patrol(ctx)
	  })
  }

// patrol 扫描所有计划的切片序号，检测间隙和回滚。

  func (gp *GapPatrol) patrol(ctx context.Context) error {
	  plans, err := gp.svc.store.ListPlans(ctx, 200)
	   if err != nil {
		   return fmt.Errorf("patrol list plans: %w", err)
	  }
	   for _, plan := range plans {
		   if err := gp.patrolPlan(ctx, plan); err != nil {
			  gp.log.Warn().Err(err).Str("plan_id", plan.ID).Msg("patrol plan error")
		   }
	  }
	   return nil
  }

// patrolPlan 逐序扫描计划的 SnapSlice 序号，找连续性异常。
   func (gp *GapPatrol) patrolPlan(ctx context.Context, plan SnapPlan) error {
	  slices, err := gp.svc.store.ListSlices(ctx, plan.ID, 1000)
	   if err != nil {
		  return err
	   }
	  if len(slices) < 2 {
		  return nil
	   }
	  for i := 1; i < len(slices); i++ {
		   prev := slices[i-1]
		   curr := slices[i]
		  if curr.GenSeq <= prev.GenSeq {
			   inc := GapIncident{
				   PlanID: plan.ID,
				   ExpectedSeq: prev.GenSeq + 1,
				   ActualSeq: curr.GenSeq,
				  Kind: "rollback",
				   At:   gp.clock(),
			  }
			   gp.log.Error().
				   Str("plan_id", plan.ID).
				   Int64("expected_seq", inc.ExpectedSeq).
				  Int64("actual_seq", inc.ActualSeq).
				   Msg("seq rollback detected")
			   if gp.alerter != nil {
				   _ = gp.alerter.PublishGap(ctx, inc)
			  }
			   continue
		  }
		   if curr.GenSeq > prev.GenSeq+1 {
			  // 序号跳跃：可能有切片未写入。
			  inc := GapIncident{
				  PlanID:      plan.ID,
				   ExpectedSeq: prev.GenSeq + 1,
				  ActualSeq: curr.GenSeq,
				  Kind:      "gap",
				  At:        gp.clock(),
			   }
			   gp.log.Warn().
				  Str("plan_id", plan.ID).
				  Int64("expected_seq", inc.ExpectedSeq).
				   Int64("actual_seq", inc.ActualSeq).
				   Msg("seq gap detected")
			   if gp.alerter != nil {
				   _ = gp.alerter.PublishGap(ctx, inc)
			   }
		   }
	   }
	  return nil
  }

// PatrolBatches 扫描失败批次，超过阈值则告警。
   func (gp *GapPatrol) PatrolBatches(ctx context.Context, failThresh int) error {
	  plans, err := gp.svc.store.ListPlans(ctx, 200)
	  if err != nil {
		  return fmt.Errorf("list plans: %w", err)
	  }
	   for _, plan := range plans {
		   batches, err := gp.svc.store.ListBatches(ctx, plan.ID, BatchFailed, 100)
		  if err != nil {
			  continue
		   }
		  if len(batches) >= failThresh {
			   gp.log.Error().
				  Str("plan_id", plan.ID).
				   Int("failed_batches", len(batches)).
				   Int("threshold", failThresh).
				   Msg("too many failed batches")
		   }
	   }
	  return nil
   }
