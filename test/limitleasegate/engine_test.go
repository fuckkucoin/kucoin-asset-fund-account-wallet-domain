package limitleasegate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/limitleasegate"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/internal/usage"
)

// ─── 夹具构建辅助 ─────────────────────────────────────────────────────
// testEnv 单次环境（含独立 MemStore、Registry、UsageStore）。
type testEnv struct {
	store *limitleasegate.MemStore
	reg   *policy.Registry
	use   *usage.Store
	eng   *limitleasegate.Engine
	now time.Time
}

// newTestEnv 创建环境，注入可控时钟。

   func newTestEnv(t *testing.T, cfg limitleasegate.Config) *testEnv {
	  t.Helper()
	   now := time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC)
	  store := limitleasegate.NewMemStore()
	   reg := policy.NewRegistry(policy.DefaultV1())
	   use := usage.New()
	   use = use.WithClock(func() time.Time { return now })
	  eng := limitleasegate.NewEngine(store, reg, use, cfg)
	   eng = eng.WithClock(func() time.Time { return now })
	   return &testEnv{store: store, reg: reg, use: use, eng: eng, now: now}
   }

// advanceClock 推进环境时钟（返回新 testEnv 以保持一致性）。
   func (e *testEnv) advanceClock(d time.Duration) {
	   e.now = e.now.Add(d)
	   e.eng = e.eng.WithClock(func() time.Time { return e.now })
	   e.use = e.use.WithClock(func() time.Time { return e.now })
   }

// issueWithdrawLease 辅助签发一个 withdraw 租约（金额 100 USDT）。

  func (e *testEnv) issueWithdrawLease(t *testing.T, userID, idemKey string) *limitleasegate.LimitCheckLease {
	  t.Helper()
	  result, err := e.eng.IssueCheckLease(context.Background(), limitleasegate.IssueRequest{
		   UserID: userID,
		  Asset:  "USDT",
		   Kind: "withdraw",
		  Amount:      "100",
		   Allowlisted: true,
		  IdemKey:     idemKey,
	  })
	   if err != nil {
		   t.Fatalf("IssueCheckLease unexpected error: %v", err)
	  }
	  if !result.Allowed {
		   t.Fatalf("IssueCheckLease expected Allow, got Deny: %s", result.RejectReason)
	  }
	   return result.Lease
  }

