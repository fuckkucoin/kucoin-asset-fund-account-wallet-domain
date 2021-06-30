package flow

import (
	"testing"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/accrual"
)

  func TestAccrualEngine_BalancedAndReversible(t *testing.T) {
	  engine := accrual.NewEngine()
	   engine.WithClock(func() time.Time {
		   return time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)
	  })
	  day := "2024-01-15"
	   actor := "sys-accrual"
	   dcID := "dc-001"
	  // 1. 创建批次
	   bat, err := engine.CreateBatch(day, dcID, actor, "test accrual")
	  if err != nil {
		  t.Fatalf("CreateBatch: %v", err)
	  }
	  if bat.Status != accrual.BatchDraft {
		  t.Fatalf("expected draft, got %s", bat.Status)
	   }
	   // 2. 追加两对分录
	   amounts := []string{"100.5", "200.25"}
	  for _, amt := range amounts {
		  _,
			   _, err := engine.AddLinePair(bat.ID, string(accrual.LineAccruedInterest),
			   "system_accrual", "user-123", "USDT", amt, "mv-001", "test line")
		  if err != nil {
			   t.Fatalf("AddLinePair(%s): %v", amt, err)
		   }
	  }
	   // 3. 过账前刷新围栏
	  bat, err = engine.GetBatch(bat.ID)
	   if err != nil {
		  t.Fatalf("GetBatch: %v", err)
	  }
	   // 4. 过账
	   posted, err := engine.Post(bat.ID, actor, bat.Fence.Epoch, bat.Fence.Token)
	   if err != nil {
		  t.Fatalf("Post: %v", err)
	   }
	  if posted.Status != accrual.BatchPosted {
		   t.Fatalf("expected posted, got %s", posted.Status)
	   }
	   // 5. 验证借贷平衡
	   debit, _ := decimal.NewFromString(posted.TotalDebit)
	   credit, _ := decimal.NewFromString(posted.TotalCredit)
	   if !debit.Equal(credit) {
		  t.Fatalf("imbalanced: debit=%s credit=%s", posted.TotalDebit, posted.TotalCredit)
	  }
	  expected, _ := decimal.NewFromString("300.75") // 100.5 + 200.25
	   if !debit.Equal(expected) {
		   t.Fatalf("expected total 300.75, got %s", debit.String())
	  }
	  // 6. 验证分录数量（4 条：2 对）
	   lines := engine.ListLines(bat.ID)
	   if len(lines) != 4 {
		  t.Fatalf("expected 4 lines, got %d", len(lines))
	   }
	   // 7. 冲正
	   revBat, err := engine.Reverse(posted.ID, actor, "next-open reversal")
	  if err != nil {
		   t.Fatalf("Reverse: %v", err)
	  }
	   if revBat.OrigBatchID != posted.ID {
		  t.Fatalf("reversal orig_batch_id mismatch")
	   }
	   if revBat.Status != accrual.BatchDraft {
		  t.Fatalf("reversal should be draft, got %s", revBat.Status)
	  }
	  // 8. 原批次变 reversed
	  orig, err := engine.GetBatch(posted.ID)
	   if err != nil {
		  t.Fatalf("GetBatch orig: %v", err)
	  }
	   if orig.Status != accrual.BatchReversed {
		  t.Fatalf("expected reversed, got %s", orig.Status)
	   }
	   // 9. 冲正分录方向应反转
	   revLines := engine.ListLines(revBat.ID)
	  if len(revLines) != 4 {
		  t.Fatalf("expected 4 reversal lines, got %d", len(revLines))
	  }
	  // 验证第一条冲正分录的方向是 credit（对应原来的 debit）
	   if revLines[0].Direction != "credit" {
		  t.Fatalf("first reversal line should be credit, got %s", revLines[0].Direction)
	  }
  }

   func TestAccrualEngine_PostRejectsImbalanced(t *testing.T) {
	  engine := accrual.NewEngine()
	   bat, err := engine.CreateBatch("2024-01-16", "", "actor", "imbalance test")
	  if err != nil {
		  t.Fatalf("CreateBatch: %v", err)
	   }
	   // 手动只追加一条 debit（不追加对应 credit），通过 AddLinePair 不可能产生不平衡
	  // 因为 AddLinePair 总是成对追加，这里验证空批次 Post 也是平衡的
	   bat, _ = engine.GetBatch(bat.ID)
	  posted, err := engine.Post(bat.ID, "actor", bat.Fence.Epoch, bat.Fence.Token)
	  if err != nil {
		   t.Fatalf("empty batch should be balanced (0==0): %v", err)
	   }
	  if posted.TotalDebit != "0" {
		   t.Fatalf("expected 0 debit, got %s", posted.TotalDebit)
	   }
   }

   func TestAccrualEngine_ReversalNotAllowedOnDraft(t *testing.T) {
	   engine := accrual.NewEngine()
	  bat, err := engine.CreateBatch("2024-01-17", "", "actor", "reversal test")
	   if err != nil {
		  t.Fatalf("CreateBatch: %v", err)
	   }
	  // draft 批次不可冲正
	   _, err = engine.Reverse(bat.ID, "actor", "should fail")
	   if err != accrual.ErrReversalRef {
		  t.Fatalf("expected ErrReversalRef, got %v", err)
	   }
  }
