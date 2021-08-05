package keycer

import (
	"context"
	"sync/atomic"
)

// RotateGenCounter 进程内轮换世代号计数器。
// keyrotgate.Gate 内部维护自己的 atomic.Int64；此处提供独立的可注入代次供使用。

type RotateGenCounter struct {
	v atomic.Int64
}

// Next 返回下一个世代号（单调递增，从 1 开始）。
   func (c *RotateGenCounter) Next() int64 {
	   return c.v.Add(1)
   }

// Current 返回当前世代号（不递增）。
   func (c *RotateGenCounter) Current() int64 {
	  return c.v.Load()
   }

// Reset 重置计数器（仅用于）。
  func (c *RotateGenCounter) Reset(v int64) {
	  c.v.Store(v)
   }

// ──────────────────────────────────────────────────────────────────────────────
// SlotReaderAdapter — keycer.Service → keyrotgate.SlotReader 适配器
// ──────────────────────────────────────────────────────────────────────────────
// SlotReaderAdapter 将 keycer.Service 适配为 keyrotgate.SlotReader 接口。
// keyrotgate 包不直接依赖 keycer 包类型；通过此适配器注入。
type SlotReaderAdapter struct {
	svc *Service
}

// NewSlotReaderAdapter 构造 SlotReaderAdapter。

   func NewSlotReaderAdapter(svc *Service) *SlotReaderAdapter {
	   return &SlotReaderAdapter{svc: svc}
  }

// ReadSlot 读取指定槽的指纹与退役状态（keyrotgate.SlotReader 实现）。
// 返回：fingerprint（槽指纹；若 Retired/Empty 则为空字符串）、slotRetired（是否已退役）、err。
  func (a *SlotReaderAdapter) ReadSlot(_ context.Context, slotID string) (fingerprint string, slotRetired bool, err error) {
	  if a == nil || a.svc == nil {
		  return "", false, ErrUnavailable
	   }
	   slot, err := a.svc.Get(slotID)
	   if err != nil {
		  return "", false, err
	  }
	  if slot.State == SlotRetired {
		   return slot.Fingerprint, true, nil
	   }
	  return slot.Fingerprint, false, nil
  }

// InflightGuard 在途签名守卫——由签名路径调用，防止轮换竞态。
// 调用方（签名仪式入口）在持有 keycer 锁前调用 Check 以确认源槽未在轮换执行中。

type InflightGuard struct {
	svc *Service
}

// NewInflightGuard 构造 InflightGuard。
   func NewInflightGuard(svc *Service) *InflightGuard {
	   return &InflightGuard{svc: svc}
   }

// SlotIsRetired 检查槽是否已退役；退役槽拒绝新的签名/解锁操作。

   func (g *InflightGuard) SlotIsRetired(slotID string) (bool, error) {
	   if g == nil || g.svc == nil {
		  return false, ErrUnavailable
	   }
	   slot, err := g.svc.Get(slotID)
	  if err != nil {
		  return false, err
	   }
	   return slot.State == SlotRetired, nil
  }

// FingerprintOf 返回槽当前指纹；供签名请求绑定时快照用。
   func (g *InflightGuard) FingerprintOf(slotID string) (string, error) {
	   if g == nil || g.svc == nil {
		   return "", ErrUnavailable
	   }
	  slot, err := g.svc.Get(slotID)
	  if err != nil {
		  return "", err
	  }
	   return slot.Fingerprint, nil
  }