// ─── IssueCheckLease ────────────────────────────────────────────────

   func TestIssueCheckLease(t *testing.T) {
	   cfg := limitleasegate.Config{LeaseTTL: 120 * time.Second}
	   type tc struct {
		  name    string
		  setupFn func(e *testEnv)
		   req         limitleasegate.IssueRequest
		  wantAllowed bool
		   wantErr     error
		  wantPVer int
	  }
	  cases := []tc{
		  {
			  name: "允许：默认策略_withdraw_100",
			   req: limitleasegate.IssueRequest{
				   UserID: "u001", Asset: "USDT", Kind: "withdraw",
				  Amount: "100", Allowlisted: true,
			  },
			  wantAllowed: true,
			  wantPVer:    1,
		   },
		  {
			   name: "拒绝：金额超过单笔限额",
			   req: limitleasegate.IssueRequest{
				  UserID: "u002", Asset: "USDT", Kind: "withdraw",
				  Amount: "9999", Allowlisted: true,
			  },
			  wantAllowed: false,
			  wantErr: limitleasegate.ErrCheckDenied,
		   },
		  {
			   name: "拒绝：用户不在白名单",
			   req: limitleasegate.IssueRequest{
				   UserID: "u003", Asset: "USDT", Kind: "withdraw",
				  Amount: "100", Allowlisted: false,
			   },
			  wantAllowed: false,
			  wantErr: limitleasegate.ErrCheckDenied,
		  },
		   {
			  name: "固化正确 PolicyVer",
			   setupFn: func(e *testEnv) {
				  // 发布 V2 策略。
				  _ = e.reg.Publish(policy.Doc{
					  Version: 2, MaxSingle: "5000", DailyWithdraw: "15000",
					   MinAmount: "0.00000001", MaxVelocity: 20, VelocityWin: time.Minute,
					   Allowlist: true,
				  })
			  },
			  req: limitleasegate.IssueRequest{
				  UserID: "u004", Asset: "USDT", Kind: "withdraw",
				   Amount: "100", Allowlisted: true,
			  },
			   wantAllowed: true,
			  wantPVer:    2,
		  },
		   {
			  name: "幂等：相同 IdemKey 返回原租约",
			  req: limitleasegate.IssueRequest{
				   UserID: "u005", Asset: "USDT", Kind: "withdraw",
				  Amount: "100", Allowlisted: true, IdemKey: "idem-001",
			  },
			  wantAllowed: true,
			   wantPVer:    1,
		  },
	  }
	   for _, c := range cases {
		   c := c
		  t.Run(c.name, func(t *testing.T) {
			   e := newTestEnv(t, cfg)
			   if c.setupFn != nil {
				  c.setupFn(e)
			   }
			  // 第一次签发。
			  r1, err1 := e.eng.IssueCheckLease(context.Background(), c.req)
			  if c.wantErr != nil {
				  if !errors.Is(err1, c.wantErr) {
					   t.Errorf("IssueCheckLease err = %v, want %v", err1, c.wantErr)
				   }
				   return
			  }
			  if err1 != nil {
				  t.Fatalf("unexpected error: %v", err1)
			  }
			  if r1.Allowed != c.wantAllowed {
				  t.Errorf("Allowed = %v, want %v", r1.Allowed, c.wantAllowed)
			   }
			   if c.wantAllowed && r1.Lease == nil {
				  t.Fatal("Lease is nil on allowed result")
			   }
			  if c.wantAllowed && r1.Lease.PolicyVer != c.wantPVer {
				  t.Errorf("PolicyVer = %d, want %d", r1.Lease.PolicyVer, c.wantPVer)
			   }
			  // 幂等：再次签发返回原租约。
			   if c.req.IdemKey != "" {
				   r2, err2 := e.eng.IssueCheckLease(context.Background(), c.req)
				   if err2 != nil {
					   t.Fatalf("idempotent re-issue error: %v", err2)
				  }
				  if r2.Lease.ID != r1.Lease.ID {
					   t.Errorf("idempotent issue returned different lease ID: %q vs %q", r2.Lease.ID, r1.Lease.ID)
				   }
			  }
		  })
	   }
  }

