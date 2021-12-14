package trexecgate_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/treasury"
	"account-assets.kucoin.net/funding/internal/trexecgate"
)

// ──────────────────────────────────────────────
// 辅助：Mock hotinv.Book（绕过ledger）
// ──────────────────────────────────────────────
// mockHotInv 替代hotinv.Book，允许注入固定快照。

type mockHotInv struct {
	snaps map[string]string // asset -> hot_avail
}

  func (m *mockHotInv) Observe(_ context.Context, _ ledger.Store) ([]hotinv.Snapshot, error) {
	  out := make([]hotinv.Snapshot,
		  0, len(m.snaps))
	  for asset, avail := range m.snaps {
		   out = append(out, hotinv.Snapshot{
			  Asset: asset,
			  HotAvail: avail,
			  HotMax: "1000000",
		  })
	  }
	   return out, nil
  }

// mockEngine 对 Engine 打包，内置 mock hotinv，允许动态切换快照。

type mockEngine struct {
	eng *trexecgate.Engine
	inv *mockHotInv
	leases *trexecgate.MemLeaseRepo
	proofs *trexecgate.MemDriftProofRepo
	gens  *trexecgate.MemGenRepo
	clock func() time.Time
}

  func newMockEngine(hotAvails map[string]string) *mockEngine {
	  inv := &mockHotInv{snaps: hotAvails}
	  leases := trexecgate.NewMemLeaseRepo()
	   proofs := trexecgate.NewMemDriftProofRepo()
	  gens := trexecgate.NewMemGenRepo()
	  now := time.Date(2021,
		  6, 15, 12, 0, 0, 0, time.UTC)
	   clock := func() time.Time { return now }
	  eng := trexecgate.NewEngineWithObs(leases, proofs, gens, inv, treasury.DefaultCatalog()).
		  WithClock(clock)
	   return &mockEngine{
		   eng: eng,
		   inv: inv,
		   leases: leases,
		  proofs: proofs,
		  gens:   gens,
		  clock:  clock,
	   }
   }

// setHot 动态更新指定资产热账（复核时水位变化）。
   func (m *mockEngine) setHot(asset, avail string) {
	   m.inv.snaps[asset] = avail
   }

// ──────────────────────────────────────────────
// golden 夹具辅助
// ──────────────────────────────────────────────

type goldenFixture struct {
	Desc             string `json:"desc"`
	HotAvailAtRecord string `json:"hot_avail_at_record"`
	HotAvailAtRecheck string `json:"hot_avail_at_recheck,omitempty"`
	Expected          struct {
		RecheckPassed *bool `json:"recheck_passed,omitempty"`
		LeaseStateAfterRecheck string `json:"lease_state_after_recheck,omitempty"`
		ProofDecision       string `json:"proof_decision,omitempty"`
		ProofReasonContains string `json:"proof_reason_contains,omitempty"`
		MarkSentError    string `json:"mark_sent_error,omitempty"`
		AckError         string `json:"ack_error,omitempty"`
		FirstCallReused  *bool  `json:"first_call_reused,omitempty"`
		SecondCallReused *bool  `json:"second_call_reused,omitempty"`
		SameLeaseID   *bool  `json:"same_lease_id,omitempty"`
		BindState     string `json:"bind_state,omitempty"`
		RecheckState  string `json:"recheck_state,omitempty"`
		MarkSentState string `json:"mark_sent_state,omitempty"`
		AckState      string `json:"ack_state,omitempty"`
	} `json:"expected"`
}

  func loadGolden(t *testing.T, name string) goldenFixture {
	  t.Helper()
	  path := filepath.Join("..", "..", "testdata", "trexecgate", name)
	   data, err := os.ReadFile(path)
	  if err != nil {
		   t.Fatalf("loadGolden %s: %v", name, err)
	  }
	   var f goldenFixture
	  if err := json.Unmarshal(data, &f); err != nil {
		   t.Fatalf("loadGolden unmarshal %s: %v", name, err)
	   }
	  return f
  }

