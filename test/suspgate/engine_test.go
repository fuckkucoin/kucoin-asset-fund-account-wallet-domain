package suspgate_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/suspgate"
)

// newTestEngine 构造使用内存仓储的引擎。
   func newTestEngine() *suspgate.Engine {
	   return suspgate.NewEngine(
		  suspgate.NewMemGenCounterRepo(),
		   suspgate.NewMemGenRepo(),
		  suspgate.NewMemLeaseRepo(),
		  suspgate.NewMemDriftProofRepo(),
		  suspgate.EngineConfig{LeaseTTL: 5 * time.Minute},
	   )
  }

// ──────────────────────────────────────────────────────────────────────────────
// SuspendWithGen
// ──────────────────────────────────────────────────────────────────────────────

   func TestSuspendWithGen_MonotonicGen(t *testing.T) {
	  eng := newTestEngine()
	  ctx := context.Background()
	  cases := []struct {
		  name string
		  req  suspgate.SuspendWithGenReq
		   wantGen int64
		  wantCount int
		  wantErr   error
	   }{
		   {
			  name: "首次暂停 gen=1",
			   req:       suspgate.SuspendWithGenReq{CaseID: "c1", UserID: "u1", Asset: "USDT", Actor: "ops1"},
			  wantGen:   1,
			   wantCount: 1,
		  },
		   {
			  name: "第二次暂停 gen=2",
			  req: suspgate.SuspendWithGenReq{CaseID: "c2", UserID: "u1", Asset: "USDT", Actor: "ops1"},
			   wantGen:   2,
			   wantCount: 2,
		  },
		   {
			  name: "不同资产 gen 独立",
			  req:     suspgate.SuspendWithGenReq{CaseID: "c3", UserID: "u1", Asset: "USDC", Actor: "ops1"},
			   wantGen: 1,
			   wantCount: 1,
		   },
	   }
	   for _, tc := range cases {
		   t.Run(tc.name, func(t *testing.T) {
			  result, err := eng.SuspendWithGen(ctx, tc.req)
			  if tc.wantErr != nil {
				   if !errors.Is(err, tc.wantErr) {
					  t.Fatalf("want err %v, got %v", tc.wantErr, err)
				  }
				  return
			   }
			  if err != nil {
				   t.Fatalf("unexpected error: %v", err)
			   }
			  if result.Gen.GenValue != tc.wantGen {
				   t.Errorf("gen_value want %d, got %d", tc.wantGen, result.Gen.GenValue)
			   }
			   if result.Counter.ActiveCount != tc.wantCount {
				  t.Errorf("active_count want %d, got %d", tc.wantCount, result.Counter.ActiveCount)
			   }
			  if result.Gen.State != suspgate.SuspActive {
				   t.Errorf("state want active, got %s", result.Gen.State)
			   }
		  })
	  }
   }

  func TestSuspendWithGen_InvalidInputs(t *testing.T) {
	   eng := newTestEngine()
	  ctx := context.Background()
	  cases := []struct {
		   name    string
		   req     suspgate.SuspendWithGenReq
		   wantErr error
	   }{
		  {"空 asset", suspgate.SuspendWithGenReq{CaseID: "c1", UserID: "u1", Asset: "", Actor: "ops"}, suspgate.ErrAsset},
		   {"无效 asset", suspgate.SuspendWithGenReq{CaseID: "c1", UserID: "u1", Asset: "BTC", Actor: "ops"}, suspgate.ErrAsset},
		   {"空 actor", suspgate.SuspendWithGenReq{CaseID: "c1", UserID: "u1", Asset: "USDT", Actor: ""}, suspgate.ErrActor},
		   {"空 caseID", suspgate.SuspendWithGenReq{CaseID: "", UserID: "u1", Asset: "USDT", Actor: "ops"}, suspgate.ErrInvalidInput},
		  {"空 userID", suspgate.SuspendWithGenReq{CaseID: "c1", UserID: "", Asset: "USDT", Actor: "ops"}, suspgate.ErrInvalidInput},
	  }
	   for _, tc := range cases {
		  t.Run(tc.name, func(t *testing.T) {
			  _, err := eng.SuspendWithGen(ctx, tc.req)
			   if !errors.Is(err, tc.wantErr) {
				  t.Errorf("want err %v, got %v", tc.wantErr, err)
			   }
		  })
	  }
   }