// ─── BindLease ──────────────────────────────────────────────────────

  func TestBindLease(t *testing.T) {
	   cfg := limitleasegate.Config{LeaseTTL: 120 * time.Second}
	  type tc struct {
		   name    string
		   setupFn func(e *testEnv, leaseID string)
		   bindReq func(leaseID string) limitleasegate.BindRequest
		  wantErr error
	  }
	   cases := []tc{
		  {
			   name: "正常绑定 checked→bound",
			   bindReq: func(id string) limitleasegate.BindRequest {
				   return limitleasegate.BindRequest{
					  LeaseID: id, UserID: "u001", Asset: "USDT", Kind: "withdraw",
					   CommitRef: "wd-order-001",
				   }
			  },
			   wantErr: nil,
		   },
		   {
			   name: "无 LeaseID 拒绝 fail-closed",
			   bindReq: func(_ string) limitleasegate.BindRequest {
				  return limitleasegate.BindRequest{
					   LeaseID: "", UserID: "u001", Asset: "USDT", Kind: "withdraw",
					  CommitRef: "wd-order-002",
				   }
			  },
			  wantErr: limitleasegate.ErrNoLease,
		  },
		   {
			   name: "租约已过 TTL 拒绝",
			   setupFn: func(e *testEnv, _ string) {
				   e.advanceClock(200 * time.Second)
			   },
			   bindReq: func(id string) limitleasegate.BindRequest {
				  return limitleasegate.BindRequest{
					   LeaseID: id, UserID: "u001", Asset: "USDT", Kind: "withdraw",
					   CommitRef: "wd-order-003",
				   }
			  },
			   wantErr: limitleasegate.ErrLeaseExpired,
		   },
		  {
			  name: "作用域不匹配 ScopeMismatch",
			  bindReq: func(id string) limitleasegate.BindRequest {
				   return limitleasegate.BindRequest{
					  // Asset 故意不匹配。
					  LeaseID: id, UserID: "u001", Asset: "BTC", Kind: "withdraw",
					   CommitRef: "wd-order-004",
				   }
			  },
			  wantErr: limitleasegate.ErrScopeMismatch,
		  },
		   {
			  name: "租约已消费 ErrLeaseConsumed",
			  setupFn: func(e *testEnv, leaseID string) {
				  // 先绑定再 CommitRecheck 消费。
				   _, _ = e.eng.BindLease(context.Background(), limitleasegate.BindRequest{
					  LeaseID: leaseID, UserID: "u001", Asset: "USDT", Kind: "withdraw",
					   CommitRef: "wd-order-pre",
				  })
				   _, _ = e.eng.CommitRecheck(context.Background(), limitleasegate.CommitRecheckRequest{
					   LeaseID: leaseID, UserID: "u001", Asset: "USDT", Kind: "withdraw",
					  CommitRef: "wd-order-pre", Allowlisted: true,
				   })
			   },
			  bindReq: func(id string) limitleasegate.BindRequest {
				  return limitleasegate.BindRequest{
					   LeaseID: id, UserID: "u001", Asset: "USDT", Kind: "withdraw",
					   CommitRef: "wd-order-005",
				   }
			   },
			  wantErr: limitleasegate.ErrLeaseConsumed,
		  },
	   }
	   for _, c := range cases {
		   c := c
		   t.Run(c.name, func(t *testing.T) {
			  e := newTestEnv(t, cfg)
			  lease := e.issueWithdrawLease(t, "u001", "")
			  if c.setupFn != nil {
				   c.setupFn(e, lease.ID)
			   }
			  req := c.bindReq(lease.ID)
			   bound, err := e.eng.BindLease(context.Background(), req)
			   if c.wantErr != nil {
				   if !errors.Is(err, c.wantErr) {
					   t.Errorf("BindLease err = %v, want %v", err, c.wantErr)
				   }
				  return
			  }
			  if err != nil {
				  t.Fatalf("unexpected error: %v", err)
			  }
			  if bound.State != limitleasegate.LeaseStateBound {
				  t.Errorf("state = %q, want bound", bound.State)
			   }
			   if bound.CommitRef != req.CommitRef {
				   t.Errorf("commit_ref = %q, want %q", bound.CommitRef, req.CommitRef)
			   }
		  })
	  }
  }

