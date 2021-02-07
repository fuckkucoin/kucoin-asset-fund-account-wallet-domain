package amldesk_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/amldesk"
	"account-assets.kucoin.net/funding/internal/amlhold"
)

// mockHoldChecker amlhold.Service 实现 HoldChecker 接口。
type mockHoldChecker struct {
	holdSvc *amlhold.Service
}

   func (m *mockHoldChecker) CheckCloseAllowed(caseID, closerActor string) error {
	   return m.holdSvc.CheckCloseAllowed(caseID, closerActor)
  }

   func (m *mockHoldChecker) CheckSameActorReleaseThenClose(caseID, closerActor string) error {
	   return m.holdSvc.CheckSameActorReleaseThenClose(caseID, closerActor)
  }

// TestHoldGateBlocksClose 检疫 hold 未释放时结案必须失败。
  func TestHoldGateBlocksClose(t *testing.T) {
	  t.Parallel()
	   holdSvc := amlhold.New()
	  checker := &mockHoldChecker{holdSvc: holdSvc}
	  book := amldesk.NewBook()
	   bwh := amldesk.NewBookWithHold(book, checker)
	  // 开 AML Case
	  rec, err := book.Open("user-gate", "", "sar", "可疑资金", "ops-alice")
	   if err != nil {
		  t.Fatal("open case:", err)
	  }
	  // 开检疫 hold
	   _, err = holdSvc.OpenHold(rec.ID, "user-gate", "USDT", "5000", "ops-alice", 86400)
	   if err != nil {
		   t.Fatal("open hold:", err)
	   }
	  // 试图结案（reviewer-bob，hold 未释放）
	   _, err = bwh.CloseFourEyeWithHoldCheck(rec.ID, "reviewer-bob", "close", rec.Fence.Epoch, rec.Fence.Token, "")
	   if err == nil {
		  t.Fatal("hold 未释放时结案应失败，但返回 nil")
	  }
   }

// TestHoldGateAllowsCloseAfterRelease hold released 后结案成功。
   func TestHoldGateAllowsCloseAfterRelease(t *testing.T) {
	   t.Parallel()
	  holdSvc := amlhold.New()
	  checker := &mockHoldChecker{holdSvc: holdSvc}
	  book := amldesk.NewBook()
	  bwh := amldesk.NewBookWithHold(book, checker)
	   rec, err := book.Open("user-gate2", "", "sar", "资金异常", "ops-alice")
	  if err != nil {
		  t.Fatal(err)
	  }
	  h, err := holdSvc.OpenHold(rec.ID, "user-gate2", "BTC", "0.5", "ops-alice", 86400)
	   if err != nil {
		  t.Fatal(err)
	  }
	  // 申请并批准解冻
	   ticket, err := holdSvc.RequestRelease(h.ID, rec.ID, "ops-charlie", "审核完毕", "")
	   if err != nil {
		  t.Fatal(err)
	   }
	   _, _, err = holdSvc.ApproveRelease(ticket.ID, "reviewer-diana", ticket.FenceEpoch, ticket.FenceToken)
	  if err != nil {
		   t.Fatal(err)
	  }
	  // 结案（reviewer-eve，不同于 ops-charlie）
	   cur, _ := book.Get(rec.ID)
	   _, err = bwh.CloseFourEyeWithHoldCheck(rec.ID, "reviewer-eve", "close", cur.Fence.Epoch, cur.Fence.Token, "")
	   if err != nil {
		   t.Fatalf("hold released 后结案应成功，得到: %v", err)
	  }
   }

// TestHoldGateMutexSameActor 同一 actor 申请解冻再结案必须失败。

   func TestHoldGateMutexSameActor(t *testing.T) {
	  t.Parallel()
	  holdSvc := amlhold.New()
	  checker := &mockHoldChecker{holdSvc: holdSvc}
	   book := amldesk.NewBook()
	   bwh := amldesk.NewBookWithHold(book, checker)
	  rec, err := book.Open("user-gate3", "", "sar", "异常转账", "ops-alice")
	  if err != nil {
		   t.Fatal(err)
	   }
	   h, err := holdSvc.OpenHold(rec.ID, "user-gate3", "ETH", "3.0", "ops-alice", 86400)
	   if err != nil {
		   t.Fatal(err)
	  }
	  // ops-bob 申请解冻
	   _, err = holdSvc.RequestRelease(h.ID, rec.ID, "ops-bob", "申请", "")
	  if err != nil {
		  t.Fatal(err)
	   }
	   // 批准解冻（reviewer-carol）
	  tickets := holdSvc.TicketsByHold(h.ID)
	  if len(tickets) == 0 {
		   t.Fatal("没有 ticket")
	  }
	   _, _, err = holdSvc.ApproveRelease(tickets[0].ID, "reviewer-carol", tickets[0].FenceEpoch, tickets[0].FenceToken)
	  if err != nil {
		   t.Fatal(err)
	   }
	  // ops-bob 试图结案（已申请过解冻，应触发互斥）
	  cur, _ := book.Get(rec.ID)
	  _, err = bwh.CloseFourEyeWithHoldCheck(rec.ID, "ops-bob", "close", cur.Fence.Epoch, cur.Fence.Token, "")
	   if err == nil {
		   t.Fatal("同一 actor 申请解冻后不得结案，但返回 nil")
	  }
  }

// TestStatusResolvedCheckBlocked StatusResolvedCheck 阻断。

   func TestStatusResolvedCheckBlocked(t *testing.T) {
	  t.Parallel()
	  holdSvc := amlhold.New()
	  checker := &mockHoldChecker{holdSvc: holdSvc}
	  book := amldesk.NewBook()
	   bwh := amldesk.NewBookWithHold(book, checker)
	  rec, err := book.Open("user-gate4", "", "kyt_hold", "KYT 拦截", "ops-alice")
	   if err != nil {
		  t.Fatal(err)
	  }
	   _, err = holdSvc.OpenHold(rec.ID, "user-gate4", "USDT", "1000", "ops-alice", 86400)
	  if err != nil {
		  t.Fatal(err)
	   }
	  err = bwh.StatusResolvedCheck(rec.ID, "reviewer-bob")
	  if err == nil {
		  t.Fatal("hold 未释放时 StatusResolvedCheck 应返回错误")
	   }
  }
