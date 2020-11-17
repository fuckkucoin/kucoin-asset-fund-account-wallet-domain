package swapthaw_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/swapthaw"
	"github.com/rs/zerolog"
)

// ──────────────────────────────────────────────
// 辅助：构造口径 Service
// ──────────────────────────────────────────────

  func newTestService(leaseTTL time.Duration) *swapthaw.Service {
	   return swapthaw.NewService(leaseTTL)
   }

   func nopLogger() zerolog.Logger {
	  return zerolog.Nop()
   }

// ──────────────────────────────────────────────
// 场景 1：同订单并发 failUnfreeze 与 settle → 仅一腿胜出
// ──────────────────────────────────────────────

  func TestConcurrentSettleAndFailUnfreeze(t *testing.T) {
	   type caseSpec struct {
		   name string
		  leg1 swapthaw.BindLeg
		   leg2         swapthaw.BindLeg
		   expectWinner swapthaw.LeaseStatus
	  }
	   cases := []caseSpec{
		  {"settle先failUnfreeze后", swapthaw.BindLegSettle, swapthaw.BindLegFailUnfreeze, swapthaw.LeaseStatusSettled},
		  {"failUnfreeze先settle后", swapthaw.BindLegFailUnfreeze, swapthaw.BindLegSettle, swapthaw.LeaseStatusThawed},
	  }
	  for _, tc := range cases {
		   tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   svc := newTestService(0)
			   orderID := "ord-conc-" + tc.name
			  userID := "user-conc-" + tc.name
			  lease, err := svc.AcquireLeaseForOrder(orderID, userID, "USDT", "100")
			  if err != nil {
				   t.Fatalf("AcquireLeaseForOrder 失败: %v", err)
			   }
			   claimReq := func(leg swapthaw.BindLeg) swapthaw.ClaimTerminalReq {
				   return swapthaw.ClaimTerminalReq{
					   OrderID: orderID,
					  LeaseID:    lease.ID,
					  LeaseToken: lease.Token,
					   Leg:        leg,
				   }
			  }
			  doClaim := func(req swapthaw.ClaimTerminalReq) error {
				  var err error
				  if req.Leg == swapthaw.BindLegSettle {
					  _, _, err = svc.ClaimSettleTerminal(req)
				  } else {
					  _,
						  _, err = svc.ClaimUnfreezeTerminal(req)
				   }
				  return err
			   }
			  // 第一腿声明。
			   if err := doClaim(claimReq(tc.leg1)); err != nil {
				   t.Fatalf("第一腿 ClaimTerminal 失败: %v", err)
			   }
			   // 第二腿声明——期望被拒绝（ErrThawMutexDenied 或 ErrDoubleRelease 均为拒绝）。
			  err2 := doClaim(claimReq(tc.leg2))
			   if err2 == nil {
				  t.Error("第二腿期望返回错误，实际 nil")
			   }
			   if !errors.Is(err2, swapthaw.ErrThawMutexDenied) && !errors.Is(err2, swapthaw.ErrDoubleRelease) {
				   t.Errorf("第二腿期望 ErrThawMutexDenied 或 ErrDoubleRelease，实际 %v", err2)
			  }
			  updatedLease, err := svc.GetLease(lease.ID)
			   if err != nil {
				   t.Fatalf("GetLease 失败: %v", err)
			  }
			   if updatedLease.Status != tc.expectWinner && updatedLease.Status != swapthaw.LeaseStatusBlocked {
				  t.Errorf("Lease 状态期望 %s 或 blocked，实际 %s", tc.expectWinner, updatedLease.Status)
			   }
		   })
	  }
   }

