package ledseal

import (
	"context"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/workerloop"
	"github.com/rs/zerolog"
)

// DiffPump 增量差分泵：消费最新两代切片，执行差分并封印。
type DiffPump struct {
	svc *Service
	alerter *Alerter
	log zerolog.Logger
	clock func() time.Time
}

// NewDiffPump 构造差分泵。

  func NewDiffPump(svc *Service, alerter *Alerter, log zerolog.Logger) *DiffPump {
	   return &DiffPump{
		   svc:     svc,
		   alerter: alerter,
		   log: log.With().Str("pump", "diff").Logger(),
		  clock: func() time.Time { return time.Now().UTC() },
	   }
   }

// WithClock 注入时钟。

   func (p *DiffPump) WithClock(c func() time.Time) *DiffPump {
	  if p != nil && c != nil {
		  p.clock = c
	  }
	   return p
  }

  // Run 阻塞运行；ctx 取消时退出。
   func (p *DiffPump) Run(ctx context.Context) error {
	   cfg := workerloop.Config{
		   Name:       "ledseal-diff",
		   Interval:   45 * time.Second,
		   MaxBackoff: 5 * time.Minute,
		  FailThresh: 5,
		  CircuitPause: 2 * time.Minute,
	  }
	   return workerloop.Run(ctx, cfg, p.log, func(ctx context.Context) error {
		   return p.tick(ctx)
	  })
  }

// tick 单次差分循环：遍历计划 → 取最新两代切片 → 检查是否已有批次 → 执行差分 → 封印。

   func (p *DiffPump) tick(ctx context.Context) error {
	  plans, err := p.svc.store.ListPlans(ctx, 200)
	   if err != nil {
		  return fmt.Errorf("list plans: %w", err)
	   }
	  for _, plan := range plans {
		  if plan.State != PlanActive {
			   continue
		   }
		   if err := p.processPlan(ctx, plan); err != nil {
			   p.log.Warn().Err(err).Str("plan_id", plan.ID).Msg("diff plan failed")
		  }
	  }
	   return nil
   }

// processPlan 对单个计划取最新两代切片并执行差分。

  func (p *DiffPump) processPlan(ctx context.Context, plan SnapPlan) error {
	   slices,
		  err := p.svc.store.ListSlices(ctx, plan.ID, 10)
	  if err != nil {
		  return fmt.Errorf("list slices: %w", err)
	   }
	   // 至少需要两代切片。
	   if len(slices) < 2 {
		   return nil
	  }
	   // 取最新两代（已按 gen_seq 升序排序）。
	  fromSl := slices[len(slices)-2]
	   toSl := slices[len(slices)-1]
	   // 检查是否已有 running/sealed 批次覆盖此区间（幂等保护）。
	  existing, _ := p.svc.store.ListBatches(ctx, plan.ID, "", 100)
	   for _, b := range existing {
		   if b.FromSliceID == fromSl.ID && b.ToSliceID == toSl.ID {
			   if b.State == BatchRunning || b.State == BatchSealed {
				  return nil
			  }
		   }
	  }
	   // 开启批次。
	   batch, err := p.svc.OpenBatch(ctx, plan.ID, fromSl.ID, toSl.ID, fromSl.GenSeq, toSl.GenSeq)
	   if err != nil {
		   return fmt.Errorf("open batch: %w", err)
	  }
	   // 执行差分+封印。
	  batch, root, err := p.svc.RunAndSealBatch(ctx, batch.ID)
	  if err != nil {
		  // 序号间隙/回滚：批次已标记 failed，发 GapIncident。
		  if err == ErrSliceGap || err == ErrSliceRollback {
			  if p.alerter != nil {
				   inc := GapIncident{
					  PlanID:      plan.ID,
					   ExpectedSeq: fromSl.GenSeq + 1,
					   ActualSeq:   toSl.GenSeq,
					  Kind: gapKind(err),
					   At: p.clock(),
				  }
				  _ = p.alerter.PublishGap(ctx, inc)
			   }
		  }
		  return fmt.Errorf("run batch %s: %w", batch.ID, err)
	   }
	  // 封印成功：若有漂移则发告警。
	  if batch.DriftCount > 0 && p.alerter != nil {
		   suggestions := p.collectBlockHints(ctx, batch.ID)
		   ev := AlertEvent{
			   BatchID: batch.ID,
			   PlanID:  plan.ID,
			   DriftCount:            batch.DriftCount,
			   MaxSeverity:           batch.MaxSeverity,
			   BlockWriteSuggestions: suggestions,
			  At: p.clock(),
		   }
		   _ = p.alerter.PublishDrift(ctx, ev)
	   }
	   p.log.Info().
		   Str("batch_id", batch.ID).
		  Str("seal_root_id", root.ID).
		   Int("drift_count", batch.DriftCount).
		   Str("max_severity", string(batch.MaxSeverity)).
		  Msg("batch sealed")
	   return nil
  }

   func (p *DiffPump) collectBlockHints(ctx context.Context, batchID string) []string {
	  rows, err := p.svc.store.ListDriftRows(ctx, batchID, 500)
	  if err != nil {
		  return nil
	   }
	  out := make([]string, 0)
	   for _, r := range rows {
		  if r.BlockWriteHint {
			  out = append(out, r.ID)
		   }
	   }
	   return out
  }

  func gapKind(err error) string {
	   if err == ErrSliceRollback {
		   return "rollback"
	   }
	   return "gap"
   }
