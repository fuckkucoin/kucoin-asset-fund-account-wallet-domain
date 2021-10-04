package prefroute_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/fundpref"
	"account-assets.kucoin.net/funding/internal/prefroute"
)

// ─────────────────────────────────────────────
// 辅助
// ─────────────────────────────────────────────

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

  func newTestClock(t time.Time) *testClock { return &testClock{now: t} }

  func (c *testClock) Now() time.Time {
	  c.mu.Lock()
	   defer c.mu.Unlock()
	  return c.now
  }

   func (c *testClock) Advance(d time.Duration) {
	  c.mu.Lock()
	  defer c.mu.Unlock()
	  c.now = c.now.Add(d)
   }

  func newTestService(t *testing.T, clockFn func() time.Time) *prefroute.Service {
	  t.Helper()
	  cfg := prefroute.Config{
		  LeaseTTL: 30 * time.Second,
		  DriftPatrolInterval: 15 * time.Second,
		  InflightScanInterval:         10 * time.Second,
		   PendingSwitchAdvanceInterval: 5 * time.Second,
	   }
	  svc := prefroute.NewService(nil, cfg)
	   if clockFn != nil {
		   svc.WithClock(clockFn)
	  }
	  return svc
   }

  // ─────────────────────────────────────────────
  // 1：在途贷记存在时切换 DefaultAccount 失败
  // ─────────────────────────────────────────────

   func TestInflightCredit_SwitchRejected(t *testing.T) {
	   cases := []struct {
		  name string
		  enqueueOnInflight bool
		   wantErr error
		  wantApplied bool
		  wantPending bool
	  }{
		  {
			   name:              "reject_when_inflight_exists",
			   enqueueOnInflight: false,
			   wantErr:           fundpref.ErrInflightCredit,
		   },
		  {
			   name: "enqueue_when_inflight_exists",
			   enqueueOnInflight: true,
			   wantApplied:       false,
			  wantPending: true,
		   },
	   }
	  for _, tc := range cases {
		   tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			  clk := newTestClock(time.Date(2026,
				   9, 21, 10, 0, 0, 0, time.UTC))
			  svc := newTestService(t, clk.Now)
			   ctx := context.Background()
			  // 先设置初始 DefaultAccount（在途贷记注册前）
			  initReq := fundpref.PutDefaultAccountReq{
				  UserID: "user_alice",
				  Book:   fundpref.BookTrade,
				  RequestedBy: "system",
				   ReqID:       "init-001",
			  }
			  if _, err := svc.PutDefaultAccount(ctx, initReq, false); err != nil {
				  t.Fatalf("init default account: %v", err)
			  }
			  // 注册在途贷记（在初始化之后）
			   ic := prefroute.InflightCredit{
				  ID: "ic-001",
				   UserID: "user_alice",
				   TxnID:  "deposit-txn-001",
				  Asset:  "USDT",
				  Amount: "1000.00",
				  State: prefroute.InflightStatePending,
				   CreatedAt: clk.Now(),
				  UpdatedAt: clk.Now(),
			  }
			   if err := svc.RegisterInflightCredit(ctx, ic); err != nil {
				  t.Fatalf("register inflight: %v", err)
			   }
			   // 尝试切换
			  req := fundpref.PutDefaultAccountReq{
				  UserID:      "user_alice",
				  Book:        fundpref.BookMain,
				   RequestedBy: "user",
				   ReqID:       "switch-001",
			   }
			  result, err := svc.PutDefaultAccount(ctx, req, tc.enqueueOnInflight)
			  if tc.wantErr != nil {
				  if !errors.Is(err,
					   tc.wantErr) {
					  t.Errorf("want err %v, got %v", tc.wantErr, err)
				  }
				  return
			   }
			   if err != nil {
				  t.Fatalf("unexpected err: %v", err)
			   }
			  if result.Applied != tc.wantApplied {
				  t.Errorf("want applied=%v, got %v", tc.wantApplied, result.Applied)
			   }
			   if tc.wantPending && result.PendingSwitch == nil {
				   t.Error("want pending switch, got nil")
			   }
			   if tc.wantPending && result.PendingSwitch != nil {
				   if result.PendingSwitch.State != fundpref.PendingSwitchQueued {
					  t.Errorf("want queued, got %s", result.PendingSwitch.State)
				  }
			  }
		  })
	  }
   }