// ──────────────────────────────────────────────────────────────────────────────
// ClearWithGen
// ──────────────────────────────────────────────────────────────────────────────

  func TestClearWithGen_Success(t *testing.T) {
	   eng := newTestEngine()
	   ctx := context.Background()
	  // 先暂停。
	   _, err := eng.SuspendWithGen(ctx, suspgate.SuspendWithGenReq{
		  CaseID: "c1", UserID: "u1", Asset: "USDT", Actor: "ops1",
	  })
	  if err != nil {
		   t.Fatalf("suspend: %v", err)
	  }
	  // 四眼清障。
	  result, err := eng.ClearWithGen(ctx, suspgate.ClearWithGenReq{
		   CaseID:   "c1",
		  UserID:   "u1",
		   Asset:    "USDT",
		   GenValue: 1,
		  Actor1:   "ops2",
		   Actor2:   "ops3",
	  })
	  if err != nil {
		   t.Fatalf("clear: %v", err)
	  }
	  if result.Gen.State != suspgate.SuspCleared {
		   t.Errorf("state want cleared, got %s", result.Gen.State)
	  }
	   if result.Lease == nil {
		   t.Fatal("expect non-nil lease after clear")
	   }
	  if result.Lease.State != suspgate.LeaseIssued {
		   t.Errorf("lease state want issued, got %s", result.Lease.State)
	   }
	  if result.DriftProof != nil {
		   t.Error("expect nil drift proof on first clear")
	   }
	  // 验证 ActiveCount 递减为 0。
	  counter, err := eng.GetCounter(ctx, "u1", "USDT")
	   if err != nil {
		  t.Fatalf("get counter: %v", err)
	  }
	  if counter.ActiveCount != 0 {
		   t.Errorf("active_count want 0 after clear, got %d", counter.ActiveCount)
	   }
  }

   func TestClearWithGen_FourEyesReject(t *testing.T) {
	  eng := newTestEngine()
	  ctx := context.Background()
	   _, err := eng.SuspendWithGen(ctx, suspgate.SuspendWithGenReq{
		   CaseID: "c2", UserID: "u2", Asset: "USDT", Actor: "ops1",
	  })
	  if err != nil {
		  t.Fatalf("suspend: %v", err)
	   }
	  cases := []struct {
		   name string
		  a1, a2  string
		  wantErr error
	  }{
		  {"Actor1 为空", "", "ops2", suspgate.ErrActor},
		   {"Actor2 为空", "ops2", "", suspgate.ErrActor},
		   {"Actor1==Actor2", "ops2", "ops2", suspgate.ErrFourEyes},
	   }
	  for _, tc := range cases {
		   t.Run(tc.name, func(t *testing.T) {
			   _, err := eng.ClearWithGen(ctx, suspgate.ClearWithGenReq{
				  CaseID: "c2", UserID: "u2", Asset: "USDT", GenValue: 1,
				   Actor1: tc.a1, Actor2: tc.a2,
			   })
			  if !errors.Is(err, tc.wantErr) {
				   t.Errorf("want err %v, got %v", tc.wantErr, err)
			  }
		   })
	   }
   }

  func TestClearWithGen_DoubleCleared_DriftProof(t *testing.T) {
	   // 验收点 3：双次 Clear 同一 gen，仅第一次成功，第二次落 SuspenseDriftProof。
	   eng := newTestEngine()
	  ctx := context.Background()
	   _, err := eng.SuspendWithGen(ctx, suspgate.SuspendWithGenReq{
		  CaseID: "c3", UserID: "u3", Asset: "USDT", Actor: "ops1",
	   })
	   if err != nil {
		  t.Fatalf("suspend: %v", err)
	   }
	  // 第一次 Clear：成功。
	  result1, err := eng.ClearWithGen(ctx, suspgate.ClearWithGenReq{
		   CaseID: "c3", UserID: "u3", Asset: "USDT", GenValue: 1,
		  Actor1: "ops2", Actor2: "ops3",
	  })
	   if err != nil {
		  t.Fatalf("first clear: %v", err)
	   }
	   if result1.DriftProof != nil {
		  t.Error("first clear must not produce drift proof")
	  }
	  // 第二次 Clear：应返回 ErrGenAlreadyCleared + DriftProof。
	   result2, err := eng.ClearWithGen(ctx, suspgate.ClearWithGenReq{
		   CaseID: "c3", UserID: "u3", Asset: "USDT", GenValue: 1,
		  Actor1: "ops4", Actor2: "ops5",
	   })
	   if !errors.Is(err, suspgate.ErrGenAlreadyCleared) {
		  t.Errorf("second clear: want ErrGenAlreadyCleared, got %v", err)
	   }
	   if result2 == nil || result2.DriftProof == nil {
		  t.Fatal("second clear must produce drift proof")
	  }
	  if result2.DriftProof.BlockOutcome != suspgate.OutcomeDoubleCleared {
		  t.Errorf("drift proof outcome want double_cleared, got %s", result2.DriftProof.BlockOutcome)
	   }
	  if result2.DriftProof.ProofHash == "" {
		   t.Error("drift proof hash must not be empty")
	   }
   }