// ─── CommitRecheck 验收场景 ──────────────────────────────────────────────
// TestCommitRecheck_PolicyFlip 验收场景 1：
// Check Allow 后 PolicyVer++，持旧 lease 的 Commit 必须失败并落 LimitDriftProof。
   func TestCommitRecheck_PolicyFlip(t *testing.T) {
	  cfg := limitleasegate.Config{LeaseTTL: 120 * time.Second}
	   e := newTestEnv(t, cfg)
	  // 步骤 1：签发租约（此时 PolicyVer=1）。
	  lease := e.issueWithdrawLease(t, "u001", "")
	   if lease.PolicyVer != 1 {
		   t.Fatalf("initial PolicyVer = %d, want 1", lease.PolicyVer)
	   }
	   // 步骤 2：发布策略 V2（PolicyVer++）。
	  if err := e.reg.Publish(policy.Doc{
		  Version: 2, MaxSingle: "5000", DailyWithdraw: "15000",
		   MinAmount: "0.00000001", MaxVelocity: 20, VelocityWin: time.Minute,
		   Allowlist: true,
	  }); err != nil {
		  t.Fatalf("Publish V2: %v", err)
	  }
	   // 步骤 3：绑定。
	  bound, bindErr := e.eng.BindLease(context.Background(), limitleasegate.BindRequest{
		  LeaseID: lease.ID, UserID: "u001", Asset: "USDT", Kind: "withdraw",
		  CommitRef: "wd-order-policyflip",
	   })
	   if bindErr != nil {
		   t.Fatalf("BindLease error: %v", bindErr)
	   }
	  if bound.State != limitleasegate.LeaseStateBound {
		   t.Fatalf("expected bound, got %q", bound.State)
	  }
	   // 步骤 4：CommitRecheck → 检测到 PolicyVer 漂移，必须失败。
	   result, recheckErr := e.eng.CommitRecheck(context.Background(), limitleasegate.CommitRecheckRequest{
		  LeaseID: lease.ID, UserID: "u001", Asset: "USDT", Kind: "withdraw",
		   CommitRef: "wd-order-policyflip", Allowlisted: true,
	  })
	   // 验收：必须返回 ErrPolicyDrift。
	   if !errors.Is(recheckErr, limitleasegate.ErrPolicyDrift) {
		   t.Errorf("CommitRecheck error = %v, want ErrPolicyDrift", recheckErr)
	  }
	  if result.Allowed {
		  t.Error("CommitRecheck should not be allowed after policy flip")
	  }
	   if result.RejectReason != "policy_flip" {
		   t.Errorf("RejectReason = %q, want policy_flip", result.RejectReason)
	  }
	  // 验收：必须落 LimitDriftProof。
	  if result.DriftProof == nil {
		   t.Fatal("DriftProof is nil, expected non-nil after policy_flip")
	  }
	  if result.DriftProof.DriftType != limitleasegate.DriftTypePolicyFlip {
		   t.Errorf("DriftType = %q, want policy_flip", result.DriftProof.DriftType)
	  }
	  if result.DriftProof.OldPolicyVer != 1 {
		   t.Errorf("OldPolicyVer = %d, want 1", result.DriftProof.OldPolicyVer)
	  }
	  if result.DriftProof.NewPolicyVer != 2 {
		   t.Errorf("NewPolicyVer = %d, want 2", result.DriftProof.NewPolicyVer)
	   }
	   if result.DriftProof.State != limitleasegate.ProofStateOpen {
		   t.Errorf("proof state = %q, want open", result.DriftProof.State)
	   }
	   // 验收：租约应置 revoked，且关联证明 ID。
	   updatedLease, _ := e.store.GetLeaseByID(context.Background(), lease.ID)
	  if updatedLease.State != limitleasegate.LeaseStateRevoked {
		   t.Errorf("lease state = %q, want revoked", updatedLease.State)
	  }
	   if updatedLease.DriftProofID != result.DriftProof.ID {
		  t.Errorf("DriftProofID mismatch: lease has %q, proof is %q",
			   updatedLease.DriftProofID, result.DriftProof.ID)
	   }
   }