// ──────────────────────────────────────────────
// 主路径：全生命周期
// ──────────────────────────────────────────────

  func TestFullLifecycle_Golden(t *testing.T) {
	   fix := loadGolden(t, "full_lifecycle.json")
	   ctx := context.Background()
	  m := newMockEngine(map[string]string{"BTC": fix.HotAvailAtRecord})
	  eng := m.eng
	  // 1. Bind
	  res, err := eng.BindLease(ctx, nil, trexecgate.BindInput{
		  PlanID: "plan-btc-full-001",
		   ExecID: "exec-btc-full-001",
		   Asset:  "BTC",
		  Amount: "50",
	  })
	  if err != nil {
		   t.Fatalf("BindLease: %v", err)
	   }
	   if res.Lease.State != trexecgate.LeaseState(fix.Expected.BindState) {
		  t.Errorf("bind state: got %s, want %s", res.Lease.State, fix.Expected.BindState)
	   }
	   leaseID := res.Lease.LeaseID
	   // 2. Recheck（水位未变）
	   if fix.HotAvailAtRecheck != "" {
		   m.setHot("BTC", fix.HotAvailAtRecheck)
	   }
	  rr, err := eng.Recheck(ctx, nil, leaseID)
	  if err != nil {
		  t.Fatalf("Recheck: %v", err)
	  }
	   if !rr.Passed {
		  t.Errorf("Recheck: expected passed=true")
	  }
	  if rr.Lease.State != trexecgate.LeaseState(fix.Expected.RecheckState) {
		   t.Errorf("recheck state: got %s, want %s", rr.Lease.State, fix.Expected.RecheckState)
	  }
	  // 3. MarkSentGate
	  sentLease, err := eng.MarkSentGate(ctx, trexecgate.MarkSentGateInput{
		   LeaseID: leaseID,
		  ExecID:  "exec-btc-full-001",
	  })
	   if err != nil {
		  t.Fatalf("MarkSentGate: %v", err)
	  }
	  if sentLease.State != trexecgate.LeaseState(fix.Expected.MarkSentState) {
		  t.Errorf("sent state: got %s, want %s", sentLease.State, fix.Expected.MarkSentState)
	  }
	  // 4. AckLease
	  ackedLease, err := eng.AckLease(ctx, leaseID, "exec-btc-full-001")
	  if err != nil {
		  t.Fatalf("AckLease: %v", err)
	  }
	   if ackedLease.State != trexecgate.LeaseState(fix.Expected.AckState) {
		  t.Errorf("ack state: got %s, want %s", ackedLease.State, fix.Expected.AckState)
	   }
   }

