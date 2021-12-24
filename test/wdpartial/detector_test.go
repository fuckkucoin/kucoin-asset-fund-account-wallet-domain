package wdpartial_test

import (
	"fmt"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/wdpartial"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

   func newTestDetector(t *testing.T, reader *wdpartial.MockBatchReader, obs *wdpartial.MockObserver) (*wdpartial.Detector, *wdpartial.MemResidueBook) {
	  t.Helper()
	   book := wdpartial.NewMemResidueBook()
	  d := wdpartial.NewDetector(reader, obs, book)
	  d.WithClock(fixedClock(time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC)))
	  return d, book
   }

  func fixedClock(t time.Time) func() time.Time {
	  return func() time.Time { return t }
   }

   func makeItems(n, failStart, failEnd, skipStart, skipEnd int) []wdpartial.BatchItemView {
	   items := make([]wdpartial.BatchItemView, n)
	   for i := 0; i < n; i++ {
		  seq := i + 1
		   status := "submitted"
		  if seq >= failStart && seq <= failEnd {
			  status = "failed"
		  } else if seq >= skipStart && seq <= skipEnd {
			  status = "skipped"
		   }
		   items[i] = wdpartial.BatchItemView{
			  Seq:    seq,
			  UserID: fmt.Sprintf("user-%03d", seq),
			  Asset:  "USDT",
			   Amount: "100",
			  Chain: "ETH",
			   ToAddress:      fmt.Sprintf("0x%040d", seq),
			  IdempotencyKey: fmt.Sprintf("idem-%03d", seq),
			   Status:         status,
			   Reason: "",
			  LedgerID: fmt.Sprintf("ledger-%03d", seq),
		  }
		   if status == "failed" {
			   items[i].Reason = "withdraw gate error"
		  } else if status == "skipped" {
			   items[i].Reason = "frozen"
		  }
	   }
	  return items
   }

// ─── 表驱动：Detect ───────────────────────────────────────────────────────

  func TestDetectorDetect(t *testing.T) {
	  t.Parallel()
	   cases := []struct {
		   name    string
		  batchID string
		  nItems  int
		  failStart int
		  failEnd   int
		   skipStart int
		   skipEnd int
		  setFreeze bool // 是否设置 FreezeOccupied=true
		   setLedger bool // 是否设置 LedgerOccupied=true
		   batchExists bool
		   wantResidues int
		  wantFailed  int
		   wantSkipped int
		   wantErr     bool
	   }{
		   {
			  name:    "验收1: 5笔中2笔failed → 2条ResidueItem",
			   batchID: "batch-acc-1",
			   nItems:  5,
			   failStart: 4,
			   failEnd:   5,
			   skipStart: 0,
			   skipEnd: 0,
			   setFreeze: true,
			  batchExists: true,
			  wantResidues: 2,
			  wantFailed:   2,
			   wantSkipped: 0,
		  },
		  {
			   name: "5笔中1失败1跳过 → 2条ResidueItem",
			   batchID: "batch-1-fail-1-skip",
			  nItems:  5,
			  failStart: 5,
			  failEnd:   5,
			   skipStart: 4,
			  skipEnd:   4,
			   setFreeze: true,
			  batchExists: true,
			  wantResidues: 2,
			  wantFailed:   1,
			   wantSkipped: 1,
		   },
		  {
			  name: "全部提交：无残留",
			   batchID: "batch-all-submitted",
			  nItems:  5,
			   failStart: 0,
			   failEnd:   0,
			  skipStart: 0,
			   skipEnd: 0,
			  batchExists:  true,
			  wantResidues: 0,
		  },
		   {
			  name: "批次不存在",
			  batchID: "batch-nonexistent",
			  batchExists: false,
			  wantErr: true,
		  },
		   {
			  name: "空批次ID",
			   batchID: "",
			   batchExists: false,
			   wantErr:     true,
		  },
		   {
			   name:    "大批次: 100笔中50失败",
			   batchID: "batch-large-100",
			  nItems:    100,
			  failStart: 51,
			   failEnd:   100,
			   setFreeze:   true,
			   batchExists: true,
			  wantResidues: 50,
			  wantFailed: 50,
		   },
		   {
			  name: "FreezeOccupied=false且LedgerOccupied=false仍创建残留",
			  batchID: "batch-no-occupy",
			   nItems:    3,
			  failStart: 3,
			   failEnd:   3,
			   setFreeze:   false,
			  setLedger:   false,
			  batchExists: true,
			   wantResidues: 1,
			  wantFailed:   1,
		  },
		   {
			   name:    "FreezeOccupied=true和LedgerOccupied=true",
			   batchID: "batch-full-occupy",
			   nItems:  3,
			  failStart: 2,
			   failEnd:   3,
			   setFreeze: true,
			  setLedger:   true,
			   batchExists: true,
			  wantResidues: 2,
			  wantFailed: 2,
		   },
		   {
			  name:    "仅1笔批次全部失败",
			  batchID: "batch-single-fail",
			   nItems:  1,
			  failStart: 1,
			  failEnd:   1,
			   setFreeze: true,
			  batchExists:  true,
			   wantResidues: 1,
			   wantFailed:   1,
		  },
	   }
	   for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			   t.Parallel()
			  reader := wdpartial.NewMockBatchReader()
			   obs := wdpartial.NewMockObserver()
			   if tc.batchExists {
				  items := makeItems(tc.nItems, tc.failStart, tc.failEnd, tc.skipStart, tc.skipEnd)
				  req := fmt.Sprintf("%d", tc.nItems*100)
				  reader.AddBatch(tc.batchID, items, req, "0", "0", "0")
				  // 设置占用状态
				  if tc.setFreeze || tc.setLedger {
					  for _, it := range items {
						  if it.Status == "failed" || it.Status == "skipped" {
							  if tc.setFreeze {
								  obs.SetFreeze(it.UserID, it.Asset, it.LedgerID, true)
							  }
							   if tc.setLedger {
								  obs.SetLedger(it.UserID, it.Asset, it.IdempotencyKey, it.LedgerID)
							   }
						  }
					   }
				  }
			  }
			   d,
				   book := newTestDetector(t, reader, obs)
			  result, err := d.Detect(tc.batchID)
			   if tc.wantErr {
				  if err == nil {
					  t.Errorf("期望 error，但未返回错误")
				  }
				  return
			   }
			  if err != nil {
				  t.Fatalf("意外错误: %v", err)
			  }
			  if result.ResidueCount != tc.wantResidues {
				   t.Errorf("残留数量: want=%d got=%d", tc.wantResidues, result.ResidueCount)
			  }
			  if result.FailedItems != tc.wantFailed {
				   t.Errorf("Failed 项数: want=%d got=%d", tc.wantFailed, result.FailedItems)
			  }
			  if result.SkippedItems != tc.wantSkipped {
				   t.Errorf("Skipped 项数: want=%d got=%d", tc.wantSkipped, result.SkippedItems)
			  }
			   // 验证持久化
			  stored, err := book.ListByBatch(tc.batchID)
			  if err != nil {
				   t.Fatalf("ListByBatch error: %v", err)
			  }
			   if len(stored) != tc.wantResidues {
				   t.Errorf("存储残留数量: want=%d got=%d", tc.wantResidues, len(stored))
			  }
		   })
	  }
  }