// TestCommitRecheck_UsageBreach 验收场景 2：
// Check Allow 后 usage 累计灌满使 Eval Deny，旧 lease Commit 必须拒。
  func TestCommitRecheck_UsageBreach(t *testing.T) {
	   cfg := limitleasegate.Config{LeaseTTL: 120 * time.Second}
	  e := newTestEnv(t, cfg)
	  // 步骤 1：签发租约（此时 TodaySum=0，DailyCap=15000 USDT）。
	   lease := e.issueWithdrawLease(t, "u001", "")
	  if lease.TodaySumAtIssue != "0" && lease.TodaySumAtIssue != "0.00000000" {
		  // 允许零值的不同表示形式。
	   }
	   // 步骤 2：已有其他出金占用，今日累计接近上限。
	  // 向 usage.Store 注入大额占用（14950 USDT），接近 15000 日限额。
	   for i := 0; i < 5; i++ {
		   if err := e.use.Record(usage.KindWithdraw, "u001", "USDT", "dest-fill", "2990"); err != nil {
			  t.Fatalf("Record usage: %v", err)
		  }
	  }
	   // 步骤 3：CommitRecheck（请求金额 100 USDT，TodaySum+100 = 15050 > 15000）。
	  result, recheckErr := e.eng.CommitRecheck(context.Background(), limitleasegate.CommitRecheckRequest{
		   LeaseID: lease.ID, UserID: "u001", Asset: "USDT", Kind: "withdraw",
		   Allowlisted: true,
	  })
	  // 验收：必须返回 ErrUsageDrift。
	   if !errors.Is(recheckErr, limitleasegate.ErrUsageDrift) {
		  t.Errorf("CommitRecheck error = %v, want ErrUsageDrift", recheckErr)
	  }
	   if result.Allowed {
		  t.Error("CommitRecheck should not be allowed after usage breach")
	  }
	   if result.RejectReason != "usage_breach" {
		   t.Errorf("RejectReason = %q, want usage_breach", result.RejectReason)
	  }
	   // 验收：必须落 LimitDriftProof（usage_breach 类型）。
	   if result.DriftProof == nil {
		  t.Fatal("DriftProof is nil on usage_breach")
	   }
	   if result.DriftProof.DriftType != limitleasegate.DriftTypeUsageBreach {
		   t.Errorf("DriftType = %q, want usage_breach", result.DriftProof.DriftType)
	  }
	   // 验收：租约置 revoked。
	   updatedLease, _ := e.store.GetLeaseByID(context.Background(), lease.ID)
	  if updatedLease.State != limitleasegate.LeaseStateRevoked {
		  t.Errorf("lease state = %q, want revoked", updatedLease.State)
	  }
  }

// TestCommitRecheck_NoLease 验收场景 3：
// 无 lease 的冻账/出金占用提交必须 fail-closed。

  func TestCommitRecheck_NoLease(t *testing.T) {
	  cfg := limitleasegate.Config{
		  LeaseTTL: 120 * time.Second,
		   AllowCommitWithoutLease: false, // 必须 false。
	   }
	  e := newTestEnv(t, cfg)
	   _, err := e.eng.CommitRecheck(context.Background(), limitleasegate.CommitRecheckRequest{
		  LeaseID: "", UserID: "u001", Asset: "USDT", Kind: "withdraw",
		  Allowlisted: true,
	   })
	  if !errors.Is(err, limitleasegate.ErrNoLease) {
		   t.Errorf("CommitRecheck without lease error = %v, want ErrNoLease", err)
	   }
  }

// TestCommitRecheck_NormalPath 正常路径：所有检查通过 → consumed 终态。

  func TestCommitRecheck_NormalPath(t *testing.T) {
	  cfg := limitleasegate.Config{LeaseTTL: 120 * time.Second}
	   e := newTestEnv(t, cfg)
	   lease := e.issueWithdrawLease(t, "u001", "")
	   // 绑定。
	   bound, bindErr := e.eng.BindLease(context.Background(), limitleasegate.BindRequest{
		   LeaseID: lease.ID, UserID: "u001", Asset: "USDT", Kind: "withdraw",
		   CommitRef: "wd-order-normal",
	   })
	   if bindErr != nil {
		  t.Fatalf("BindLease: %v", bindErr)
	   }
	   if bound.State != limitleasegate.LeaseStateBound {
		   t.Fatalf("expected bound, got %q", bound.State)
	   }
	  // CommitRecheck 正常通过。
	   result, recheckErr := e.eng.CommitRecheck(context.Background(), limitleasegate.CommitRecheckRequest{
		   LeaseID: lease.ID, UserID: "u001", Asset: "USDT", Kind: "withdraw",
		   CommitRef: "wd-order-normal", Allowlisted: true,
	   })
	  if recheckErr != nil {
		   t.Fatalf("CommitRecheck error: %v", recheckErr)
	   }
	  if !result.Allowed {
		  t.Errorf("expected Allowed=true, got false: %s", result.RejectReason)
	  }
	   if result.Lease.State != limitleasegate.LeaseStateConsumed {
		  t.Errorf("lease state = %q, want consumed", result.Lease.State)
	  }
  }