// ──────────────────────────────────────────────
// 场景 2：已 settle 订单再次 failUnfreeze → ErrDoubleRelease + ThawCase
// ──────────────────────────────────────────────

  func TestDoubleReleaseAfterSettle(t *testing.T) {
	  svc := newTestService(0)
	   orderID := "ord-dr-001"
	   userID := "user-dr-001"
	   lease, err := svc.AcquireLeaseForOrder(orderID, userID, "BTC", "0.5")
	  if err != nil {
		   t.Fatalf("AcquireLeaseForOrder: %v", err)
	  }
	   req := swapthaw.ClaimTerminalReq{
		   OrderID:    orderID,
		   LeaseID:    lease.ID,
		   LeaseToken: lease.Token,
	   }
	  _, _, err = svc.ClaimSettleTerminal(req)
	   if err != nil {
		  t.Fatalf("ClaimSettleTerminal: %v", err)
	  }
	  l, _ := svc.GetLease(lease.ID)
	   if l.Status != swapthaw.LeaseStatusSettled {
		  t.Errorf("settle 后 Lease 状态期望 settled，实际 %s", l.Status)
	   }
	  req2 := swapthaw.ClaimTerminalReq{
		   OrderID:    orderID,
		  LeaseID:    lease.ID,
		   LeaseToken: lease.Token,
		   FrozenSnapshot:    "50",
		   AvailableSnapshot: "200",
	   }
	  _, tc, err2 := svc.ClaimUnfreezeTerminal(req2)
	   if !errors.Is(err2, swapthaw.ErrDoubleRelease) {
		   t.Errorf("已 settle 后 failUnfreeze 期望 ErrDoubleRelease，实际 %v", err2)
	   }
	   if tc == nil {
		  t.Fatal("ErrDoubleRelease 时 ThawCase 不应为 nil")
	  }
	   if tc.Kind != swapthaw.ThawCaseKindDoubleRelease {
		   t.Errorf("ThawCase.Kind 期望 double_release，实际 %s", tc.Kind)
	  }
	  if tc.Status != swapthaw.ThawCaseStatusOpen {
		   t.Errorf("ThawCase.Status 期望 open，实际 %s", tc.Status)
	  }
	  l2, _ := svc.GetLease(lease.ID)
	   if !l2.Status.IsTerminal() {
		   t.Errorf("双释后 Lease 期望处于终态（settled/thawed/blocked），实际 %s", l2.Status)
	   }
	   errGuard := svc.GuardNewFreeze(userID)
	   if !errors.Is(errGuard, swapthaw.ErrThawCaseOpen) {
		  t.Errorf("存在 ThawCase 时 GuardNewFreeze 期望 ErrThawCaseOpen，实际 %v", errGuard)
	  }
  }

// ──────────────────────────────────────────────
// 场景 3：ThawCase 证明流程——缺证明挡关闭，证明后可关闭，关闭后解除阻断
// ──────────────────────────────────────────────

  func TestThawCaseProveAndClose(t *testing.T) {
	  svc := newTestService(0)
	  orderID := "ord-proof-001"
	  userID := "user-proof-001"
	   lease, _ := svc.AcquireLeaseForOrder(orderID, userID, "USDT", "100")
	  req := swapthaw.ClaimTerminalReq{OrderID: orderID, LeaseID: lease.ID, LeaseToken: lease.Token}
	   svc.ClaimSettleTerminal(req)
	   req2 := swapthaw.ClaimTerminalReq{
		   OrderID:        orderID,
		  LeaseID:        lease.ID,
		  LeaseToken:     lease.Token,
		  FrozenSnapshot: "100",
		  AvailableSnapshot: "0",
	   }
	  _, tc, _ := svc.ClaimUnfreezeTerminal(req2)
	  if tc == nil {
		   t.Fatal("期望产生 ThawCase")
	   }
	  t.Run("无证明时关闭失败", func(t *testing.T) {
		  _, err := svc.CloseThawCase(swapthaw.CloseThawCaseReq{CaseID: tc.ID, Approver: "ops-alice"})
		  if !errors.Is(err, swapthaw.ErrThawCaseProofMissing) {
			  t.Errorf("无证明时 CloseThawCase 期望 ErrThawCaseProofMissing，实际 %v", err)
		   }
	  })
	  t.Run("证明固化成功", func(t *testing.T) {
		   proved, err := svc.ProveThawCase(swapthaw.ProveThawCaseReq{
			  CaseID:       tc.ID,
			   ProofPayload: `{"amount":"100","snapshot_hash":"abc123"}`,
			   Approver:     "ops-alice",
		   })
		  if err != nil {
			  t.Fatalf("ProveThawCase: %v", err)
		   }
		  if proved.Status != swapthaw.ThawCaseStatusProven {
			   t.Errorf("证明后 ThawCase 状态期望 proven，实际 %s", proved.Status)
		   }
		   if proved.ProvenAt == nil {
			  t.Error("proven_at 不应为 nil")
		   }
	  })
	   t.Run("proven 后用户仍被阻断", func(t *testing.T) {
		   err := svc.GuardNewFreeze(userID)
		   if !errors.Is(err, swapthaw.ErrThawCaseOpen) {
			   t.Errorf("proven 状态仍应阻断新冻额，实际 %v", err)
		  }
	   })
	   t.Run("收口成功后解除阻断", func(t *testing.T) {
		  closed, err := svc.CloseThawCase(swapthaw.CloseThawCaseReq{CaseID: tc.ID, Approver: "ops-bob"})
		   if err != nil {
			  t.Fatalf("CloseThawCase: %v", err)
		  }
		   if closed.Status != swapthaw.ThawCaseStatusClosed {
			  t.Errorf("收口后 ThawCase 状态期望 closed，实际 %s", closed.Status)
		  }
		  err2 := svc.GuardNewFreeze(userID)
		  if err2 != nil {
			  t.Errorf("ThawCase closed 后 GuardNewFreeze 期望 nil，实际 %v", err2)
		   }
	  })
  }