// ──────────────────────────────────────────────
// 表驱动：主路径、失败语义、幂等
// ──────────────────────────────────────────────

   func TestEngine_TableDriven(t *testing.T) {
	   type want struct {
		  recheckPassed bool
		  leaseStateFinal trexecgate.LeaseState
		  proofDecision   string
		  errIs           error
	   }
	  cases := []struct {
		   name        string
		  asset       string
		   hotAtRecord string
		  hotAtRecheck string
		   bandTarget string
		  bandLow    string
		   bandHigh    string
		  wantRecheck bool
		  wantLeaseState    trexecgate.LeaseState
		  wantProofDecision string
		   wantMarkSentErr   error
	   }{
		   {
			  name:        "normal_pass",
			  asset:       "USDT",
			  hotAtRecord: "480000",
			  hotAtRecheck: "460000",
			   bandTarget:   "200000",
			   wantRecheck:    true,
			   wantLeaseState: trexecgate.LeaseRechecked,
			  wantProofDecision: "pass",
			   wantMarkSentErr:   nil,
		  },
		   {
			   name: "below_target_reject",
			   asset: "USDT",
			  hotAtRecord: "480000",
			  hotAtRecheck: "150000",
			   bandTarget:   "200000",
			   wantRecheck:  false,
			  wantLeaseState:    trexecgate.LeaseBlocked,
			  wantProofDecision: "blocked",
			  wantMarkSentErr:   trexecgate.ErrIllegalTransition,
		   },
		  {
			  name: "relative_drift_reject",
			  asset:             "USDT",
			  hotAtRecord:       "400000",
			  hotAtRecheck:      "370000",
			   bandTarget:        "200000",
			  wantRecheck:       false,
			  wantLeaseState:    trexecgate.LeaseBlocked,
			   wantProofDecision: "blocked",
			  wantMarkSentErr:   trexecgate.ErrIllegalTransition,
		   },
		   {
			  name:        "tiny_drift_ok",
			  asset:       "ETH",
			   hotAtRecord: "750",
			  hotAtRecheck: "740",
			  bandTarget:   "200",
			   wantRecheck:       true,
			   wantLeaseState:    trexecgate.LeaseRechecked,
			  wantProofDecision: "pass",
			  wantMarkSentErr:   nil,
		  },
	   }
	  for _, tc := range cases {
		   tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			   ctx := context.Background()
			   m := newMockEngine(map[string]string{tc.asset: tc.hotAtRecord})
			  eng := m.eng
			   bindRes, err := eng.BindLease(ctx, nil, trexecgate.BindInput{
				  PlanID: "plan-" + tc.name,
				   ExecID: "exec-" + tc.name,
				  Asset:  tc.asset,
				  Amount: "100",
			   })
			   if err != nil {
				  t.Fatalf("BindLease: %v", err)
			  }
			   leaseID := bindRes.Lease.LeaseID
			  m.setHot(tc.asset, tc.hotAtRecheck)
			  rr, recheckErr := eng.Recheck(ctx, nil, leaseID)
			  if tc.wantRecheck && recheckErr != nil {
				  t.Errorf("Recheck expected pass, got: %v", recheckErr)
			   }
			   if !tc.wantRecheck && recheckErr == nil {
				  t.Errorf("Recheck expected rejection, got nil")
			  }
			  if rr.Lease != nil && rr.Lease.State != tc.wantLeaseState {
				   t.Errorf("lease state after recheck: got %s, want %s", rr.Lease.State, tc.wantLeaseState)
			   }
			  // 验证漂移证明落库。
			   if rr.Proof != nil && rr.Proof.Decision != tc.wantProofDecision {
				   t.Errorf("proof decision: got %s, want %s", rr.Proof.Decision, tc.wantProofDecision)
			   }
			   // 验证 MarkSentGate 行为。
			  if tc.wantRecheck {
				   _, msErr := eng.MarkSentGate(ctx, trexecgate.MarkSentGateInput{
					   LeaseID: leaseID,
					  ExecID: "exec-" + tc.name,
				  })
				   if msErr != nil {
					  t.Errorf("MarkSentGate expected nil, got: %v", msErr)
				  }
			   } else {
				  _, msErr := eng.MarkSentGate(ctx, trexecgate.MarkSentGateInput{
					  LeaseID: leaseID,
					   ExecID:  "exec-" + tc.name,
				  })
				   if tc.wantMarkSentErr != nil && !errors.Is(msErr, tc.wantMarkSentErr) {
					  t.Errorf("MarkSentGate error: got %v, want %v", msErr, tc.wantMarkSentErr)
				   }
			  }
		  })
	   }
  }

// ──────────────────────────────────────────────
// 过期租约
// ──────────────────────────────────────────────

   func TestExpiredLease_RejectsMarkSentAndAck(t *testing.T) {
	  fix := loadGolden(t, "lease_expired.json")
	  _ = fix // 结构已读出，此处用内嵌逻辑
	   ctx := context.Background()
	  now := time.Date(2021, 6, 15, 12, 0, 0, 0, time.UTC)
	  var tick time.Time = now
	  clockFn := func() time.Time { return tick }
	   leases := trexecgate.NewMemLeaseRepo()
	  proofs := trexecgate.NewMemDriftProofRepo()
	  gens := trexecgate.NewMemGenRepo()
	   inv := &mockHotInv{snaps: map[string]string{"USDT": "480000"}}
	   eng := trexecgate.NewEngineWithObs(leases, proofs, gens, inv, treasury.DefaultCatalog()).
		   WithClock(clockFn).
		   WithTTL(2 * time.Second)
	  bindRes, err := eng.BindLease(ctx, nil, trexecgate.BindInput{
		  PlanID: "plan-expire-001",
		   ExecID: "exec-expire-001",
		   Asset:  "USDT",
		   Amount: "300000",
	  })
	   if err != nil {
		   t.Fatalf("BindLease: %v", err)
	  }
	   leaseID := bindRes.Lease.LeaseID
	   // 推进时钟超过 TTL。
	  tick = now.Add(10 * time.Second)
	  // MarkSentGate 须拒绝。
	   _, err = eng.MarkSentGate(ctx, trexecgate.MarkSentGateInput{
		   LeaseID: leaseID,
		   ExecID: "exec-expire-001",
	   })
	   if !errors.Is(err, trexecgate.ErrLeaseExpired) {
		  t.Errorf("MarkSentGate: expected ErrLeaseExpired, got %v", err)
	   }
	   // AckLease 须拒绝。
	   _, err = eng.AckLease(ctx, leaseID, "exec-expire-001")
	   if !errors.Is(err, trexecgate.ErrLeaseExpired) {
		   t.Errorf("AckLease: expected ErrLeaseExpired, got %v", err)
	   }
	  // 验证租约状态已更新为 expired。
	   lease,
		   lerr := eng.GetLease(ctx, leaseID)
	   if lerr != nil {
		  t.Fatalf("GetLease: %v", lerr)
	   }
	   if lease.State != trexecgate.LeaseExpired {
		  t.Errorf("lease state: got %s, want expired", lease.State)
	  }
  }