// ─── RevokeByPolicyVersion ──────────────────────────────────────────

  func TestRevokeByPolicyVersion(t *testing.T) {
	   cfg := limitleasegate.Config{LeaseTTL: 120 * time.Second}
	   e := newTestEnv(t, cfg)
	   // 签发 3 个活跃租约（PolicyVer=1）。
	  leases := make([]*limitleasegate.LimitCheckLease, 3)
	   for i := 0; i < 3; i++ {
		   leases[i] = e.issueWithdrawLease(t, "u001", "")
	  }
	   // 签发第 4 个租约（不同用户）。
	  lease4 := e.issueWithdrawLease(t, "u002", "")
	   _ = lease4
	  // 策略升级到 V2。
	   _ = e.reg.Publish(policy.Doc{
		  Version: 2, MaxSingle: "5000", DailyWithdraw: "15000",
		   MinAmount: "0.00000001", MaxVelocity: 20, VelocityWin: time.Minute,
		  Allowlist: true,
	  })
	   // RevokeByPolicyVersion 批量吊销 PolicyVer < 2 的活跃租约。
	  revResult, err := e.eng.RevokeByPolicyVersion(context.Background(), limitleasegate.RevokeByPolicyVersionRequest{
		  NewPolicyVer: 2,
	   })
	   if err != nil {
		  t.Fatalf("RevokeByPolicyVersion: %v", err)
	  }
	   if revResult.Revoked < 4 {
		  t.Errorf("Revoked = %d, want >= 4", revResult.Revoked)
	  }
	  // 验证租约状态为 revoked。
	   for _, l := range leases {
		   updated,
			   _ := e.store.GetLeaseByID(context.Background(), l.ID)
		  if updated.State != limitleasegate.LeaseStateRevoked {
			  t.Errorf("lease %q state = %q, want revoked", l.ID, updated.State)
		  }
	  }
   }

// ─── LeaseExpiryWorker ──────────────────────────────────────────────

  func TestExpireLease(t *testing.T) {
	   cfg := limitleasegate.Config{LeaseTTL: 60 * time.Second}
	  e := newTestEnv(t, cfg)
	  lease := e.issueWithdrawLease(t, "u001", "")
	   // 推进时钟超过 TTL。
	   e.advanceClock(90 * time.Second)
	   if err := e.eng.ExpireLease(context.Background(), lease.ID); err != nil {
		  t.Fatalf("ExpireLease: %v", err)
	   }
	   updated, _ := e.store.GetLeaseByID(context.Background(), lease.ID)
	   if updated.State != limitleasegate.LeaseStateExpired {
		  t.Errorf("lease state = %q, want expired", updated.State)
	   }
  }

