package hotbreach_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/hotbreach"
)

// ─────────────────────────────────────────────
// 辅助：可控时钟
// ─────────────────────────────────────────────

type fakeClock struct {
	now time.Time
}

   func (f *fakeClock) Now() time.Time { return f.now }

  func (f *fakeClock) Advance(d time.Duration) { f.now = f.now.Add(d) }

   func newClock(t time.Time) *fakeClock { return &fakeClock{now: t} }

// ─────────────────────────────────────────────
// 辅助：构造用 Service（纯内存，无热库存）
// ─────────────────────────────────────────────

  func newTestSvc(t *testing.T, cl *fakeClock) *hotbreach.Service {
	  t.Helper()
	  svc := hotbreach.NewService(nil, nil, nil)
	  if cl != nil {
		   svc.WithClock(cl.Now)
	   }
	  return svc
   }

// ─────────────────────────────────────────────
// 1. Guard 检查：无租约时挡写
// ─────────────────────────────────────────────

  func TestGuard_NoLease_Allowed(t *testing.T) {
	   svc := newTestSvc(t, nil)
	   ctx := context.Background()
	  // 无活跃 fence、无击穿 → 应允许
	   err := svc.Guard(ctx, hotbreach.GuardReq{Asset: "USDT", Party: "withdraw-001"})
	  if err != nil {
		   t.Fatalf("Guard should allow when no fence/breach, got: %v", err)
	  }
  }

  func TestGuard_BreachGated_Blocks(t *testing.T) {
	  svc := newTestSvc(t, nil)
	  ctx := context.Background()
	   // 签发租约（会创建 DrainFence）
	   lease, err := svc.IssueLease(ctx, hotbreach.IssueLeaseReq{Asset: "BTC", IssuedBy: "req-001"})
	  if err != nil {
		  t.Fatalf("IssueLease: %v", err)
	  }
	   // 直接注入击穿案例（并发扣减超 cap）
	  _, berr := svc.InjectBreachCase(ctx, "BTC", lease.ID, "concurrent-party-001")
	  if berr != nil {
		   t.Fatalf("InjectBreachCase: %v", berr)
	   }
	   // Patrol 推进 open→gated
	   if _, perr := svc.Patrol(ctx); perr != nil {
		   t.Fatalf("Patrol: %v", perr)
	   }
	  // Guard 应该挡写（ErrBreachGated）
	  gerr := svc.Guard(ctx, hotbreach.GuardReq{Asset: "BTC", Party: "withdraw-002"})
	  if !errors.Is(gerr, hotbreach.ErrBreachGated) {
		   t.Fatalf("Guard should return ErrBreachGated after breach gated, got: %v", gerr)
	   }
  }

// ─────────────────────────────────────────────
// 2. 租约签发与消费主路径
// ─────────────────────────────────────────────

   func TestIssueLease_Basic(t *testing.T) {
	  svc := newTestSvc(t, nil)
	   ctx := context.Background()
	   lease, err := svc.IssueLease(ctx, hotbreach.IssueLeaseReq{
		  Asset: "USDT",
		   IssuedBy: "withdraw-req-001",
	  })
	  if err != nil {
		   t.Fatalf("IssueLease: %v", err)
	  }
	  if lease.ID == "" {
		   t.Fatal("lease ID should not be empty")
	   }
	   if lease.State != hotbreach.LeaseIssued {
		   t.Fatalf("expected state=issued, got %s", lease.State)
	  }
	   if lease.Asset != "USDT" {
		  t.Fatalf("expected asset=USDT, got %s", lease.Asset)
	   }
  }

  func TestConsumeLease_Valid(t *testing.T) {
	   svc := newTestSvc(t, nil)
	   ctx := context.Background()
	  lease, err := svc.IssueLease(ctx, hotbreach.IssueLeaseReq{Asset: "USDT", IssuedBy: "req-002"})
	  if err != nil {
		   t.Fatalf("IssueLease: %v", err)
	  }
	  consumed, cerr := svc.ConsumeLease(ctx, hotbreach.ConsumeLeaseReq{
		  LeaseID: lease.ID,
		   Asset:   "USDT",
		   ConsumedBy: "txn-001",
	   })
	  if cerr != nil {
		  t.Fatalf("ConsumeLease: %v", cerr)
	  }
	  if consumed.State != hotbreach.LeaseConsumed {
		  t.Fatalf("expected state=consumed, got %s", consumed.State)
	  }
	  if consumed.ConsumedBy != "txn-001" {
		  t.Fatalf("expected consumed_by=txn-001, got %s", consumed.ConsumedBy)
	  }
   }

   func TestConsumeLease_DoubleConsume(t *testing.T) {
	  svc := newTestSvc(t, nil)
	   ctx := context.Background()
	   lease, _ := svc.IssueLease(ctx, hotbreach.IssueLeaseReq{Asset: "USDT", IssuedBy: "req-003"})
	   _, _ = svc.ConsumeLease(ctx, hotbreach.ConsumeLeaseReq{LeaseID: lease.ID, Asset: "USDT", ConsumedBy: "txn-001"})
	  // 第二次消费应失败
	   _, err := svc.ConsumeLease(ctx, hotbreach.ConsumeLeaseReq{
		   LeaseID: lease.ID,
		   Asset: "USDT",
		   ConsumedBy: "txn-002",
	  })
	  if !errors.Is(err, hotbreach.ErrLeaseConsumed) {
		  t.Fatalf("expected ErrLeaseConsumed on double consume, got: %v", err)
	   }
   }

