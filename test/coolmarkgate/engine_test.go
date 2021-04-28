package coolmarkgate_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/coolmarkgate"
)

// newTestEngine 构造使用内存仓储的引擎，time 由外部控制。

  func newTestEngine(clock func() time.Time) *coolmarkgate.Engine {
	  eng := coolmarkgate.NewEngine(
		coolmarkgate.NewMemCoolLeaseRepo(),
		coolmarkgate.NewMemBreachCaseRepo(),
		coolmarkgate.NewMemCoolProofRepo(),
		coolmarkgate.NewMemCoolGenRepo(),
	)
	if clock != nil {
		eng = eng.WithClock(clock)
	}
	return eng
}

// ──────────────────────────────────────────────────────────────────────────────
// 一、主路径
// ──────────────────────────────────────────────────────────────────────────────

   func TestMark_BasicPath(t *testing.T) {
	   base := time.Date(2021, 9, 10, 10, 0, 0, 0, time.UTC)
	   eng := newTestEngine(func() time.Time { return base })
	  res, err := eng.Mark(context.Background(), coolmarkgate.MarkInput{
		  Asset:  "USDT",
		  Reason: "hot over cap",
		  Window: time.Hour,
	   })
	  if err != nil {
		   t.Fatalf("Mark failed: %v", err)
	   }
	  if res.Generation != 1 {
		  t.Errorf("want Generation=1, got %d", res.Generation)
	   }
	   if res.Reused {
		  t.Errorf("want Reused=false for first Mark")
	  }
	   if res.Lease == nil {
		  t.Fatal("want non-nil Lease")
	   }
	   if res.Lease.State != coolmarkgate.CoolMarked {
		   t.Errorf("want State=marked, got %s", res.Lease.State)
	   }
	  wantUntil := base.Add(time.Hour)
	  if !res.Lease.CoolUntil.Equal(wantUntil) {
		  t.Errorf("want CoolUntil=%v, got %v", wantUntil, res.Lease.CoolUntil)
	   }
  }

   func TestMark_ReusesActiveWindow(t *testing.T) {
	  base := time.Date(2021, 9, 10, 10, 0, 0, 0, time.UTC)
	  eng := newTestEngine(func() time.Time { return base })
	  ctx := context.Background()
	   r1, err := eng.Mark(ctx, coolmarkgate.MarkInput{Asset: "BTC", Window: time.Hour})
	  if err != nil {
		  t.Fatalf("first Mark: %v", err)
	  }
	  r2, err := eng.Mark(ctx, coolmarkgate.MarkInput{Asset: "BTC", Window: time.Hour})
	  if err != nil {
		   t.Fatalf("second Mark: %v", err)
	   }
	  if !r2.Reused {
		  t.Error("want Reused=true for second Mark within active window")
	  }
	  if r2.Generation != r1.Generation {
		  t.Errorf("want same Generation, got %d vs %d", r2.Generation, r1.Generation)
	   }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 二、冷却窗活跃时 Exec 必须被拒绝
// ──────────────────────────────────────────────────────────────────────────────

   func TestCheckExecAllowed_BlockedDuringCoolWindow(t *testing.T) {
	  base := time.Date(2021, 9, 10, 10, 0, 0, 0, time.UTC)
	   now := base
	  eng := newTestEngine(func() time.Time { return now })
	  ctx := context.Background()
	  markRes, err := eng.Mark(ctx, coolmarkgate.MarkInput{
		   Asset:  "USDT",
		   Reason: "test",
		   Window: time.Hour,
	  })
	  if err != nil {
		  t.Fatalf("Mark: %v", err)
	  }
	   // 冷却窗内执行验证。
	   res, err := eng.CheckExecAllowed(ctx, coolmarkgate.ExecVerifyInput{
		  Asset: "USDT",
		  CoolGen: markRes.Generation,
		   ExecID:  "exec-001",
	   })
	  if err == nil {
		   t.Fatal("want error, got nil")
	  }
	  if !errors.Is(err, coolmarkgate.ErrCoolWindowActive) {
		   t.Errorf("want ErrCoolWindowActive, got %v", err)
	  }
	   if res.Allowed {
		   t.Error("want Allowed=false")
	  }
	   if res.BreachCaseID == "" {
		  t.Error("want non-empty BreachCaseID")
	   }
   }

  func TestCheckExecAllowed_AllowedAfterClearCool(t *testing.T) {
	  base := time.Date(2021,
		   9, 10, 10, 0, 0, 0, time.UTC)
	  now := base
	   eng := newTestEngine(func() time.Time { return now })
	   ctx := context.Background()
	  markRes, err := eng.Mark(ctx, coolmarkgate.MarkInput{
		   Asset: "ETH",
		   Window: 30 * time.Minute,
	   })
	   if err != nil {
		  t.Fatalf("Mark: %v", err)
	   }
	   // 推进时钟超过冷却窗。
	   now = base.Add(31 * time.Minute)
	   // ClearCool 签发证明。
	  proof, err := eng.ClearCool(ctx, markRes.Lease.LeaseID)
	  if err != nil {
		   t.Fatalf("ClearCool: %v", err)
	   }
	  if proof == nil || proof.ProofID == "" {
		  t.Fatal("want non-nil proof with ProofID")
	  }
	   // 执行验证应放行。
	   res, err := eng.CheckExecAllowed(ctx, coolmarkgate.ExecVerifyInput{
		   Asset: "ETH",
		  CoolGen: markRes.Generation,
		  ExecID:  "exec-002",
	  })
	   if err != nil {
		   t.Fatalf("CheckExecAllowed after clear: %v", err)
	   }
	  if !res.Allowed {
		  t.Errorf("want Allowed=true, got false, reason: %s", res.Reason)
	   }
	   if res.ProofID != proof.ProofID {
		   t.Errorf("want ProofID=%s, got %s", proof.ProofID, res.ProofID)
	   }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 三、世代不符必须拒绝
// ──────────────────────────────────────────────────────────────────────────────

  func TestCheckExecAllowed_GenMismatch(t *testing.T) {
	  base := time.Date(2021, 9, 10, 10, 0, 0, 0, time.UTC)
	   now := base
	  eng := newTestEngine(func() time.Time { return now })
	   ctx := context.Background()
	  // Mark 两次抬升世代到 2。
	  r1, _ := eng.Mark(ctx, coolmarkgate.MarkInput{Asset: "BTC", Window: time.Minute})
	  now = base.Add(2 * time.Minute)
	   _, _ = eng.ClearCool(ctx, r1.Lease.LeaseID)
	   _, _ = eng.Mark(ctx, coolmarkgate.MarkInput{Asset: "BTC", Window: time.Hour})
	  // 携带旧世代 1 执行验证。
	   res, err := eng.CheckExecAllowed(ctx, coolmarkgate.ExecVerifyInput{
		   Asset: "BTC",
		   CoolGen: 1,
		  ExecID: "exec-stale",
	  })
	  if err == nil {
		  t.Fatal("want error for stale gen")
	   }
	   if !errors.Is(err, coolmarkgate.ErrGenMismatch) {
		   t.Errorf("want ErrGenMismatch, got %v", err)
	  }
	  if res.BreachCaseID == "" {
		  t.Error("want BreachCaseID for gen mismatch")
	   }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 四、无证明不得放行
// ──────────────────────────────────────────────────────────────────────────────

  func TestCheckExecAllowed_NoProofBlocked(t *testing.T) {
	   base := time.Date(2021, 9, 10, 10, 0, 0, 0, time.UTC)
	  now := base
	   eng := newTestEngine(func() time.Time { return now })
	   ctx := context.Background()
	   markRes, _ := eng.Mark(ctx, coolmarkgate.MarkInput{Asset: "USDT", Window: 30 * time.Minute})
	  // 推进时钟超过冷却窗，但不调用 ClearCool（不签发证明）。
	   now = base.Add(31 * time.Minute)
	   res, err := eng.CheckExecAllowed(ctx, coolmarkgate.ExecVerifyInput{
		  Asset:   "USDT",
		  CoolGen: markRes.Generation,
		   ExecID:  "exec-no-proof",
	   })
	  if err == nil {
		  t.Fatal("want error when proof missing")
	  }
	   if !errors.Is(err, coolmarkgate.ErrNoProof) {
		   t.Errorf("want ErrNoProof, got %v", err)
	   }
	  if res.Allowed {
		   t.Error("want Allowed=false when proof missing")
	  }
	   if res.BreachCaseID == "" {
		  t.Error("want BreachCaseID when proof missing")
	   }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 五、并发 Mark↔Exec 竞态
// ──────────────────────────────────────────────────────────────────────────────

   func TestConcurrentMarkExec_OnlyOneWins(t *testing.T) {
	   base := time.Date(2021, 9, 10, 10, 0, 0, 0, time.UTC)
	  eng := newTestEngine(func() time.Time { return base })
	  ctx := context.Background()
	   const goroutines = 20
	  var (
		  wg          sync.WaitGroup
		   markSuccess int64
		   execSuccess int64
		   start = make(chan struct{})
	  )
	   // 先 Mark 一次，建立世代 1。
	   _, err := eng.Mark(ctx, coolmarkgate.MarkInput{Asset: "USDT", Window: time.Hour})
	   if err != nil {
		  t.Fatalf("initial Mark: %v", err)
	   }
	   // 并发发出多个 CheckExecAllowed（冷却窗内，world 一定会被拒绝）。
	   for i := 0; i < goroutines; i++ {
		   wg.Add(1)
		  go func(idx int) {
			  defer wg.Done()
			  <-start
			  res, e := eng.CheckExecAllowed(ctx, coolmarkgate.ExecVerifyInput{
				   Asset:   "USDT",
				   CoolGen: 1,
				  ExecID: "exec-race-" + itoa(idx),
			   })
			  if e == nil && res != nil && res.Allowed {
				   atomic.AddInt64(&execSuccess, 1)
			   }
		  }(i)
	  }
	   close(start)
	  wg.Wait()
	  // 冷却窗活跃时，所有 Exec 必须被拒绝。
	   if execSuccess > 0 {
		  t.Errorf("want 0 exec success during cool window, got %d", execSuccess)
	   }
	  _ = markSuccess
  }

  func TestConcurrentMark_SingleGeneration(t *testing.T) {
	  base := time.Date(2021, 9, 10, 10, 0, 0, 0, time.UTC)
	  eng := newTestEngine(func() time.Time { return base })
	  ctx := context.Background()
	   const goroutines = 10
	   var (
		  wg sync.WaitGroup
		   generations = make([]int64, goroutines)
		   start       = make(chan struct{})
	   )
	  for i := 0; i < goroutines; i++ {
		  wg.Add(1)
		   go func(idx int) {
			  defer wg.Done()
			  <-start
			  res, _ := eng.Mark(ctx, coolmarkgate.MarkInput{
				   Asset:  "ETH",
				  Window: time.Hour,
			  })
			  if res != nil {
				   generations[idx] = res.Generation
			  }
		  }(i)
	   }
	  close(start)
	   wg.Wait()
	   // 所有 goroutine 应该得到相同的世代（第一次创建为 1，后续均为 reuse）。
	   for i, g := range generations {
		  if g != 1 {
			  t.Errorf("goroutine %d got generation %d, want 1", i, g)
		   }
	   }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 六、ClearCool 幂等
// ──────────────────────────────────────────────────────────────────────────────

  func TestClearCool_Idempotent(t *testing.T) {
	   base := time.Date(2021, 9, 10, 10, 0, 0, 0, time.UTC)
	  now := base
	   eng := newTestEngine(func() time.Time { return now })
	   ctx := context.Background()
	  markRes, _ := eng.Mark(ctx, coolmarkgate.MarkInput{Asset: "BTC", Window: 30 * time.Minute})
	   now = base.Add(31 * time.Minute)
	  proof1,
		  err := eng.ClearCool(ctx, markRes.Lease.LeaseID)
	   if err != nil {
		   t.Fatalf("first ClearCool: %v", err)
	  }
	  proof2, err := eng.ClearCool(ctx, markRes.Lease.LeaseID)
	  if err != nil {
		   t.Fatalf("second ClearCool: %v", err)
	  }
	  if proof1.ProofID != proof2.ProofID {
		   t.Errorf("want same ProofID on idempotent ClearCool, got %s vs %s",
			   proof1.ProofID, proof2.ProofID)
	  }
  }

   func TestClearCool_RejectActiveWindow(t *testing.T) {
	  base := time.Date(2021, 9, 10, 10, 0, 0, 0, time.UTC)
	  eng := newTestEngine(func() time.Time { return base })
	   ctx := context.Background()
	  markRes, _ := eng.Mark(ctx, coolmarkgate.MarkInput{Asset: "USDT", Window: time.Hour})
	  _, err := eng.ClearCool(ctx, markRes.Lease.LeaseID)
	  if err == nil {
		   t.Fatal("want error when clearing active window")
	  }
	   if !errors.Is(err, coolmarkgate.ErrCoolWindowActive) {
		  t.Errorf("want ErrCoolWindowActive, got %v", err)
	  }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 七、RecordSentCheck 作为独立最后防线
// ──────────────────────────────────────────────────────────────────────────────

   func TestRecordSentCheck_BlockedDuringCoolWindow(t *testing.T) {
	   base := time.Date(2021, 9, 10, 10, 0, 0, 0, time.UTC)
	   eng := newTestEngine(func() time.Time { return base })
	   ctx := context.Background()
	   markRes, _ := eng.Mark(ctx, coolmarkgate.MarkInput{Asset: "USDT", Window: time.Hour})
	   res, err := eng.RecordSentCheck(ctx, coolmarkgate.ExecVerifyInput{
		   Asset: "USDT",
		   CoolGen: markRes.Generation,
		   ExecID: "marksent-001",
	   })
	   if err == nil {
		  t.Fatal("RecordSentCheck: want error in cool window")
	   }
	   if !errors.Is(err, coolmarkgate.ErrCoolWindowActive) {
		  t.Errorf("want ErrCoolWindowActive, got %v", err)
	   }
	   if res.Allowed {
		   t.Error("want Allowed=false in RecordSentCheck during cool window")
	   }
  }

// ──────────────────────────────────────────────────────────────────────────────
// 八、GetCoolGen 只读输出
// ──────────────────────────────────────────────────────────────────────────────

  func TestGetCoolGen_BeforeAndAfterMark(t *testing.T) {
	   base := time.Date(2021, 9, 10, 10, 0, 0, 0, time.UTC)
	  eng := newTestEngine(func() time.Time { return base })
	   ctx := context.Background()
	   // 未 Mark 时世代为 0。
	   gen, err := eng.GetCoolGen(ctx, "XRP")
	  if err != nil {
		  t.Fatalf("GetCoolGen before mark: %v", err)
	  }
	   if gen != 0 {
		   t.Errorf("want gen=0 before mark, got %d", gen)
	   }
	  // Mark 后世代为 1。
	  _, _ = eng.Mark(ctx, coolmarkgate.MarkInput{Asset: "XRP", Window: time.Hour})
	   gen, err = eng.GetCoolGen(ctx, "XRP")
	  if err != nil {
		   t.Fatalf("GetCoolGen after mark: %v", err)
	   }
	  if gen != 1 {
		  t.Errorf("want gen=1 after mark, got %d", gen)
	   }
  }

// ──────────────────────────────────────────────────────────────────────────────
// 九、FSM 非法跃迁
// ──────────────────────────────────────────────────────────────────────────────

   func TestFSM_IllegalTransition(t *testing.T) {
	   lease := &coolmarkgate.CoolLease{
		  LeaseID: "test-lease",
		   Asset:     "USDT",
		  State:     coolmarkgate.CoolCleared,
		   CoolUntil: time.Now().UTC(),
	   }
	  err := coolmarkgate.Transition(lease, coolmarkgate.CoolMarked)
	  if err == nil {
		  t.Fatal("want error on illegal transition cleared->marked")
	  }
	   if !errors.Is(err, coolmarkgate.ErrIllegalTransition) {
		  t.Errorf("want ErrIllegalTransition, got %v", err)
	  }
  }

   func TestFSM_LegalTransitions(t *testing.T) {
	   tests := []struct {
		   from coolmarkgate.CoolLeaseState
		   to   coolmarkgate.CoolLeaseState
		   want bool
	  }{
		  {coolmarkgate.CoolMarked,
			  coolmarkgate.CoolBlocking, true},
		   {coolmarkgate.CoolMarked, coolmarkgate.CoolCleared, true},
		   {coolmarkgate.CoolBlocking, coolmarkgate.CoolCleared, true},
		   {coolmarkgate.CoolCleared, coolmarkgate.CoolMarked, false},
		  {coolmarkgate.CoolCleared, coolmarkgate.CoolBlocking, false},
		   {coolmarkgate.CoolBlocking, coolmarkgate.CoolMarked, false},
	   }
	  for _, tt := range tests {
		   got := coolmarkgate.CanTransition(tt.from, tt.to)
		   if got != tt.want {
			  t.Errorf("CanTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
		  }
	  }
  }

// ──────────────────────────────────────────────────────────────────────────────
// 十、StalePlanDetectorWorker 漂移发现
// ──────────────────────────────────────────────────────────────────────────────

   func TestStalePlanDetector_BreachOnSentDuringCoolWindow(t *testing.T) {
	  base := time.Date(2021, 9, 10, 10, 0, 0, 0, time.UTC)
	  eng := newTestEngine(func() time.Time { return base })
	   ctx := context.Background()
	  markRes, _ := eng.Mark(ctx, coolmarkgate.MarkInput{
		  Asset: "USDT",
		   Window: time.Hour,
	   })
	  detector := coolmarkgate.NewStalePlanDetectorWorker(eng, 64)
	  // 提交一个在冷却窗内发生的 MarkSent 事件。
	  sentAt := base.Add(30 * time.Minute)
	  submitted := detector.Submit(coolmarkgate.StalePlanEvent{
		  Asset: "USDT",
		  ExecID:  "exec-stale-001",
		   SentAt:  sentAt,
		   CoolGen: markRes.Generation,
	   })
	  if !submitted {
		   t.Fatal("want Submit=true")
	   }
	  // 用带超时的 context 运行一轮。
	   runCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	  defer cancel()
	  go detector.Run(runCtx)
	   <-runCtx.Done()
	  // 验证 BreachCase 已落库。
	   cases, err := eng.ListBreachCasesByAsset(ctx, "USDT", 10)
	  if err != nil {
		  t.Fatalf("ListBreachCasesByAsset: %v", err)
	  }
	   found := false
	  for _, bc := range cases {
		  if bc.ExecID == "exec-stale-001" && bc.Decision == coolmarkgate.BreachDrifted {
			   found = true
			  break
		   }
	   }
	   if !found {
		   t.Errorf("want drifted BreachCase for exec-stale-001, cases: %+v", cases)
	  }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 十一、sweeppol.CoolGenBook 集成路径
// ──────────────────────────────────────────────────────────────────────────────

   func TestCoolGenBook_EvaluateBlockedDuringCoolWindow(t *testing.T) {
	  base := time.Date(2021, 9, 10, 10, 0, 0, 0, time.UTC)
	  eng := newTestEngine(func() time.Time { return base })
	   ctx := context.Background()
	  // Mark USDT 进入冷却。
	   _, err := eng.Mark(ctx, coolmarkgate.MarkInput{Asset: "USDT", Window: time.Hour})
	   if err != nil {
		  t.Fatalf("Mark: %v", err)
	   }
	   // 验证 CheckExecAllowed 会拒绝。
	  gen, _ := eng.GetCoolGen(ctx, "USDT")
	   res, err := eng.CheckExecAllowed(ctx, coolmarkgate.ExecVerifyInput{
		  Asset: "USDT",
		  CoolGen: gen,
		  ExecID:  "eval-exec-001",
	   })
	  if err == nil || res.Allowed {
		  t.Errorf("want exec blocked during cool window")
	   }
  }

// ──────────────────────────────────────────────────────────────────────────────
// 辅助函数
// ──────────────────────────────────────────────────────────────────────────────

   func itoa(n int) string {
	  if n == 0 {
		  return "0"
	   }
	  digits := make([]byte, 0, 4)
	   for n > 0 {
		   digits = append([]byte{byte('0' + n%10)}, digits...)
		   n /= 10
	  }
	   return string(digits)
   }
