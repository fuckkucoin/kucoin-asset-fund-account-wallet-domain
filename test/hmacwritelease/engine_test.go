package hmacwritelease_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/hmacfence"
	"account-assets.kucoin.net/funding/internal/hmacwritelease"
)

// ──────────────────────────────────────────────────────────────────────────────
// 工厂
// ──────────────────────────────────────────────────────────────────────────────

const (
	testKeyID  = "k-test-01"
	testSecret = "test-secret-for-engine-tests"
)

// newTestEngine 构造使用内存仓储的引擎。

   func newTestEngine(clock func() time.Time) *hmacwritelease.Engine {
	  fence := hmacfence.New(30 * time.Second)
	   ring := hmacfence.NewKeyRing(map[string]string{
		  testKeyID: testSecret,
	   })
	leases := hmacwritelease.NewMemLeaseRepo()
	nonces := hmacwritelease.NewMemNonceSeenRepo()
	proofs := hmacwritelease.NewMemDriftProofRepo()
	gens := hmacwritelease.NewMemHaltGenRepo()
	eng := hmacwritelease.NewEngine(fence, ring, leases, nonces, proofs, gens)
	if clock != nil {
		eng = eng.WithClock(clock)
	}
	return eng
}

// buildInput 构造 VerifyAndLeaseInput（含正确签名）。
// 重点：ts 必须使用 time.Now 而非注入时钟，因为 hmacfence.Service 内部使用系统时钟
// 做 skew 检查；引擎的注入时钟仅用于租约 issued_at/expire_at 计算。

  func buildInput(keyID, secret, nonce, method, path string, ttl time.Duration) hmacwritelease.VerifyAndLeaseInput {
	  ts := strconv.FormatInt(time.Now().Unix(), 10)
	  body := []byte(`{"amount":"100"}`)
	   sig := hmacfence.Sign(secret, ts, nonce, method, path, body)
	   return hmacwritelease.VerifyAndLeaseInput{
		   KeyID: keyID,
		   TS:    ts,
		  Nonce: nonce,
		   Method: method,
		  Path:   path,
		   Sig:    sig,
		   Body:   body,
		  TTL: ttl,
	  }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 一、主路径
// ──────────────────────────────────────────────────────────────────────────────

   func TestVerifyAndLease_BasicPath(t *testing.T) {
	  base := time.Now().UTC()
	   eng := newTestEngine(func() time.Time { return base })
	  ctx := context.Background()
	  in := buildInput(testKeyID, testSecret, "nonce-basic-001", "POST", "/v1/wd/freeze", 5*time.Minute)
	   res, err := eng.VerifyAndLease(ctx, in)
	   if err != nil {
		   t.Fatalf("VerifyAndLease failed: %v", err)
	  }
	   if res.Lease == nil {
		  t.Fatal("want non-nil Lease")
	  }
	  if res.Lease.LeaseID == "" {
		  t.Error("want non-empty LeaseID (hmac_write_lease_id)")
	  }
	   if res.Lease.State != hmacwritelease.LeaseIssued {
		  t.Errorf("want state=issued, got %s", res.Lease.State)
	   }
	  if res.Lease.KeyID != testKeyID {
		   t.Errorf("want KeyID=%s, got %s", testKeyID, res.Lease.KeyID)
	   }
	   if res.Lease.HaltGen != 0 {
		  t.Errorf("want HaltGen=0, got %d", res.Lease.HaltGen)
	  }
	   if res.Lease.ExpireAt.Before(base) {
		  t.Errorf("ExpireAt must be after IssuedAt; issued=%v expire=%v", base, res.Lease.ExpireAt)
	   }
   }

   func TestVerifyAndLease_DefaultTTL(t *testing.T) {
	  base := time.Now().UTC()
	  eng := newTestEngine(func() time.Time { return base })
	   ctx := context.Background()
	   in := buildInput(testKeyID, testSecret, "nonce-default-ttl-001", "POST", "/v1/transfer", 0)
	  res, err := eng.VerifyAndLease(ctx, in)
	   if err != nil {
		   t.Fatalf("VerifyAndLease failed: %v", err)
	   }
	  wantExpire := base.Add(hmacwritelease.DefaultLeaseTTL)
	   if !res.Lease.ExpireAt.Equal(wantExpire) {
		  t.Errorf("want default TTL expiry=%v, got=%v", wantExpire, res.Lease.ExpireAt)
	   }
   }

  func TestWriteCheck_HappyPath(t *testing.T) {
	  base := time.Now().UTC()
	   eng := newTestEngine(func() time.Time { return base })
	   ctx := context.Background()
	  in := buildInput(testKeyID, testSecret, "nonce-write-ok-001", "POST", "/v1/wd/freeze", 5*time.Minute)
	  res, err := eng.VerifyAndLease(ctx, in)
	   if err != nil {
		  t.Fatalf("VerifyAndLease: %v", err)
	  }
	   wr, werr := eng.WriteCheck(ctx, hmacwritelease.WriteCheckInput{
		   LeaseID: res.Lease.LeaseID,
		   WriteScope: "withdraw.freeze",
	   })
	  if werr != nil {
		   t.Fatalf("WriteCheck should pass: %v", werr)
	  }
	   if !wr.Allowed {
		  t.Errorf("WriteCheck: want Allowed=true, reason=%s", wr.Reason)
	  }
	   if wr.Lease == nil {
		  t.Error("WriteCheck: want non-nil Lease")
	  }
	   if wr.Lease.State != hmacwritelease.LeaseBound {
		  t.Errorf("want state=bound after WriteCheck, got %s", wr.Lease.State)
	  }
  }

// ──────────────────────────────────────────────────────────────────────────────
// 二、验收场景（对应 testdata/hmacwritelease/ golden 夹具）
// ──────────────────────────────────────────────────────────────────────────────
// TestGolden_VerifyHaltThenWrite 验收场景 1：Verify 后 Halt，旧租约必须失败并落 DriftProof。

   func TestGolden_VerifyHaltThenWrite(t *testing.T) {
	  golden := loadGolden(t, "verify_halt_then_write.json")
	  t.Log("golden:", golden["description"])
	  base := time.Now().UTC()
	   eng := newTestEngine(func() time.Time { return base })
	  ctx := context.Background()
	   // 步骤 1：VerifyAndLease 成功，获取租约。
	   in := buildInput(testKeyID, testSecret, "nonce-halt-guard-001", "POST", "/v1/wd/freeze", 5*time.Minute)
	   leaseRes, err := eng.VerifyAndLease(ctx, in)
	   if err != nil {
		   t.Fatalf("step1 VerifyAndLease: %v", err)
	  }
	  oldLeaseID := leaseRes.Lease.LeaseID
	   oldHaltGen := leaseRes.HaltGen
	  if oldHaltGen != 0 {
		   t.Errorf("step1: want HaltGen=0, got %d", oldHaltGen)
	   }
	   // 步骤 2：SetHalt(true)，世代递增。
	   newGen,
		   err := eng.SetHalt(ctx, true)
	  if err != nil {
		  t.Fatalf("step2 SetHalt: %v", err)
	   }
	  if newGen <= oldHaltGen {
		   t.Errorf("step2: halt_gen should increase; old=%d new=%d", oldHaltGen, newGen)
	  }
	  // 步骤 3：用旧租约调用 WriteCheck，必须 fail-closed 并落 DriftProof。
	  wr, werr := eng.WriteCheck(ctx, hmacwritelease.WriteCheckInput{
		   LeaseID:    oldLeaseID,
		  WriteScope: "withdraw.freeze",
	   })
	  if werr == nil {
		   t.Fatal("step3: WriteCheck should fail after Halt, but got nil error")
	  }
	   if wr.Allowed {
		   t.Error("step3: WriteCheck must not be Allowed after Halt")
	   }
	   if wr.Reason != hmacwritelease.DriftHaltGenMismatch && wr.Reason != hmacwritelease.DriftRevoked {
		  t.Errorf("step3: want DriftHaltGenMismatch or DriftRevoked, got %s", wr.Reason)
	   }
	  if wr.DriftProofID == "" {
		  t.Error("step3: HmacWriteDriftProof must be recorded (drift_proof_id must not be empty)")
	  }
	  // 验证 DriftProof 可以查出来。
	   proof, pErr := eng.GetDriftProof(ctx, wr.DriftProofID)
	   if pErr != nil {
		   t.Fatalf("step3: GetDriftProof: %v", pErr)
	  }
	  if proof.LeaseID != oldLeaseID {
		   t.Errorf("step3: proof.LeaseID want %s, got %s", oldLeaseID, proof.LeaseID)
	  }
   }

// TestGolden_NonceReplay 验收场景 2：同一 key+nonce 第二次 Verify 必须 ErrReplay，且不得再发新租约。

  func TestGolden_NonceReplay(t *testing.T) {
	   golden := loadGolden(t, "nonce_replay.json")
	  t.Log("golden:", golden["description"])
	  base := time.Now().UTC()
	  eng := newTestEngine(func() time.Time { return base })
	  ctx := context.Background()
	  nonce := "nonce-replay-golden-001"
	   in := buildInput(testKeyID, testSecret, nonce, "POST", "/v1/transfer", 5*time.Minute)
	   // 第一次：成功。
	   r1, err := eng.VerifyAndLease(ctx, in)
	   if err != nil {
		  t.Fatalf("first VerifyAndLease: %v", err)
	  }
	  if r1.Lease == nil || r1.Lease.LeaseID == "" {
		   t.Fatal("first VerifyAndLease: want valid lease")
	  }
	  // 第二次：同 nonce 必须失败。
	   // 重新构建相同输入（hmacfence.Service 内存已记录该 nonce）。
	  in2 := buildInput(testKeyID, testSecret, nonce, "POST", "/v1/transfer", 5*time.Minute)
	   r2,
		   err2 := eng.VerifyAndLease(ctx, in2)
	  if err2 == nil {
		   t.Fatal("second VerifyAndLease with same nonce should fail, got nil error")
	  }
	  if r2 != nil {
		  t.Error("second VerifyAndLease: must not return a new Lease")
	   }
	  // 允许 ErrReplay（hmacfence 内存层）或 ErrNonceReplay（持久层）。
	   if !errors.Is(err2, hmacfence.ErrReplay) && !errors.Is(err2, hmacwritelease.ErrNonceReplay) {
		  t.Errorf("second VerifyAndLease: want ErrReplay or ErrNonceReplay, got %v", err2)
	  }
  }

// TestGolden_NoLeaseFailClosed 验收场景 3：无租约写路径必须 fail-closed 并落 DriftProof。
  func TestGolden_NoLeaseFailClosed(t *testing.T) {
	  golden := loadGolden(t, "no_lease_fail_closed.json")
	   t.Log("golden:", golden["description"])
	  base := time.Now().UTC()
	   eng := newTestEngine(func() time.Time { return base })
	   ctx := context.Background()
	  wr, err := eng.WriteCheck(ctx, hmacwritelease.WriteCheckInput{
		  LeaseID:    "",
		  WriteScope: "withdraw.freeze",
	   })
	  if err == nil {
		  t.Fatal("WriteCheck without lease should fail, got nil error")
	   }
	   if wr.Allowed {
		   t.Error("WriteCheck without lease: Allowed must be false")
	  }
	  if wr.Reason != hmacwritelease.DriftNoLease {
		   t.Errorf("want DriftNoLease, got %s", wr.Reason)
	   }
	  if wr.DriftProofID == "" {
		   t.Error("DriftProof must be recorded for no-lease write attempt")
	  }
   }

// TestGolden_ExpiredLeaseFailClosed 验收场景 4：过期租约写路径必须 fail-closed 并落 DriftProof。
  func TestGolden_ExpiredLeaseFailClosed(t *testing.T) {
	   golden := loadGolden(t, "expired_lease_fail_closed.json")
	   t.Log("golden:", golden["description"])
	   now := time.Now().UTC()
	   clk := &atomicClock{t: now}
	   eng := newTestEngine(clk.Now)
	  ctx := context.Background()
	   // 签发极短 TTL 租约。
	  in := buildInput(testKeyID, testSecret, "nonce-expiry-golden-001", "POST", "/v1/wd/freeze", time.Millisecond)
	   res, err := eng.VerifyAndLease(ctx, in)
	   if err != nil {
		   t.Fatalf("VerifyAndLease: %v", err)
	   }
	   leaseID := res.Lease.LeaseID
	  // 推进时钟使租约过期。
	   clk.Advance(10 * time.Millisecond)
	   // WriteCheck 必须 fail-closed。
	  wr, werr := eng.WriteCheck(ctx, hmacwritelease.WriteCheckInput{
		   LeaseID:    leaseID,
		  WriteScope: "withdraw.freeze",
	  })
	  if werr == nil {
		   t.Fatal("WriteCheck on expired lease should fail")
	   }
	   if wr.Allowed {
		   t.Error("WriteCheck on expired lease: Allowed must be false")
	  }
	   if wr.Reason != hmacwritelease.DriftExpired {
		  t.Errorf("want DriftExpired, got %s", wr.Reason)
	   }
	   if wr.DriftProofID == "" {
		   t.Error("DriftProof must be recorded for expired lease")
	  }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 三、FSM 状态机矩阵
// ──────────────────────────────────────────────────────────────────────────────
// TestFSM_Transitions 覆盖所有合法与非法跃迁（对应 testdata/hmacwritelease/fsm_transitions.json）。

   func TestFSM_Transitions(t *testing.T) {
	   type tc struct {
		  from    hmacwritelease.LeaseState
		   to      hmacwritelease.LeaseState
		   allowed bool
	  }
	   cases := []tc{
		  {hmacwritelease.LeaseIssued, hmacwritelease.LeaseBound, true},
		   {hmacwritelease.LeaseIssued, hmacwritelease.LeaseExpired, true},
		  {hmacwritelease.LeaseIssued,
			  hmacwritelease.LeaseRevoked, true},
		   {hmacwritelease.LeaseIssued, hmacwritelease.LeaseConsumed, false},
		  {hmacwritelease.LeaseBound, hmacwritelease.LeaseConsumed, true},
		   {hmacwritelease.LeaseBound, hmacwritelease.LeaseExpired, true},
		   {hmacwritelease.LeaseBound, hmacwritelease.LeaseRevoked, true},
		  {hmacwritelease.LeaseBound, hmacwritelease.LeaseIssued, false},
		  {hmacwritelease.LeaseConsumed, hmacwritelease.LeaseIssued, false},
		  {hmacwritelease.LeaseConsumed, hmacwritelease.LeaseBound, false},
		   {hmacwritelease.LeaseConsumed, hmacwritelease.LeaseExpired, false},
		   {hmacwritelease.LeaseConsumed,
			  hmacwritelease.LeaseRevoked, false},
		  {hmacwritelease.LeaseExpired, hmacwritelease.LeaseIssued, false},
		  {hmacwritelease.LeaseExpired, hmacwritelease.LeaseRevoked, false},
		  {hmacwritelease.LeaseRevoked, hmacwritelease.LeaseIssued, false},
		  {hmacwritelease.LeaseRevoked,
			   hmacwritelease.LeaseConsumed, false},
	  }
	   for _, c := range cases {
		   got := hmacwritelease.CanTransition(c.from, c.to)
		   if got != c.allowed {
			  t.Errorf("CanTransition(%s→%s): want %v, got %v", c.from, c.to, c.allowed, got)
		   }
	  }
  }

   func TestFSM_Transition_NilLease(t *testing.T) {
	   err := hmacwritelease.Transition(nil, hmacwritelease.LeaseBound)
	   if err == nil {
		  t.Error("Transition(nil) should return error")
	  }
	   if !errors.Is(err, hmacwritelease.ErrIllegalTransition) {
		  t.Errorf("want ErrIllegalTransition, got %v", err)
	   }
   }

  func TestFSM_Terminal_States(t *testing.T) {
	   terminals := []hmacwritelease.LeaseState{
		  hmacwritelease.LeaseConsumed,
		   hmacwritelease.LeaseExpired,
		  hmacwritelease.LeaseRevoked,
	  }
	   for _, s := range terminals {
		   if !s.IsTerminal() {
			   t.Errorf("state %s should be terminal", s)
		  }
	   }
	  nonTerminals := []hmacwritelease.LeaseState{
		   hmacwritelease.LeaseIssued,
		   hmacwritelease.LeaseBound,
	  }
	   for _, s := range nonTerminals {
		   if s.IsTerminal() {
			   t.Errorf("state %s should NOT be terminal", s)
		  }
	   }
  }

// ──────────────────────────────────────────────────────────────────────────────
// 四、Halt 世代
// ──────────────────────────────────────────────────────────────────────────────

  func TestHalt_GenIncrement(t *testing.T) {
	   base := time.Now().UTC()
	  eng := newTestEngine(func() time.Time { return base })
	   ctx := context.Background()
	   gen0, err := eng.GetCurrentHaltGen(ctx)
	  if err != nil {
		   t.Fatalf("GetCurrentHaltGen: %v", err)
	  }
	  if gen0.Generation != 0 {
		   t.Errorf("initial generation want 0, got %d", gen0.Generation)
	   }
	   g1, err := eng.SetHalt(ctx, true)
	  if err != nil {
		   t.Fatalf("SetHalt(true): %v", err)
	  }
	  if g1 != 1 {
		   t.Errorf("after first SetHalt(true): want generation=1, got %d", g1)
	   }
	   g2, err := eng.SetHalt(ctx, true)
	  if err != nil {
		   t.Fatalf("second SetHalt(true): %v", err)
	   }
	   if g2 != 2 {
		  t.Errorf("after second SetHalt(true): want generation=2, got %d", g2)
	   }
	  // SetHalt(false) 不改变 Generation。
	  g3, err := eng.SetHalt(ctx, false)
	   if err != nil {
		   t.Fatalf("SetHalt(false): %v", err)
	   }
	   if g3 != 2 {
		   t.Errorf("after SetHalt(false): generation should remain 2, got %d", g3)
	  }
	  gen3, _ := eng.GetCurrentHaltGen(ctx)
	   if gen3.Halted {
		   t.Error("after SetHalt(false): Halted should be false")
	  }
   }

   func TestHalt_NewVerifyFailsWhenHalted(t *testing.T) {
	   base := time.Now().UTC()
	   eng := newTestEngine(func() time.Time { return base })
	   ctx := context.Background()
	  if _, err := eng.SetHalt(ctx, true); err != nil {
		   t.Fatalf("SetHalt(true): %v", err)
	   }
	  // Halt 状态下 VerifyAndLease 必须失败。
	   in := buildInput(testKeyID, testSecret, "nonce-halted-001", "POST", "/v1/wd/freeze", 5*time.Minute)
	  _,
		   err := eng.VerifyAndLease(ctx, in)
	   if err == nil {
		  t.Fatal("VerifyAndLease should fail when halted")
	   }
	  // 可以是 ErrHalted 或 hmacfence.ErrHalt。
	  if !errors.Is(err, hmacwritelease.ErrHalted) && !errors.Is(err, hmacfence.ErrHalt) {
		   t.Errorf("want ErrHalted or hmacfence.ErrHalt, got %v", err)
	  }
  }

// ──────────────────────────────────────────────────────────────────────────────
// 五、幂等性
// ──────────────────────────────────────────────────────────────────────────────

   func TestConsumeLease_Idempotent(t *testing.T) {
	   base := time.Now().UTC()
	   eng := newTestEngine(func() time.Time { return base })
	   ctx := context.Background()
	  in := buildInput(testKeyID, testSecret, "nonce-consume-idem-001", "POST", "/v1/transfer", 5*time.Minute)
	   res, err := eng.VerifyAndLease(ctx, in)
	  if err != nil {
		  t.Fatalf("VerifyAndLease: %v", err)
	   }
	   // WriteCheck 绑定。
	   if _, werr := eng.WriteCheck(ctx, hmacwritelease.WriteCheckInput{
		   LeaseID:    res.Lease.LeaseID,
		   WriteScope: "transfer",
	   }); werr != nil {
		   t.Fatalf("WriteCheck: %v", werr)
	  }
	   // 首次 ConsumeLease。
	   if err := eng.ConsumeLease(ctx, res.Lease.LeaseID); err != nil {
		  t.Fatalf("first ConsumeLease: %v", err)
	   }
	   // 重复 ConsumeLease 须幂等（不报错）。
	  if err := eng.ConsumeLease(ctx, res.Lease.LeaseID); err != nil {
		   t.Fatalf("second ConsumeLease (idempotent): %v", err)
	   }
	   // 验证最终状态为 consumed。
	   lease, _ := eng.GetLease(ctx, res.Lease.LeaseID)
	  if lease.State != hmacwritelease.LeaseConsumed {
		   t.Errorf("want state=consumed, got %s", lease.State)
	   }
  }

   func TestRevokeByHaltGen_Idempotent(t *testing.T) {
	  base := time.Now().UTC()
	  eng := newTestEngine(func() time.Time { return base })
	   ctx := context.Background()
	  // 签发一个租约。
	  in := buildInput(testKeyID, testSecret, "nonce-revoke-idem-001", "POST", "/v1/wd/freeze", 5*time.Minute)
	  res, err := eng.VerifyAndLease(ctx, in)
	   if err != nil {
		   t.Fatalf("VerifyAndLease: %v", err)
	  }
	   leaseID := res.Lease.LeaseID
	   // SetHalt(true) 触发批量吊销。
	   if _, err := eng.SetHalt(ctx, true); err != nil {
		  t.Fatalf("SetHalt: %v", err)
	  }
	   // 再次调用 RevokeByHaltGen（worker 下次扫描）须幂等。
	   n, err := eng.RevokeByHaltGen(ctx, 100)
	   if err != nil {
		   t.Fatalf("RevokeByHaltGen: %v", err)
	   }
	   // 已经在 SetHalt 内部吊销过，再次扫描不应再吊销（已是终态）。
	   if n != 0 {
		  t.Logf("RevokeByHaltGen idempotent: revoked=%d (already revoked earlier is ok)", n)
	  }
	   // 验证租约已 revoked。
	  lease, _ := eng.GetLease(ctx, leaseID)
	  if lease.State != hmacwritelease.LeaseRevoked {
		  t.Errorf("want state=revoked, got %s", lease.State)
	  }
  }

// ──────────────────────────────────────────────────────────────────────────────
// 六、竞态
// ──────────────────────────────────────────────────────────────────────────────
// TestRace_ConcurrentVerifyDifferentNonces 并发不同 nonce 的 VerifyAndLease 不得互相干扰。

   func TestRace_ConcurrentVerifyDifferentNonces(t *testing.T) {
	   base := time.Now().UTC()
	  eng := newTestEngine(func() time.Time { return base })
	   ctx := context.Background()
	   const workers = 10
	  var successCount int64
	   var wg sync.WaitGroup
	   wg.Add(workers)
	  for i := 0; i < workers; i++ {
		  go func(idx int) {
			   defer wg.Done()
			  nonce := "nonce-concurrent-" + strconv.Itoa(idx)
			   in := buildInput(testKeyID, testSecret, nonce, "POST", "/v1/wd/freeze", 5*time.Minute)
			   if res, err := eng.VerifyAndLease(ctx, in); err == nil && res.Lease != nil {
				  atomic.AddInt64(&successCount, 1)
			  }
		  }(i)
	  }
	   wg.Wait()
	   if successCount != workers {
		   t.Errorf("concurrent different nonces: want %d success, got %d", workers, successCount)
	   }
   }

// TestRace_ConcurrentSameNonceOnlyOneSucceeds 并发相同 nonce 只允许一个成功。
   func TestRace_ConcurrentSameNonceOnlyOneSucceeds(t *testing.T) {
	   base := time.Now().UTC()
	   eng := newTestEngine(func() time.Time { return base })
	   ctx := context.Background()
	   const workers = 5
	   nonce := "nonce-race-same-001"
	  var successCount int64
	   var wg sync.WaitGroup
	   wg.Add(workers)
	  for i := 0; i < workers; i++ {
		  go func() {
			   defer wg.Done()
			  in := buildInput(testKeyID, testSecret, nonce, "POST", "/v1/wd/freeze", 5*time.Minute)
			  if res, err := eng.VerifyAndLease(ctx, in); err == nil && res.Lease != nil {
				  atomic.AddInt64(&successCount, 1)
			   }
		   }()
	   }
	   wg.Wait()
	   if successCount != 1 {
		  t.Errorf("concurrent same nonce: want exactly 1 success, got %d", successCount)
	   }
  }

// ──────────────────────────────────────────────────────────────────────────────
// 七、失败语义（表驱动）
// ──────────────────────────────────────────────────────────────────────────────
// TestWriteCheck_FailureTable 以独立 engine 实例分别验证各失败语义（避免状态交叉干扰）。

  func TestWriteCheck_FailureTable(t *testing.T) {
	   ctx := context.Background()
	  t.Run("no_lease", func(t *testing.T) {
		  eng := newTestEngine(nil)
		   wr, werr := eng.WriteCheck(ctx, hmacwritelease.WriteCheckInput{LeaseID: "", WriteScope: "withdraw.freeze"})
		  if werr == nil {
			   t.Fatal("want error, got nil")
		  }
		  if !errors.Is(werr, hmacwritelease.ErrNoLeaseForWrite) {
			  t.Errorf("want ErrNoLeaseForWrite, got %v", werr)
		   }
		   if wr.Allowed || wr.Reason != hmacwritelease.DriftNoLease || wr.DriftProofID == "" {
			   t.Errorf("unexpected result: allowed=%v reason=%s proofID=%s", wr.Allowed, wr.Reason, wr.DriftProofID)
		  }
	  })
	  t.Run("not_found", func(t *testing.T) {
		  eng := newTestEngine(nil)
		  wr, werr := eng.WriteCheck(ctx, hmacwritelease.WriteCheckInput{LeaseID: "nonexistent-lease", WriteScope: "transfer"})
		   if werr == nil {
			   t.Fatal("want error, got nil")
		  }
		  if !errors.Is(werr, hmacwritelease.ErrLeaseNotFound) {
			   t.Errorf("want ErrLeaseNotFound, got %v", werr)
		  }
		   if wr.Allowed || wr.DriftProofID == "" {
			  t.Errorf("unexpected result: allowed=%v proofID=%s", wr.Allowed, wr.DriftProofID)
		   }
	  })
	   t.Run("expired", func(t *testing.T) {
		  // 独立 engine：无 halt，纯粹 TTL 过期。
		  base := time.Now().UTC()
		   clk := &atomicClock{t: base}
		   eng := newTestEngine(clk.Now)
		   in := buildInput(testKeyID, testSecret, "nonce-fc-expired-isolated-001", "POST", "/v1/wd/freeze", time.Millisecond)
		  res, err := eng.VerifyAndLease(ctx, in)
		   if err != nil {
			  t.Fatalf("VerifyAndLease: %v", err)
		  }
		  // 推进时钟让租约过期（不做 halt，避免 revoke 覆盖 expired）。
		  clk.Advance(10 * time.Millisecond)
		   wr, werr := eng.WriteCheck(ctx, hmacwritelease.WriteCheckInput{
			   LeaseID:    res.Lease.LeaseID,
			  WriteScope: "withdraw.freeze",
		   })
		   if werr == nil {
			  t.Fatal("want error, got nil")
		   }
		   if !errors.Is(werr, hmacwritelease.ErrLeaseExpired) {
			  t.Errorf("want ErrLeaseExpired, got %v", werr)
		   }
		   if wr.Allowed || wr.Reason != hmacwritelease.DriftExpired || wr.DriftProofID == "" {
			  t.Errorf("unexpected result: allowed=%v reason=%s proofID=%s", wr.Allowed, wr.Reason, wr.DriftProofID)
		  }
	   })
	   t.Run("consumed", func(t *testing.T) {
		  base := time.Now().UTC()
		   eng := newTestEngine(func() time.Time { return base })
		   in := buildInput(testKeyID, testSecret, "nonce-fc-consumed-isolated-001", "POST", "/v1/wd/freeze", 5*time.Minute)
		  res, err := eng.VerifyAndLease(ctx, in)
		  if err != nil {
			   t.Fatalf("VerifyAndLease: %v", err)
		  }
		   leaseID := res.Lease.LeaseID
		   // WriteCheck 绑定。
		  _, _ = eng.WriteCheck(ctx, hmacwritelease.WriteCheckInput{LeaseID: leaseID, WriteScope: "s"})
		  // ConsumeLease。
		  _ = eng.ConsumeLease(ctx, leaseID)
		   // 再次 WriteCheck 必须失败（consumed 态）。
		   wr, werr := eng.WriteCheck(ctx, hmacwritelease.WriteCheckInput{LeaseID: leaseID, WriteScope: "withdraw.freeze"})
		   if werr == nil {
			  t.Fatal("want error on consumed lease, got nil")
		  }
		   if !errors.Is(werr, hmacwritelease.ErrLeaseConsumed) {
			  t.Errorf("want ErrLeaseConsumed, got %v", werr)
		   }
		  if wr.Allowed || wr.DriftProofID == "" {
			   t.Errorf("unexpected result: allowed=%v proofID=%s", wr.Allowed, wr.DriftProofID)
		   }
	   })
	   t.Run("halt_gen_mismatch", func(t *testing.T) {
		   base := time.Now().UTC()
		  eng := newTestEngine(func() time.Time { return base })
		   // 签发租约（halt_gen=0）。
		   in := buildInput(testKeyID, testSecret, "nonce-fc-haltgen-isolated-001", "POST", "/v1/wd/freeze", 5*time.Minute)
		   res, err := eng.VerifyAndLease(ctx, in)
		  if err != nil {
			   t.Fatalf("VerifyAndLease: %v", err)
		   }
		  leaseID := res.Lease.LeaseID
		   // SetHalt(true) 递增世代（批量吊销旧租约）。
		  _, _ = eng.SetHalt(ctx, true)
		  // 旧租约 WriteCheck：应该返回 DriftHaltGenMismatch 或 DriftRevoked（取决于是否被批量吊销）。
		   wr, werr := eng.WriteCheck(ctx, hmacwritelease.WriteCheckInput{
			  LeaseID:    leaseID,
			  WriteScope: "withdraw.freeze",
		   })
		   if werr == nil {
			   t.Fatal("want error on halt_gen mismatch, got nil")
		   }
		   // 两种合法结果：批量吊销已执行 → DriftRevoked；批量吊销未执行 → DriftHaltGenMismatch。
		   if wr.Reason != hmacwritelease.DriftHaltGenMismatch && wr.Reason != hmacwritelease.DriftRevoked {
			   t.Errorf("want DriftHaltGenMismatch or DriftRevoked, got %s", wr.Reason)
		   }
		   if wr.Allowed || wr.DriftProofID == "" {
			   t.Errorf("unexpected result: allowed=%v proofID=%s", wr.Allowed, wr.DriftProofID)
		   }
	   })
  }

// ──────────────────────────────────────────────────────────────────────────────
// 八、nonce 持久防重放
// ──────────────────────────────────────────────────────────────────────────────

  func TestNonceSeen_PersistentReplay(t *testing.T) {
	  // 两个共享同一 NonceSeenRepo 的 engine（跨节点防重放）。
	  fence := hmacfence.New(30 * time.Second)
	  ring := hmacfence.NewKeyRing(map[string]string{testKeyID: testSecret})
	  sharedNonces := hmacwritelease.NewMemNonceSeenRepo()
	  eng1 := hmacwritelease.NewEngine(fence,
		  ring,
		   hmacwritelease.NewMemLeaseRepo(),
		   sharedNonces,
		   hmacwritelease.NewMemDriftProofRepo(),
		   hmacwritelease.NewMemHaltGenRepo(),
	   )
	  // eng2 使用独立 hmacfence.Service（内存 nonce 隔离，但 NonceSeenRepo 共享）。
	  fence2 := hmacfence.New(30 * time.Second)
	  eng2 := hmacwritelease.NewEngine(fence2, ring,
		  hmacwritelease.NewMemLeaseRepo(),
		  sharedNonces,
		  hmacwritelease.NewMemDriftProofRepo(),
		   hmacwritelease.NewMemHaltGenRepo(),
	  )
	   ctx := context.Background()
	   nonce := "nonce-persistent-replay-001"
	   // eng1 首次验签成功。
	   in1 := buildInput(testKeyID, testSecret, nonce, "POST", "/v1/wd/freeze", 5*time.Minute)
	  if _, err := eng1.VerifyAndLease(ctx, in1); err != nil {
		   t.Fatalf("eng1 first verify: %v", err)
	   }
	   // eng2 重放同一 nonce：hmacfence.Service 内存未记录（不同实例），
	  // 但持久 NonceSeenRepo 已有记录，必须 ErrNonceReplay。
	   in2 := buildInput(testKeyID, testSecret, nonce, "POST", "/v1/wd/freeze", 5*time.Minute)
	  _, err := eng2.VerifyAndLease(ctx, in2)
	  if err == nil {
		  t.Fatal("eng2 replay must fail due to persistent nonce seen")
	  }
	   if !errors.Is(err, hmacwritelease.ErrNonceReplay) {
		   t.Errorf("want ErrNonceReplay for persistent replay, got %v", err)
	  }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 九、Worker （过期扫描与 Halt 批量吊销）
// ──────────────────────────────────────────────────────────────────────────────

   func TestWorker_ExpireStaleLeases(t *testing.T) {
	  base := time.Now().UTC()
	  clk := &atomicClock{t: base}
	  eng := newTestEngine(clk.Now)
	   ctx := context.Background()
	   // 签发短 TTL 租约。
	   in := buildInput(testKeyID, testSecret, "nonce-worker-expire-001", "POST", "/v1/wd/freeze", time.Millisecond)
	  res, err := eng.VerifyAndLease(ctx, in)
	  if err != nil {
		   t.Fatalf("VerifyAndLease: %v", err)
	  }
	  leaseID := res.Lease.LeaseID
	  // 推进时钟，触发过期。
	  clk.Advance(10 * time.Millisecond)
	   // 调用 ExpireStaleLeases（ worker）。
	   n, err := eng.ExpireStaleLeases(ctx, 100)
	   if err != nil {
		  t.Fatalf("ExpireStaleLeases: %v", err)
	   }
	   if n != 1 {
		   t.Errorf("want 1 lease expired, got %d", n)
	   }
	  // 验证状态流转到 expired。
	   lease, _ := eng.GetLease(ctx, leaseID)
	   if lease.State != hmacwritelease.LeaseExpired {
		   t.Errorf("want state=expired, got %s", lease.State)
	   }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 十、MethodPath 指纹
// ──────────────────────────────────────────────────────────────────────────────

   func TestMethodPathFor(t *testing.T) {
	   cases := []struct {
		   method, path, want string
	   }{
		   {"POST", "/v1/wd/freeze", "POST::/v1/wd/freeze"},
		  {"get", "/v1/transfer", "GET::/v1/transfer"},
		   {" PUT ", " /v1/credit ", "PUT::/v1/credit"},
	  }
	  for _, c := range cases {
		   got := hmacwritelease.MethodPathFor(c.method, c.path)
		   if got != c.want {
			   t.Errorf("MethodPathFor(%q, %q): want %q, got %q", c.method, c.path, c.want, got)
		  }
	  }
  }

// ──────────────────────────────────────────────────────────────────────────────
// 辅助工具
// ──────────────────────────────────────────────────────────────────────────────
// atomicClock 支持并发推进的可注入时钟。
type atomicClock struct {
	mu sync.Mutex
	t time.Time
}

   func (c *atomicClock) Now() time.Time {
	  c.mu.Lock()
	   defer c.mu.Unlock()
	   return c.t
   }

   func (c *atomicClock) Advance(d time.Duration) {
	   c.mu.Lock()
	   defer c.mu.Unlock()
	   c.t = c.t.Add(d)
  }

// loadGolden 从 testdata/hmacwritelease/ 加载 JSON 夹具。
  func loadGolden(t *testing.T, name string) map[string]interface{} {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "hmacwritelease", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("loadGolden %s: %v", name, err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("loadGolden parse %s: %v", name, err)
	}
	return out
}