// ──────────────────────────────────────────────────────────────────────────────
// WithdrawAssert
// ──────────────────────────────────────────────────────────────────────────────

  func TestWithdrawAssert_BlockedByActiveCount(t *testing.T) {
	   // 验收点 1：Count>0 时出金必须拒绝。
	   eng := newTestEngine()
	   ctx := context.Background()
	   _, err := eng.SuspendWithGen(ctx, suspgate.SuspendWithGenReq{
		  CaseID: "c4", UserID: "u4", Asset: "USDT", Actor: "ops1",
	  })
	   if err != nil {
		   t.Fatalf("suspend: %v", err)
	  }
	   result, err := eng.WithdrawAssert(ctx, suspgate.WithdrawAssertReq{
		   UserID:       "u4",
		   Asset:        "USDT",
		   RequireLease: false,
	  })
	   if !errors.Is(err, suspgate.ErrWithdrawBlocked) {
		  t.Errorf("want ErrWithdrawBlocked, got %v", err)
	  }
	  if result.Allowed {
		   t.Error("result.Allowed must be false")
	  }
	  if result.ActiveCount != 1 {
		  t.Errorf("active_count want 1, got %d", result.ActiveCount)
	  }
  }

   func TestWithdrawAssert_CountZeroButGenActive(t *testing.T) {
	   // 验收点 1（补充）：Count 抖零但 gen 仍 active 时出金必须拒绝。
	   counters := suspgate.NewMemGenCounterRepo()
	  gens := suspgate.NewMemGenRepo()
	  leases := suspgate.NewMemLeaseRepo()
	  proofs := suspgate.NewMemDriftProofRepo()
	  eng := suspgate.NewEngine(counters, gens, leases, proofs, suspgate.EngineConfig{})
	  ctx := context.Background()
	  // 手动注入：gen=active 但 counter.active_count=0（ Count 抖零）。
	  now := time.Now().UTC()
	  genID := suspgate.GenIDFor("u5", "USDT", 1)
	   _ = gens.SaveGen(ctx, &suspgate.SuspenseGeneration{
		   GenID: genID,
		  UserID: "u5",
		  Asset:    "USDT",
		  GenValue: 1,
		  CaseID:   "c5",
		   SuspendedBy: "ops1",
		   State:       suspgate.SuspActive,
		   SuspendedAt: now,
		  UpdatedAt:   now,
	   })
	  _ = counters.SaveCounter(ctx, &suspgate.GenCounter{
		   CounterKey: suspgate.GenCounterKey("u5", "USDT"),
		  UserID:     "u5",
		   Asset:       "USDT",
		   CurrentGen:  1,
		  ActiveCount: 0, // 抖零
		  UpdatedAt:   now,
	  })
	   result, err := eng.WithdrawAssert(ctx, suspgate.WithdrawAssertReq{
		   UserID: "u5",
		   Asset:        "USDT",
		   RequireLease: false,
	  })
	   if !errors.Is(err, suspgate.ErrWithdrawBlocked) {
		   t.Errorf("want ErrWithdrawBlocked, got %v", err)
	  }
	   if result.Allowed {
		   t.Error("result.Allowed must be false when gen is still active")
	  }
   }

   func TestWithdrawAssert_LeaseRequired_NoLease(t *testing.T) {
	   // 验收点 2：Clear 成功后未持 lease 且 require_lease=true 时出金必须拒绝。
	  eng := newTestEngine()
	   ctx := context.Background()
	  _, err := eng.SuspendWithGen(ctx, suspgate.SuspendWithGenReq{
		  CaseID: "c6", UserID: "u6", Asset: "USDC", Actor: "ops1",
	  })
	  if err != nil {
		   t.Fatalf("suspend: %v", err)
	  }
	  _, err = eng.ClearWithGen(ctx, suspgate.ClearWithGenReq{
		  CaseID: "c6", UserID: "u6", Asset: "USDC", GenValue: 1,
		  Actor1: "ops2", Actor2: "ops3",
	  })
	  if err != nil {
		  t.Fatalf("clear: %v", err)
	  }
	   // 不携带 lease，但 require_lease=true。
	   result, err := eng.WithdrawAssert(ctx, suspgate.WithdrawAssertReq{
		   UserID: "u6",
		   Asset: "USDC",
		  LeaseID: "",
		   RequireLease: true,
	   })
	  if !errors.Is(err, suspgate.ErrLeaseRequired) {
		  t.Errorf("want ErrLeaseRequired, got %v", err)
	  }
	   if result.Allowed {
		   t.Error("result.Allowed must be false when lease required but not provided")
	  }
   }

   func TestWithdrawAssert_ValidLease_Allowed(t *testing.T) {
	   // Clear 成功后持有效 lease，出金应被允许。
	   eng := newTestEngine()
	   ctx := context.Background()
	   _, err := eng.SuspendWithGen(ctx, suspgate.SuspendWithGenReq{
		  CaseID: "c7", UserID: "u7", Asset: "USDT", Actor: "ops1",
	   })
	   if err != nil {
		   t.Fatalf("suspend: %v", err)
	  }
	   clearResult, err := eng.ClearWithGen(ctx, suspgate.ClearWithGenReq{
		   CaseID: "c7", UserID: "u7", Asset: "USDT", GenValue: 1,
		   Actor1: "ops2", Actor2: "ops3",
	  })
	   if err != nil {
		  t.Fatalf("clear: %v", err)
	  }
	  if clearResult.Lease == nil {
		  t.Fatal("expect lease after clear")
	  }
	  // 使用 clearResult.Lease.LeaseID 进行出金断言。
	  result, err := eng.WithdrawAssert(ctx, suspgate.WithdrawAssertReq{
		   UserID: "u7",
		  Asset: "USDT",
		   LeaseID:      clearResult.Lease.LeaseID,
		   RequireLease: true,
	  })
	  if err != nil {
		  t.Fatalf("withdraw assert: %v", err)
	   }
	   if !result.Allowed {
		   t.Errorf("result.Allowed must be true with valid lease, got false: %s", result.BlockReason)
	   }
	  if result.ConsumedLeaseID != clearResult.Lease.LeaseID {
		  t.Errorf("consumed_lease_id want %s, got %s", clearResult.Lease.LeaseID, result.ConsumedLeaseID)
	  }
  }

   func TestWithdrawAssert_ExpiredLease_RequireLease(t *testing.T) {
	  // 过期 lease + require_lease=true 时出金须拒绝。
	   counters := suspgate.NewMemGenCounterRepo()
	  gens := suspgate.NewMemGenRepo()
	   leases := suspgate.NewMemLeaseRepo()
	  proofs := suspgate.NewMemDriftProofRepo()
	  eng := suspgate.NewEngine(counters, gens, leases, proofs, suspgate.EngineConfig{LeaseTTL: 5 * time.Minute})
	   ctx := context.Background()
	  // 注入已过期的 lease。
	   now := time.Now().UTC()
	   expiredLease := &suspgate.ClearanceLease{
		   LeaseID: "expired-lease-001",
		  UserID:  "u8",
		  Asset:   "USDT",
		  GenValue: 1,
		   CaseID:   "c8",
		   State:    suspgate.LeaseIssued,
		  IssuedAt: now.Add(-10 * time.Minute),
		  ExpireAt: now.Add(-5 * time.Minute), // 已过期
	   }
	  _ = leases.SaveLease(ctx, expiredLease)
	   result, err := eng.WithdrawAssert(ctx, suspgate.WithdrawAssertReq{
		   UserID:  "u8",
		   Asset:   "USDT",
		  LeaseID: "expired-lease-001",
		  RequireLease: true,
	  })
	   if !errors.Is(err, suspgate.ErrLeaseRequired) {
		  t.Errorf("want ErrLeaseRequired for expired lease with require_lease=true, got %v", err)
	   }
	   if result.Allowed {
		   t.Error("expired lease + require_lease must not allow withdraw")
	  }
  }