// ─── 幂等性 ───────────────────────────────────────────────────────────────

  func TestDetectorIdempotent(t *testing.T) {
	  t.Parallel()
	  reader := wdpartial.NewMockBatchReader()
	   obs := wdpartial.NewMockObserver()
	   batchID := "batch-idem-001"
	   items := makeItems(5, 4, 5, 0, 0)
	  reader.AddBatch(batchID, items, "500", "300", "0", "200")
	  d, book := newTestDetector(t, reader, obs)
	   // 第一次检测
	  r1, err := d.Detect(batchID)
	  if err != nil {
		   t.Fatalf("第一次检测错误: %v", err)
	   }
	  if r1.ResidueCount != 2 {
		   t.Fatalf("第一次检测: want 2 residues, got %d", r1.ResidueCount)
	   }
	  // 第二次检测（幂等）
	   r2, err := d.Detect(batchID)
	   if err != nil {
		  t.Fatalf("第二次检测错误: %v", err)
	  }
	   if len(r2.NewResidues) != 0 {
		   t.Errorf("幂等检测不应产生新残留: got %d new residues", len(r2.NewResidues))
	   }
	  // 存储数量不变
	  stored, _ := book.ListByBatch(batchID)
	  if len(stored) != 2 {
		  t.Errorf("幂等后存储数量应为 2, got %d", len(stored))
	  }
   }

// ─── RefreshOccupancy ───────────────────────────────────────────────────

  func TestDetectorRefreshOccupancy(t *testing.T) {
	   t.Parallel()
	   reader := wdpartial.NewMockBatchReader()
	  obs := wdpartial.NewMockObserver()
	  batchID := "batch-refresh-001"
	  items := makeItems(3, 3, 3, 0, 0)
	  // 初始设置为有冻结占用
	  obs.SetFreeze(items[2].UserID, items[2].Asset, items[2].LedgerID, true)
	   reader.AddBatch(batchID, items, "300", "200", "0", "100")
	  d, book := newTestDetector(t, reader, obs)
	  _, err := d.Detect(batchID)
	  if err != nil {
		   t.Fatalf("Detect error: %v", err)
	   }
	  stored, _ := book.ListByBatch(batchID)
	   if len(stored) != 1 {
		   t.Fatalf("期望 1 条残留, got %d", len(stored))
	  }
	   if !stored[0].FreezeOccupied {
		  t.Errorf("初始 FreezeOccupied 应为 true")
	   }
	   // 清除冻结占用
	  obs.SetFreeze(items[2].UserID, items[2].Asset, items[2].LedgerID, false)
	  // 刷新
	  if err := d.RefreshOccupancy(stored[0]); err != nil {
		   t.Fatalf("RefreshOccupancy error: %v", err)
	   }
	   if stored[0].FreezeOccupied {
		  t.Errorf("刷新后 FreezeOccupied 应为 false")
	   }
   }
