package flow

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/clearing"
)

  func TestDayCloseFence_BlocksCrossDayWithoutTag(t *testing.T) {
	  cal := clearing.NewCalendar()
	  reg := clearing.NewDayCloseRegistry(cal)
	   now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	   reg.WithClock(func() time.Time { return now })
	  day := "2024-01-15"
	  actor := "ops-test"
	   // 1. 开日
	   dc, err := reg.OpenDay(day, actor)
	   if err != nil {
		   t.Fatalf("OpenDay: %v", err)
	   }
	  if dc.Status != clearing.DayCloseOpen {
		   t.Fatalf("expected open, got %s", dc.Status)
	  }
	  // 2. 封锁
	  dc, err = reg.Freeze(dc.ID, actor, dc.Fence.Epoch, dc.Fence.Token)
	  if err != nil {
		  t.Fatalf("Freeze: %v", err)
	   }
	  if dc.Status != clearing.DayCloseFreeze {
		  t.Fatalf("expected freezing, got %s", dc.Status)
	  }
	  // 3. 验证封锁状态
	  if !reg.IsFenced(day) {
		   t.Fatal("expected day to be fenced after Freeze")
	  }
	   // 4. 无 CrossDayTag → 应被拒绝
	   err = reg.CheckCrossDayFence(day,
		   false)
	  if err != clearing.ErrDayCloseFenced {
		  t.Fatalf("expected ErrDayCloseFenced, got %v", err)
	   }
	   // 5. 带 CrossDayTag → 应被允许
	  err = reg.CheckCrossDayFence(day, true)
	  if err != nil {
		  t.Fatalf("expected nil for tagged item, got %v", err)
	   }
  }

   func TestDayCloseFence_OpenStateAllowsAll(t *testing.T) {
	   cal := clearing.NewCalendar()
	   reg := clearing.NewDayCloseRegistry(cal)
	  day := "2024-01-16"
	   actor := "ops-test"
	   dc, err := reg.OpenDay(day, actor)
	   if err != nil {
		  t.Fatalf("OpenDay: %v", err)
	   }
	   if dc.Status != clearing.DayCloseOpen {
		   t.Fatalf("expected open, got %s", dc.Status)
	  }
	   // open 状态下任意 item 均允许
	  if err := reg.CheckCrossDayFence(day, false); err != nil {
		   t.Fatalf("open state should allow non-tagged, got %v", err)
	  }
	   if err := reg.CheckCrossDayFence(day, true); err != nil {
		   t.Fatalf("open state should allow tagged, got %v", err)
	  }
   }

  func TestDayCloseTransition_IllegalSequence(t *testing.T) {
	   cal := clearing.NewCalendar()
	   reg := clearing.NewDayCloseRegistry(cal)
	  day := "2024-01-17"
	   actor := "ops-test"
	   dc,
		   err := reg.OpenDay(day, actor)
	   if err != nil {
		  t.Fatalf("OpenDay: %v", err)
	  }
	   // 跳过 freeze 直接 close → 应失败
	   _, err = reg.Close(dc.ID, actor, dc.Fence.Epoch, dc.Fence.Token)
	   if err != clearing.ErrDayCloseTransition {
		  t.Fatalf("expected ErrDayCloseTransition, got %v", err)
	  }
   }

   func TestDayCloseReopen_RequiresReason(t *testing.T) {
	  cal := clearing.NewCalendar()
	  reg := clearing.NewDayCloseRegistry(cal)
	   day := "2024-01-18"
	   actor := "ops-test"
	   dc, err := reg.OpenDay(day, actor)
	  if err != nil {
		  t.Fatalf("OpenDay: %v", err)
	   }
	  dc,
		  err = reg.Freeze(dc.ID, actor, dc.Fence.Epoch, dc.Fence.Token)
	  if err != nil {
		  t.Fatalf("Freeze: %v", err)
	   }
	  dc, err = reg.Close(dc.ID, actor, dc.Fence.Epoch, dc.Fence.Token)
	   if err != nil {
		  t.Fatalf("Close: %v", err)
	  }
	  // 无原因 reopen → 应失败
	   _, err = reg.Reopen(dc.ID, actor, "", dc.Fence.Epoch, dc.Fence.Token)
	   if err != clearing.ErrDayCloseReopenReason {
		   t.Fatalf("expected ErrDayCloseReopenReason, got %v", err)
	   }
	  // 有原因 reopen → 应成功
	  dc2, err := reg.Reopen(dc.ID, actor, "emergency ops correction", dc.Fence.Epoch, dc.Fence.Token)
	   if err != nil {
		   t.Fatalf("Reopen: %v", err)
	   }
	  if dc2.Status != clearing.DayCloseReopened {
		  t.Fatalf("expected reopened_limited, got %s", dc2.Status)
	  }
   }