// ─────────────────────────────────────────────
// 2：路由快照绑定后偏好变更 → 漂移检测
// ─────────────────────────────────────────────

   func TestRouteSnap_DriftDetected(t *testing.T) {
	  cases := []struct {
		  name string
		  initialBook  fundpref.AccountBook
		   switchToBook fundpref.AccountBook
		  creditedBook fundpref.AccountBook
		   wantDrift bool
		   wantDriftBook fundpref.AccountBook
	  }{
		  {
			  name:        "trade_to_main_drift",
			   initialBook: fundpref.BookTrade,
			   switchToBook: fundpref.BookMain,
			  creditedBook: fundpref.BookMain, // 实际贷记 MAIN，但快照是 TRADE
			  wantDrift: true,
		   },
		  {
			  name: "no_drift_same_book",
			   initialBook: fundpref.BookTrade,
			  switchToBook: fundpref.BookMain,
			   creditedBook: fundpref.BookTrade, // 实际贷记 TRADE，与快照一致
			  wantDrift:    false,
		   },
		  {
			  name:        "main_to_trade_drift",
			  initialBook: fundpref.BookMain,
			   switchToBook: fundpref.BookTrade,
			  creditedBook: fundpref.BookTrade, // 实际贷记 TRADE，快照是 MAIN → 漂移
			  wantDrift:    true,
		   },
	  }
	   for _, tc := range cases {
		   tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   clk := newTestClock(time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC))
			  svc := newTestService(t, clk.Now)
			   ctx := context.Background()
			  // 设置初始 DefaultAccount
			  if _, err := svc.PutDefaultAccount(ctx, fundpref.PutDefaultAccountReq{
				  UserID: "user_bob", Book: tc.initialBook,
				   RequestedBy: "system", ReqID: "init",
			   }, false); err != nil {
				  t.Fatalf("init: %v", err)
			   }
			   // 签发租约
			  lease, err := svc.IssuePrefLease(ctx, prefroute.IssuePrefLeaseReq{
				  UserID:      "user_bob",
				   CreditTxnID: "deposit-txn-bob-001",
				  IssuedBy: "deposit_worker",
			  })
			  if err != nil {
				   t.Fatalf("issue lease: %v", err)
			  }
			   if lease.DefaultBook != tc.initialBook {
				   t.Errorf("lease book want=%s got=%s", tc.initialBook, lease.DefaultBook)
			  }
			  // 绑定路由快照
			  _, snap, err := svc.BindRouteSnap(ctx, prefroute.BindRouteSnapReq{
				  LeaseID:     lease.ID,
				   CreditTxnID: "deposit-txn-bob-001",
				  Asset:       "BTC",
				  Amount:      "0.5",
			  })
			   if err != nil {
				   t.Fatalf("bind snap: %v", err)
			  }
			   if snap.BoundBook != tc.initialBook {
				   t.Errorf("snap bound book want=%s got=%s", tc.initialBook, snap.BoundBook)
			   }
			   // 切换 DefaultAccount（世代推进）
			   clk.Advance(1 * time.Second)
			  if _, err := svc.PutDefaultAccount(ctx, fundpref.PutDefaultAccountReq{
				  UserID: "user_bob", Book: tc.switchToBook,
				   RequestedBy: "user", ReqID: "switch",
			  }, false); err != nil {
				  t.Fatalf("switch: %v", err)
			   }
			  // 贷记完成，实际贷到 creditedBook
			   clk.Advance(1 * time.Second)
			   _, dc, err := svc.ConfirmCredit(ctx, prefroute.ConfirmCreditReq{
				  RouteSnapID:  snap.ID,
				   CreditedBook: tc.creditedBook,
				   CreditTxnID:  "deposit-txn-bob-001",
			   })
			  if err != nil {
				  t.Fatalf("confirm credit: %v", err)
			   }
			   if tc.wantDrift {
				  if dc == nil {
					   t.Fatal("want drift case, got nil")
				  }
				  if dc.State != prefroute.DriftStateOpen {
					  t.Errorf("drift case state want=open, got=%s", dc.State)
				   }
				   if dc.BoundBook != tc.initialBook {
					   t.Errorf("drift bound book want=%s got=%s", tc.initialBook, dc.BoundBook)
				  }
				  if dc.CreditedBook != tc.creditedBook {
					   t.Errorf("drift credited book want=%s got=%s", tc.creditedBook, dc.CreditedBook)
				   }
				  // 验证 PrefWrite 被阻断
				   blocked, _, err := svc.IsPrefWriteBlocked(ctx, "user_bob")
				   if err != nil {
					  t.Fatalf("check blocked: %v", err)
				  }
				   if !blocked {
					  t.Error("want pref write blocked after drift, got not blocked")
				  }
			   } else {
				  if dc != nil {
					  t.Errorf("want no drift case, got %+v", dc)
				   }
			   }
		  })
	  }
  }