// ─────────────────────────────────────────────
// 3. 租约 TTL 到期
// ─────────────────────────────────────────────

  func TestLease_TTLExpiry(t *testing.T) {
	   cl := newClock(time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC))
	   svc := newTestSvc(t, cl)
	  ctx := context.Background()
	   lease, err := svc.IssueLease(ctx, hotbreach.IssueLeaseReq{
		  Asset:    "ETH",
		  IssuedBy: "req-eth-001",
		   TTL:      30 * time.Second,
	  })
	  if err != nil {
		   t.Fatalf("IssueLease: %v", err)
	  }
	  // 推进时钟超过 TTL
	  cl.Advance(35 * time.Second)
	  // Expire Worker 扫描
	   n, eerr := svc.ExpireLeases(ctx)
	   if eerr != nil {
		   t.Fatalf("ExpireLeases: %v", eerr)
	  }
	   if n != 1 {
		   t.Fatalf("expected 1 expired, got %d", n)
	   }
	   // 尝试消费应返回 ErrLeaseExpired
	  _, cerr := svc.ConsumeLease(ctx, hotbreach.ConsumeLeaseReq{
		   LeaseID:    lease.ID,
		   Asset:      "ETH",
		   ConsumedBy: "txn-eth-001",
	  })
	   if !errors.Is(cerr, hotbreach.ErrLeaseExpired) {
		   t.Fatalf("expected ErrLeaseExpired after TTL, got: %v", cerr)
	  }
  }

   func TestLease_ExpiredAtConsume(t *testing.T) {
	   cl := newClock(time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC))
	   svc := newTestSvc(t, cl)
	  ctx := context.Background()
	  lease, _ := svc.IssueLease(ctx, hotbreach.IssueLeaseReq{
		  Asset:    "ETH",
		  IssuedBy: "req-eth-002",
		  TTL:      10 * time.Second,
	  })
	   // 时钟超过 TTL，但未运行 ExpireLeases
	   cl.Advance(20 * time.Second)
	  // ConsumeLease 内部应检测到过期
	   _, cerr := svc.ConsumeLease(ctx, hotbreach.ConsumeLeaseReq{
		   LeaseID: lease.ID,
		  Asset:   "ETH",
		  ConsumedBy: "txn-eth-002",
	  })
	  if !errors.Is(cerr, hotbreach.ErrLeaseExpired) {
		   t.Fatalf("expected ErrLeaseExpired when clock past TTL, got: %v", cerr)
	  }
   }

// ─────────────────────────────────────────────
// 4. IssueLease 在击穿 gated 时被拒
// ─────────────────────────────────────────────

  func TestIssueLease_BlockedByBreach(t *testing.T) {
	  svc := newTestSvc(t, nil)
	  ctx := context.Background()
	  // 制造 gated breach
	   lease, _ := svc.IssueLease(ctx, hotbreach.IssueLeaseReq{Asset: "BTC", IssuedBy: "req-btc-001"})
	  _, berr2 := svc.InjectBreachCase(ctx, "BTC", lease.ID, "concurrent-sweep-001")
	   if berr2 != nil {
		  t.Fatalf("InjectBreachCase: %v", berr2)
	  }
	   _, _ = svc.Patrol(ctx)
	   // 尝试再签发应被拒
	  _, err := svc.IssueLease(ctx, hotbreach.IssueLeaseReq{Asset: "BTC", IssuedBy: "req-btc-002"})
	  if !errors.Is(err, hotbreach.ErrBreachGated) {
		   t.Fatalf("expected ErrBreachGated when breach gated, got: %v", err)
	  }
   }

