package probetokengate_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/probetokengate"
	"account-assets.kucoin.net/funding/internal/writegate"
)

// ─── 辅助 ─────────────────────────────────────────────────────────────────

type testFixture struct {
	store        *probetokengate.MemStore
	probeReader  *writegate.MemProbeMutexReader
	tokenReader  *writegate.MemGenTokenReader
	poisonReader *writegate.MemPoisonProofReader
	checker      *writegate.Checker
	engine       *probetokengate.Engine
	now time.Time
}

   func newFixture(t *testing.T, cfgOpts ...func(*probetokengate.Config)) *testFixture {
	  t.Helper()
	  store := probetokengate.NewMemStore()
	   probeReader := writegate.NewMemProbeMutexReader()
	  tokenReader := writegate.NewMemGenTokenReader()
	   poisonReader := writegate.NewMemPoisonProofReader()
	  checker := writegate.NewChecker(probeReader, tokenReader, poisonReader)
	   now := time.Date(2022, 3, 15, 10, 0, 0, 0, time.UTC)
	   cfg := probetokengate.Config{
		  LeaseTTL: 120 * time.Second,
		   RevokeBatchSize: 100,
		  ProofBatchSize:  100,
	   }
	  for _, opt := range cfgOpts {
		  opt(&cfg)
	   }
	   engine := probetokengate.NewEngine(store,
		checker, cfg).WithClock(func() time.Time { return now })
	checker.WithClock(func() time.Time { return now })
	return &testFixture{
		store:       store,
		probeReader: probeReader,
		tokenReader: tokenReader,
		poisonReader: poisonReader,
		checker:      checker,
		engine: engine,
		now: now,
	}
}

   func (f *testFixture) setProbe(scope writegate.CheckScope, isProbing bool, pairID, fp string) {
	  f.probeReader.SetState(scope, writegate.ProbeState{
		   IsProbing: isProbing,
		  PairID:           pairID,
		   StateFingerprint: fp,
	   })
   }

  func (f *testFixture) setToken(scope writegate.CheckScope, gen int64, state writegate.TokenState, hash string) {
	  f.tokenReader.SetSnap(scope, writegate.GenTokenSnap{
		   Generation: gen,
		  State:     state,
		   TokenHash: hash,
	   })
   }

   func (f *testFixture) setPoison(scope writegate.CheckScope, active bool) {
	   f.poisonReader.SetActive(scope,
		   active)
  }

  func makeScope(kind, subjectID, chain, asset string) writegate.CheckScope {
	   return writegate.CheckScope{
		   Kind:      writegate.ScopeKind(kind),
		   SubjectID: subjectID,
		   Chain: chain,
		  Asset: asset,
	   }
   }

  func mustJointCheck(t *testing.T, engine *probetokengate.Engine, scope writegate.CheckScope, idemKey string) *probetokengate.ProbeTokenLease {
	  t.Helper()
	   result, err := engine.JointCheck(context.Background(), probetokengate.JointCheckRequest{
		  Scope: scope,
		   IdemKey: idemKey,
	   })
	   if err != nil {
		   t.Fatalf("JointCheck failed: %v", err)
	  }
	  if !result.Allowed {
		   t.Fatalf("JointCheck not allowed: %s", result.RejectReason)
	   }
	   return result.Lease
   }