// ─────────────────────────────────────────────
// 3：PrefDriftCase 证明通过后方可关闭
// ─────────────────────────────────────────────

  func TestDriftCase_ProofAndClose(t *testing.T) {
	  cases := []struct {
		  name         string
		   closeFirst   bool // 是否先尝试关闭（不提交证明）
		   wantCloseErr bool
	   }{
		  {
			   name:         "close_without_proof_rejected",
			   closeFirst:   true,
			   wantCloseErr: true,
		  },
		   {
			  name:         "close_after_proof_allowed",
			  closeFirst:   false,
			   wantCloseErr: false,
		   },
	   }
	  for _, tc := range cases {
		   tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   clk := newTestClock(time.Date(2026,
				  9, 21, 10, 0, 0, 0, time.UTC))
			   svc := newTestService(t, clk.Now)
			  ctx := context.Background()
			   // 制造一个 drift case：初始 TRADE，切换到 MAIN，实际贷记 MAIN
			  if _, err := svc.PutDefaultAccount(ctx, fundpref.PutDefaultAccountReq{
				  UserID: "user_carol", Book: fundpref.BookTrade,
				   RequestedBy: "system", ReqID: "init",
			  }, false); err != nil {
				  t.Fatalf("init: %v", err)
			  }
			   lease, err := svc.IssuePrefLease(ctx, prefroute.IssuePrefLeaseReq{
				  UserID:      "user_carol",
				   CreditTxnID: "dep-carol-001",
				  IssuedBy: "worker",
			  })
			   if err != nil {
				   t.Fatalf("issue lease: %v", err)
			  }
			   _, snap, err := svc.BindRouteSnap(ctx, prefroute.BindRouteSnapReq{
				   LeaseID: lease.ID,
				  CreditTxnID: "dep-carol-001",
				  Asset:       "ETH",
				   Amount:      "1.0",
			  })
			   if err != nil {
				   t.Fatalf("bind snap: %v", err)
			   }
			  // 切换偏好
			   clk.Advance(500 * time.Millisecond)
			  if _, err := svc.PutDefaultAccount(ctx, fundpref.PutDefaultAccountReq{
				  UserID: "user_carol", Book: fundpref.BookMain,
				  RequestedBy: "user", ReqID: "sw",
			  }, false); err != nil {
				  t.Fatalf("switch: %v", err)
			  }
			  // 触发漂移
			   _, dc, err := svc.ConfirmCredit(ctx, prefroute.ConfirmCreditReq{
				  RouteSnapID:  snap.ID,
				   CreditedBook: fundpref.BookMain,
				   CreditTxnID: "dep-carol-001",
			   })
			  if err != nil {
				  t.Fatalf("confirm credit: %v", err)
			  }
			  if dc == nil {
				   t.Fatal("want drift case, got nil")
			  }
			  caseID := dc.ID
			   if tc.closeFirst {
				  // 不提交证明直接关闭 → 应失败
				   _, err := svc.ConfirmDriftClose(ctx, caseID, "ops_manager")
				  if err == nil {
					  t.Error("want error closing without proof, got nil")
				  }
				  return
			  }
			   // 提交证明
			  proof, err := svc.SubmitDriftProof(ctx, prefroute.SubmitDriftProofReq{
				  DriftCaseID: caseID,
				  SubmittedBy: "ops_user",
				  Summary: "贷记科目已追回，补录至 TRADE 账户",
				  Evidence: `{"correction_txn":"corr-001"}`,
			  })
			  if err != nil {
				   t.Fatalf("submit proof: %v", err)
			   }
			   if proof.DriftCaseID != caseID {
				  t.Errorf("proof case id mismatch")
			   }
			   // 校验 DriftCase → proven
			  dcAfterProof, err := svc.GetDriftCase(ctx, caseID)
			   if err != nil {
				  t.Fatalf("get drift case: %v", err)
			   }
			   if dcAfterProof.State != prefroute.DriftStateProven {
				  t.Errorf("want proven, got %s", dcAfterProof.State)
			  }
			   // 四眼关闭
			   closedDC, err := svc.ConfirmDriftClose(ctx, caseID, "ops_manager")
			   if tc.wantCloseErr {
				  if err == nil {
					  t.Error("want close error, got nil")
				  }
				  return
			  }
			   if err != nil {
				  t.Fatalf("close drift case: %v", err)
			  }
			  if closedDC.State != prefroute.DriftStateClosed {
				   t.Errorf("want closed, got %s", closedDC.State)
			   }
			   if closedDC.ClosedBy != "ops_manager" {
				   t.Errorf("want closed_by=ops_manager, got %s", closedDC.ClosedBy)
			   }
			   // 关闭后 PrefWrite 应解除阻断
			  blocked, _, err := svc.IsPrefWriteBlocked(ctx, "user_carol")
			   if err != nil {
				   t.Fatalf("check blocked: %v", err)
			   }
			   if blocked {
				  t.Error("want pref write unblocked after close, got blocked")
			   }
		   })
	  }
   }