// ─────────────────────────────────────────────
// 5. 击穿案例四眼解除
// ─────────────────────────────────────────────

  func TestClearBreach_SameApproverFails(t *testing.T) {
	  svc := newTestSvc(t, nil)
	  ctx := context.Background()
	   // 制造 gated breach
	   lease, _ := svc.IssueLease(ctx, hotbreach.IssueLeaseReq{Asset: "USDT", IssuedBy: "req-usdt-001"})
	   bc0, berr := svc.InjectBreachCase(ctx, "USDT", lease.ID, "concurrent-001")
	  if berr != nil {
		  t.Fatalf("InjectBreachCase: %v", berr)
	   }
	  _, _ = svc.Patrol(ctx)
	  // 同一人尝试解除
	   _, err := svc.ClearBreach(ctx, hotbreach.ClearBreachReq{
		   BreachID: bc0.ID,
		  ApproverA: "admin-alice",
		  ApproverB: "admin-alice",
		   Note: "test",
	  })
	  if !errors.Is(err, hotbreach.ErrClearNotApproved) {
		   t.Fatalf("expected ErrClearNotApproved with same approver, got: %v", err)
	  }
   }

  func TestClearBreach_TwoApproversSucceeds(t *testing.T) {
	   svc := newTestSvc(t, nil)
	  ctx := context.Background()
	   // 制造 gated breach
	   lease, _ := svc.IssueLease(ctx, hotbreach.IssueLeaseReq{Asset: "USDT", IssuedBy: "req-usdt-002"})
	  bc0, berr := svc.InjectBreachCase(ctx, "USDT", lease.ID, "concurrent-002")
	  if berr != nil {
		   t.Fatalf("InjectBreachCase: %v", berr)
	  }
	   _, _ = svc.Patrol(ctx)
	  // 两人解除
	   bc, err := svc.ClearBreach(ctx, hotbreach.ClearBreachReq{
		   BreachID: bc0.ID,
		   ApproverA: "admin-alice",
		   ApproverB: "admin-bob",
		  Note:      "manually verified hotinv restored",
	  })
	  if err != nil {
		  t.Fatalf("ClearBreach: %v", err)
	   }
	   if bc.State != hotbreach.BreachCleared {
		   t.Fatalf("expected state=cleared, got %s", bc.State)
	  }
	   if bc.ClearedBy != "admin-alice+admin-bob" {
		  t.Fatalf("unexpected cleared_by: %s", bc.ClearedBy)
	   }
  }

// ─────────────────────────────────────────────
// 6. 撤销租约
// ─────────────────────────────────────────────

  func TestRevokeLease(t *testing.T) {
	   svc := newTestSvc(t, nil)
	   ctx := context.Background()
	  lease, _ := svc.IssueLease(ctx, hotbreach.IssueLeaseReq{Asset: "USDT", IssuedBy: "req-revoke-001"})
	   if err := svc.RevokeLease(ctx, lease.ID, "manual admin revoke"); err != nil {
		  t.Fatalf("RevokeLease: %v", err)
	   }
	   _, cerr := svc.ConsumeLease(ctx, hotbreach.ConsumeLeaseReq{
		  LeaseID: lease.ID,
		  Asset: "USDT",
		   ConsumedBy: "txn-revoke-001",
	   })
	   if !errors.Is(cerr, hotbreach.ErrLeaseRevoked) {
		  t.Fatalf("expected ErrLeaseRevoked after revoke, got: %v", cerr)
	  }
   }

// ─────────────────────────────────────────────
// 7. 击穿证明 CompositeHash 验证
// ─────────────────────────────────────────────

  func TestBreachProof_Verify(t *testing.T) {
	  svc := newTestSvc(t, nil)
	  ctx := context.Background()
	   lease, _ := svc.IssueLease(ctx, hotbreach.IssueLeaseReq{Asset: "ETH", IssuedBy: "req-proof-001"})
	  bc, berr := svc.InjectBreachCase(ctx, "ETH", lease.ID, "concurrent-proof-001")
	  if berr != nil {
		  t.Fatalf("InjectBreachCase: %v", berr)
	   }
	   // 击穿注入的证明可能为空（InjectBreachCase 不自动生成证明）
	  // 直接用 QueryBreaches 看证明存不存在
	  resp, qerr := svc.QueryBreaches(ctx, "ETH", true, 10)
	   if qerr != nil {
		   t.Fatalf("QueryBreaches: %v", qerr)
	   }
	   _ = bc
	   if len(resp.Proofs) == 0 {
		   t.Skip("no proof generated (InjectBreachCase does not auto-build proof)")
	  }
	  if err := hotbreach.VerifyProof(resp.Proofs[0]); err != nil {
		   t.Fatalf("VerifyProof: %v", err)
	   }
  }