// ──────────────────────────────────────────────
// 场景 4：Lease 过期后声明终态腿 → ErrLeaseExpired
// ──────────────────────────────────────────────

  func TestLeaseExpiredClaim(t *testing.T) {
	   svc := swapthaw.NewService(1 * time.Millisecond)
	  orderID := "ord-expired-001"
	   userID := "user-expired-001"
	   lease, err := svc.AcquireLeaseForOrder(orderID, userID, "ETH", "2")
	  if err != nil {
		  t.Fatalf("AcquireLeaseForOrder: %v", err)
	   }
	   time.Sleep(10 * time.Millisecond)
	  req := swapthaw.ClaimTerminalReq{
		   OrderID: orderID,
		  LeaseID: lease.ID,
		   LeaseToken: lease.Token,
	   }
	   _, _, err = svc.ClaimSettleTerminal(req)
	   if !errors.Is(err, swapthaw.ErrLeaseExpired) {
		  t.Errorf("Lease 过期后 ClaimSettleTerminal 期望 ErrLeaseExpired，实际 %v", err)
	   }
  }

// ──────────────────────────────────────────────
// 场景 5：FreezeGeneration CAS 保证世代单调递增
// ──────────────────────────────────────────────

  func TestFreezeGenerationCAS(t *testing.T) {
	  svc := newTestService(0)
	   orderID := "ord-gen-001"
	  userID := "user-gen-001"
	   lease1, err := svc.AcquireLeaseForOrder(orderID, userID, "USDT", "100")
	   if err != nil {
		  t.Fatalf("第一次 AcquireLeaseForOrder: %v", err)
	   }
	   gen1, err := svc.QueryGeneration(orderID)
	   if err != nil {
		   t.Fatalf("QueryGeneration: %v", err)
	  }
	  if gen1.Generation != 1 {
		  t.Errorf("第一次冻额后 generation 期望 1，实际 %d", gen1.Generation)
	   }
	  if gen1.ActiveLeaseID != lease1.ID {
		   t.Errorf("ActiveLeaseID 期望 %s，实际 %s", lease1.ID, gen1.ActiveLeaseID)
	  }
	  req := swapthaw.ClaimTerminalReq{
		   OrderID:    orderID,
		   LeaseID:    lease1.ID,
		   LeaseToken: lease1.Token,
	  }
	  _, _, err = svc.ClaimSettleTerminal(req)
	   if err != nil {
		  t.Fatalf("ClaimSettleTerminal: %v", err)
	  }
	   _, err = svc.AcquireLeaseForOrder(orderID, userID, "USDT", "50")
	  if err != nil {
		  t.Fatalf("第二次 AcquireLeaseForOrder: %v", err)
	   }
	  gen2, _ := svc.QueryGeneration(orderID)
	  if gen2.Generation != 2 {
		  t.Errorf("第二次冻额后 generation 期望 2，实际 %d", gen2.Generation)
	   }
   }

// ──────────────────────────────────────────────
// 场景 6：Token 不匹配 → ErrLeaseConflict
// ──────────────────────────────────────────────

  func TestLeaseTokenMismatch(t *testing.T) {
	  svc := newTestService(0)
	   orderID := "ord-token-001"
	   userID := "user-token-001"
	  lease, err := svc.AcquireLeaseForOrder(orderID, userID, "USDT", "100")
	   if err != nil {
		   t.Fatalf("AcquireLeaseForOrder: %v", err)
	   }
	  req := swapthaw.ClaimTerminalReq{
		  OrderID: orderID,
		  LeaseID:    lease.ID,
		   LeaseToken: "wrong-token-12345",
	  }
	   _,
		   _, err = svc.ClaimSettleTerminal(req)
	  if !errors.Is(err, swapthaw.ErrLeaseConflict) {
		   t.Errorf("Token 不匹配期望 ErrLeaseConflict，实际 %v", err)
	  }
  }

