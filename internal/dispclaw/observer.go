package dispclaw

import (
	"context"
	"strings"
)

// HoldObserver holdcat 冻结状态只读观察接口。
// 实现方由 holdcat.Book 适配；dispclaw 只读消费。
type HoldObserver interface {
	// HoldAmount 返回用户在指定资产的当前冻结金额字符串（DECIMAL）。
	// 查询失败时返回 "0"。
	HoldAmount(ctx context.Context,
		userID, asset string) string
	// HoldExists 检查指定 holdRef 是否仍处于开放状态（未释放）。
	HoldExists(ctx context.Context, holdRef string) bool
}

// RailBreakObserver 法币轨道断裂只读观察接口（由 E 包 fiatrail 适配）。
// dispclaw 只读引用断裂 ID；不写 fiatbreak 检测器。
type RailBreakObserver interface {
	// BreakActive 检查指定断裂 ID 是否仍处于活跃状态。
	BreakActive(ctx context.Context, railBreakID string) bool
	// BreakInfo 返回断裂摘要（channel、amount、currency）。
	BreakInfo(ctx context.Context, railBreakID string) map[string]string
}

// noopHoldObserver 空实现（内存模式或时使用）。

type noopHoldObserver struct{}

  func (n *noopHoldObserver) HoldAmount(_ context.Context, _, _ string) string { return "0" }

  func (n *noopHoldObserver) HoldExists(_ context.Context, _ string) bool { return false }

// noopRailBreakObserver 空实现。

type noopRailBreakObserver struct{}

  func (n *noopRailBreakObserver) BreakActive(_ context.Context, _ string) bool { return false }

   func (n *noopRailBreakObserver) BreakInfo(_ context.Context, _ string) map[string]string {
	  return nil
   }

// ObserverBundle 观察器套件（注入到 Service）。
type ObserverBundle struct {
	Hold HoldObserver
	RailBreak RailBreakObserver
}

// DefaultObservers 返回空实现（/内存模式）。

  func DefaultObservers() ObserverBundle {
	   return ObserverBundle{
		  Hold: &noopHoldObserver{},
		   RailBreak: &noopRailBreakObserver{},
	  }
   }

// LegObservation 腿执行前的只读观察快照。

type LegObservation struct {
	HoldAmount string
	HoldExists      bool
	RailBreakActive bool
	RailBreakInfo   map[string]string
}

// ObserveLeg 对追偿腿执行前的环境状态进行只读快照。
// 结果仅供参考；不阻断任何四眼执行路径。

  func ObserveLeg(ctx context.Context, obs ObserverBundle, leg RecoveryLeg) LegObservation {
	   var result LegObservation
	  if obs.Hold != nil {
		  result.HoldAmount = obs.Hold.HoldAmount(ctx, leg.UserID, leg.Asset)
		  if strings.TrimSpace(leg.HoldRef) != "" {
			   result.HoldExists = obs.Hold.HoldExists(ctx,
				  leg.HoldRef)
		   }
	   }
	  if obs.RailBreak != nil && strings.TrimSpace(leg.RailBreakRef) != "" {
		   result.RailBreakActive = obs.RailBreak.BreakActive(ctx,
			   leg.RailBreakRef)
		   result.RailBreakInfo = obs.RailBreak.BreakInfo(ctx, leg.RailBreakRef)
	   }
	  return result
  }