// ─── JointCheck ──────────────────────────────────────────────────────

  func TestJointCheck_Table(t *testing.T) {
	   type tc struct {
		  name      string
		   scopeKind string
		  subjectID string
		  chain string
		   asset   string
		   idemKey string
		   isProbing bool
		  tokenGen     int64
		  tokenState   writegate.TokenState
		   activePoison bool
		   wantAllowed  bool
		  wantErr      bool
	   }
	   cases := []tc{
		  {
			  name:      "正常放行_无探针_令牌有效",
			  scopeKind: "withdraw",
			  subjectID: "u1",
			   chain:     "ETH",
			  asset:   "USDT",
			   idemKey: "ck-001",
			  isProbing: false,
			  tokenGen:   0,
			  tokenState: writegate.TokenStateActive,
			   wantAllowed: true,
		  },
		   {
			   name:      "令牌已撤令_拒绝",
			  scopeKind: "withdraw",
			  subjectID: "u2",
			   chain:       "ETH",
			  asset:       "BTC",
			   idemKey:     "ck-002",
			  tokenState:  writegate.TokenStateRevoked,
			   wantAllowed: false,
			  wantErr: true,
		  },
		  {
			   name:      "毒价挡写_拒绝",
			   scopeKind: "withdraw",
			   subjectID: "u3",
			   chain:     "BTC",
			  asset:     "BTC",
			   idemKey: "ck-003",
			   tokenState:   writegate.TokenStateActive,
			  activePoison: true,
			  wantAllowed:  false,
			  wantErr:      true,
		  },
		  {
			  name:      "划转作用域_正常放行",
			   scopeKind: "transfer",
			   subjectID:   "u4",
			   chain:       "",
			   asset:       "USDC",
			   idemKey:     "ck-004",
			  tokenState:  writegate.TokenStateActive,
			   wantAllowed: true,
		  },
		  {
			   name:      "令牌已过期_拒绝",
			   scopeKind: "settle",
			   subjectID: "u5",
			  chain:      "",
			   asset:      "ETH",
			  idemKey:    "ck-005",
			   tokenState: writegate.TokenStateExpired,
			  wantAllowed: false,
			  wantErr:     true,
		  },
		  {
			  name:      "空 subject_id 拒绝",
			   scopeKind: "withdraw",
			   subjectID: "",
			   chain: "ETH",
			   asset: "USDT",
			  wantErr: true,
		  },
		  {
			   name:      "非法 scope_kind 拒绝",
			  scopeKind: "invalid_kind",
			   subjectID: "u6",
			   wantErr:   true,
		  },
	   }
	   for _, tc := range cases {
		  t.Run(tc.name, func(t *testing.T) {
			   f := newFixture(t)
			  scope := makeScope(tc.scopeKind, tc.subjectID, tc.chain, tc.asset)
			  f.setToken(scope, tc.tokenGen, tc.tokenState, fmt.Sprintf("th-%d", tc.tokenGen))
			  f.setPoison(scope, tc.activePoison)
			   result, err := f.engine.JointCheck(context.Background(), probetokengate.JointCheckRequest{
				  Scope: scope,
				  IdemKey: tc.idemKey,
			  })
			  if tc.wantErr && err == nil {
				  t.Errorf("expected error, got nil")
			   }
			  if !tc.wantErr && err != nil {
				  t.Errorf("unexpected error: %v", err)
			   }
			   if result != nil && result.Allowed != tc.wantAllowed {
				   t.Errorf("Allowed=%v, want %v", result.Allowed, tc.wantAllowed)
			   }
			   if tc.wantAllowed && result != nil && result.Lease != nil {
				  if result.Lease.State != probetokengate.LeaseStateChecked {
					  t.Errorf("lease state=%s, want checked", result.Lease.State)
				   }
				   if result.Lease.SubjectID != tc.subjectID {
					  t.Errorf("lease subject_id=%s, want %s", result.Lease.SubjectID, tc.subjectID)
				  }
			   }
		   })
	  }
   }

// ─── 幂等性 ───────────────────────────────────────────────────────────

   func TestJointCheck_Idempotent(t *testing.T) {
	  f := newFixture(t)
	   scope := makeScope("withdraw", "u10", "ETH", "USDT")
	  f.setToken(scope, 0, writegate.TokenStateActive, "th-0")
	   lease1 := mustJointCheck(t, f.engine, scope, "idem-x")
	   lease2 := mustJointCheck(t, f.engine, scope, "idem-x")
	   if lease1.ID != lease2.ID {
		   t.Errorf("幂等键命中应返回同一租约: got %s vs %s", lease1.ID, lease2.ID)
	   }
  }

