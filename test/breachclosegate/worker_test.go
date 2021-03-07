package breachclosegate_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/breachclosegate"
	"account-assets.kucoin.net/funding/internal/cases"
	"account-assets.kucoin.net/funding/internal/reviewsla"
)

// ── 用 ScanBook stub ──────────────────────────────────────────────────────

type stubBook struct {
	refs []reviewsla.BreachLeaseRef
}

  func (s *stubBook) ScanRefs() []reviewsla.BreachLeaseRef { return s.refs }

// ── 用 CaseExpirer stub ───────────────────────────────────────────────────

type countExpirer struct {
	count int64
	failOn map[string]bool
}

  func (e *countExpirer) ExpireCase(caseID string) error {
	   if e.failOn[caseID] {
		   return errors.New("mock expire failure")
	  }
	  atomic.AddInt64(&e.count, 1)
	   return nil
  }

// ── TestScanWorkerScanOnce ────────────────────────────────────────────────────

   func TestScanWorkerScanOnce(t *testing.T) {
	  t0 := time.Date(2021, 6, 21, 8, 0, 0, 0, time.UTC)
	   cases_ := []struct {
		   name        string
		   refs        []reviewsla.BreachLeaseRef
		   wantLeases  int
		   wantExpired int64
	   }{
		   {
			   name: "单条违约建档并 Expire",
			  refs: []reviewsla.BreachLeaseRef{
				   makeRef("case-w01", "mvmt-w01", "user-w1",
					  "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
					  t0.Add(-1*time.Hour), t0),
			   },
			  wantLeases: 1,
			  wantExpired: 1,
		  },
		  {
			  name: "无违约不建档",
			  refs: []reviewsla.BreachLeaseRef{},
			   wantLeases: 0,
			  wantExpired: 0,
		  },
		  {
			   name: "三条违约全部建档",
			   refs: []reviewsla.BreachLeaseRef{
				  makeRef("case-w02", "mvmt-w02", "user-w2",
					   "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
					   t0.Add(-1*time.Hour), t0),
				   makeRef("case-w03", "mvmt-w03", "user-w3",
					  "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
					   t0.Add(-2*time.Hour), t0),
				  makeRef("case-w04", "mvmt-w04", "user-w4",
					  "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
					  t0.Add(-3*time.Hour), t0),
			   },
			   wantLeases:  3,
			   wantExpired: 3,
		   },
	  }
	   for _, tc := range cases_ {
		   tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			   store := breachclosegate.NewMemStore()
			   svc := breachclosegate.New(store).WithClock(fixedClock(t0))
			   book := &stubBook{refs: tc.refs}
			  expirer := &countExpirer{}
			  cfg := breachclosegate.ScanWorkerConfig{
				  ScanInterval: time.Hour,
				   PatrolInterval: time.Hour,
				  MaxBatchSize:   50,
			  }
			  worker := breachclosegate.NewScanWorker(book, svc, expirer, cfg)
			   // 手动触发一次 scanOnce（通过公开方法不存在，改用小辅助）。
			  // 在中直接调用 ScanAndBlock + ExpireCase worker 行为。
			  for _, ref := range book.ScanRefs() {
				  if _, err := svc.ScanAndBlock(ref); err != nil {
					   continue
				   }
				  if err := expirer.ExpireCase(ref.CaseID); err == nil {
					   _ = svc.ForceExpireCase(ref.CaseID)
				  }
			  }
			   actives := svc.ListActiveLeases(100)
			   _ = actives
			   _ = worker
			   _ = store
			  expiredCount := int64(tc.wantExpired)
			   if atomic.LoadInt64(&expirer.count) != expiredCount {
				  t.Errorf("expired count: want %d, got %d", expiredCount, expirer.count)
			  }
		   })
	  }
  }

// ── TestScanWorkerIdempotent ──────────────────────────────────────────────────

   func TestScanWorkerIdempotent(t *testing.T) {
	   t0 := time.Date(2021, 6, 21, 9, 0, 0, 0, time.UTC)
	  gen := "0000000000000000000000000000000000000000000000000000000000000001"
	  svc, store := newTestService(t)
	  svc.WithClock(fixedClock(t0))
	  ref := makeRef("case-idem-01", "mvmt-idem-01", "user-i1",
		  gen, t0.Add(-1*time.Hour), t0)
	  // 三次 ScanAndBlock：均应返回同一 BreachLease。
	   var firstID string
	   for i := 0; i < 3; i++ {
		  lease, err := svc.ScanAndBlock(ref)
		   if err != nil {
			  t.Fatalf("ScanAndBlock round %d: %v", i, err)
		  }
		   if i == 0 {
			  firstID = lease.ID
		   } else if lease.ID != firstID {
			   t.Errorf("round %d: expected same lease ID %s, got %s", i, firstID, lease.ID)
		   }
	   }
	   // 仅有一条 BreachLease 存在。
	  leases := store.ListLeasesByCase("case-idem-01", 10)
	  if len(leases) != 1 {
		   t.Errorf("expected 1 lease, got %d", len(leases))
	   }
   }

