package viptiergate_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/viptiergate"
)

   func newTestService(t *testing.T) *viptiergate.Service {
	  t.Helper()
	  return viptiergate.NewService(nil)
  }

   func fixedClock(ts time.Time) func() time.Time {
	   return func() time.Time { return ts }
  }

// ─────────────────────────────────────────────────────────────────────────────
// 1. 抬档完整路径
// ─────────────────────────────────────────────────────────────────────────────

   func TestGatedUpgrade_HappyPath(t *testing.T) {
	   ctx := context.Background()
	  svc := newTestService(t)
	  cases := []struct {
		  name     string
		  userID   string
		   fromTier viptiergate.Tier
		  toTier   viptiergate.Tier
		   triggerKind string
		  triggerRef  string
		  expectedGen int64
		  wantNewGen int64
	  }{
		   {
			  name:     "standard → vip",
			   userID:   "u-001",
			   fromTier: viptiergate.TierStandard,
			   toTier:      viptiergate.TierVIP,
			   triggerKind: "pin",
			  triggerRef: "u-001|pin|20260921100000",
			  expectedGen: 0,
			   wantNewGen: 1,
		   },
		   {
			   name: "vip → vip+ via volume",
			  userID:   "u-002",
			   fromTier: viptiergate.TierVIP,
			   toTier:      viptiergate.TierVIPPlus,
			   triggerKind: "volume",
			  triggerRef:  "fill-abc-001",
			  expectedGen: 0,
			   wantNewGen: 1,
		   },
		   {
			   name: "kyc0 → standard via pin",
			   userID:   "u-003",
			   fromTier: viptiergate.TierKYC0,
			  toTier: viptiergate.TierStandard,
			   triggerKind: "pin",
			   triggerRef:  "u-003|pin|20260921110000",
			   expectedGen: 0,
			  wantNewGen: 1,
		  },
	   }
	  for _, tc := range cases {
		  t.Run(tc.name, func(t *testing.T) {
			  lease, newGen, err := svc.GatedUpgrade(ctx, viptiergate.AcquireLeaseReq{
				  UserID:   tc.userID,
				  FromTier: tc.fromTier,
				  ToTier:   tc.toTier,
				   TriggerKind: tc.triggerKind,
				  TriggerRef:  tc.triggerRef,
				   ExpectedGen: tc.expectedGen,
			   })
			   if err != nil {
				   t.Fatalf("unexpected error: %v", err)
			  }
			   if newGen != tc.wantNewGen {
				  t.Errorf("newGen: got %d, want %d", newGen, tc.wantNewGen)
			   }
			  if lease == nil {
				   t.Fatal("lease must not be nil")
			  }
			  if lease.UserID != tc.userID {
				   t.Errorf("lease.UserID: got %s, want %s", lease.UserID, tc.userID)
			  }
			   if lease.ToTier != tc.toTier {
				  t.Errorf("lease.ToTier: got %s, want %s", lease.ToTier, tc.toTier)
			   }
			  // 验证世代已推进。
			   gen, err := svc.GetGeneration(ctx, tc.userID)
			   if err != nil {
				  t.Fatalf("GetGeneration: %v", err)
			  }
			   if gen.Generation != tc.wantNewGen {
				   t.Errorf("stored generation: got %d, want %d", gen.Generation, tc.wantNewGen)
			  }
		   })
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 2. CAS 并发竞争——多 goroutine 同时抬档，仅一方 CAS 成功
// ─────────────────────────────────────────────────────────────────────────────

  func TestGatedUpgrade_ConcurrentCAS(t *testing.T) {
	  ctx := context.Background()
	   svc := newTestService(t)
	  userID := "u-concurrent-001"
	  const goroutines = 8
	   var (
		  successCount int32
		   staleCount int32
		  wg sync.WaitGroup
	  )
	  wg.Add(goroutines)
	  for i := 0; i < goroutines; i++ {
		   go func(idx int) {
			   defer wg.Done()
			  _, _, err := svc.GatedUpgrade(ctx, viptiergate.AcquireLeaseReq{
				   UserID:   userID,
				   FromTier: viptiergate.TierStandard,
				  ToTier:   viptiergate.TierVIP,
				   TriggerKind: "volume",
				  TriggerRef: fmt.Sprintf("fill-conc-%d", idx),
				   ExpectedGen: 0, // 所有 goroutine 都以世代 0 作为 CAS 前置条件
			   })
			   if err == nil {
				  atomic.AddInt32(&successCount,
					  1)
			   } else if err == viptiergate.ErrStaleGen {
				   atomic.AddInt32(&staleCount, 1)
			  }
		  }(i)
	   }
	  wg.Wait()
	   // 恰好 1 个 goroutine 成功（CAS 保证互斥）。
	  if successCount != 1 {
		   t.Errorf("expected exactly 1 CAS success, got %d", successCount)
	  }
	  // 其余 7 个 goroutine 应该得到 ErrStaleGen。
	  if staleCount != goroutines-1 {
		   t.Errorf("expected %d ErrStaleGen, got %d", goroutines-1, staleCount)
	   }
	  // 最终世代应为 1。
	  gen,
		   _ := svc.GetGeneration(ctx, userID)
	   if gen.Generation != 1 {
		   t.Errorf("final generation: got %d, want 1", gen.Generation)
	  }
  }

// ─────────────────────────────────────────────────────────────────────────────
// 3. 降档被在途冻额拦截
// ─────────────────────────────────────────────────────────────────────────────

  func TestGatedDowngrade_BlockedByInflightFreeze(t *testing.T) {
	   ctx := context.Background()
	   svc := newTestService(t)
	  userID := "u-freeze-001"
	   // 注册一笔活跃冻额。
	  freeze, err := svc.RegisterFreeze(ctx, userID, "BTC", "0.1", 5, viptiergate.TierVIP, 0)
	   if err != nil {
		  t.Fatalf("RegisterFreeze: %v", err)
	  }
	   // 降档尝试 → 应被 ErrInflightFreeze 拦截。
	   _, _, downErr := svc.GatedDowngrade(ctx, viptiergate.AcquireLeaseReq{
		   UserID:   userID,
		   FromTier: viptiergate.TierVIP,
		  ToTier:      viptiergate.TierStandard,
		   TriggerKind: "clear_pin",
		  TriggerRef: userID + "|clear_pin|ops1",
		   ExpectedGen: 0,
	   })
	   if downErr != viptiergate.ErrInflightFreeze {
		   t.Errorf("expected ErrInflightFreeze, got %v", downErr)
	   }
	  // 释放冻额后降档应成功。
	  if err := svc.ReleaseFreeze(ctx, freeze.ID); err != nil {
		  t.Fatalf("ReleaseFreeze: %v", err)
	  }
	  _, newGen, err := svc.GatedDowngrade(ctx, viptiergate.AcquireLeaseReq{
		   UserID: userID,
		   FromTier:    viptiergate.TierVIP,
		  ToTier:      viptiergate.TierStandard,
		  TriggerKind: "clear_pin",
		   TriggerRef:  userID + "|clear_pin|ops2",
		   ExpectedGen: 0,
	  })
	   if err != nil {
		   t.Errorf("after release, GatedDowngrade should succeed, got %v", err)
	   }
	  if newGen != 1 {
		   t.Errorf("newGen after downgrade: got %d, want 1", newGen)
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 4. 降档被在途 QuoteBind 拦截
// ─────────────────────────────────────────────────────────────────────────────

   func TestGatedDowngrade_BlockedByInflightQuote(t *testing.T) {
	   ctx := context.Background()
	  now := time.Now().UTC()
	   svc := newTestService(t)
	   svc.WithClock(fixedClock(now))
	  userID := "u-quote-block-001"
	   // 颁发一个活跃 QuoteBind（TTL 5 分钟）。
	  qb, err := svc.IssueQuote(ctx, viptiergate.IssueQuoteReq{
		   UserID: userID,
		   Asset: "ETH",
		  Gross: "1.0",
		  Fee:   "0.005",
		  Net:   "0.995",
		   BPS: 5,
		   Tier: viptiergate.TierVIP,
	   })
	  if err != nil {
		   t.Fatalf("IssueQuote: %v", err)
	  }
	   // 降档 → 被 ErrInflightQuote 拦截。
	   _, _, err = svc.GatedDowngrade(ctx, viptiergate.AcquireLeaseReq{
		  UserID:   userID,
		   FromTier: viptiergate.TierVIP,
		  ToTier:      viptiergate.TierStandard,
		   TriggerKind: "clear_pin",
		  TriggerRef:  userID + "|clear_pin|ops1",
		   ExpectedGen: 0,
	  })
	  if err != viptiergate.ErrInflightQuote {
		   t.Errorf("expected ErrInflightQuote, got %v", err)
	   }
	   // Quote 过期后降档应成功。
	  svc.WithClock(fixedClock(now.Add(10 * time.Minute)))
	   _, _, err = svc.GatedDowngrade(ctx, viptiergate.AcquireLeaseReq{
		  UserID: userID,
		   FromTier:    viptiergate.TierVIP,
		  ToTier:      viptiergate.TierStandard,
		  TriggerKind: "clear_pin",
		   TriggerRef:  userID + "|clear_pin|ops2",
		  ExpectedGen: 0,
	   })
	  if err != nil {
		   t.Errorf("after quote expiry, GatedDowngrade should succeed, got %v", err)
	  }
	  _ = qb
  }

// ─────────────────────────────────────────────────────────────────────────────
// 5. QuoteBind 结算世代不符——自动开立 DriftCase
// ─────────────────────────────────────────────────────────────────────────────

  func TestSettleQuote_GenMismatch_AutoOpensDriftCase(t *testing.T) {
	  ctx := context.Background()
	   now := time.Now().UTC()
	  svc := newTestService(t)
	  svc.WithClock(fixedClock(now))
	   userID := "u-gencheck-001"
	  // 颁发 QuoteBind（世代 0）。
	  qb, err := svc.IssueQuote(ctx, viptiergate.IssueQuoteReq{
		   UserID: userID,
		  Asset: "BTC",
		  Gross: "0.5",
		   Fee:   "0.0025",
		  Net: "0.4975",
		  BPS: 5,
		   Tier: viptiergate.TierVIP,
	   })
	   if err != nil {
		   t.Fatalf("IssueQuote: %v", err)
	   }
	  // 档位切换（世代 0 → 1）。
	  _, _, err = svc.GatedUpgrade(ctx, viptiergate.AcquireLeaseReq{
		   UserID:   userID,
		   FromTier: viptiergate.TierVIP,
		   ToTier: viptiergate.TierVIPPlus,
		  TriggerKind: "volume",
		  TriggerRef:  "fill-gencheck-001",
		  ExpectedGen: 0,
	   })
	   if err != nil {
		   t.Fatalf("GatedUpgrade: %v", err)
	   }
	  // 尝试结算旧世代 Quote → ErrQuoteGenMismatch。
	  _, err = svc.SettleQuote(ctx, viptiergate.SettleQuoteReq{
		  QuoteBindID: qb.ID,
		  UserID:     userID,
		  CurrentGen: 0, // 调用方仍持有旧世代
	  })
	  if err != viptiergate.ErrQuoteGenMismatch {
		   t.Errorf("expected ErrQuoteGenMismatch, got %v", err)
	   }
	   // 验证 DriftCase 已被自动开立。
	   cases, err := svc.ListOpenDriftCasesForUser(ctx, userID)
	  if err != nil {
		  t.Fatalf("ListOpenDriftCasesForUser: %v", err)
	   }
	   if len(cases) == 0 {
		  t.Error("expected at least one DriftCase to be auto-opened after QuoteGenMismatch")
	   }
	   if cases[0].Kind != viptiergate.DriftQuoteGenStale {
		  t.Errorf("drift case kind: got %s, want %s", cases[0].Kind, viptiergate.DriftQuoteGenStale)
	  }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 6. DriftCase 四眼收口完整路径
// ─────────────────────────────────────────────────────────────────────────────

  func TestDriftCase_FourEyeClose(t *testing.T) {
	  ctx := context.Background()
	  svc := newTestService(t)
	  userID := "u-foureyetest-001"
	  // 手动开立漂移案。
	   dc, err := svc.OpenDriftCase(ctx, viptiergate.OpenDriftCaseReq{
		   UserID: userID,
		   Asset:        "USDT",
		   Kind:         viptiergate.DriftBPSStale,
		  ObservedTier: viptiergate.TierStandard,
		   ExpectedTier: viptiergate.TierVIP,
		  ObservedBPS:  10,
		  ExpectedBPS:  5,
		  Amount:       "1000.0",
		   Generation: 1,
		   Note:       "test drift case",
	   })
	   if err != nil {
		  t.Fatalf("OpenDriftCase: %v", err)
	  }
	   // 抬档操作应被 ErrDriftBlocked 拦截。
	  _, _, blockErr := svc.GatedUpgrade(ctx, viptiergate.AcquireLeaseReq{
		  UserID: userID,
		   FromTier: viptiergate.TierStandard,
		   ToTier:      viptiergate.TierVIP,
		   TriggerKind: "pin",
		  TriggerRef: userID + "|pin|blocked",
		   ExpectedGen: 0,
	  })
	  if blockErr == nil {
		  t.Error("expected ErrDriftBlocked, got nil")
	   }
	   // 构建并验证证明。
	   proof,
		   err := svc.BuildAndVerifyProof(ctx, dc.ID)
	   if err != nil {
		  t.Fatalf("BuildAndVerifyProof: %v", err)
	  }
	  if proof.Status != viptiergate.DriftProofVerified {
		  t.Errorf("proof status: got %s, want verified", proof.Status)
	   }
	  // 第一人确认。
	   dc2, err := svc.ConfirmDriftClose(ctx, viptiergate.ConfirmDriftReq{
		  CaseID:   dc.ID,
		  Operator: "ops-alice",
	  })
	   if err != nil {
		   t.Fatalf("first ConfirmDriftClose: %v", err)
	   }
	  if dc2.Status == viptiergate.DriftClosed {
		   t.Error("should not be closed after first confirm only")
	  }
	   // 同人重复确认 → ErrDuplicateConfirm。
	   _, dupErr := svc.ConfirmDriftClose(ctx, viptiergate.ConfirmDriftReq{
		   CaseID:   dc.ID,
		  Operator: "ops-alice", // 同人
	  })
	  if dupErr != viptiergate.ErrDuplicateConfirm {
		   t.Errorf("expected ErrDuplicateConfirm, got %v", dupErr)
	  }
	  // 第二人确认 → 关闭。
	   dc3, err := svc.ConfirmDriftClose(ctx, viptiergate.ConfirmDriftReq{
		  CaseID: dc.ID,
		  Operator: "ops-bob", // 不同操作员
	  })
	  if err != nil {
		  t.Fatalf("second ConfirmDriftClose: %v", err)
	   }
	  if dc3.Status != viptiergate.DriftClosed {
		  t.Errorf("drift case status: got %s, want closed", dc3.Status)
	   }
	   // 关闭后，抬档操作应成功（不再被 ErrDriftBlocked 拦截）。
	  _, _, err = svc.GatedUpgrade(ctx, viptiergate.AcquireLeaseReq{
		   UserID: userID,
		   FromTier: viptiergate.TierStandard,
		  ToTier:   viptiergate.TierVIP,
		  TriggerKind: "pin",
		   TriggerRef:  userID + "|pin|after-close",
		  ExpectedGen: 0,
	  })
	   if err != nil {
		  t.Errorf("after drift close, GatedUpgrade should succeed, got %v", err)
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 7. 证明摘要防篡改
// ─────────────────────────────────────────────────────────────────────────────

   func TestDriftProof_DigestTamper(t *testing.T) {
	   ctx := context.Background()
	   svc := newTestService(t)
	  userID := "u-digest-tamper-001"
	  dc, _ := svc.OpenDriftCase(ctx, viptiergate.OpenDriftCaseReq{
		  UserID: userID,
		   Asset: "BTC",
		  Kind:         viptiergate.DriftBPSStale,
		  ObservedTier: viptiergate.TierStandard,
		  ExpectedTier: viptiergate.TierVIP,
		   ObservedBPS:  10,
		  ExpectedBPS:  5,
		  Amount: "100.0",
		   Generation: 2,
	   })
	  proof, err := svc.BuildAndVerifyProof(ctx, dc.ID)
	  if err != nil {
		  t.Fatalf("BuildAndVerifyProof: %v", err)
	   }
	   if proof.Status != viptiergate.DriftProofVerified {
		  t.Fatalf("proof not verified: status=%s", proof.Status)
	   }
	  // 验证 VerifyDigestOnly 通过。
	  if !viptiergate.VerifyDigestOnly(proof) {
		   t.Error("VerifyDigestOnly should return true for untampered proof")
	   }
	   // 篡改 ObservedBPS。
	  tampered := proof.Copy()
	   tampered.ObservedBPS = 99
	  if viptiergate.VerifyDigestOnly(&tampered) {
		   t.Error("VerifyDigestOnly should return false for tampered proof")
	   }
	  // 篡改 Amount。
	   tampered2 := proof.Copy()
	   tampered2.Amount = "9999.0"
	   if viptiergate.VerifyDigestOnly(&tampered2) {
		  t.Error("VerifyDigestOnly should return false after Amount tamper")
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 8. 幂等：重复 GatedUpgrade（同 TriggerRef）返回已有租约
// ─────────────────────────────────────────────────────────────────────────────

  func TestGatedUpgrade_Idempotency(t *testing.T) {
	  ctx := context.Background()
	  svc := newTestService(t)
	   userID := "u-idem-001"
	  ref := "fill-idem-001"
	  lease1, gen1, err := svc.GatedUpgrade(ctx, viptiergate.AcquireLeaseReq{
		   UserID: userID,
		  FromTier: viptiergate.TierStandard,
		   ToTier:   viptiergate.TierVIP,
		   TriggerKind: "volume",
		   TriggerRef: ref,
		   ExpectedGen: 0,
	  })
	   if err != nil {
		  t.Fatalf("first GatedUpgrade: %v", err)
	   }
	   // 相同 TriggerRef 再次调用，应返回已有租约（幂等）。
	   lease2, gen2, err := svc.GatedUpgrade(ctx, viptiergate.AcquireLeaseReq{
		  UserID: userID,
		   FromTier:    viptiergate.TierStandard,
		   ToTier:      viptiergate.TierVIP,
		   TriggerKind: "volume",
		  TriggerRef:  ref,
		   ExpectedGen: 1, // 世代已推进，CAS 应走幂等路径
	   })
	   if err != nil {
		  t.Fatalf("idempotent GatedUpgrade: %v", err)
	  }
	  if lease1.ID != lease2.ID {
		   t.Errorf("idempotent call should return same lease: %s vs %s", lease1.ID, lease2.ID)
	  }
	  if gen2 != gen1 {
		   t.Errorf("idempotent call should return same gen: %d vs %d", gen2, gen1)
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 9. QuoteBind 正常结算路径（世代匹配）
// ─────────────────────────────────────────────────────────────────────────────

  func TestSettleQuote_HappyPath(t *testing.T) {
	  ctx := context.Background()
	   now := time.Now().UTC()
	  svc := newTestService(t)
	   svc.WithClock(fixedClock(now))
	   userID := "u-settle-001"
	   qb, err := svc.IssueQuote(ctx, viptiergate.IssueQuoteReq{
		  UserID: userID,
		   Asset: "ETH",
		  Gross: "2.0",
		  Fee:   "0.01",
		  Net: "1.99",
		  BPS: 5,
		  Tier: viptiergate.TierVIP,
	  })
	  if err != nil {
		  t.Fatalf("IssueQuote: %v", err)
	  }
	   // 结算（世代匹配，未切换）。
	  settled, err := svc.SettleQuote(ctx, viptiergate.SettleQuoteReq{
		  QuoteBindID: qb.ID,
		   UserID:      userID,
		   CurrentGen:  0,
	  })
	   if err != nil {
		  t.Fatalf("SettleQuote: %v", err)
	  }
	  if settled.Status != viptiergate.QuoteBindSettled {
		   t.Errorf("status: got %s, want settled", settled.Status)
	  }
  }

// ─────────────────────────────────────────────────────────────────────────────
// 10. 证明需通过才能收口漂移案（未证明不可收口）
// ─────────────────────────────────────────────────────────────────────────────

  func TestDriftCase_CannotCloseWithoutProof(t *testing.T) {
	  ctx := context.Background()
	   svc := newTestService(t)
	   userID := "u-noproof-001"
	  dc, _ := svc.OpenDriftCase(ctx, viptiergate.OpenDriftCaseReq{
		   UserID: userID,
		  Asset: "USDT",
		  Kind:         viptiergate.DriftFreezeStale,
		   ObservedTier: viptiergate.TierVIP,
		   ExpectedTier: viptiergate.TierVIPPlus,
		   ObservedBPS: 5,
		   ExpectedBPS: 2,
		   Amount:      "500.0",
		   Generation: 1,
	  })
	   // 在无证明的情况下尝试收口 → ErrProofMissing。
	   _, err := svc.ConfirmDriftClose(ctx, viptiergate.ConfirmDriftReq{
		   CaseID: dc.ID,
		   Operator: "ops-alice",
	  })
	  if err != viptiergate.ErrProofMissing {
		  t.Errorf("expected ErrProofMissing, got %v", err)
	   }
  }

// ─────────────────────────────────────────────────────────────────────────────
// 11. IsSettlementBlocked 综合检查
// ─────────────────────────────────────────────────────────────────────────────

   func TestIsSettlementBlocked(t *testing.T) {
	  ctx := context.Background()
	   svc := newTestService(t)
	   userID := "u-blocked-check-001"
	  // 初始状态：未阻塞。
	  blocked, _, err := svc.IsSettlementBlocked(ctx, userID)
	   if err != nil || blocked {
		   t.Errorf("expected not blocked: blocked=%v err=%v", blocked, err)
	   }
	   // 开立 DriftCase 后：阻塞。
	   svc.OpenDriftCase(ctx, viptiergate.OpenDriftCaseReq{
		  UserID: userID,
		   Asset:  "BTC",
		   Kind:         viptiergate.DriftBPSReverse,
		  ObservedTier: viptiergate.TierVIPPlus,
		  ExpectedTier: viptiergate.TierVIP,
		  ObservedBPS: 4,
		   ExpectedBPS: 5,
		  Amount:      "10.0",
		  Generation:  3,
	  })
	   blocked, reason, err := svc.IsSettlementBlocked(ctx, userID)
	  if err != nil {
		   t.Fatalf("IsSettlementBlocked error: %v", err)
	   }
	   if !blocked {
		  t.Error("expected blocked after open drift case")
	   }
	   if reason == "" {
		   t.Error("reason should not be empty when blocked")
	  }
  }

// ─────────────────────────────────────────────────────────────────────────────
// 12. 多档位世代递增序列
// ─────────────────────────────────────────────────────────────────────────────

  func TestMultipleTierSwitches_GenerationSequence(t *testing.T) {
	   ctx := context.Background()
	   svc := newTestService(t)
	  userID := "u-multigen-001"
	   // 期望世代序列：0 → 1 → 2 → 3
	   upgrades := []struct {
		   from viptiergate.Tier
		   to viptiergate.Tier
		   triggerRef  string
		  expectedGen int64
		  wantNewGen int64
	   }{
		   {viptiergate.TierStandard, viptiergate.TierVIP, "fill-mg-001", 0, 1},
		  {viptiergate.TierVIP, viptiergate.TierVIPPlus, "fill-mg-002", 1, 2},
		  {viptiergate.TierVIPPlus, viptiergate.TierVIP, "clear-mg-001", 2, 3},
	  }
	   for i, u := range upgrades {
		  var (
			  lease *viptiergate.TierLease
			  newGen int64
			   err    error
		   )
		  if viptiergate.IsUpgrade(u.from, u.to) {
			   lease, newGen, err = svc.GatedUpgrade(ctx, viptiergate.AcquireLeaseReq{
				  UserID:   userID,
				  FromTier: u.from,
				   ToTier:      u.to,
				  TriggerKind: "volume",
				   TriggerRef: u.triggerRef,
				  ExpectedGen: u.expectedGen,
			   })
		  } else {
			  lease, newGen, err = svc.GatedDowngrade(ctx, viptiergate.AcquireLeaseReq{
				  UserID: userID,
				  FromTier: u.from,
				   ToTier:   u.to,
				  TriggerKind: "clear_pin",
				  TriggerRef: u.triggerRef,
				  ExpectedGen: u.expectedGen,
			   })
		  }
		  if err != nil {
			  t.Fatalf("switch[%d]: %v", i, err)
		   }
		   if newGen != u.wantNewGen {
			  t.Errorf("switch[%d] newGen: got %d, want %d", i, newGen, u.wantNewGen)
		   }
		   _ = lease
	  }
	  // 最终世代应为 3。
	   gen, _ := svc.GetGeneration(ctx, userID)
	  if gen.Generation != 3 {
		   t.Errorf("final generation: got %d, want 3", gen.Generation)
	   }
  }