// ─── BindLease ───────────────────────────────────────────────────────

   func TestBindLease_Table(t *testing.T) {
	   type tc struct {
		  name      string
		  wantErr   bool
		   wantState probetokengate.LeaseState
		  setup func(f *testFixture, scope writegate.CheckScope) string // returns leaseID
		  req   func(leaseID string) probetokengate.BindRequest
	   }
	   cases := []tc{
		   {
			   name: "正常绑定_checked→bound",
			   setup: func(f *testFixture, scope writegate.CheckScope) string {
				   f.setToken(scope, 0, writegate.TokenStateActive, "th-0")
				  l := mustJointCheck(t, f.engine, scope, "bind-001")
				  return l.ID
			  },
			   req: func(leaseID string) probetokengate.BindRequest {
				   return probetokengate.BindRequest{
					  LeaseID:   leaseID,
					   CommitRef: "wd-001",
				  }
			  },
			  wantState: probetokengate.LeaseStateBound,
		  },
		  {
			   name: "重复绑定相同 CommitRef_幂等返回",
			  setup: func(f *testFixture, scope writegate.CheckScope) string {
				   f.setToken(scope, 0, writegate.TokenStateActive, "th-0")
				  l := mustJointCheck(t, f.engine, scope, "bind-002")
				  _, _ = f.engine.BindLease(context.Background(), probetokengate.BindRequest{
					  LeaseID: l.ID,
					   CommitRef: "wd-002",
				   })
				   return l.ID
			   },
			   req: func(leaseID string) probetokengate.BindRequest {
				  return probetokengate.BindRequest{
					  LeaseID:   leaseID,
					  CommitRef: "wd-002",
				   }
			   },
			  wantState: probetokengate.LeaseStateBound,
		  },
		   {
			   name: "空 LeaseID_ErrNoLease",
			   setup: func(f *testFixture, scope writegate.CheckScope) string {
				  return ""
			  },
			   req: func(_ string) probetokengate.BindRequest {
				  return probetokengate.BindRequest{LeaseID: "", CommitRef: "wd-003"}
			   },
			  wantErr: true,
		   },
		   {
			  name: "不存在 LeaseID_ErrNoLease",
			  setup: func(f *testFixture, scope writegate.CheckScope) string {
				  return "nonexistent-id"
			   },
			  req: func(leaseID string) probetokengate.BindRequest {
				   return probetokengate.BindRequest{LeaseID: leaseID, CommitRef: "wd-004"}
			   },
			   wantErr: true,
		   },
	  }
	   for _, tc := range cases {
		   t.Run(tc.name, func(t *testing.T) {
			  f := newFixture(t)
			  scope := makeScope("withdraw", "u-bind", "ETH", "USDT")
			   leaseID := tc.setup(f, scope)
			  result, err := f.engine.BindLease(context.Background(), tc.req(leaseID))
			   if tc.wantErr {
				  if err == nil {
					   t.Errorf("expected error, got nil")
				   }
				  return
			  }
			   if err != nil {
				   t.Fatalf("unexpected error: %v", err)
			   }
			  if result.Lease.State != tc.wantState {
				  t.Errorf("state=%s, want %s", result.Lease.State, tc.wantState)
			  }
		  })
	   }
   }

// ─── CommitCheck 核心 ─────────────────────────────────────────────────
// TestCommitCheck_NoLease 验收：无 lease 写提交必须拒绝（fail-closed）。

   func TestCommitCheck_NoLease(t *testing.T) {
	  f := newFixture(t)
	  _, err := f.engine.CommitCheck(context.Background(), probetokengate.CommitCheckRequest{
		   LeaseID: "",
		  CommitRef: "wd-nolease",
	   })
	  if !errors.Is(err, probetokengate.ErrNoLease) {
		  t.Errorf("expected ErrNoLease, got %v", err)
	  }
   }

// TestCommitCheck_HappyPath 正常路径：探针/令牌均未漂移，consumed 终态。

  func TestCommitCheck_HappyPath(t *testing.T) {
	   f := newFixture(t)
	  scope := makeScope("withdraw", "u20", "ETH", "USDT")
	  f.setToken(scope, 3, writegate.TokenStateActive, "th-3")
	  f.setProbe(scope, false, "", "fp-stable")
	  lease := mustJointCheck(t, f.engine, scope, "cc-happy-001")
	   // BindLease。
	   _, err := f.engine.BindLease(context.Background(), probetokengate.BindRequest{
		   LeaseID:   lease.ID,
		   CommitRef: "wd-happy-001",
	   })
	   if err != nil {
		   t.Fatalf("BindLease failed: %v", err)
	  }
	   // CommitCheck（探针/令牌不变）。
	   result, err := f.engine.CommitCheck(context.Background(), probetokengate.CommitCheckRequest{
		  LeaseID:   lease.ID,
		  CommitRef: "wd-happy-001",
	  })
	  if err != nil {
		   t.Fatalf("CommitCheck failed: %v", err)
	   }
	   if result.Drifted {
		   t.Errorf("expected no drift, got drifted=true")
	  }
	  if result.Lease.State != probetokengate.LeaseStateConsumed {
		  t.Errorf("lease state=%s, want consumed", result.Lease.State)
	  }
  }