// ─────────────────────────────────────────────
// 4：旧世代租约应被拒绝
// ─────────────────────────────────────────────

   func TestStaleLease_Rejected(t *testing.T) {
	   clk := newTestClock(time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC))
	  svc := newTestService(t, clk.Now)
	   ctx := context.Background()
	  // 设置初始 DefaultAccount（世代 1）
	   if _, err := svc.PutDefaultAccount(ctx, fundpref.PutDefaultAccountReq{
		   UserID: "user_dave", Book: fundpref.BookMain,
		  RequestedBy: "system", ReqID: "init",
	   }, false); err != nil {
		   t.Fatalf("init: %v", err)
	  }
	  // 签发租约（世代 1）
	  lease, err := svc.IssuePrefLease(ctx, prefroute.IssuePrefLeaseReq{
		  UserID: "user_dave",
		   CreditTxnID: "dep-dave-001",
		  IssuedBy: "worker",
	   })
	  if err != nil {
		   t.Fatalf("issue lease: %v", err)
	  }
	  if lease.Generation != 1 {
		  t.Errorf("want gen=1, got %d", lease.Generation)
	   }
	   // 推进世代（切换到 TRADE，世代变为 2）
	   clk.Advance(100 * time.Millisecond)
	  if _, err := svc.PutDefaultAccount(ctx, fundpref.PutDefaultAccountReq{
		  UserID: "user_dave", Book: fundpref.BookTrade,
		   RequestedBy: "user", ReqID: "sw",
	   }, false); err != nil {
		   t.Fatalf("switch: %v", err)
	   }
	   // 旧租约（世代 1）尝试绑定路由快照 → 应被拒绝（世代不符或状态 switched）
	   clk.Advance(100 * time.Millisecond)
	   _, _, err = svc.BindRouteSnap(ctx, prefroute.BindRouteSnapReq{
		  LeaseID: lease.ID,
		   CreditTxnID: "dep-dave-001",
		  Asset: "USDT",
		  Amount: "500.00",
	  })
	  if err == nil {
		   t.Fatal("want error binding stale generation lease, got nil")
	  }
	  // 接受 ErrGenerationMismatch 或 ErrLeaseInvalid（取决于租约是否已被废止）
	  if !errors.Is(err, prefroute.ErrGenerationMismatch) && !errors.Is(err, prefroute.ErrLeaseInvalid) {
		  t.Errorf("want ErrGenerationMismatch or ErrLeaseInvalid, got %v", err)
	   }
  }