// ──────────────────────────────────────────────
// 场景 7：GuardNewFreeze 无 ThawCase 时允许通过
// ──────────────────────────────────────────────

  func TestGuardNewFreezeNoBlock(t *testing.T) {
	   svc := newTestService(0)
	  err := svc.GuardNewFreeze("user-clean-001")
	  if err != nil {
		   t.Errorf("无 ThawCase 时 GuardNewFreeze 期望 nil，实际 %v", err)
	  }
  }

// ──────────────────────────────────────────────
// 场景 8：并发 goroutine 竞争同一订单 Lease 互斥
// ──────────────────────────────────────────────

  func TestConcurrentGoroutineRace(t *testing.T) {
	  svc := newTestService(0)
	   orderID := "ord-goroutine-001"
	   userID := "user-goroutine-001"
	  lease, err := svc.AcquireLeaseForOrder(orderID, userID, "USDT", "100")
	   if err != nil {
		   t.Fatalf("AcquireLeaseForOrder: %v", err)
	  }
	   const goroutines = 20
	  results := make([]error, goroutines)
	  var wg sync.WaitGroup
	   wg.Add(goroutines)
	   for i := 0; i < goroutines; i++ {
		  i := i
		  go func() {
			   defer wg.Done()
			  req := swapthaw.ClaimTerminalReq{
				  OrderID:    orderID,
				   LeaseID:    lease.ID,
				   LeaseToken: lease.Token,
			  }
			   var e error
			  if i%2 == 0 {
				  _, _, e = svc.ClaimSettleTerminal(req)
			  } else {
				  _, _, e = svc.ClaimUnfreezeTerminal(req)
			  }
			   results[i] = e
		  }()
	  }
	   wg.Wait()
	   successCount := 0
	   for _, e := range results {
		   if e == nil {
			  successCount++
		  }
	   }
	  if successCount == 0 {
		   t.Error("并发竞争中期望至少一个 goroutine 成功")
	   }
	  if successCount > 2 {
		   t.Errorf("并发竞争中成功次数不应超过 2，实际 %d", successCount)
	  }
   }

// ──────────────────────────────────────────────
// 场景 9：孤儿 Frozen 扫描——无活跃 Lease 时 Worker 升 ThawCase
// ──────────────────────────────────────────────

  func TestOrphanFrozenWorker(t *testing.T) {
	   svc := newTestService(0)
	   fetchFn := swapthaw.FrozenOrderFetcher(func(_ context.Context) ([]*swapthaw.FrozenOrder, error) {
		   return []*swapthaw.FrozenOrder{
			   {
				   OrderID: "ord-orphan-001",
				   UserID:  "user-orphan-001",
				  Asset:   "USDT",
				   FrozenAmount: "100",
				  CreatedAt:    time.Now().UTC().Add(-2 * time.Hour),
			  },
		   }, nil
	  })
	  worker := swapthaw.NewWorker(svc, fetchFn, nil, nopLogger())
	  worker.WithOrphanAge(30 * time.Minute)
	  if err := worker.OrphanFrozenScanner(context.Background()); err != nil {
		   t.Fatalf("OrphanFrozenScanner: %v", err)
	   }
	  tc, err := svc.GetThawCaseByOrder("ord-orphan-001")
	   if err != nil {
		  t.Fatalf("GetThawCaseByOrder: %v", err)
	   }
	   if tc.Kind != swapthaw.ThawCaseKindOrphanFrozen {
		  t.Errorf("期望 orphan_frozen，实际 %s", tc.Kind)
	  }
  }