// TestCommitCheck_ProbeFlip 验收 1：Check 后探针翻转 → CommitCheck 拒绝 + ProbeTokenDriftProof。

  func TestCommitCheck_ProbeFlip(t *testing.T) {
	   f := newFixture(t)
	   scope := makeScope("withdraw", "u30", "ETH", "USDT")
	   f.setToken(scope, 0, writegate.TokenStateActive, "th-0")
	   f.setProbe(scope, false, "", "fp-before")
	  lease := mustJointCheck(t, f.engine, scope, "cc-flip-001")
	   if lease.ProbeFingerprint != "fp-before" {
		  t.Errorf("expected probe fp fp-before, got %s", lease.ProbeFingerprint)
	   }
	  // 探针翻转：更新探针状态。
	   f.setProbe(scope, true, "probe-pair-001", "fp-after")
	   // CommitCheck 复验：应检测到 probe_flip。
	   result, err := f.engine.CommitCheck(context.Background(), probetokengate.CommitCheckRequest{
		  LeaseID: lease.ID,
		  Scope:   scope,
	  })
	  if !errors.Is(err, probetokengate.ErrProbeDrift) {
		  t.Errorf("expected ErrProbeDrift, got %v", err)
	  }
	   if result == nil || !result.Drifted {
		  t.Errorf("expected drifted=true")
	   }
	   if result != nil && result.DriftType != probetokengate.DriftTypeProbeFlip {
		   t.Errorf("drift_type=%s, want probe_flip", result.DriftType)
	  }
	  if result != nil && result.DriftProofID == "" {
		  t.Errorf("expected non-empty drift_proof_id")
	   }
	   // 验证租约已置 revoked。
	   updatedLease,
		   err2 := f.engine.GetLease(context.Background(), lease.ID)
	  if err2 != nil {
		  t.Fatalf("GetLease failed: %v", err2)
	  }
	  if updatedLease.State != probetokengate.LeaseStateRevoked {
		  t.Errorf("lease state=%s, want revoked", updatedLease.State)
	   }
	  // 验证证明已开启。
	   if result.DriftProofID != "" {
		   proof,
			   err3 := f.engine.GetProof(context.Background(), result.DriftProofID)
		  if err3 != nil {
			   t.Fatalf("GetProof failed: %v", err3)
		   }
		   if proof.DriftType != probetokengate.DriftTypeProbeFlip {
			   t.Errorf("proof drift_type=%s, want probe_flip", proof.DriftType)
		   }
		  if proof.OldProbeFingerprint != "fp-before" {
			   t.Errorf("old_probe_fp=%s, want fp-before", proof.OldProbeFingerprint)
		   }
		   if proof.State != probetokengate.ProofStateOpen {
			   t.Errorf("proof state=%s, want open", proof.State)
		   }
	  }
  }

// TestCommitCheck_PoisonRevoke 验收 2：Check 后毒价撤令 → CommitCheck 拒绝 + ProbeTokenDriftProof。
  func TestCommitCheck_PoisonRevoke(t *testing.T) {
	   f := newFixture(t)
	  scope := makeScope("withdraw", "u40", "ETH", "USDT")
	  f.setToken(scope, 5, writegate.TokenStateActive, "th-5")
	   f.setProbe(scope, false, "", "fp-stable")
	   lease := mustJointCheck(t, f.engine, scope, "cc-poison-001")
	  if lease.TokenGeneration != 5 {
		  t.Errorf("expected token_gen=5, got %d", lease.TokenGeneration)
	  }
	   // 毒价撤令：令牌世代推进 + 状态置 revoked。
	   f.setToken(scope, 6, writegate.TokenStateRevoked, "th-6")
	  // CommitCheck 复验。
	   result, err := f.engine.CommitCheck(context.Background(), probetokengate.CommitCheckRequest{
		  LeaseID: lease.ID,
		   Scope: scope,
	  })
	   if !errors.Is(err, probetokengate.ErrTokenDrift) {
		   t.Errorf("expected ErrTokenDrift, got %v", err)
	   }
	  if result == nil || !result.Drifted {
		   t.Errorf("expected drifted=true")
	   }
	  if result != nil && result.DriftType != probetokengate.DriftTypePoisonRevoke {
		  t.Errorf("drift_type=%s, want poison_revoke", result.DriftType)
	  }
	   // 验证 DriftProof.PoisonRevoked=true。
	  if result != nil && result.DriftProofID != "" {
		  proof,
			   err2 := f.engine.GetProof(context.Background(), result.DriftProofID)
		  if err2 != nil {
			  t.Fatalf("GetProof failed: %v", err2)
		   }
		   if !proof.PoisonRevoked {
			   t.Errorf("expected poison_revoked=true")
		   }
		   if proof.OldTokenGeneration != 5 || proof.NewTokenGeneration != 6 {
			  t.Errorf("old_gen=%d new_gen=%d, want 5/6", proof.OldTokenGeneration, proof.NewTokenGeneration)
		  }
	   }
   }