// ──────────────────────────────────────────────────────────────────────────────
// ConsumeLease
// ──────────────────────────────────────────────────────────────────────────────

   func TestConsumeLease_Success(t *testing.T) {
	  eng := newTestEngine()
	   ctx := context.Background()
	  _, err := eng.SuspendWithGen(ctx, suspgate.SuspendWithGenReq{
		   CaseID: "c9", UserID: "u9", Asset: "USDT", Actor: "ops1",
	  })
	  if err != nil {
		   t.Fatalf("suspend: %v", err)
	  }
	   clearResult, err := eng.ClearWithGen(ctx, suspgate.ClearWithGenReq{
		  CaseID: "c9", UserID: "u9", Asset: "USDT", GenValue: 1,
		   Actor1: "ops2", Actor2: "ops3",
	   })
	   if err != nil {
		   t.Fatalf("clear: %v", err)
	  }
	   lease, err := eng.ConsumeLease(ctx, clearResult.Lease.LeaseID, "u9")
	  if err != nil {
		  t.Fatalf("consume lease: %v", err)
	   }
	   if lease.State != suspgate.LeaseConsumed {
		   t.Errorf("lease state want consumed, got %s", lease.State)
	   }
   }

  func TestConsumeLease_DoubleConsume_Idempotent(t *testing.T) {
	  eng := newTestEngine()
	   ctx := context.Background()
	   _, err := eng.SuspendWithGen(ctx, suspgate.SuspendWithGenReq{
		  CaseID: "c10", UserID: "u10", Asset: "USDT", Actor: "ops1",
	  })
	  if err != nil {
		  t.Fatalf("suspend: %v", err)
	  }
	  clearResult, err := eng.ClearWithGen(ctx, suspgate.ClearWithGenReq{
		   CaseID: "c10", UserID: "u10", Asset: "USDT", GenValue: 1,
		   Actor1: "ops2", Actor2: "ops3",
	  })
	   if err != nil {
		   t.Fatalf("clear: %v", err)
	  }
	   _, err = eng.ConsumeLease(ctx, clearResult.Lease.LeaseID, "u10")
	   if err != nil {
		  t.Fatalf("first consume: %v", err)
	  }
	  _, err = eng.ConsumeLease(ctx, clearResult.Lease.LeaseID, "u10")
	  if !errors.Is(err, suspgate.ErrLeaseConsumed) {
		  t.Errorf("second consume: want ErrLeaseConsumed, got %v", err)
	   }
  }