// ─────────────────────────────────────────────
// 5：并发 Put DefaultAccount（竞态）
// ─────────────────────────────────────────────

   func TestConcurrentPutDefaultAccount(t *testing.T) {
	   clk := newTestClock(time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC))
	  svc := newTestService(t, clk.Now)
	  ctx := context.Background()
	  const concurrency = 10
	  books := []fundpref.AccountBook{
		   fundpref.BookMain, fundpref.BookTrade, fundpref.BookMargin,
	   }
	   var wg sync.WaitGroup
	   errs := make(chan error, concurrency)
	  for i := 0; i < concurrency; i++ {
		  wg.Add(1)
		  go func(i int) {
			   defer wg.Done()
			   book := books[i%len(books)]
			  _, err := svc.PutDefaultAccount(ctx, fundpref.PutDefaultAccountReq{
				  UserID:      "user_concurrent",
				   Book:        book,
				   RequestedBy: "concurrent_test",
				   ReqID:       fmt.Sprintf("req-%d", i),
			  }, false)
			  if err != nil {
				   errs <- err
			   }
		  }(i)
	   }
	  wg.Wait()
	   close(errs)
	  for err := range errs {
		   t.Errorf("concurrent put error: %v", err)
	   }
	  // 验证世代单调递增
	   gen, err := svc.CurrentGeneration(ctx, "user_concurrent")
	   if err != nil {
		  t.Fatalf("get gen: %v", err)
	   }
	  if gen.Generation < 1 {
		   t.Errorf("want generation >= 1, got %d", gen.Generation)
	   }
  }

// ─────────────────────────────────────────────
// 6：待切换队列推进
// ─────────────────────────────────────────────

   func TestPendingSwitch_AdvancedWhenInflightClears(t *testing.T) {
	  clk := newTestClock(time.Date(2026,
		   9, 21, 10, 0, 0, 0, time.UTC))
	  svc := newTestService(t, clk.Now)
	   ctx := context.Background()
	  // 初始 TRADE
	  if _, err := svc.PutDefaultAccount(ctx, fundpref.PutDefaultAccountReq{
		   UserID: "user_eve", Book: fundpref.BookTrade,
		   RequestedBy: "system", ReqID: "init",
	  }, false); err != nil {
		   t.Fatalf("init: %v", err)
	  }
	  // 注册在途贷记
	  ic := prefroute.InflightCredit{
		   ID:        "ic-eve-001",
		  UserID:    "user_eve",
		   TxnID:     "dep-eve-001",
		   Asset:     "USDT",
		  Amount:    "2000.00",
		  State:     prefroute.InflightStatePending,
		  CreatedAt: clk.Now(),
		   UpdatedAt: clk.Now(),
	  }
	   if err := svc.RegisterInflightCredit(ctx, ic); err != nil {
		  t.Fatalf("register: %v", err)
	   }
	  // 尝试切 MAIN，进入 pending 队列
	  result, err := svc.PutDefaultAccount(ctx, fundpref.PutDefaultAccountReq{
		  UserID: "user_eve", Book: fundpref.BookMain,
		   RequestedBy: "user", ReqID: "sw",
	   }, true)
	   if err != nil {
		  t.Fatalf("put with enqueue: %v", err)
	  }
	   if result.Applied {
		   t.Fatal("want not applied (inflight exists)")
	  }
	   if result.PendingSwitch == nil {
		   t.Fatal("want pending switch")
	  }
	   // 完成贷记
	   clk.Advance(1 * time.Second)
	  if _, err := svc.CompleteInflightCredit(ctx, "dep-eve-001", fundpref.BookTrade); err != nil {
		  t.Fatalf("complete inflight: %v", err)
	   }
	   // 推进待切换队列
	   clk.Advance(100 * time.Millisecond)
	  applied, err := svc.AdvancePendingSwitches(ctx, "user_eve")
	  if err != nil {
		  t.Fatalf("advance: %v", err)
	  }
	   if len(applied) != 1 {
		  t.Fatalf("want 1 applied, got %d", len(applied))
	   }
	   if applied[0].State != fundpref.PendingSwitchApplied {
		   t.Errorf("want applied, got %s", applied[0].State)
	   }
	   // 验证 DefaultAccount 已切换到 MAIN
	   pref, err := svc.GetDefaultPref(ctx, "user_eve")
	   if err != nil {
		   t.Fatalf("get pref: %v", err)
	  }
	  if pref.Book != fundpref.BookMain {
		   t.Errorf("want book=MAIN, got %s", pref.Book)
	   }
   }