// ─── RevokeOnProbeFlip 批量吊销 ───────────────────────────────────────

   func TestRevokeOnProbeFlip_BatchRevoke(t *testing.T) {
	  f := newFixture(t)
	  scope := makeScope("withdraw", "u50", "ETH", "USDT")
	  f.setToken(scope, 0, writegate.TokenStateActive, "th-0")
	   f.setProbe(scope, false, "", "fp-pre-flip")
	   // 签发 3 个租约（同一 scope）。
	   leases := make([]*probetokengate.ProbeTokenLease, 3)
	   for i := 0; i < 3; i++ {
		   leases[i] = mustJointCheck(t, f.engine, scope, fmt.Sprintf("batch-flip-%d", i))
	  }
	  // 触发探针翻转批量吊销。
	  n, err := f.engine.RevokeOnProbeFlip(context.Background(), probetokengate.ProbeFlipEvent{
		   ScopeKey: scope.ScopeKey(),
		   PairID:              "probe-pair-batch",
		   NewProbeFingerprint: "fp-post-flip",
	   })
	  if err != nil {
		   t.Fatalf("RevokeOnProbeFlip failed: %v", err)
	  }
	  if n != 3 {
		  t.Errorf("revoked=%d, want 3", n)
	  }
	   // 验证所有租约已置 revoked。
	   for _, l := range leases {
		  updated, err := f.engine.GetLease(context.Background(), l.ID)
		   if err != nil {
			   t.Fatalf("GetLease failed: %v", err)
		  }
		   if updated.State != probetokengate.LeaseStateRevoked {
			  t.Errorf("lease %s state=%s, want revoked", l.ID, updated.State)
		   }
	  }
   }

// ─── RevokeOnPoisonRevoke 批量吊销 ────────────────────────────────────

   func TestRevokeOnPoisonRevoke_BatchRevoke(t *testing.T) {
	  f := newFixture(t)
	  scope := makeScope("withdraw", "u60", "BTC", "BTC")
	  f.setToken(scope, 7, writegate.TokenStateActive, "th-7")
	  f.setProbe(scope, false, "", "fp-stable")
	   // 签发 2 个租约。
	  l1 := mustJointCheck(t, f.engine, scope, "poison-rev-1")
	   l2 := mustJointCheck(t, f.engine, scope, "poison-rev-2")
	  // 触发毒价撤令批量吊销（new_gen=8）。
	  n, err := f.engine.RevokeOnPoisonRevoke(context.Background(), probetokengate.PoisonRevokeEvent{
		  ScopeKey:           scope.ScopeKey(),
		  NewTokenGeneration: 8,
	  })
	   if err != nil {
		   t.Fatalf("RevokeOnPoisonRevoke failed: %v", err)
	   }
	   if n != 2 {
		  t.Errorf("revoked=%d, want 2", n)
	   }
	  // 验证两个租约均已置 revoked。
	  for _, id := range []string{l1.ID, l2.ID} {
		   updated,
			   _ := f.engine.GetLease(context.Background(), id)
		   if updated.State != probetokengate.LeaseStateRevoked {
			  t.Errorf("lease %s state=%s, want revoked", id, updated.State)
		  }
		  if updated.RevokeReason != string(probetokengate.RevokeReasonPoisonRevoke) {
			  t.Errorf("revoke_reason=%s, want poison_revoke", updated.RevokeReason)
		   }
	   }
  }