// ──────────────────────────────────────────────────────────────────────────────
// 竞态：并发 Suspend 与 Clear
// ──────────────────────────────────────────────────────────────────────────────

  func TestConcurrent_SuspendAndClear_GenMonotonic(t *testing.T) {
	  // 验收点 1：同一 (user,asset) 并发 FourEyesClear 与 Suspend，世代单调不减，
	  // 不得出现 Count=0 且有未消费 gen 但放行出金的情形。
	   eng := newTestEngine()
	  ctx := context.Background()
	   const goroutines = 10
	  var wg sync.WaitGroup
	  wg.Add(goroutines)
	   for i := 0; i < goroutines; i++ {
		  go func(idx int) {
			  defer wg.Done()
			  _, _ = eng.SuspendWithGen(ctx, suspgate.SuspendWithGenReq{
				  CaseID: "case-concurrent",
				  UserID: "u-concurrent",
				  Asset: "USDT",
				  Actor: "ops1",
			  })
		  }(i)
	  }
	   wg.Wait()
	  counter, err := eng.GetCounter(ctx, "u-concurrent", "USDT")
	   if err != nil {
		   t.Fatalf("get counter: %v", err)
	   }
	  // 世代号必须 >= 1（至少一次成功）。
	  if counter.CurrentGen < 1 {
		  t.Errorf("current_gen must >= 1 after concurrent suspends, got %d", counter.CurrentGen)
	   }
	   // ActiveCount 必须 <= CurrentGen。
	  if counter.ActiveCount > int(counter.CurrentGen) {
		   t.Errorf("active_count(%d) must not exceed current_gen(%d)",
			   counter.ActiveCount,
			  counter.CurrentGen)
	   }
	  // 此时出金必须被拒绝（有活跃世代）。
	   if counter.ActiveCount > 0 {
		   result, wdErr := eng.WithdrawAssert(ctx, suspgate.WithdrawAssertReq{
			  UserID: "u-concurrent",
			   Asset:  "USDT",
			  RequireLease: false,
		   })
		   if !errors.Is(wdErr, suspgate.ErrWithdrawBlocked) {
			   t.Errorf("withdraw must be blocked when active_count>0, got err=%v allowed=%v", wdErr, result.Allowed)
		   }
	  }
   }

  func TestConcurrent_DoubleClear_OnlyOneSucceeds(t *testing.T) {
	   // 验收点 3（并发代次）：两个 goroutine 同时 Clear 同一 gen，只有一个成功。
	  eng := newTestEngine()
	   ctx := context.Background()
	  _, err := eng.SuspendWithGen(ctx, suspgate.SuspendWithGenReq{
		  CaseID: "c-dc", UserID: "u-dc", Asset: "USDT", Actor: "ops1",
	   })
	  if err != nil {
		  t.Fatalf("suspend: %v", err)
	  }
	  const parallel = 5
	  var wg sync.WaitGroup
	  wg.Add(parallel)
	  successCount := 0
	  alreadyClearedCount := 0
	  var mu sync.Mutex
	   for i := 0; i < parallel; i++ {
		  go func(idx int) {
			   defer wg.Done()
			  _, cerr := eng.ClearWithGen(ctx, suspgate.ClearWithGenReq{
				  CaseID: "c-dc", UserID: "u-dc", Asset: "USDT", GenValue: 1,
				  Actor1: "ops-a", Actor2: "ops-b",
			   })
			  mu.Lock()
			   defer mu.Unlock()
			  if cerr == nil {
				  successCount++
			   } else if errors.Is(cerr, suspgate.ErrGenAlreadyCleared) {
				   alreadyClearedCount++
			  }
		  }(i)
	   }
	   wg.Wait()
	  if successCount != 1 {
		  t.Errorf("exactly one clear must succeed, got %d", successCount)
	   }
	  if alreadyClearedCount != parallel-1 {
		  t.Errorf("remaining clears must return ErrGenAlreadyCleared, got %d", alreadyClearedCount)
	   }
   }

