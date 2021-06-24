package flow

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/accrual"
	"account-assets.kucoin.net/funding/internal/clearing"
)

  func TestSuspense_SLAEscalateAndResolve(t *testing.T) {
	  base := time.Date(2024,
		  1, 15, 12, 0, 0, 0, time.UTC)
	   tck := base
	  clockFn := func() time.Time { return tck }
	   sb := clearing.NewSuspenseBook().WithClock(clockFn).WithSLA(time.Second)
	   // 1. 新建挂账分录
	   entry, err := sb.Park(
		   "batch-001", "item-001", "ex-001", "diff-001",
		   "user-123", "USDT", "500", "debit",
		   "cross-day netting diff", "ops-1",
	  )
	   if err != nil {
		   t.Fatalf("Park: %v", err)
	  }
	  if entry.Status != clearing.SuspenseOpen {
		  t.Fatalf("expected open, got %s", entry.Status)
	  }
	   // 2. 验证 SLA 内无超时
	   overdue := sb.ScanOverdue(tck)
	   if len(overdue) != 0 {
		  t.Fatalf("should not be overdue yet, got %d", len(overdue))
	  }
	  // 3. 前移时钟超过 SLA
	   tck = base.Add(2 * time.Second)
	   overdue = sb.ScanOverdue(tck)
	  if len(overdue) != 1 {
		  t.Fatalf("expected 1 overdue, got %d", len(overdue))
	  }
	  // 4. Worker 升级
	   escalated := false
	  cal := clearing.NewCalendar()
	  dcReg := clearing.NewDayCloseRegistry(cal)
	   cfg := accrual.DefaultWorkerConfig()
	   cfg.SuspenseEscalate = func(e clearing.SuspenseEntry) {
		  escalated = true
		   if e.ID != entry.ID {
			  t.Errorf("wrong escalated entry: %s", e.ID)
		  }
	   }
	  engine := accrual.NewEngine()
	   w := accrual.NewWorker(cfg, engine, dcReg, sb)
	  w.WithClock(clockFn)
	  upgraded := w.RunSuspenseEscalate()
	   if len(upgraded) != 1 {
		  t.Fatalf("expected 1 escalated, got %d", len(upgraded))
	   }
	  if !escalated {
		  t.Fatal("escalate callback not called")
	  }
	  // 5. 标记后 EscalatedAt 非空
	  e2, _ := sb.Get(entry.ID)
	  if e2.EscalatedAt == nil {
		   t.Fatal("EscalatedAt should be set after escalation")
	   }
	   // 6. Resolve
	   resolved, err := sb.Resolve(entry.ID, "ops-1", "netting reconciled", entry.Fence.Epoch, entry.Fence.Token)
	   if err != nil {
		  t.Fatalf("Resolve: %v", err)
	   }
	   if resolved.Status != clearing.SuspenseResolved {
		   t.Fatalf("expected resolved, got %s", resolved.Status)
	   }
	   // 7. HasOpenEntries 应返回 false
	  if sb.HasOpenEntries("batch-001") {
		   t.Fatal("HasOpenEntries should be false after resolve")
	  }
   }

  func TestSuspense_RejectFlow(t *testing.T) {
	   sb := clearing.NewSuspenseBook().WithSLA(time.Hour)
	   entry, err := sb.Park(
		  "batch-002", "", "", "",
		   "user-456", "BTC", "0.001", "credit",
		   "suspense parking test", "ops-2",
	   )
	   if err != nil {
		   t.Fatalf("Park: %v", err)
	  }
	  // 无 note 拒绝 → 应失败
	  _, err = sb.Reject(entry.ID, "ops-2", "", entry.Fence.Epoch, entry.Fence.Token)
	   if err != clearing.ErrSuspenseNote {
		   t.Fatalf("expected ErrSuspenseNote, got %v", err)
	   }
	   // 有 note 拒绝 → 成功
	   rejected, err := sb.Reject(entry.ID, "ops-2", "duplicate entry rejected", entry.Fence.Epoch, entry.Fence.Token)
	   if err != nil {
		  t.Fatalf("Reject: %v", err)
	  }
	   if rejected.Status != clearing.SuspenseRejected {
		  t.Fatalf("expected rejected, got %s", rejected.Status)
	   }
	   // 再次操作 → 已非 open，应失败
	  _, err = sb.Resolve(entry.ID, "ops-2", "try again", rejected.Fence.Epoch, rejected.Fence.Token)
	  if err != clearing.ErrSuspenseNotOpen {
		  t.Fatalf("expected ErrSuspenseNotOpen, got %v", err)
	   }
  }