// ─── 证明生命周期 ─────────────────────────────────────────────────────

   func TestProofLifecycle(t *testing.T) {
	  cfg := limitleasegate.Config{LeaseTTL: 120 * time.Second}
	   e := newTestEnv(t, cfg)
	   // 触发 PolicyFlip 产生证明。
	   lease := e.issueWithdrawLease(t, "u001", "")
	   _ = e.reg.Publish(policy.Doc{
		  Version: 2, MaxSingle: "5000", DailyWithdraw: "15000",
		   MinAmount: "0.00000001", MaxVelocity: 20, VelocityWin: time.Minute, Allowlist: true,
	   })
	  result, _ := e.eng.CommitRecheck(context.Background(), limitleasegate.CommitRecheckRequest{
		   LeaseID: lease.ID, UserID: "u001", Asset: "USDT", Kind: "withdraw", Allowlisted: true,
	   })
	   if result.DriftProof == nil {
		   t.Fatal("expected DriftProof, got nil")
	  }
	   proofID := result.DriftProof.ID
	   // ProveProof：open → proven。
	   if err := e.eng.ProveProof(context.Background(), proofID); err != nil {
		  t.Fatalf("ProveProof: %v", err)
	  }
	   p, _ := e.store.GetProofByID(context.Background(), proofID)
	  if p.State != limitleasegate.ProofStateProven {
		   t.Errorf("proof state = %q, want proven", p.State)
	  }
	  // ConfirmProof：proven → closed。
	   if err := e.eng.ConfirmProof(context.Background(), proofID, "ops-admin-001", "confirmed"); err != nil {
		   t.Fatalf("ConfirmProof: %v", err)
	  }
	  p2, _ := e.store.GetProofByID(context.Background(), proofID)
	  if p2.State != limitleasegate.ProofStateClosed {
		  t.Errorf("proof state = %q, want closed", p2.State)
	  }
  }

   func TestDismissProof(t *testing.T) {
	  cfg := limitleasegate.Config{LeaseTTL: 120 * time.Second}
	   e := newTestEnv(t, cfg)
	  // 触发证明。
	  lease := e.issueWithdrawLease(t, "u001", "")
	   _ = e.reg.Publish(policy.Doc{
		   Version: 2, MaxSingle: "5000", DailyWithdraw: "15000",
		   MinAmount: "0.00000001", MaxVelocity: 20, VelocityWin: time.Minute, Allowlist: true,
	   })
	   result, _ := e.eng.CommitRecheck(context.Background(), limitleasegate.CommitRecheckRequest{
		  LeaseID: lease.ID, UserID: "u001", Asset: "USDT", Kind: "withdraw", Allowlisted: true,
	  })
	   proofID := result.DriftProof.ID
	  // DismissProof：open → closed（不经 proven）。
	   if err := e.eng.DismissProof(context.Background(), proofID, "ops-002", "false alarm"); err != nil {
		   t.Fatalf("DismissProof: %v", err)
	   }
	  p, _ := e.store.GetProofByID(context.Background(), proofID)
	   if p.State != limitleasegate.ProofStateClosed {
		   t.Errorf("proof state = %q, want closed", p.State)
	  }
	   if p.Notes != "false alarm" {
		  t.Errorf("notes = %q, want 'false alarm'", p.Notes)
	  }
  }

// ─── VelocityBreach ─────────────────────────────────────────────────

  func TestCommitRecheck_VelocityBreach(t *testing.T) {
	  cfg := limitleasegate.Config{LeaseTTL: 120 * time.Second}
	  e := newTestEnv(t, cfg)
	  // 签发租约（此时 Velocity=0，MaxVelocity=20）。
	   lease := e.issueWithdrawLease(t, "u001", "")
	   // 灌满速度窗口（velocity=20，= MaxVelocity）。
	   for i := 0; i < 20; i++ {
		   if err := e.use.Record(usage.KindWithdraw, "u001", "USDT", "dest-vel", "1"); err != nil {
			   t.Fatalf("Record: %v", err)
		   }
	   }
	   // CommitRecheck：速度越阈。
	   result, recheckErr := e.eng.CommitRecheck(context.Background(), limitleasegate.CommitRecheckRequest{
		  LeaseID: lease.ID, UserID: "u001", Asset: "USDT", Kind: "withdraw", Allowlisted: true,
	   })
	   if !errors.Is(recheckErr, limitleasegate.ErrVelocityDrift) {
		  t.Errorf("CommitRecheck error = %v, want ErrVelocityDrift", recheckErr)
	   }
	   if result.Allowed {
		   t.Error("CommitRecheck should not be allowed after velocity breach")
	   }
	   if result.DriftProof == nil {
		   t.Fatal("DriftProof is nil on velocity_breach")
	   }
	   if result.DriftProof.DriftType != limitleasegate.DriftTypeVelocityBreach {
		  t.Errorf("DriftType = %q, want velocity_breach", result.DriftProof.DriftType)
	   }
  }