// ─── 租约过期 ─────────────────────────────────────────────────────────

   func TestLeaseExpiry_CommitCheckRejects(t *testing.T) {
	   f := newFixture(t, func(c *probetokengate.Config) {
		  c.LeaseTTL = 60 * time.Second
	  })
	  scope := makeScope("withdraw", "u70", "ETH", "ETH")
	   f.setToken(scope, 0, writegate.TokenStateActive, "th-0")
	  lease := mustJointCheck(t, f.engine, scope, "expire-001")
	  // 推进时钟超过 TTL。
	   expired := f.now.Add(90 * time.Second)
	  expiredEngine := probetokengate.NewEngine(f.store, f.checker, probetokengate.Config{
		  LeaseTTL: 60 * time.Second,
	  }).WithClock(func() time.Time { return expired })
	  // ExpireLease 手动置过期（ Worker 行为）。
	  if err := expiredEngine.ExpireLease(context.Background(), lease.ID); err != nil {
		  t.Fatalf("ExpireLease failed: %v", err)
	  }
	  // CommitCheck 应返回 ErrLeaseExpired。
	  _, err := expiredEngine.CommitCheck(context.Background(), probetokengate.CommitCheckRequest{
		   LeaseID: lease.ID,
	   })
	  if !errors.Is(err, probetokengate.ErrLeaseExpired) {
		  t.Errorf("expected ErrLeaseExpired, got %v", err)
	   }
   }

// ─── 终态不变性 ───────────────────────────────────────────────────────

   func TestLeaseStateMachine_TerminalImmutability(t *testing.T) {
	  f := newFixture(t)
	  scope := makeScope("withdraw", "u80", "ETH", "USDT")
	  f.setToken(scope, 0, writegate.TokenStateActive, "th-0")
	  lease := mustJointCheck(t, f.engine, scope, "term-001")
	  // consumed 终态后不可再消费。
	   _, _ = f.engine.BindLease(context.Background(), probetokengate.BindRequest{
		  LeaseID: lease.ID,
		   CommitRef: "wd-term-001",
	  })
	   _, _ = f.engine.CommitCheck(context.Background(), probetokengate.CommitCheckRequest{
		  LeaseID: lease.ID,
	   })
	   // 再次 CommitCheck 应返回 ErrLeaseConsumed。
	   _, err := f.engine.CommitCheck(context.Background(), probetokengate.CommitCheckRequest{
		   LeaseID: lease.ID,
	   })
	  if !errors.Is(err, probetokengate.ErrLeaseConsumed) {
		  t.Errorf("expected ErrLeaseConsumed, got %v", err)
	   }
   }

// ─── 证明生命周期 ─────────────────────────────────────────────────────

  func TestProofLifecycle_OpenProvenClosed(t *testing.T) {
	  f := newFixture(t)
	   scope := makeScope("withdraw", "u90", "ETH", "USDT")
	   f.setToken(scope, 0, writegate.TokenStateActive, "th-0")
	  f.setProbe(scope, false, "", "fp-a")
	   lease := mustJointCheck(t, f.engine, scope, "proof-lc-001")
	   // 触发探针翻转 → 开证明。
	   f.setProbe(scope, true, "pair-001", "fp-b")
	   result, _ := f.engine.CommitCheck(context.Background(), probetokengate.CommitCheckRequest{
		   LeaseID: lease.ID,
		   Scope:   scope,
	  })
	  if result == nil || result.DriftProofID == "" {
		  t.Fatal("expected drift proof to be opened")
	   }
	  proofID := result.DriftProofID
	  // open → proven。
	  proven, err := f.engine.ProveProof(context.Background(), proofID)
	   if err != nil {
		  t.Fatalf("ProveProof failed: %v", err)
	  }
	   if proven.State != probetokengate.ProofStateProven {
		  t.Errorf("state=%s, want proven", proven.State)
	  }
	  // proven → closed（ConfirmProof）。
	  closed, err := f.engine.ConfirmProof(context.Background(), proofID, "admin-001", "verified")
	  if err != nil {
		   t.Fatalf("ConfirmProof failed: %v", err)
	   }
	   if closed.State != probetokengate.ProofStateClosed {
		  t.Errorf("state=%s, want closed", closed.State)
	  }
	   if closed.ClosedBy != "admin-001" {
		  t.Errorf("closed_by=%s, want admin-001", closed.ClosedBy)
	   }
	   // 已关闭：ConfirmProof 幂等。
	  closed2, err := f.engine.ConfirmProof(context.Background(), proofID, "admin-002", "re-confirm")
	  if err != nil {
		  t.Fatalf("ConfirmProof idempotent failed: %v", err)
	  }
	   if closed2.ClosedBy != "admin-001" {
		  t.Errorf("幂等：closed_by 不应改变，got %s", closed2.ClosedBy)
	  }
   }