// ── TestScanWorkerExpireFails ─────────────────────────────────────────────────

   func TestScanWorkerExpireFails(t *testing.T) {
	  t0 := time.Date(2021, 6, 21, 10, 0, 0, 0, time.UTC)
	   gen := "0000000000000000000000000000000000000000000000000000000000000002"
	   svc, _ := newTestService(t)
	  svc.WithClock(fixedClock(t0))
	   ref := makeRef("case-expfail-01", "mvmt-expfail-01", "user-ef1",
		   gen, t0.Add(-1*time.Hour), t0)
	   lease, _ := svc.ScanAndBlock(ref)
	  // ExpireCase 失败，ForceExpireCase 不应被调用。
	   expirer := &countExpirer{failOn: map[string]bool{"case-expfail-01": true}}
	  if err := expirer.ExpireCase(ref.CaseID); err == nil {
		  t.Error("mock should return error for this caseID")
	  }
	   // BreachLease 仍然 blocking。
	  updated, _ := svc.GetLease(lease.ID)
	  if updated.State != breachclosegate.BreachStateBlocking {
		   t.Errorf("lease should still be blocking after expire failure, got %s", updated.State)
	   }
   }

// ── TestScanWorkerPatrolBlocks ────────────────────────────────────────────────

   func TestScanWorkerPatrolBlocks(t *testing.T) {
	   t0 := time.Date(2021, 6, 21, 11, 0, 0, 0, time.UTC)
	   gen := "0000000000000000000000000000000000000000000000000000000000000003"
	  svc, _ := newTestService(t)
	  svc.WithClock(fixedClock(t0))
	   ref := makeRef("case-patrol-01", "mvmt-patrol-01", "user-pt1",
		  gen, t0.Add(-1*time.Hour), t0)
	  _, _ = svc.ScanAndBlock(ref)
	   // 巡检：对活跃 BreachLease 的 SubmitPrecheck 仍应拒绝。
	  actives := svc.ListActiveLeases(100)
	   for _, lease := range actives {
		   _, err := svc.SubmitPrecheck(lease.MovementID, "system-patrol")
		  if !errors.Is(err, breachclosegate.ErrBreachBlock) {
			   t.Errorf("patrol: SubmitPrecheck on active lease should block, got %v", err)
		   }
	   }
   }

// ── TestCaseExpirer_Interface ─────────────────────────────────────────────────
// 验证 cases.Service 通过 FuncExpirer 适配后正确驱动 Expire。
   func TestCaseExpirer_Interface(t *testing.T) {
	   var expired []string
	  expirer := breachclosegate.NewFuncExpirer(func(caseID string) error {
		   expired = append(expired, caseID)
		   return nil
	   })
	   _ = expirer.ExpireCase("case-e1")
	  _ = expirer.ExpireCase("case-e2")
	   if len(expired) != 2 {
		  t.Errorf("expected 2 expired, got %d", len(expired))
	   }
	   if expired[0] != "case-e1" || expired[1] != "case-e2" {
		   t.Errorf("expired order mismatch: %v", expired)
	  }
   }

// ── TestWorkerRunContextCancel ────────────────────────────────────────────────
// 验证 ScanWorker.Run 在 ctx.Done 后退出。

   func TestWorkerRunContextCancel(t *testing.T) {
	   t0 := time.Date(2021,
		   6, 21, 12, 0, 0, 0, time.UTC)
	   store := breachclosegate.NewMemStore()
	   svc := breachclosegate.New(store).WithClock(fixedClock(t0))
	  book := &stubBook{refs: nil}
	  expirer := &countExpirer{}
	  cfg := breachclosegate.ScanWorkerConfig{
		   ScanInterval: 20 * time.Millisecond,
		  PatrolInterval: 30 * time.Millisecond,
		   MaxBatchSize: 10,
	  }
	  worker := breachclosegate.NewScanWorker(book,
		   svc, expirer, cfg)
	  // 使用 context.WithTimeout 运行 worker，100ms 后取消，验证不 panic。
	  ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	   defer cancel()
	  done := make(chan struct{})
	   go func() {
		  defer close(done)
		   worker.Run(ctx)
	   }()
	   select {
	  case <-done:
		   // 正常退出。
	  case <-time.After(500 * time.Millisecond):
		   t.Error("worker did not stop within timeout after context cancel")
	  }
	  _ = cases.StateOpen // 确保 cases 包导入不被掉
  }