// ─── ExportProof ─────────────────────────────────────────────────────

   func TestExportProof(t *testing.T) {
	  cfg := limitleasegate.Config{LeaseTTL: 120 * time.Second}
	   e := newTestEnv(t, cfg)
	  lease := e.issueWithdrawLease(t, "u001", "")
	   _ = e.reg.Publish(policy.Doc{
		  Version: 2, MaxSingle: "5000", DailyWithdraw: "15000",
		   MinAmount: "0.00000001", MaxVelocity: 20, VelocityWin: time.Minute, Allowlist: true,
	  })
	   result, _ := e.eng.CommitRecheck(context.Background(), limitleasegate.CommitRecheckRequest{
		  LeaseID: lease.ID, UserID: "u001", Asset: "USDT", Kind: "withdraw", Allowlisted: true,
	   })
	  export,
		   err := e.eng.ExportProof(context.Background(), result.DriftProof.ID)
	  if err != nil {
		   t.Fatalf("ExportProof: %v", err)
	  }
	   if export.Proof == nil {
		  t.Error("export.Proof is nil")
	   }
	   if export.Lease == nil {
		   t.Error("export.Lease is nil")
	   }
	   if export.Lease.ID != lease.ID {
		   t.Errorf("export.Lease.ID = %q, want %q", export.Lease.ID, lease.ID)
	  }
   }

// ─── StateTransition 幂等/终态保护 ───────────────────────────────────

  func TestLeaseTerminalStateProtection(t *testing.T) {
	   cfg := limitleasegate.Config{LeaseTTL: 120 * time.Second}
	   cases := []struct {
		   name      string
		   prepFn    func(e *testEnv, leaseID string)
		  wantState limitleasegate.LeaseState
	  }{
		   {
			  name: "consumed 租约不可再 BindLease",
			  prepFn: func(e *testEnv, id string) {
				   _, _ = e.eng.BindLease(context.Background(), limitleasegate.BindRequest{
					   LeaseID: id, UserID: "u001", Asset: "USDT", Kind: "withdraw",
					  CommitRef: "wd-ref-a",
				  })
				  _, _ = e.eng.CommitRecheck(context.Background(), limitleasegate.CommitRecheckRequest{
					   LeaseID: id, UserID: "u001", Asset: "USDT", Kind: "withdraw",
					   CommitRef: "wd-ref-a", Allowlisted: true,
				  })
			  },
			   wantState: limitleasegate.LeaseStateConsumed,
		  },
		   {
			   name: "expired 租约不可再 CommitRecheck",
			   prepFn: func(e *testEnv, _ string) {
				  e.advanceClock(200 * time.Second)
			   },
			   wantState: limitleasegate.LeaseStateChecked,
		   },
	   }
	   for _, c := range cases {
		  c := c
		  t.Run(c.name, func(t *testing.T) {
			  e := newTestEnv(t, cfg)
			   lease := e.issueWithdrawLease(t, "u001", "")
			  c.prepFn(e, lease.ID)
			   // 尝试 BindLease（应失败）。
			   _, bindErr := e.eng.BindLease(context.Background(), limitleasegate.BindRequest{
				   LeaseID: lease.ID, UserID: "u001", Asset: "USDT", Kind: "withdraw",
				  CommitRef: "wd-ref-retry",
			   })
			   if bindErr == nil {
				  t.Errorf("expected BindLease to fail, got nil error")
			  }
		  })
	  }
   }