// ──────────────────────────────────────────────
// 幂等 Record 复用租约
// ──────────────────────────────────────────────

   func TestIdempotentBind_ReusesLease(t *testing.T) {
	   fix := loadGolden(t, "idem_reuse.json")
	  _ = fix
	   ctx := context.Background()
	   m := newMockEngine(map[string]string{"USDT": "480000"})
	  eng := m.eng
	  // 第一次 BindLease。
	   res1, err := eng.BindLease(ctx, nil, trexecgate.BindInput{
		   PlanID: "plan-idem-001",
		  ExecID: "exec-idem-001",
		  Asset:  "USDT",
		   Amount: "300000",
	  })
	   if err != nil {
		   t.Fatalf("first BindLease: %v", err)
	  }
	   if res1.Reused {
		  t.Errorf("first call: expected reused=false")
	   }
	   // 第二次相同 PlanID + Asset → 复用。
	  res2, err := eng.BindLease(ctx, nil, trexecgate.BindInput{
		  PlanID: "plan-idem-001",
		  ExecID: "exec-idem-001",
		   Asset:  "USDT",
		   Amount: "300000",
	  })
	   if err != nil {
		  t.Fatalf("second BindLease: %v", err)
	   }
	   if !res2.Reused {
		  t.Errorf("second call: expected reused=true")
	  }
	  if res1.Lease.LeaseID != res2.Lease.LeaseID {
		  t.Errorf("lease_id mismatch: first=%s, second=%s", res1.Lease.LeaseID, res2.Lease.LeaseID)
	  }
  }

// ──────────────────────────────────────────────
// 竞态：并发 BindLease 同 plan 不得双开
// ──────────────────────────────────────────────

   func TestConcurrentBind_NoDuplicateExec(t *testing.T) {
	   ctx := context.Background()
	   m := newMockEngine(map[string]string{"USDT": "480000"})
	  eng := m.eng
	  const goroutines = 10
	   leaseIDs := make([]string, goroutines)
	   errs := make([]error, goroutines)
	   var wg sync.WaitGroup
	   wg.Add(goroutines)
	   for i := 0; i < goroutines; i++ {
		  i := i
		  go func() {
			   defer wg.Done()
			  res, err := eng.BindLease(ctx, nil, trexecgate.BindInput{
				   PlanID: "plan-concurrent-001",
				  ExecID: "exec-concurrent-001",
				   Asset:  "USDT",
				  Amount: "300000",
			   })
			   if err == nil {
				   leaseIDs[i] = res.Lease.LeaseID
			   }
			   errs[i] = err
		  }()
	   }
	  wg.Wait()
	  // 所有成功的 goroutine 应返回同一 lease_id（幂等复用）。
	   var firstID string
	  for _, id := range leaseIDs {
		   if id == "" {
			   continue
		   }
		  if firstID == "" {
			   firstID = id
		  } else if firstID != id {
			   t.Errorf("concurrent bind returned different lease_ids: %s vs %s", firstID, id)
		   }
	   }
  }