// ─── DismissProof ────────────────────────────────────────────────────

   func TestProofLifecycle_Dismiss(t *testing.T) {
	  f := newFixture(t)
	  scope := makeScope("withdraw", "u95", "ETH", "USDT")
	   f.setToken(scope, 0, writegate.TokenStateActive, "th-0")
	  f.setProbe(scope, false, "", "fp-a")
	   lease := mustJointCheck(t, f.engine, scope, "dismiss-001")
	   f.setProbe(scope, true, "pair-001", "fp-b")
	   result, _ := f.engine.CommitCheck(context.Background(), probetokengate.CommitCheckRequest{
		  LeaseID: lease.ID,
		  Scope:   scope,
	  })
	   if result == nil || result.DriftProofID == "" {
		  t.Fatal("expected drift proof to be opened")
	   }
	  proofID := result.DriftProofID
	  // DismissProof：open → closed（无需四眼）。
	   dismissed, err := f.engine.DismissProof(context.Background(), proofID, "ops-001", "false positive")
	   if err != nil {
		  t.Fatalf("DismissProof failed: %v", err)
	   }
	   if dismissed.State != probetokengate.ProofStateClosed {
		   t.Errorf("state=%s, want closed", dismissed.State)
	  }
   }

// ─── ExportProof ─────────────────────────────────────────────────────

  func TestExportProof_Summary(t *testing.T) {
	   f := newFixture(t)
	  scope := makeScope("withdraw", "u100", "ETH", "USDT")
	   f.setToken(scope, 10, writegate.TokenStateActive, "th-10")
	  f.setProbe(scope, false, "", "fp-export-a")
	  lease := mustJointCheck(t, f.engine, scope, "export-001")
	   f.setProbe(scope, true, "pair-export", "fp-export-b")
	  result, _ := f.engine.CommitCheck(context.Background(), probetokengate.CommitCheckRequest{
		  LeaseID: lease.ID,
		  Scope:   scope,
	   })
	  if result == nil || result.DriftProofID == "" {
		   t.Fatal("expected drift proof")
	   }
	  summary, err := f.engine.ExportProof(context.Background(), result.DriftProofID)
	  if err != nil {
		  t.Fatalf("ExportProof failed: %v", err)
	  }
	   if summary.DriftType != string(probetokengate.DriftTypeProbeFlip) {
		   t.Errorf("drift_type=%s, want probe_flip", summary.DriftType)
	  }
	   if summary.LeaseID != lease.ID {
		  t.Errorf("lease_id mismatch")
	  }
	  if summary.SubjectID != "u100" {
		   t.Errorf("subject_id=%s, want u100", summary.SubjectID)
	   }
  }

// ─── FSM 状态转换合法性 ───────────────────────────────────────────────

  func TestCanTransit_Table(t *testing.T) {
	  type tc struct {
		  from probetokengate.LeaseState
		   to   probetokengate.LeaseState
		   allowed bool
	  }
	  cases := []tc{
		   {probetokengate.LeaseStateChecked,
			  probetokengate.LeaseStateBound, true},
		   {probetokengate.LeaseStateChecked, probetokengate.LeaseStateConsumed, true},
		  {probetokengate.LeaseStateChecked, probetokengate.LeaseStateExpired, true},
		  {probetokengate.LeaseStateChecked, probetokengate.LeaseStateRevoked, true},
		  {probetokengate.LeaseStateBound,
			  probetokengate.LeaseStateConsumed, true},
		   {probetokengate.LeaseStateBound, probetokengate.LeaseStateExpired, true},
		   {probetokengate.LeaseStateBound, probetokengate.LeaseStateRevoked, true},
		  {probetokengate.LeaseStateBound, probetokengate.LeaseStateChecked, false},
		  {probetokengate.LeaseStateConsumed, probetokengate.LeaseStateRevoked, false},
		  {probetokengate.LeaseStateExpired, probetokengate.LeaseStateBound, false},
		   {probetokengate.LeaseStateRevoked, probetokengate.LeaseStateChecked, false},
	  }
	  for _, tc := range cases {
		  t.Run(fmt.Sprintf("%s→%s", tc.from, tc.to), func(t *testing.T) {
			   got := probetokengate.CanTransit(tc.from, tc.to)
			   if got != tc.allowed {
				  t.Errorf("CanTransit(%s, %s)=%v, want %v", tc.from, tc.to, got, tc.allowed)
			  }
		   })
	   }
   }

