package p2pabandon

import (
	"context"
	"time"
)

// HoldObserver holdcat 只读观察接口（由调用方实现并注入）。
// 本包不写 holdcat 核心 FSM，通过此接口只读消费 hold 状态。
type HoldObserver interface {
	// ObserveHold 按 hold_id 读取 hold 行（只读）。
	// 若 hold 不存在或已释放，返回 open=false。
	ObserveHold(ctx context.Context, userID, holdID string) (HoldObservation, error)
	// ObserveOpenHolds 枚举用户当前所有 open hold（只读）。
	ObserveOpenHolds(ctx context.Context, userID string) ([]HoldObservation, error)
	// HoldBirth 返回 hold 的创建时间（由 holdcat.Row.CreatedAt 解析）。
	// 用于计算 HoldAge；若 hold 不存在返回 zero time。
	HoldBirth(ctx context.Context, planID, holdID string) (time.Time, error)
}

// CooldownObserver 出金冷却只读观察接口（由调用方实现并注入）。
// 本包不改冷却语义，通过此接口只读检查冷却状态。
type CooldownObserver interface {
	// ObserveCooldown 读取用户出金冷却状态（只读）。
	ObserveCooldown(ctx context.Context, userID string) (CooldownObservation, error)
}

// OrderTimeoutSource 订单超时信号来源接口（由调用方实现并注入）。
// 本包不写 P2P 撮合；调用方（如订单管理服务）注入超时信号后，
// Worker 调用 PollTimeoutSignals 获取待处理信号并创建 AbandonPlan。

type OrderTimeoutSource interface {
	// PollTimeoutSignals 拉取超时信号（每次最多 limit 条）。
	// 调用方在信号被处理后调用 AckTimeoutSignal 确认消费。
	PollTimeoutSignals(ctx context.Context, limit int) ([]OrderTimeoutSignal, error)
	// AckTimeoutSignal 确认信号已消费（幂等）。
	AckTimeoutSignal(ctx context.Context, orderID string) error
}

// ─────────────────────────── 静态默认实现（/空实现） ───────────────────────────
// NoopHoldObserver holdcat 空实现（用；不依赖holdcat）。
type NoopHoldObserver struct{}

  func (n NoopHoldObserver) ObserveHold(_ context.Context, _, holdID string) (HoldObservation, error) {
	   return HoldObservation{HoldID: holdID, Open: true, CreatedAt: time.Now().UTC()}, nil
   }

   func (n NoopHoldObserver) ObserveOpenHolds(_ context.Context, userID string) ([]HoldObservation, error) {
	   return nil, nil
  }

   func (n NoopHoldObserver) HoldBirth(_ context.Context, planID, holdID string) (time.Time, error) {
	   return time.Time{}, nil
   }

// NoopCooldownObserver 冷却空实现（用）。
type NoopCooldownObserver struct{}

  func (n NoopCooldownObserver) ObserveCooldown(_ context.Context, userID string) (CooldownObservation, error) {
	   return CooldownObservation{UserID: userID, Locked: false}, nil
   }

// StaticTimeoutSource 静态超时信号来源（用，持有一组预设信号）。
type StaticTimeoutSource struct {
	Signals []OrderTimeoutSignal
	Acked map[string]bool
}

   func NewStaticTimeoutSource(sigs []OrderTimeoutSignal) *StaticTimeoutSource {
	  return &StaticTimeoutSource{
		   Signals: sigs,
		  Acked:   make(map[string]bool),
	   }
   }

  func (s *StaticTimeoutSource) PollTimeoutSignals(_ context.Context, limit int) ([]OrderTimeoutSignal, error) {
	  out := make([]OrderTimeoutSignal, 0, limit)
	  for _, sig := range s.Signals {
		  if s.Acked[sig.OrderID] {
			  continue
		  }
		   out = append(out, sig)
		  if len(out) >= limit {
			  break
		  }
	   }
	  return out, nil
  }

   func (s *StaticTimeoutSource) AckTimeoutSignal(_ context.Context, orderID string) error {
	  s.Acked[orderID] = true
	  return nil
  }

// ─────────────────────────── 带时钟注入的 HoldObserver 包装 ───────────────────────────
// TimedHoldObserver 包装 HoldObserver，注入可控时钟（场景下 hold 年龄）。
// 在中，可设置 BirthOverride 来特定诞生时间。
type TimedHoldObserver struct {
	Inner HoldObserver
	BirthOverride map[string]time.Time // hold_id → birth time（覆盖业务 hold 创建时间）
}

   func NewTimedHoldObserver(inner HoldObserver) *TimedHoldObserver {
	  return &TimedHoldObserver{Inner: inner, BirthOverride: make(map[string]time.Time)}
  }

   func (t *TimedHoldObserver) ObserveHold(ctx context.Context, userID, holdID string) (HoldObservation, error) {
	  return t.Inner.ObserveHold(ctx, userID, holdID)
   }

   func (t *TimedHoldObserver) ObserveOpenHolds(ctx context.Context, userID string) ([]HoldObservation, error) {
	   return t.Inner.ObserveOpenHolds(ctx, userID)
  }

  func (t *TimedHoldObserver) HoldBirth(ctx context.Context, planID, holdID string) (time.Time, error) {
	   if birth, ok := t.BirthOverride[holdID]; ok {
		  return birth, nil
	   }
	  return t.Inner.HoldBirth(ctx, planID, holdID)
  }

   func (t *TimedHoldObserver) SetBirthOverride(holdID string, birth time.Time) {
	  if t.BirthOverride == nil {
		  t.BirthOverride = make(map[string]time.Time)
	  }
	   t.BirthOverride[holdID] = birth
   }