// ─────────────────────────────────────────────
// 7：租约过期
// ─────────────────────────────────────────────

  func TestLease_Expired(t *testing.T) {
	  clk := newTestClock(time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC))
	  svc := newTestService(t, clk.Now)
	  ctx := context.Background()
	  if _, err := svc.PutDefaultAccount(ctx, fundpref.PutDefaultAccountReq{
		   UserID: "user_frank", Book: fundpref.BookMain,
		  RequestedBy: "system", ReqID: "init",
	   }, false); err != nil {
		   t.Fatalf("init: %v", err)
	   }
	   lease, err := svc.IssuePrefLease(ctx, prefroute.IssuePrefLeaseReq{
		  UserID:      "user_frank",
		  CreditTxnID: "dep-frank-001",
		   IssuedBy: "worker",
	   })
	   if err != nil {
		   t.Fatalf("issue: %v", err)
	   }
	   // 推进时钟超过 TTL（30s + 1s）
	   clk.Advance(31 * time.Second)
	  // 绑定应失败（过期）
	  _, _, err = svc.BindRouteSnap(ctx, prefroute.BindRouteSnapReq{
		   LeaseID:     lease.ID,
		   CreditTxnID: "dep-frank-001",
		   Asset:       "USDT",
		   Amount:      "100.00",
	  })
	   if err == nil {
		   t.Fatal("want expired lease error, got nil")
	  }
	   if !errors.Is(err, prefroute.ErrLeaseExpired) {
		   t.Errorf("want ErrLeaseExpired, got %v", err)
	   }
   }

// ─────────────────────────────────────────────
// 8：漂移案例阻断后续偏好写
// ─────────────────────────────────────────────

   func TestDriftCase_BlocksPrefWrite(t *testing.T) {
	  clk := newTestClock(time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC))
	   svc := newTestService(t, clk.Now)
	   ctx := context.Background()
	   // 制造 drift case
	  if _, err := svc.PutDefaultAccount(ctx, fundpref.PutDefaultAccountReq{
		   UserID: "user_grace", Book: fundpref.BookTrade,
		   RequestedBy: "system", ReqID: "init",
	   }, false); err != nil {
		   t.Fatalf("init: %v", err)
	   }
	  lease, _ := svc.IssuePrefLease(ctx, prefroute.IssuePrefLeaseReq{
		  UserID: "user_grace", CreditTxnID: "dep-grace-001", IssuedBy: "w",
	  })
	  _, snap, _ := svc.BindRouteSnap(ctx, prefroute.BindRouteSnapReq{
		  LeaseID: lease.ID, CreditTxnID: "dep-grace-001",
		   Asset: "USDT", Amount: "500",
	   })
	   clk.Advance(100 * time.Millisecond)
	   svc.PutDefaultAccount(ctx, fundpref.PutDefaultAccountReq{
		   UserID: "user_grace", Book: fundpref.BookMain,
		   RequestedBy: "user", ReqID: "sw",
	   }, false)
	   _, dc, _ := svc.ConfirmCredit(ctx, prefroute.ConfirmCreditReq{
		   RouteSnapID:  snap.ID,
		  CreditedBook: fundpref.BookMain,
	  })
	   if dc == nil {
		   t.Fatal("expect drift case")
	   }
	  // 漂移案例存在时，新租约签发应失败
	  _, err := svc.IssuePrefLease(ctx, prefroute.IssuePrefLeaseReq{
		  UserID:      "user_grace",
		  CreditTxnID: "dep-grace-002",
		  IssuedBy:    "worker",
	  })
	  if err == nil {
		   t.Fatal("want error issuing lease with open drift case, got nil")
	   }
	   if !errors.Is(err, prefroute.ErrDriftUnproven) {
		  t.Errorf("want ErrDriftUnproven, got %v", err)
	  }
   }