// ─── 无探针活跃场景（AllowCommitWithoutLease=true 集成覆盖） ───────────────

  func TestCommitCheck_AllowWithoutLease_IntegrationMode(t *testing.T) {
	  store := probetokengate.NewMemStore()
	  probeReader := writegate.NewMemProbeMutexReader()
	   tokenReader := writegate.NewMemGenTokenReader()
	   checker := writegate.NewChecker(probeReader, tokenReader, nil)
	   engine := probetokengate.NewEngine(store, checker, probetokengate.Config{
		   LeaseTTL: 120 * time.Second,
		  AllowCommitWithoutLease: true,
	  })
	  result, err := engine.CommitCheck(context.Background(), probetokengate.CommitCheckRequest{
		   LeaseID:   "",
		   CommitRef: "wd-nolease-inttest",
	   })
	  if err != nil {
		  t.Fatalf("AllowCommitWithoutLease=true: unexpected error: %v", err)
	  }
	  if result.Drifted {
		  t.Errorf("expected not drifted in allow-mode")
	  }
   }

// ─── GetBind ─────────────────────────────────────────────────────────

   func TestGetBind_AfterCommit(t *testing.T) {
	   f := newFixture(t)
	   scope := makeScope("withdraw", "u110", "ETH", "USDT")
	  f.setToken(scope, 0, writegate.TokenStateActive, "th-0")
	   lease := mustJointCheck(t, f.engine, scope, "bind-getbind-001")
	  bindResult, err := f.engine.BindLease(context.Background(), probetokengate.BindRequest{
		   LeaseID: lease.ID,
		  CommitRef: "wd-getbind-001",
	  })
	  if err != nil {
		  t.Fatalf("BindLease failed: %v", err)
	   }
	   // CommitCheck 消费后 ConsumedAt 应非空。
	   _, err = f.engine.CommitCheck(context.Background(), probetokengate.CommitCheckRequest{
		  LeaseID:   lease.ID,
		   CommitRef: "wd-getbind-001",
	  })
	  if err != nil {
		   t.Fatalf("CommitCheck failed: %v", err)
	   }
	   bind, err := f.engine.GetBind(context.Background(), lease.ID)
	   if err != nil {
		   t.Fatalf("GetBind failed: %v", err)
	   }
	   if bind.CommitRef != "wd-getbind-001" {
		  t.Errorf("commit_ref=%s, want wd-getbind-001", bind.CommitRef)
	  }
	  if bind.ConsumedAt == nil {
		  t.Errorf("expected ConsumedAt non-nil after CommitCheck consumed")
	   }
	   _ = bindResult
  }

// ─── WriteScopeBind 独立验证 ──────────────────────────────────────────────

   func TestWriteScopeBind_Isolation(t *testing.T) {
	  // WriteScopeBind 记录唯一：同一 lease_id 不可重复绑定。
	  f := newFixture(t)
	  scope := makeScope("withdraw", "u120", "ETH", "USDT")
	   f.setToken(scope, 0, writegate.TokenStateActive, "th-0")
	  lease := mustJointCheck(t, f.engine, scope, "wsb-iso-001")
	   _, err := f.engine.BindLease(context.Background(), probetokengate.BindRequest{
		   LeaseID:   lease.ID,
		  CommitRef: "wd-iso-001",
	   })
	   if err != nil {
		  t.Fatalf("first BindLease failed: %v", err)
	   }
	  // 第二次相同 CommitRef：幂等返回，不报错。
	   _, err = f.engine.BindLease(context.Background(), probetokengate.BindRequest{
		   LeaseID:   lease.ID,
		   CommitRef: "wd-iso-001",
	  })
	   if err != nil {
		   t.Errorf("second BindLease (same commit_ref) should be idempotent, got: %v", err)
	   }
	  // 第三次不同 CommitRef：应拒绝（已绑定其他 commit_ref）。
	  _, err = f.engine.BindLease(context.Background(), probetokengate.BindRequest{
		   LeaseID: lease.ID,
		  CommitRef: "wd-iso-002",
	   })
	   if !errors.Is(err, probetokengate.ErrCommitRefMismatch) {
		   t.Errorf("expected ErrCommitRefMismatch for different commit_ref, got %v", err)
	   }
  }
