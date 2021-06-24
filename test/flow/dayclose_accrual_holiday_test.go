package flow

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/accrual"
	"account-assets.kucoin.net/funding/internal/clearing"
)

  func TestHolidayCalendar_SkipAndCatchUp(t *testing.T) {
	   cal := clearing.NewCalendar()
	  // 手动添加假日：2024-01-20 和 2024-01-21
	  _ = cal.Add("2024-01-20")
	   _ = cal.Add("2024-01-21")
	  reg := clearing.NewDayCloseRegistry(cal)
	   actor := "sys-daycut"
	  // 1. 假日直接拒绝
	  _, err := reg.OpenDay("2024-01-20", actor)
	  if err != clearing.ErrDayCloseHoliday {
		   t.Fatalf("expected ErrDayCloseHoliday for holiday day, got %v", err)
	  }
	  // 2. HolidayCatchUp 登记
	   dc, err := reg.OpenHolidayCatchUp("2024-01-20", "2024-01-22", actor)
	  if err != nil {
		   t.Fatalf("OpenHolidayCatchUp: %v", err)
	  }
	   if !dc.IsHoliday {
		   t.Fatal("expected IsHoliday=true")
	  }
	  if dc.CatchUpDay != "2024-01-22" {
		  t.Fatalf("expected CatchUpDay=2024-01-22, got %s", dc.CatchUpDay)
	   }
	  if dc.Status != clearing.DayCloseClosed {
		   t.Fatalf("expected closed for holiday, got %s", dc.Status)
	   }
	  // 3. 次一工作日 2024-01-22 可正常开日
	  dc22, err := reg.OpenDay("2024-01-22", actor)
	  if err != nil {
		  t.Fatalf("OpenDay 2024-01-22: %v", err)
	  }
	  if dc22.Status != clearing.DayCloseOpen {
		  t.Fatalf("expected open for next biz day, got %s", dc22.Status)
	   }
   }

  func TestWorker_RunDayCut_HolidayBranch(t *testing.T) {
	  cal := clearing.NewCalendar()
	  _ = cal.Add("2024-01-20")
	  dcReg := clearing.NewDayCloseRegistry(cal)
	   suspBook := clearing.NewSuspenseBook()
	  engine := accrual.NewEngine()
	  now := time.Date(2024,
		  1, 20, 16, 0, 0, 0, time.UTC)
	   clockFn := func() time.Time { return now }
	   dcReg.WithClock(clockFn)
	  engine.WithClock(clockFn)
	  cfg := accrual.DefaultWorkerConfig()
	   w := accrual.NewWorker(cfg, engine, dcReg, suspBook)
	  w.WithClock(clockFn)
	  dc, bat, err := w.RunDayCut("2024-01-20", "sys-daycut", nil, "")
	   if err != nil {
		   t.Fatalf("RunDayCut holiday: %v", err)
	   }
	   if !dc.IsHoliday {
		  t.Fatal("expected IsHoliday=true from RunDayCut")
	  }
	   if bat.ID != "" {
		  t.Fatal("holiday skip should produce no accrual batch")
	   }
	  if dc.CatchUpDay == "" {
		   t.Fatal("CatchUpDay should be set for holiday")
	   }
  }

  func TestWorker_RunDayCut_NormalDay(t *testing.T) {
	   cal := clearing.NewCalendar()
	   dcReg := clearing.NewDayCloseRegistry(cal)
	   suspBook := clearing.NewSuspenseBook()
	  engine := accrual.NewEngine()
	  now := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)
	   clockFn := func() time.Time { return now }
	  dcReg.WithClock(clockFn)
	  engine.WithClock(clockFn)
	  cfg := accrual.DefaultWorkerConfig()
	   w := accrual.NewWorker(cfg,
		  engine, dcReg, suspBook)
	  w.WithClock(clockFn)
	   // 无在途 items → 无应计批次
	   dc, bat, err := w.RunDayCut("2024-01-15", "sys-daycut", nil, "")
	  if err != nil {
		  t.Fatalf("RunDayCut: %v", err)
	   }
	   if dc.Status != clearing.DayCloseClosed {
		   t.Fatalf("expected closed after RunDayCut, got %s", dc.Status)
	   }
	   if bat.ID != "" {
		   t.Fatalf("no cross-day items, expected empty batch, got %s", bat.ID)
	   }
  }

  func TestWorker_RunDayCut_WithCrossDay(t *testing.T) {
	  cal := clearing.NewCalendar()
	   dcReg := clearing.NewDayCloseRegistry(cal)
	   suspBook := clearing.NewSuspenseBook()
	  engine := accrual.NewEngine()
	  now := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)
	   clockFn := func() time.Time { return now }
	  dcReg.WithClock(clockFn)
	  engine.WithClock(clockFn)
	   cfg := accrual.DefaultWorkerConfig()
	   w := accrual.NewWorker(cfg, engine, dcReg, suspBook)
	   w.WithClock(clockFn)
	  // 构造跨日在途 item（ValueDate > 2024-01-15）
	   crossDayItems := []clearing.Item{
		   {
			  ID:      "item-cross-001",
			  BatchID: "batch-001",
			   UserID: "user-123",
			  Asset:  "USDT",
			   Amount: "1000",
			   MovementID: "mv-001",
			   Status:     clearing.StatusPending,
			   ValueDate: time.Date(2024, // T+2 跨日
				   1, 17, 0, 0, 0, 0, time.UTC),
		   },
	  }
	   dc, bat, err := w.RunDayCut("2024-01-15", "sys-daycut", crossDayItems, "batch-001")
	  if err != nil {
		   t.Fatalf("RunDayCut with cross-day: %v", err)
	  }
	  if dc.Status != clearing.DayCloseClosed {
		   t.Fatalf("expected closed, got %s", dc.Status)
	   }
	   if bat.ID == "" {
		  t.Fatal("expected accrual batch for cross-day items")
	  }
	  if bat.Status != accrual.BatchPosted {
		  t.Fatalf("expected posted batch, got %s", bat.Status)
	   }
	  // 验证应计批次平衡
	  if bat.TotalDebit != bat.TotalCredit {
		  t.Fatalf("accrual batch imbalanced: debit=%s credit=%s", bat.TotalDebit, bat.TotalCredit)
	  }
  }
