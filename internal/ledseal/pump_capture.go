package ledseal

import (
	"context"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/workerloop"
	"github.com/rs/zerolog"
)

// CapturePump 快照捕获泵：按到期计划触发快照抓拍并登记切片。
type CapturePump struct {
	svc *Service
	log   zerolog.Logger
	clock func() time.Time
}

// NewCapturePump 构造捕获泵。
  func NewCapturePump(svc *Service, log zerolog.Logger) *CapturePump {
	   return &CapturePump{
		  svc: svc,
		   log:   log.With().Str("pump", "capture").Logger(),
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入时钟。
   func (p *CapturePump) WithClock(c func() time.Time) *CapturePump {
	   if p != nil && c != nil {
		   p.clock = c
	   }
	   return p
   }

   // Run 阻塞运行；ctx 取消时退出。

  func (p *CapturePump) Run(ctx context.Context) error {
	  cfg := workerloop.Config{
		  Name:     "ledseal-capture",
		   Interval: 30 * time.Second,
		   MaxBackoff: 5 * time.Minute,
		  FailThresh:   5,
		   CircuitPause: 2 * time.Minute,
	  }
	  return workerloop.Run(ctx, cfg, p.log, func(ctx context.Context) error {
		  return p.tick(ctx)
	   })
  }

// tick 单次捕获循环：取到期计划 → 抓拍 → 登记切片 → 更新 next_run_at。
  func (p *CapturePump) tick(ctx context.Context) error {
	  now := p.clock()
	  plans, err := p.svc.store.ListActivePlans(ctx, now)
	  if err != nil {
		   return fmt.Errorf("list active plans: %w", err)
	   }
	  for _, plan := range plans {
		   if err := p.processPlan(ctx, plan, now); err != nil {
			  p.log.Warn().Err(err).Str("plan_id", plan.ID).Str("plan_name", plan.Name).Msg("capture plan failed")
			  // 单个计划失败不中断整体循环。
		  }
	   }
	  return nil
   }

// processPlan 对单个计划执行快照抓拍。

  func (p *CapturePump) processPlan(ctx context.Context, plan SnapPlan, now time.Time) error {
	   if p.svc.snapSvc == nil {
		  return ErrUnavailable
	  }
	   var (
		   snapID string
		   userID string
		   assetFamily string
		   checksum string
		   positionCount int
		   fenceEpoch int64
		  fenceToken string
	   )
	   switch plan.PlanScope {
	  case ScopeUserShard:
		   // 按用户分片：抓拍指定 shard_key 用户的快照。
		  userID = plan.ShardKey
		  if userID == "" {
			   return fmt.Errorf("%w: shard_key required for user_shard scope", ErrParam)
		   }
		   snp, err := p.svc.snapSvc.Capture(ctx, userID)
		   if err != nil {
			  return fmt.Errorf("capture user=%s: %w", userID, err)
		   }
		  snapID = snp.ID
		   checksum = snp.Checksum
		  positionCount = snp.Count
		   fenceEpoch = snp.Fence.Epoch
		  fenceToken = snp.Fence.Token
	  case ScopeAssetFamily:
		  // 资产族模式：此处只记录空快照切片作为调度标记；
		   // 实际按资产族汇总由调用方按需实现（不进用户快照）。
		   assetFamily = plan.AssetFamily
		   snapID = fmt.Sprintf("asset-family-%s-%d", assetFamily, now.Unix())
		  checksum = ""
		  positionCount = 0
	  case ScopeFullDay:
		   // 全量采样：记录调度标记，实际快照由外部大批量任务完成。
		  snapID = fmt.Sprintf("full-day-%d", now.UnixNano())
		   checksum = ""
		   positionCount = 0
	  default:
		  return fmt.Errorf("%w: unknown scope %q", ErrParam, plan.PlanScope)
	  }
	   // 登记切片。
	  _, err := p.svc.RegisterSlice(ctx, plan.ID, snapID, userID, assetFamily, checksum,
		  positionCount, fenceEpoch, fenceToken, now)
	   if err != nil {
		   return fmt.Errorf("register slice: %w", err)
	   }
	   // 更新计划的 last_run_at 和 next_run_at。
	   plan.LastRunAt = &now
	   next := now.Add(time.Duration(plan.IntervalSeconds) * time.Second)
	  plan.NextRunAt = &next
	   plan.UpdatedAt = now
	  if err := p.svc.store.PutPlan(ctx, plan); err != nil {
		   return fmt.Errorf("update plan schedule: %w", err)
	   }
	   p.log.Info().
		   Str("plan_id", plan.ID).
		   Str("snap_id", snapID).
		   Str("scope", string(plan.PlanScope)).
		   Int("positions", positionCount).
		   Msg("slice captured")
	   return nil
   }