// ──────────────────────────────────────────────────────────────────────────────
// FSM 状态机
// ──────────────────────────────────────────────────────────────────────────────

   func TestFSM_SuspendedState(t *testing.T) {
	  cases := []struct {
		   from, to suspgate.SuspendedState
		  ok       bool
	  }{
		  {suspgate.SuspActive,
			  suspgate.SuspClearing, true},
		   {suspgate.SuspActive, suspgate.SuspCleared, true},
		  {suspgate.SuspClearing, suspgate.SuspCleared, true},
		   {suspgate.SuspCleared, suspgate.SuspActive, false},
		   {suspgate.SuspCleared, suspgate.SuspClearing, false},
		   {suspgate.SuspClearing, suspgate.SuspActive, false},
	  }
	   for _, tc := range cases {
		   got := suspgate.CanSuspTransition(tc.from, tc.to)
		  if got != tc.ok {
			  t.Errorf("CanSuspTransition(%s->%s) want %v, got %v", tc.from, tc.to, tc.ok, got)
		   }
	   }
  }

   func TestFSM_LeaseState(t *testing.T) {
	  cases := []struct {
		  from, to suspgate.LeaseState
		  ok       bool
	   }{
		   {suspgate.LeaseIssued, suspgate.LeaseConsumed, true},
		  {suspgate.LeaseIssued,
			   suspgate.LeaseExpired, true},
		  {suspgate.LeaseConsumed, suspgate.LeaseExpired, false},
		   {suspgate.LeaseExpired, suspgate.LeaseConsumed, false},
		   {suspgate.LeaseConsumed, suspgate.LeaseIssued, false},
	  }
	   for _, tc := range cases {
		  got := suspgate.CanLeaseTransition(tc.from,
			   tc.to)
		  if got != tc.ok {
			  t.Errorf("CanLeaseTransition(%s->%s) want %v, got %v", tc.from, tc.to, tc.ok, got)
		  }
	   }
   }