// ──────────────────────────────────────────────
// 场景 10：证明缺失时挡新 Quote 冻额（用户阻断→证明→关闭→解除）
// ──────────────────────────────────────────────

   func TestBlockNewFreezeUntilCaseClosed(t *testing.T) {
	   svc := newTestService(0)
	  userID := "user-block-001"
	   orderID := "ord-block-001"
	  lease, _ := svc.AcquireLeaseForOrder(orderID, userID, "USDT", "100")
	  req := swapthaw.ClaimTerminalReq{OrderID: orderID, LeaseID: lease.ID, LeaseToken: lease.Token}
	  svc.ClaimSettleTerminal(req)
	   _, tc, _ := svc.ClaimUnfreezeTerminal(swapthaw.ClaimTerminalReq{
		   OrderID:    orderID,
		  LeaseID:    lease.ID,
		  LeaseToken: lease.Token,
	  })
	  if tc == nil {
		  t.Fatal("期望产生 ThawCase")
	  }
	   if err := svc.GuardNewFreeze(userID); !errors.Is(err, swapthaw.ErrThawCaseOpen) {
		  t.Errorf("阻断期间期望 ErrThawCaseOpen，实际 %v", err)
	  }
	  _, err := svc.CloseThawCase(swapthaw.CloseThawCaseReq{CaseID: tc.ID})
	  if !errors.Is(err, swapthaw.ErrThawCaseProofMissing) {
		  t.Errorf("期望 ErrThawCaseProofMissing，实际 %v", err)
	   }
	  svc.ProveThawCase(swapthaw.ProveThawCaseReq{
		   CaseID:       tc.ID,
		  ProofPayload: `{"verified":true}`,
		   Approver:     "ops-alice",
	   })
	   if err := svc.GuardNewFreeze(userID); !errors.Is(err, swapthaw.ErrThawCaseOpen) {
		  t.Errorf("proven 但未 closed 时仍应阻断，实际 %v", err)
	  }
	   svc.CloseThawCase(swapthaw.CloseThawCaseReq{CaseID: tc.ID, Approver: "ops-bob"})
	   if err := svc.GuardNewFreeze(userID); err != nil {
		  t.Errorf("关闭后 GuardNewFreeze 期望 nil，实际 %v", err)
	   }
   }

// ──────────────────────────────────────────────
// 场景 11：从 golden 夹具验证 cvfund 隔离
// ──────────────────────────────────────────────

  func TestCvfundIsolationGolden(t *testing.T) {
	   f, err := os.ReadFile("../../testdata/swapthaw/cvfund_isolation.json")
	   if err != nil {
		  t.Skipf("golden 夹具未找到: %v", err)
	  }
	   var fixture map[string]any
	  if err := json.Unmarshal(f, &fixture); err != nil {
		  t.Fatalf("夹具解析失败: %v", err)
	   }
	   expected, ok := fixture["expected"].(map[string]any)
	  if !ok {
		  t.Fatal("夹具 expected 字段缺失")
	   }
	   if expected["no_cross_reference"] != true {
		  t.Error("夹具期望 no_cross_reference=true")
	  }
	   if expected["cvfund_table"] != "cvfund_path_occupancies" {
		   t.Error("cvfund 表名不符预期")
	  }
	   if expected["swap_thaw_table"] != "sw_settle_binds" {
		   t.Error("swap thaw 表名不符预期")
	   }
   }

// ──────────────────────────────────────────────
// 场景 12：FSM 校验——非法 Lease 状态转换被拒绝
// ──────────────────────────────────────────────

   func TestFSMIllegalLeaseTransition(t *testing.T) {
	   type spec struct {
		   name   string
		  from   swapthaw.LeaseStatus
		   to     swapthaw.LeaseStatus
		  wantOK bool
	  }
	  cases := []spec{
		   {"open→settled", swapthaw.LeaseStatusOpen, swapthaw.LeaseStatusSettled, true},
		  {"open→thawed", swapthaw.LeaseStatusOpen, swapthaw.LeaseStatusThawed, true},
		   {"open→blocked", swapthaw.LeaseStatusOpen, swapthaw.LeaseStatusBlocked, true},
		   {"settled→open", swapthaw.LeaseStatusSettled, swapthaw.LeaseStatusOpen, false},
		  {"settled→thawed", swapthaw.LeaseStatusSettled, swapthaw.LeaseStatusThawed, false},
		   {"thawed→settled", swapthaw.LeaseStatusThawed, swapthaw.LeaseStatusSettled, false},
		  {"blocked→settled", swapthaw.LeaseStatusBlocked, swapthaw.LeaseStatusSettled, false},
	   }
	   for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   lease := &swapthaw.FreezeLease{
				   ID: "test-lease",
				  Status: tc.from,
			   }
			   err := swapthaw.TransitLease(lease,
				  tc.to, time.Now().UTC())
			   if tc.wantOK && err != nil {
				   t.Errorf("期望转换成功，实际错误: %v", err)
			  }
			  if !tc.wantOK && err == nil {
				  t.Errorf("期望转换失败，实际成功")
			   }
		   })
	   }
   }