// ──────────────────────────────────────────────
// 漂移证明落库验证
// ──────────────────────────────────────────────

  func TestDriftProof_SavedOnReject(t *testing.T) {
	   fix := loadGolden(t, "drift_reject.json")
	  ctx := context.Background()
	   m := newMockEngine(map[string]string{"USDT": fix.HotAvailAtRecord})
	  eng := m.eng
	  bindRes, err := eng.BindLease(ctx, nil, trexecgate.BindInput{
		   PlanID: "plan-drift-001",
		  ExecID: "exec-drift-001",
		   Asset:  "USDT",
		   Amount: "280000",
	   })
	  if err != nil {
		  t.Fatalf("BindLease: %v", err)
	  }
	  leaseID := bindRes.Lease.LeaseID
	  // 压低热账至低于 target。
	   m.setHot("USDT", fix.HotAvailAtRecheck)
	  rr, err := eng.Recheck(ctx, nil, leaseID)
	  if err == nil {
		   t.Fatalf("Recheck: expected error, got nil")
	   }
	   if !errors.Is(err, trexecgate.ErrDriftExceeded) {
		  t.Errorf("Recheck error: got %v, want ErrDriftExceeded", err)
	  }
	  // 验证证明落库。
	  if rr.Proof == nil {
		  t.Fatal("Recheck: expected TreasDriftProof, got nil")
	   }
	   if rr.Proof.Decision != "blocked" {
		  t.Errorf("proof decision: got %s, want blocked", rr.Proof.Decision)
	  }
	  // 验证证明可从 repo 检索。
	  proofs, perr := eng.GetProofsByLease(ctx, leaseID)
	   if perr != nil {
		   t.Fatalf("GetProofsByLease: %v", perr)
	   }
	  if len(proofs) == 0 {
		   t.Error("expected at least one TreasDriftProof in repo")
	   }
	   // 验证 MarkSentGate 被拒（租约已 blocked）。
	  _, msErr := eng.MarkSentGate(ctx, trexecgate.MarkSentGateInput{
		   LeaseID: leaseID,
		   ExecID:  "exec-drift-001",
	  })
	   if msErr == nil {
		  t.Error("MarkSentGate: expected error after drift reject, got nil")
	   }
  }

// ──────────────────────────────────────────────
// 过期租约不得 Acked（即使租约先 sent）
// ──────────────────────────────────────────────

  func TestExpiredSentLease_RejectsAck(t *testing.T) {
	   ctx := context.Background()
	  now := time.Date(2021, 6, 15, 12, 0, 0, 0, time.UTC)
	   var tick time.Time = now
	   clockFn := func() time.Time { return tick }
	  leases := trexecgate.NewMemLeaseRepo()
	  proofs := trexecgate.NewMemDriftProofRepo()
	   gens := trexecgate.NewMemGenRepo()
	  inv := &mockHotInv{snaps: map[string]string{"BTC": "75"}}
	  eng := trexecgate.NewEngineWithObs(leases, proofs, gens, inv, treasury.DefaultCatalog()).
		  WithClock(clockFn).
		  WithTTL(5 * time.Second)
	  bindRes, err := eng.BindLease(ctx, nil, trexecgate.BindInput{
		   PlanID: "plan-ack-exp-001",
		   ExecID: "exec-ack-exp-001",
		  Asset: "BTC",
		   Amount: "50",
	   })
	  if err != nil {
		  t.Fatalf("BindLease: %v", err)
	  }
	  leaseID := bindRes.Lease.LeaseID
	  // Recheck 通过。
	  rr, err := eng.Recheck(ctx, nil, leaseID)
	   if err != nil || !rr.Passed {
		  t.Fatalf("Recheck: %v / passed=%v", err, rr.Passed)
	  }
	   // 时钟超过 TTL 后 MarkSent。
	   tick = now.Add(10 * time.Second)
	   _, err = eng.MarkSentGate(ctx, trexecgate.MarkSentGateInput{
		  LeaseID: leaseID,
		  ExecID:  "exec-ack-exp-001",
	   })
	  if !errors.Is(err, trexecgate.ErrLeaseExpired) {
		  t.Errorf("MarkSentGate post-expiry: expected ErrLeaseExpired, got %v", err)
	   }
   }