// ─────────────────────────────────────────────
// 表驱动：主状态机覆盖
// ─────────────────────────────────────────────

type leaseStateCase struct {
	name      string
	setup     func(ctx context.Context, svc *hotbreach.Service, cl *fakeClock) (leaseID string)
	consume   bool
	expectErr error
	expectState hotbreach.LeaseState
}

  func TestLeaseStateMachine_Table(t *testing.T) {
	  t0 := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	  cases := []leaseStateCase{
		  {
			   name: "issued→consumed",
			  setup: func(ctx context.Context, svc *hotbreach.Service, cl *fakeClock) string {
				   l, _ := svc.IssueLease(ctx, hotbreach.IssueLeaseReq{Asset: "USDT", IssuedBy: "r-c-1"})
				   return l.ID
			   },
			  consume:     true,
			  expectErr:   nil,
			  expectState: hotbreach.LeaseConsumed,
		  },
		   {
			  name: "issued→expired (TTL expired before consume)",
			   setup: func(ctx context.Context, svc *hotbreach.Service, cl *fakeClock) string {
				  l, _ := svc.IssueLease(ctx, hotbreach.IssueLeaseReq{Asset: "USDT", IssuedBy: "r-e-1", TTL: 5 * time.Second})
				   cl.Advance(10 * time.Second)
				  _, _ = svc.ExpireLeases(ctx)
				   return l.ID
			   },
			   consume: true,
			  expectErr: hotbreach.ErrLeaseExpired,
			  expectState: hotbreach.LeaseExpired,
		  },
		   {
			  name: "issued→revoked",
			   setup: func(ctx context.Context, svc *hotbreach.Service, cl *fakeClock) string {
				   l, _ := svc.IssueLease(ctx, hotbreach.IssueLeaseReq{Asset: "USDT", IssuedBy: "r-rv-1"})
				   _ = svc.RevokeLease(ctx, l.ID, "admin")
				   return l.ID
			  },
			   consume:   true,
			  expectErr: hotbreach.ErrLeaseRevoked,
			  expectState: hotbreach.LeaseRevoked,
		  },
		   {
			  name: "issued→consumed (asset mismatch)",
			   setup: func(ctx context.Context, svc *hotbreach.Service, cl *fakeClock) string {
				  l, _ := svc.IssueLease(ctx, hotbreach.IssueLeaseReq{Asset: "USDT", IssuedBy: "r-am-1"})
				   return l.ID
			   },
			   consume:   false,
			   expectErr: hotbreach.ErrInvalidInput,
		  },
	   }
	  for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			  cl := newClock(t0)
			   svc := newTestSvc(t, cl)
			  ctx := context.Background()
			   leaseID := tc.setup(ctx, svc, cl)
			   var finalState hotbreach.LeaseState
			   if tc.name == "issued→consumed (asset mismatch)" {
				  // 消费 asset 不匹配
				  _, err := svc.ConsumeLease(ctx, hotbreach.ConsumeLeaseReq{
					  LeaseID: leaseID,
					   Asset: "ETH", // 故意错误
					   ConsumedBy: "txn-mismatch",
				  })
				   if tc.expectErr != nil && !errors.Is(err, tc.expectErr) {
					   t.Fatalf("expected %v, got %v", tc.expectErr, err)
				   }
				  return
			   }
			   if tc.consume {
				   consumed, err := svc.ConsumeLease(ctx, hotbreach.ConsumeLeaseReq{
					   LeaseID:    leaseID,
					  Asset:      "USDT",
					   ConsumedBy: "txn-table",
				  })
				   if tc.expectErr == nil {
					   if err != nil {
						   t.Fatalf("unexpected error: %v", err)
					  }
					   finalState = consumed.State
				   } else {
					   if !errors.Is(err, tc.expectErr) {
						   t.Fatalf("expected %v, got %v", tc.expectErr, err)
					  }
					   // 读取实际状态
					   resp, rerr := svc.GetLease(ctx, leaseID)
					   if rerr != nil {
						   t.Fatalf("GetLease: %v", rerr)
					  }
					   if resp.Lease != nil {
						   finalState = resp.Lease.State
					   }
				   }
			  }
			   if tc.expectState != "" && finalState != tc.expectState {
				  t.Fatalf("expected state=%s, got %s", tc.expectState, finalState)
			  }
		  })
	   }
  }

