package wrfreeze

import (
	"context"
	"time"
)

// LegacySwitchReader 读取旧版 warroom FreezeAll 布尔状态（只读接口）。
// 由 warroom.Service 实现；不得调用任何写方法。
type LegacySwitchReader interface {
	// GlobalFrozenAll 返回当前全站冻结布尔值及操作者/原因/时间（只读）。
	GlobalFrozenAll() (frozen bool, actor, reason string, at time.Time)
}

// CompatService FreezeAll 兼容投影服务。
type CompatService struct {
	legacy LegacySwitchReader
	store  Store
}

// NewCompatService 构造兼容服务。

   func NewCompatService(legacy LegacySwitchReader, store Store) *CompatService {
	  return &CompatService{legacy: legacy, store: store}
   }

// GlobalProjection 返回全站冻结状态的只读 Plan 投影。
// 若存在 scope=global 且非终态的 CascadePlan，则附加 PlanID。
// 此方法不写库，不修改任何状态。
  func (cs *CompatService) GlobalProjection(ctx context.Context) (GlobalPlanProjection, error) {
	  if cs == nil {
		  return GlobalPlanProjection{Source: "legacy_switch"}, nil
	   }
	  frozen,
		  actor, reason, at := cs.legacy.GlobalFrozenAll()
	  proj := GlobalPlanProjection{
		  Source: "legacy_switch",
		  FrozenAll: frozen,
		  Actor:     actor,
		   Reason:    reason,
		   At:        at,
	  }
	  // 尝试查找对应全局 Plan（只读）
	  if cs.store != nil {
		  plans, err := cs.store.ListPlans(ctx, "", 50)
		  if err == nil {
			   for _, p := range plans {
				  if p.Scope == ScopeGlobal && !p.Status.Terminal() {
					  proj.PlanID = p.ID
					  break
				  }
			  }
		   }
	  }
	   return proj, nil
   }

// IsFrozen 统一冻结查询（兼容旧开关 + Plan 执行态）。
// 先级：legacy FreezeAll > 计划执行中且命中 wave 目标用户。
// 仅做只读查询，不修改任何冻结状态。
   func (cs *CompatService) IsFrozen(ctx context.Context, userID string) (bool, error) {
	  if cs == nil {
		  return false, nil
	  }
	   // 旧布尔开关先
	   if frozen, _, _, _ := cs.legacy.GlobalFrozenAll(); frozen {
		   return true, nil
	   }
	   // 检查是否在活跃 Plan 的 UserSet 中
	  if cs.store != nil {
		   plans, err := cs.store.ListPlans(ctx, string(PlanExecuting), 20)
		   if err == nil {
			   for _, p := range plans {
				  for _, u := range p.UserSet {
					   if u == userID {
						   return true, nil
					   }
				  }
			  }
		   }
	   }
	  return false, nil
   }