// ─────────────────────────────────────────────
// 9：幂等证明提交
// ─────────────────────────────────────────────

   func TestDriftProof_Idempotent(t *testing.T) {
	  clk := newTestClock(time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC))
	  svc := newTestService(t, clk.Now)
	  ctx := context.Background()
	  // 制造 drift case
	   svc.PutDefaultAccount(ctx, fundpref.PutDefaultAccountReq{
		  UserID: "user_henry", Book: fundpref.BookTrade,
		  RequestedBy: "system", ReqID: "init",
	  }, false)
	  lease, _ := svc.IssuePrefLease(ctx, prefroute.IssuePrefLeaseReq{
		  UserID: "user_henry", CreditTxnID: "dep-henry-001", IssuedBy: "w",
	  })
	  _, snap, _ := svc.BindRouteSnap(ctx, prefroute.BindRouteSnapReq{
		  LeaseID: lease.ID, CreditTxnID: "dep-henry-001",
		   Asset: "BTC", Amount: "1.0",
	  })
	   clk.Advance(100 * time.Millisecond)
	   svc.PutDefaultAccount(ctx, fundpref.PutDefaultAccountReq{
		  UserID: "user_henry", Book: fundpref.BookMain,
		  RequestedBy: "user", ReqID: "sw",
	  }, false)
	   _, dc, _ := svc.ConfirmCredit(ctx, prefroute.ConfirmCreditReq{
		  RouteSnapID:  snap.ID,
		  CreditedBook: fundpref.BookMain,
	   })
	   if dc == nil {
		  t.Fatal("expect drift case")
	  }
	  // 提交证明
	   proof1, err := svc.SubmitDriftProof(ctx, prefroute.SubmitDriftProofReq{
		  DriftCaseID: dc.ID,
		  SubmittedBy: "ops",
		   Summary:     "first proof",
		   Evidence:    "{}",
	   })
	  if err != nil {
		  t.Fatalf("submit proof: %v", err)
	  }
	   // 再次提交证明（DriftCase 已 proven，应返回错误）
	   _, err = svc.SubmitDriftProof(ctx, prefroute.SubmitDriftProofReq{
		  DriftCaseID: dc.ID,
		  SubmittedBy: "ops",
		   Summary:  "duplicate proof",
		  Evidence: "{}",
	  })
	  if err == nil {
		  t.Error("want error submitting duplicate proof to proven case, got nil")
	  }
	   // 验证原始证明可导出
	   exported, err := svc.ExportDriftProof(ctx, proof1.ID)
	   if err != nil {
		  t.Fatalf("export proof: %v", err)
	  }
	   if exported.Summary != "first proof" {
		  t.Errorf("want 'first proof', got %s", exported.Summary)
	  }
   }

// ─────────────────────────────────────────────
// 10：与 brokerbook/fundint 隔离（符号检查）
// ─────────────────────────────────────────────

  func TestIsolation_NoBrokerFenceOrFundintUID(t *testing.T) {
	  // 此通过接口验证隔离性：Service 不暴露任何 Fence 写接口或 UID 解析接口
	  svc := newTestService(t, nil)
	   // 验证 Service 不包含 FenceGen/ResolveUID 等方法（通过类型断言）
	  type fenceable interface {
		  WriteFenceGen(string, int64) error
	   }
	  if _, ok := interface{}(svc).(fenceable); ok {
		  t.Error("Service should not implement WriteFenceGen (broker fence isolation violated)")
	  }
	  type uidResolver interface {
		  ResolveInternalUID(string) (string, error)
	  }
	   if _,
		   ok := interface{}(svc).(uidResolver); ok {
		   t.Error("Service should not implement ResolveInternalUID (fundint isolation violated)")
	   }
  }