// ─────────────────────────────────────────────
// 表驱动：击穿状态机
// ─────────────────────────────────────────────

type breachStateCase struct {
	name   string
	setup  func(ctx context.Context, svc *hotbreach.Service) string // 返回 breachID
	patrol bool
	clearApprA string
	clearApprB string
	expectState    hotbreach.BreachState
	expectClearErr error
}

   func TestBreachStateMachine_Table(t *testing.T) {
	   cases := []breachStateCase{
		  {
			  name: "open→gated via patrol",
			  setup: func(ctx context.Context, svc *hotbreach.Service) string {
				  l, _ := svc.IssueLease(ctx, hotbreach.IssueLeaseReq{Asset: "BTC", IssuedBy: "bs-1"})
				   bc, _ := svc.InjectBreachCase(ctx, "BTC", l.ID, "cp-1")
				   return bc.ID
			  },
			   patrol:      true,
			   expectState: hotbreach.BreachGated,
		   },
		  {
			  name: "gated→cleared via four-eyes",
			  setup: func(ctx context.Context, svc *hotbreach.Service) string {
				   l, _ := svc.IssueLease(ctx, hotbreach.IssueLeaseReq{Asset: "ETH", IssuedBy: "bs-2"})
				  bc, _ := svc.InjectBreachCase(ctx, "ETH", l.ID, "cp-2")
				  _, _ = svc.Patrol(ctx)
				   return bc.ID
			  },
			  patrol: false,
			   clearApprA:  "alice",
			  clearApprB:  "bob",
			   expectState: hotbreach.BreachCleared,
		   },
		   {
			   name: "gated clear same-person rejected",
			   setup: func(ctx context.Context, svc *hotbreach.Service) string {
				   l, _ := svc.IssueLease(ctx, hotbreach.IssueLeaseReq{Asset: "USDT", IssuedBy: "bs-3"})
				  bc, _ := svc.InjectBreachCase(ctx, "USDT", l.ID, "cp-3")
				  _, _ = svc.Patrol(ctx)
				   return bc.ID
			   },
			   patrol:     false,
			  clearApprA: "alice",
			   clearApprB:  "alice",
			  expectState: hotbreach.BreachGated, // 未解除
			  expectClearErr: hotbreach.ErrClearNotApproved,
		   },
	  }
	   for _, tc := range cases {
		   tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   svc := newTestSvc(t, nil)
			  ctx := context.Background()
			   breachID := tc.setup(ctx, svc)
			  if breachID == "" {
				  t.Skip("no breach ID")
			   }
			  if tc.patrol {
				   if _, err := svc.Patrol(ctx); err != nil {
					   t.Fatalf("Patrol: %v", err)
				   }
			   }
			  if tc.clearApprA != "" {
				  _, cerr := svc.ClearBreach(ctx, hotbreach.ClearBreachReq{
					   BreachID: breachID,
					   ApproverA: tc.clearApprA,
					   ApproverB: tc.clearApprB,
					  Note:      "test clear",
				  })
				  if tc.expectClearErr != nil {
					  if !errors.Is(cerr, tc.expectClearErr) {
						   t.Fatalf("expected clear err %v, got %v", tc.expectClearErr, cerr)
					   }
				  } else if cerr != nil {
					   t.Fatalf("ClearBreach: %v", cerr)
				  }
			  }
			  bc, berr := svc.GetBreach(ctx, breachID)
			   if berr != nil {
				   t.Fatalf("GetBreach: %v", berr)
			   }
			  if bc.State != tc.expectState {
				  t.Fatalf("expected breach state=%s, got %s", tc.expectState, bc.State)
			   }
		  })
	   }
  }

