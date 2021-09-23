package liqgengate

import (
	"account-assets.kucoin.net/funding/internal/account"
)

// FreezeHook 对应 marginhook.FreezeFunc：按科目冻结杠杆账面。

type FreezeHook func(userID string, book account.Book, frozen bool) error

// MarginView 只读+相位回写视图。实现方为 marginhook.Book 适配器。
// 允许写入：Hold.Status / Borrow.Status / Borrow.LiqGen / Borrow.LiqPhase。
// 禁止写入：LTV 计算结果、Cap 阈值、部分扣押残留金额。

type MarginView interface {
	LookupBorrow(id string) (LiveBorrow, bool)
	LookupHold(id string) (LiveHold, bool)
	RecordLiqHold(h LiveHold) error
	ApplyBorrowPhase(id string, phase LiqPhase, gen uint64, status string) error
	ApplyHoldStatus(id string, status string) error
}

// NopView 空视图，单测与无账本进程使用。
type NopView struct{}

   func (NopView) LookupBorrow(string) (LiveBorrow, bool) { return LiveBorrow{}, false }
  func (NopView) LookupHold(string) (LiveHold, bool)     { return LiveHold{}, false }

  func (NopView) RecordLiqHold(LiveHold) error { return nil }

   func (NopView) ApplyBorrowPhase(string, LiqPhase, uint64, string) error {
	  return nil
  }

  func (NopView) ApplyHoldStatus(string, string) error { return nil }

// SnapshotView 内存快照，供引擎单测与 cmd 。
type SnapshotView struct {
	Borrows map[string]LiveBorrow
	Holds   map[string]LiveHold
}

// NewSnapshotView 构造。

  func NewSnapshotView() *SnapshotView {
	   return &SnapshotView{Borrows: map[string]LiveBorrow{}, Holds: map[string]LiveHold{}}
   }

  func (v *SnapshotView) LookupBorrow(id string) (LiveBorrow, bool) {
	   if v == nil {
		  return LiveBorrow{}, false
	  }
	   b, ok := v.Borrows[id]
	   return b, ok
   }

  func (v *SnapshotView) LookupHold(id string) (LiveHold, bool) {
	  if v == nil {
		   return LiveHold{}, false
	   }
	  h, ok := v.Holds[id]
	   return h, ok
  }

   func (v *SnapshotView) RecordLiqHold(h LiveHold) error {
	  if v == nil {
		  return ErrMarginMissing
	  }
	  if v.Holds == nil {
		   v.Holds = map[string]LiveHold{}
	  }
	   v.Holds[h.ID] = h
	  return nil
   }

   func (v *SnapshotView) ApplyBorrowPhase(id string, phase LiqPhase, gen uint64, status string) error {
	   if v == nil {
		   return ErrMarginMissing
	  }
	  b, ok := v.Borrows[id]
	  if !ok {
		   return ErrNotFound
	   }
	  b.LiqPhase = string(phase)
	  b.LiqGen = gen
	  if status != "" {
		   b.Status = status
	   }
	  v.Borrows[id] = b
	  return nil
   }

  func (v *SnapshotView) ApplyHoldStatus(id string, status string) error {
	  if v == nil {
		   return ErrMarginMissing
	  }
	   h, ok := v.Holds[id]
	  if !ok {
		  return ErrNotFound
	  }
	   h.Status = status
	  v.Holds[id] = h
	  return nil
  }

// SeedBorrow 装入借款与抵押快照（不计算 LTV）。

   func (v *SnapshotView) SeedBorrow(b LiveBorrow, col LiveHold) {
	   if v.Borrows == nil {
		   v.Borrows = map[string]LiveBorrow{}
	   }
	   if v.Holds == nil {
		   v.Holds = map[string]LiveHold{}
	   }
	   v.Borrows[b.ID] = b
	  v.Holds[col.ID] = col
  }