// ──────────────────────────────────────────────
// 场景 13：ThawCase FSM 校验
// ──────────────────────────────────────────────

  func TestThawCaseFSM(t *testing.T) {
	   type spec struct {
		  name string
		   from swapthaw.ThawCaseStatus
		   to   swapthaw.ThawCaseStatus
		  proofPayload string
		  wantOK bool
	   }
	  cases := []spec{
		   {"open→proven(有proof)", swapthaw.ThawCaseStatusOpen, swapthaw.ThawCaseStatusProven, `{"ok":1}`, true},
		  {"open→proven(无proof)", swapthaw.ThawCaseStatusOpen, swapthaw.ThawCaseStatusProven, "", false},
		   {"proven→closed", swapthaw.ThawCaseStatusProven, swapthaw.ThawCaseStatusClosed, "", true},
		  {"open→closed(跳步)", swapthaw.ThawCaseStatusOpen, swapthaw.ThawCaseStatusClosed, "", false},
		  {"closed→open(回转)", swapthaw.ThawCaseStatusClosed, swapthaw.ThawCaseStatusOpen, "", false},
	  }
	   for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  c := &swapthaw.ThawCase{
				  ID:           "tc-fsm-test",
				   Status:       tc.from,
				  ProofPayload: tc.proofPayload,
			  }
			   err := swapthaw.TransitThawCase(c,
				   tc.to, time.Now().UTC())
			  if tc.wantOK && err != nil {
				  t.Errorf("期望转换成功，实际错误: %v", err)
			  }
			   if !tc.wantOK && err == nil {
				   t.Errorf("期望转换失败，实际成功")
			   }
		  })
	   }
   }

// ──────────────────────────────────────────────
// 场景 14：从 golden 夹具验证双释流程
// ──────────────────────────────────────────────

   func TestDoubleReleaseGolden(t *testing.T) {
	   f, err := os.ReadFile("../../testdata/swapthaw/double_release.json")
	   if err != nil {
		   t.Skipf("golden 夹具未找到: %v", err)
	   }
	   var fixture struct {
		  OrderID string `json:"order_id"`
		   UserID       string `json:"user_id"`
		  Asset        string `json:"asset"`
		  FrozenAmount string `json:"frozen_amount"`
		  FirstTerminalLeg string `json:"first_terminal_leg"`
		   SecondTerminalLeg string `json:"second_terminal_leg"`
		  ExpectedThawCase  struct {
			   Kind   string `json:"kind"`
			  Status string `json:"status"`
		   } `json:"expected_thaw_case"`
	   }
	  if err := json.Unmarshal(f, &fixture); err != nil {
		   t.Fatalf("夹具解析失败: %v", err)
	   }
	   svc := newTestService(0)
	   lease, err := svc.AcquireLeaseForOrder(fixture.OrderID, fixture.UserID, fixture.Asset, fixture.FrozenAmount)
	   if err != nil {
		   t.Fatalf("AcquireLeaseForOrder: %v", err)
	  }
	   req := swapthaw.ClaimTerminalReq{OrderID: fixture.OrderID, LeaseID: lease.ID, LeaseToken: lease.Token}
	   // 第一腿（settle）。
	  _, _, err = svc.ClaimSettleTerminal(req)
	   if err != nil {
		   t.Fatalf("第一腿 ClaimSettleTerminal: %v", err)
	   }
	  // 第二腿（fail_unfreeze）。
	   _,
		  tc, err2 := svc.ClaimUnfreezeTerminal(req)
	   if !errors.Is(err2, swapthaw.ErrDoubleRelease) {
		  t.Errorf("第二腿期望 ErrDoubleRelease，实际 %v", err2)
	   }
	  if tc == nil || string(tc.Kind) != fixture.ExpectedThawCase.Kind {
		   t.Errorf("ThawCase.Kind 期望 %s，实际 %v", fixture.ExpectedThawCase.Kind, tc)
	   }
	  if tc != nil && string(tc.Status) != fixture.ExpectedThawCase.Status {
		   t.Errorf("ThawCase.Status 期望 %s，实际 %s", fixture.ExpectedThawCase.Status, tc.Status)
	   }
   }

// ──────────────────────────────────────────────
// 场景 15：LeaseNotFound——不存在的 Lease ID
// ──────────────────────────────────────────────

   func TestClaimWithNonExistentLease(t *testing.T) {
	   svc := newTestService(0)
	  req := swapthaw.ClaimTerminalReq{
		  OrderID:    "ord-nonexistent-001",
		   LeaseID:    "lease-nonexistent-001",
		   LeaseToken: "token-nonexistent",
	  }
	  _, _, err := svc.ClaimSettleTerminal(req)
	  if err == nil {
		  t.Error("不存在 Lease 时期望返回错误，实际 nil")
	   }
  }