// ─────────────────────────────────────────────
// 8. Worker 单轮
// ─────────────────────────────────────────────

   func TestLeaseExpiryWorker_Tick(t *testing.T) {
	   cl := newClock(time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC))
	   svc := newTestSvc(t, cl)
	  ctx := context.Background()
	   // 签发 3 个短 TTL 租约
	   for i := 0; i < 3; i++ {
		   _, err := svc.IssueLease(ctx, hotbreach.IssueLeaseReq{
			  Asset: "USDT",
			   IssuedBy: "expiry-worker-" + string(rune('A'+i)),
			   TTL:      5 * time.Second,
		   })
		   if err != nil {
			  t.Fatalf("IssueLease %d: %v", i, err)
		   }
	  }
	   // 不超期，不应回收
	  w := hotbreach.NewLeaseExpiryWorker(svc, 0)
	   w.WithClock(cl.Now)
	  rep, err := w.Tick(ctx)
	  if err != nil {
		  t.Fatalf("Tick: %v", err)
	   }
	  if rep.Expired != 0 {
		  t.Fatalf("expected 0 expired before TTL, got %d", rep.Expired)
	  }
	   // 推进时钟
	   cl.Advance(10 * time.Second)
	  rep, err = w.Tick(ctx)
	   if err != nil {
		   t.Fatalf("Tick after advance: %v", err)
	  }
	  if rep.Expired != 3 {
		   t.Fatalf("expected 3 expired after TTL, got %d", rep.Expired)
	  }
   }

   func TestBreachPatrolWorker_Tick(t *testing.T) {
	   svc := newTestSvc(t, nil)
	   ctx := context.Background()
	  // 无击穿时巡检应成功（0 scanned）
	  w := hotbreach.NewBreachPatrolWorker(svc,
		  0)
	   rep, err := w.Tick(ctx)
	  if err != nil {
		  t.Fatalf("Tick: %v", err)
	  }
	  if rep.Scanned != 0 {
		   t.Fatalf("expected 0 scanned, got %d", rep.Scanned)
	   }
   }

  func TestWriteBlockClearWorker_Tick_NoAlert(t *testing.T) {
	  svc := newTestSvc(t, nil)
	   ctx := context.Background()
	   // 无击穿时监控应成功
	   w := hotbreach.NewWriteBlockClearWorker(svc, 0, 5*time.Minute)
	   rep, err := w.Tick(ctx)
	  if err != nil {
		   t.Fatalf("Tick: %v", err)
	  }
	  if rep.Checked != 0 {
		   t.Fatalf("expected 0 checked, got %d", rep.Checked)
	   }
  }

// ─────────────────────────────────────────────
// 9. 无租约出金被挡写（ErrNoLease，存在活跃 fence 时）
// ─────────────────────────────────────────────

  func TestGuard_NoLeaseWithActiveFence_Blocked(t *testing.T) {
	   svc := newTestSvc(t, nil)
	  ctx := context.Background()
	  // 签发租约（会创建活跃 fence）
	   _, err := svc.IssueLease(ctx, hotbreach.IssueLeaseReq{Asset: "USDT", IssuedBy: "fence-req-001"})
	   if err != nil {
		  t.Fatalf("IssueLease: %v", err)
	  }
	  // 无租约 Guard 应被拦截（存在 fence）
	   gerr := svc.Guard(ctx, hotbreach.GuardReq{Asset: "USDT", Party: "concurrent-withdraw"})
	  if gerr == nil {
		   // fence 存在时应拒绝无 lease 出金
		  t.Fatalf("expected Guard to block no-lease debit with active fence")
	   }
   }

// ─────────────────────────────────────────────
// 10. 查询端点覆盖
// ─────────────────────────────────────────────

  func TestQueryBreaches_Empty(t *testing.T) {
	  svc := newTestSvc(t, nil)
	  ctx := context.Background()
	  resp, err := svc.QueryBreaches(ctx, "USDT", false, 50)
	   if err != nil {
		  t.Fatalf("QueryBreaches: %v", err)
	  }
	   if resp.Total != 0 {
		  t.Fatalf("expected 0 total, got %d", resp.Total)
	  }
   }

  func TestListLeases_AfterIssue(t *testing.T) {
	   svc := newTestSvc(t, nil)
	   ctx := context.Background()
	  for i := 0; i < 3; i++ {
		  _, _ = svc.IssueLease(ctx, hotbreach.IssueLeaseReq{Asset: "USDT", IssuedBy: "list-req-" + string(rune('0'+i))})
	  }
	  leases, err := svc.ListLeases(ctx, "USDT", 10)
	  if err != nil {
		   t.Fatalf("ListLeases: %v", err)
	   }
	   if len(leases) != 3 {
		  t.Fatalf("expected 3 leases, got %d", len(leases))
	   }
  }
