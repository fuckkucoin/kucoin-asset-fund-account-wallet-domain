package restoregate

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

// GenerationGate 世代闸：在 Import 开始前校验目标世代与活账水位。
// 规则：
// 1. LiveWatermark > TargetGen → ErrGenConflict（零写入）
// 2. TargetGen < SystemGen → ErrGenLag（拒绝回灌）
// 3. 两者均通过 → 允许，创建 GenGate 记录
type GenerationGate struct {
	store PlanStore
}

// NewGenerationGate 构造世代闸；store 不能为 nil。
   func NewGenerationGate(store PlanStore) *GenerationGate {
	  return &GenerationGate{store: store}
  }

// CheckAndRecord 世代合法性检查并写入审计记录。
// planID 当前 RestorePlan ID
// targetGen 计划声明的目标世代
// liveWmark 活账水位（由调用方从账本读取，不由本闸查询，避免竞态）
  func (g *GenerationGate) CheckAndRecord(ctx context.Context, planID string, targetGen, liveWmark int64) error {
	   if g == nil || g.store == nil {
		   return ErrUnavailable
	   }
	   planID = strings.TrimSpace(planID)
	  if planID == "" {
		   return ErrEmpty
	  }
	   rec, err := g.store.GetGenRecord(ctx)
	  if err != nil {
		   return err
	   }
	   systemGen := rec.CurrentGen
	  gate := GenGate{
		   ID: uuid.New().String(),
		   PlanID:        planID,
		   TargetGen:     targetGen,
		  SystemGen:     systemGen,
		   LiveWatermark: liveWmark,
		   CreatedAt: time.Now().UTC(),
	   }
	   var denyReason string
	  switch {
	  case liveWmark > targetGen:
		   denyReason = "live watermark exceeds target generation"
	   case targetGen < systemGen:
		   denyReason = "target generation lags system generation"
	  }
	  if denyReason != "" {
		  gate.Allowed = false
		   gate.DenyReason = denyReason
		   _ = g.store.CreateGenGate(ctx, gate)
		   if liveWmark > targetGen {
			   return ErrGenConflict
		   }
		   return ErrGenLag
	  }
	   gate.Allowed = true
	  if err := g.store.CreateGenGate(ctx, gate); err != nil {
		  return err
	   }
	   return nil
   }

// AdvanceSystemGen 推进系统世代（回灌成功后调用，令 currentGen = targetGen）。
   func (g *GenerationGate) AdvanceSystemGen(ctx context.Context, targetGen int64) error {
	  if g == nil || g.store == nil {
		   return ErrUnavailable
	   }
	  rec, err := g.store.GetGenRecord(ctx)
	  if err != nil {
		   return err
	   }
	  if targetGen <= rec.CurrentGen {
		   return ErrGenConflict
	  }
	   rec.CurrentGen = targetGen
	   rec.UpdatedAt = time.Now().UTC()
	   return g.store.UpsertGenRecord(ctx, rec)
   }

// CurrentSystemGen 查询当前系统世代水位。

  func (g *GenerationGate) CurrentSystemGen(ctx context.Context) (int64, error) {
	   if g == nil || g.store == nil {
		  return 0, ErrUnavailable
	  }
	  rec, err := g.store.GetGenRecord(ctx)
	   if err != nil {
		  return 0, err
	   }
	   return rec.CurrentGen, nil
   }