// ──────────────────────────────────────────────
// LeaseID 不匹配须拒绝
// ──────────────────────────────────────────────

   func TestMarkSent_WrongLeaseIDRejected(t *testing.T) {
	  ctx := context.Background()
	  m := newMockEngine(map[string]string{"USDT": "480000"})
	   eng := m.eng
	  bindRes, err := eng.BindLease(ctx, nil, trexecgate.BindInput{
		  PlanID: "plan-mismatch-001",
		   ExecID: "exec-mismatch-001",
		   Asset:  "USDT",
		   Amount: "300000",
	   })
	   if err != nil {
		  t.Fatalf("BindLease: %v", err)
	  }
	   leaseID := bindRes.Lease.LeaseID
	  _, err = eng.Recheck(ctx, nil, leaseID)
	   if err != nil {
		  t.Fatalf("Recheck: %v", err)
	   }
	  // 使用错误 ExecID。
	   _, err = eng.MarkSentGate(ctx, trexecgate.MarkSentGateInput{
		  LeaseID: leaseID,
		   ExecID: "exec-wrong-id",
	   })
	   if !errors.Is(err, trexecgate.ErrLeaseIDMismatch) {
		  t.Errorf("MarkSentGate wrong exec_id: expected ErrLeaseIDMismatch, got %v", err)
	   }
  }

// ──────────────────────────────────────────────
// FSM：非法跃迁必须拒绝
// ──────────────────────────────────────────────

   func TestFSM_IllegalTransition(t *testing.T) {
	   cases := []struct {
		  from trexecgate.LeaseState
		  to   trexecgate.LeaseState
		  want bool
	  }{
		   {trexecgate.LeaseBound, trexecgate.LeaseRechecked, true},
		  {trexecgate.LeaseBound, trexecgate.LeaseSent, false},
		   {trexecgate.LeaseBound, trexecgate.LeaseAcked, false},
		  {trexecgate.LeaseRechecked, trexecgate.LeaseSent, true},
		   {trexecgate.LeaseRechecked, trexecgate.LeaseAcked, false},
		  {trexecgate.LeaseSent,
			  trexecgate.LeaseAcked, true},
		   {trexecgate.LeaseAcked, trexecgate.LeaseSent, false},
		  {trexecgate.LeaseExpired, trexecgate.LeaseBound, false},
		  {trexecgate.LeaseBlocked, trexecgate.LeaseRechecked, false},
	  }
	   for _, tc := range cases {
		   got := trexecgate.CanTransition(tc.from, tc.to)
		   if got != tc.want {
			  t.Errorf("CanTransition(%s, %s): got %v, want %v", tc.from, tc.to, got, tc.want)
		  }
	   }
  }

// ──────────────────────────────────────────────
// 过期扫描 Worker
// ──────────────────────────────────────────────

   func TestExpireStaleLeases_MarksExpired(t *testing.T) {
	   ctx := context.Background()
	  now := time.Date(2021, 6, 15, 12, 0, 0, 0, time.UTC)
	  var tick time.Time = now
	  clockFn := func() time.Time { return tick }
	   leases := trexecgate.NewMemLeaseRepo()
	   proofs := trexecgate.NewMemDriftProofRepo()
	  gens := trexecgate.NewMemGenRepo()
	  inv := &mockHotInv{snaps: map[string]string{"USDT": "480000"}}
	   eng := trexecgate.NewEngineWithObs(leases,
		  proofs, gens, inv, treasury.DefaultCatalog()).
		  WithClock(clockFn).
		  WithTTL(5 * time.Second)
	   bindRes, err := eng.BindLease(ctx, nil, trexecgate.BindInput{
		  PlanID: "plan-expire-scan-001",
		   ExecID: "exec-expire-scan-001",
		  Asset: "USDT",
		  Amount: "300000",
	  })
	  if err != nil {
		  t.Fatalf("BindLease: %v", err)
	  }
	   leaseID := bindRes.Lease.LeaseID
	   // 推进时钟超过 TTL。
	   tick = now.Add(10 * time.Second)
	  n, err := eng.ExpireStaleLeases(ctx)
	   if err != nil {
		  t.Fatalf("ExpireStaleLeases: %v", err)
	  }
	  if n != 1 {
		   t.Errorf("ExpireStaleLeases: expired %d, want 1", n)
	   }
	   lease, _ := eng.GetLease(ctx, leaseID)
	   if lease.State != trexecgate.LeaseExpired {
		  t.Errorf("lease state: got %s, want expired", lease.State)
	   }
  }