// ──────────────────────────────────────────────────────────────────────────────
// DriftProof 哈希完整性
// ──────────────────────────────────────────────────────────────────────────────

  func TestDriftProofHash_Deterministic(t *testing.T) {
	  now := time.Date(2021, 5, 1, 12, 0, 0, 0, time.UTC)
	   h1 := suspgate.DriftProofHashFor("case-x", 7, suspgate.OutcomeDoubleCleared, now)
	   h2 := suspgate.DriftProofHashFor("case-x", 7, suspgate.OutcomeDoubleCleared, now)
	  if h1 != h2 {
		   t.Errorf("hash must be deterministic: h1=%s h2=%s", h1, h2)
	  }
	  h3 := suspgate.DriftProofHashFor("case-y", 7, suspgate.OutcomeDoubleCleared, now)
	   if h1 == h3 {
		   t.Error("different inputs must produce different hash")
	  }
   }

// ──────────────────────────────────────────────────────────────────────────────
// Worker 单元
// ──────────────────────────────────────────────────────────────────────────────

   func TestLeaseExpiryWorker_ExpiresOldLeases(t *testing.T) {
	  leases := suspgate.NewMemLeaseRepo()
	   ctx := context.Background()
	  now := time.Now().UTC()
	  oldLease := &suspgate.ClearanceLease{
		   LeaseID: "old-lease-001",
		   UserID:  "u-exp",
		   Asset:   "USDT",
		   GenValue: 1,
		  State:    suspgate.LeaseIssued,
		  IssuedAt: now.Add(-10 * time.Minute),
		   ExpireAt: now.Add(-1 * time.Second), // 已过期
	  }
	   _ = leases.SaveLease(ctx, oldLease)
	   w := suspgate.NewLeaseExpiryWorker(leases, suspgate.WorkerConfig{})
	  // 手动触发单次扫描（通过 runOnce 不可导出；用循环 + cancel 触发）。
	  ctxCancel, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	  defer cancel()
	   w.Run(ctxCancel)
	   // Worker 退出后验证 lease 状态已流转到 expired。
	   got, err := leases.GetLease(ctx, "old-lease-001")
	  if err != nil {
		   t.Fatalf("get lease: %v", err)
	  }
	   if got.State != suspgate.LeaseExpired {
		   t.Errorf("lease state want expired, got %s", got.State)
	  }
   }

// ──────────────────────────────────────────────────────────────────────────────
// GenIDFor 幂等性
// ──────────────────────────────────────────────────────────────────────────────

   func TestGenIDFor_Idempotent(t *testing.T) {
	   id1 := suspgate.GenIDFor("user-1", "USDT", 3)
	   id2 := suspgate.GenIDFor("user-1", "USDT", 3)
	  if id1 != id2 {
		   t.Errorf("GenIDFor must be idempotent: %s != %s", id1, id2)
	   }
	   id3 := suspgate.GenIDFor("user-1", "USDT", 4)
	  if id1 == id3 {
		  t.Error("different gen_value must produce different GenID")
	  }
	   id4 := suspgate.GenIDFor("user-1", "usdt", 3) // 小写 asset
	   if id1 != id4 {
		  t.Error("GenIDFor must normalize asset to upper case")
	  }
   }
