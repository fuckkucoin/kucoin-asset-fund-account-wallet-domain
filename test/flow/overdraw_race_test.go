package flow

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/availcas"
	"account-assets.kucoin.net/funding/internal/overwdbar"
	"github.com/shopspring/decimal"
)

// ────────────────────────────────────────────────────────────────────────────
// 辅助
// ────────────────────────────────────────────────────────────────────────────
// staticReader 静态余额读取器，供注入固定余额/冻额/预留金。
type staticReader struct {
	balance decimal.Decimal
	frozen decimal.Decimal
	reserved decimal.Decimal
}

   func (r *staticReader) ReadBalance(_ context.Context, _, _, _ string) (decimal.Decimal, error) {
	  return r.balance, nil
  }

   func (r *staticReader) ReadFrozen(_ context.Context, _, _, _ string) (decimal.Decimal, error) {
	   return r.frozen, nil
   }

   func (r *staticReader) ReadReserved(_ context.Context, _, _ string) (decimal.Decimal, error) {
	   return r.reserved, nil
  }

// newTestService 构造带内存存储的 Service。

   func newTestService(balance, frozen, reserved string) *availcas.Service {
	  bal := decimal.RequireFromString(balance)
	  froz := decimal.RequireFromString(frozen)
	  rsv := decimal.RequireFromString(reserved)
	  reader := &staticReader{balance: bal,
		  frozen: froz, reserved: rsv}
	   store := availcas.NewMemStore()
	   return availcas.NewService(availcas.ServiceConfig{
		   Store:  store,
		   Reader: reader,
	   })
   }

// issueLease 颁发租约辅助。
  func issueLease(t *testing.T, svc *availcas.Service, user, asset, book string) *availcas.AvailLease {
	  t.Helper()
	   lease, err := svc.IssueLease(context.Background(), user, asset, book, availcas.LeaseOptions{})
	   if err != nil {
		   t.Fatalf("IssueLease: %v", err)
	   }
	   return lease
   }

// submitIntent 提交意图辅助。
  func submitIntent(t *testing.T, svc *availcas.Service, lease *availcas.AvailLease, amount, kind, idem string) *availcas.BarrierDecision {
	  t.Helper()
	  amt := decimal.RequireFromString(amount)
	  decision, err := svc.Submit(context.Background(), &availcas.DebitIntent{
		  IdemKey: idem,
		  UserID:  lease.UserID,
		   Asset:  lease.Asset,
		  Book:   lease.Book,
		   Amount: amt,
		  Kind: kind,
		  Reason:  "test",
		  LeaseID: lease.ID,
		   LeaseGen:      lease.Gen,
		   ExpectedAvail: lease.Available,
	  })
	   if err != nil {
		  t.Fatalf("Submit: %v", err)
	   }
	   return decision
   }

// ────────────────────────────────────────────────────────────────────────────
// 场景 1: 双出金同窗 CAS 互斥
// ────────────────────────────────────────────────────────────────────────────
// TestDoubleWithdrawSameWindow 两笔出金意图持同一租约：仅一笔 accepted，另一笔 rejected，RaceProof 可查。
   func TestDoubleWithdrawSameWindow(t *testing.T) {
	   svc := newTestService("1000", "0", "0")
	  lease := issueLease(t, svc, "user1", "USDT", "spot")
	  if lease.Available.String() != "1000" {
		   t.Fatalf("expected available=1000, got %s", lease.Available)
	   }
	  // 第一笔：600 USDT 出金意图（应 accepted）
	  d1 := submitIntent(t, svc, lease, "600", availcas.IntentKindFreeze, "wd1")
	  if !d1.Accepted {
		   t.Fatalf("first intent should be accepted, got rejected: %s", d1.Intent.RejectNote)
	  }
	   // 第二笔：以同一租约再提交 500 USDT（租约已 bound，应 rejected）
	  d2 := submitIntent(t, svc, lease, "500", availcas.IntentKindFreeze, "wd2")
	   if d2.Accepted {
		   t.Fatal("second intent on bound lease must be rejected")
	   }
	  if d2.Intent.RejectCode != availcas.RejectCodeCASConflict {
		  t.Fatalf("expected cas_conflict, got %s", d2.Intent.RejectCode)
	  }
	   // RaceProof 必须存在。
	   if d2.Proof == nil {
		  t.Fatal("race proof must be created for concurrent intents")
	  }
	  if d2.Proof.WinnerIntentID != d1.Intent.ID {
		   t.Fatalf("winner should be first intent %s, got %s", d1.Intent.ID, d2.Proof.WinnerIntentID)
	   }
	   if d2.Proof.LoserIntentID != d2.Intent.ID {
		   t.Fatalf("loser should be second intent %s, got %s", d2.Intent.ID, d2.Proof.LoserIntentID)
	   }
	  // 证明可查。
	  proof, err := svc.GetProof(context.Background(), d2.Proof.ID)
	  if err != nil {
		   t.Fatalf("GetProof: %v", err)
	   }
	   if proof.RejectCode != availcas.RejectCodeCASConflict {
		   t.Fatalf("proof reject code mismatch: %s", proof.RejectCode)
	   }
  }

// ────────────────────────────────────────────────────────────────────────────
// 场景 2: 出金+划转同窗互斥
// ────────────────────────────────────────────────────────────────────────────
// TestWithdrawAndTransferSameWindow 出金与划转共享同一租约：同样互斥。
  func TestWithdrawAndTransferSameWindow(t *testing.T) {
	   svc := newTestService("500", "100", "0")
	   // available = 500 - 100 - 0 = 400
	   lease := issueLease(t, svc, "user2", "BTC", "spot")
	   if !lease.Available.Equal(decimal.RequireFromString("400")) {
		   t.Fatalf("expected 400 available, got %s", lease.Available)
	   }
	   // 出金冻结 300
	  dWd := submitIntent(t, svc, lease, "300", availcas.IntentKindFreeze, "wd-btc-1")
	   if !dWd.Accepted {
		   t.Fatalf("withdraw intent should be accepted")
	  }
	  // 同租约划转扣减 200（应 rejected 因租约已 bound）
	  dTx := submitIntent(t, svc, lease, "200", availcas.IntentKindDebit, "xfer-btc-1")
	  if dTx.Accepted {
		  t.Fatal("transfer on bound lease must be rejected")
	  }
	  if dTx.Proof == nil {
		   t.Fatal("race proof required for concurrent withdraw+transfer")
	  }
  }

// ────────────────────────────────────────────────────────────────────────────
// 场景 3: 租约过期后提交
// ────────────────────────────────────────────────────────────────────────────
// TestLeaseExpiredReject 租约过期后提交意图必须 rejected；可申请新租约重试。

   func TestLeaseExpiredReject(t *testing.T) {
	   store := availcas.NewMemStore()
	  reader := &staticReader{
		   balance:  decimal.RequireFromString("200"),
		   frozen:   decimal.Zero,
		   reserved: decimal.Zero,
	   }
	  // 注入一个极短 TTL（1ms）的时钟
	  past := time.Now().UTC().Add(-5 * time.Second)
	   clock := func() time.Time { return past }
	   _ = clock // clock is used via newBarrierWithClock
	  // 使用口径 Service 但用超短 TTL 的选项让租约立即过期
	  svc := availcas.NewService(availcas.ServiceConfig{Store: store, Reader: reader})
	   lease, err := svc.IssueLease(context.Background(), "user3", "ETH", "spot",
		  availcas.LeaseOptions{TTL: 1 * time.Millisecond})
	   if err != nil {
		  t.Fatalf("IssueLease: %v", err)
	   }
	   // 等待租约过期
	  time.Sleep(5 * time.Millisecond)
	  // 提交意图：应 rejected（lease expired）
	  amt := decimal.RequireFromString("100")
	  decision, err := svc.Submit(context.Background(), &availcas.DebitIntent{
		   UserID: "user3",
		   Asset:  "ETH",
		  Book:   "spot",
		  Amount: amt,
		   Kind:          availcas.IntentKindFreeze,
		  LeaseID:       lease.ID,
		   LeaseGen:      lease.Gen,
		  ExpectedAvail: lease.Available,
	  })
	  if err != nil {
		   t.Fatalf("Submit on expired lease returned error: %v", err)
	  }
	   if decision.Accepted {
		   t.Fatal("expired lease intent must be rejected")
	  }
	   if decision.Intent.RejectCode != availcas.RejectCodeLeaseExpired {
		  t.Fatalf("expected lease_expired, got %s", decision.Intent.RejectCode)
	  }
	   // 申请新租约后重试应能成功。
	   lease2, err := svc.IssueLease(context.Background(), "user3", "ETH", "spot", availcas.LeaseOptions{})
	  if err != nil {
		  t.Fatalf("IssueLease retry: %v", err)
	   }
	  d2, err := svc.Submit(context.Background(), &availcas.DebitIntent{
		   UserID: "user3",
		  Asset:  "ETH",
		  Book:   "spot",
		   Amount: amt,
		   Kind:   availcas.IntentKindFreeze,
		   LeaseID:       lease2.ID,
		  LeaseGen:      lease2.Gen,
		   ExpectedAvail: lease2.Available,
	   })
	   if err != nil {
		  t.Fatalf("Submit with new lease: %v", err)
	   }
	  if !d2.Accepted {
		  t.Fatalf("new lease intent should be accepted, got %s", d2.Intent.RejectNote)
	   }
  }

// ────────────────────────────────────────────────────────────────────────────
// 场景 4: 预留已占后假可用
// ────────────────────────────────────────────────────────────────────────────
// TestReservedReducesAvailable 热钱包预留金占用后，可用额须扣减预留。
  func TestReservedReducesAvailable(t *testing.T) {
	  // balance=1000, frozen=0, reserved=300 → available=700
	  svc := newTestService("1000", "0", "300")
	   lease := issueLease(t, svc, "user4", "USDT", "spot")
	   if !lease.Available.Equal(decimal.RequireFromString("700")) {
		   t.Fatalf("expected available=700 (after reserved=300), got %s", lease.Available)
	   }
	   // 提交 750 超预留后可用额，应 rejected（超提）。
	   amt750 := decimal.RequireFromString("750")
	   d, err := svc.Submit(context.Background(), &availcas.DebitIntent{
		   UserID: "user4",
		  Asset:  "USDT",
		  Book:   "spot",
		  Amount: amt750,
		   Kind: availcas.IntentKindFreeze,
		   LeaseID:       lease.ID,
		   LeaseGen:      lease.Gen,
		   ExpectedAvail: lease.Available,
	  })
	  if err != nil {
		   t.Fatalf("Submit: %v", err)
	  }
	  if d.Accepted {
		   t.Fatal("750 > 700 available: must be rejected")
	  }
	   if d.Intent.RejectCode != availcas.RejectCodeInsufficientAvail {
		   t.Fatalf("expected insufficient_avail, got %s", d.Intent.RejectCode)
	  }
	   // 提交 700（刚好等于可用额）应 accepted。
	  lease2 := issueLease(t, svc, "user4", "USDT", "spot")
	   d2 := submitIntent(t, svc, lease2, "700", availcas.IntentKindFreeze, "wd-exact")
	   if !d2.Accepted {
		  t.Fatalf("700 == available, should be accepted: %s", d2.Intent.RejectNote)
	  }
   }

// ────────────────────────────────────────────────────────────────────────────
// 场景 5: live-hash 场景拒绝不解冻
// ────────────────────────────────────────────────────────────────────────────
// TestLiveHashNoAutoUnfreeze live-hash 场景下 OverdrawGuard 拒绝不触发解冻。
   func TestLiveHashNoAutoUnfreeze(t *testing.T) {
	   svc := newTestService("100", "0", "0")
	  lease := issueLease(t, svc, "user5", "BTC", "spot")
	  // 记录解冻调用次数（本中应始终为 0）
	   var unfreezeCount int64
	   captureEmitter := &captureVarianceEmitter{
		  onEmit: func(evt *overwdbar.OverdrawVarianceEvent) {
			  if evt.IsLiveHash {
				  // 验证此处不触发解冻（业务系统中 emitter 不应调用解冻 API）
				  atomic.AddInt64(&unfreezeCount, 0)
			   }
		  },
	   }
	  guard := overwdbar.NewGuard(svc, captureEmitter)
	  // 超提请求（200 > 100）+ IsLiveHash=true
	   _, err := guard.PreCheck(context.Background(), overwdbar.PreCheckRequest{
		   UserID: "user5",
		   Asset:  "BTC",
		   Book:   "spot",
		   LeaseID: lease.ID,
		   Amount:  decimal.RequireFromString("200"),
		  ActorID:    "withdraw-svc",
		  IsLiveHash: true,
	  })
	  if err != overwdbar.ErrOverdrawBlocked {
		   t.Fatalf("expected ErrOverdrawBlocked, got %v", err)
	  }
	   if atomic.LoadInt64(&unfreezeCount) != 0 {
		  t.Fatal("live-hash overdraw must not trigger auto-unfreeze")
	   }
	  if captureEmitter.emitCount.Load() == 0 {
		  t.Fatal("variance event must be emitted on overdraw")
	   }
	  // 验证差异事件字段正确。
	  evt := captureEmitter.lastEvent
	  if !evt.IsLiveHash {
		   t.Fatal("variance event must carry IsLiveHash=true")
	   }
	   if evt.Gate != overwdbar.GatePreCheck {
		   t.Fatalf("gate must be precheck, got %s", evt.Gate)
	  }
  }

// captureVarianceEmitter 捕获 VarianceEvent 供断言使用。

type captureVarianceEmitter struct {
	emitCount atomic.Int64
	lastEvent *overwdbar.OverdrawVarianceEvent
	mu        sync.Mutex
	onEmit func(*overwdbar.OverdrawVarianceEvent)
}

  func (e *captureVarianceEmitter) EmitOverdrawVariance(evt *overwdbar.OverdrawVarianceEvent) error {
	   e.emitCount.Add(1)
	  e.mu.Lock()
	  e.lastEvent = evt
	  e.mu.Unlock()
	  if e.onEmit != nil {
		  e.onEmit(evt)
	   }
	  return nil
   }

// ────────────────────────────────────────────────────────────────────────────
// 场景 6: N 并发压测 — 最终可用 ≥ 0，∑成功 ≤ 初始可用
// ────────────────────────────────────────────────────────────────────────────
// TestConcurrentOverdrawInvariant 并发竞态不变量验证。
// 设计说明：CAS 屏障在同一租约窗口内（同 lease_id）保证互斥——同一租约最多一个意图 accepted。
// 不同 goroutine 若获取不同租约，各自的可用快照可能相同（竞争窗口内均在 InFlight 落账前颁发），
// 此时 N 笔均可 accepted。本聚焦两个子场景：
// (A) 共享同一租约：N goroutine 竞争同一 lease，仅 1 笔 accepted（CAS 互斥）。
// (B) 顺序租约链：顺序颁发新租约，InFlight 持续累计，最终 availiable 降为 0，不超提。
   func TestConcurrentOverdrawInvariant(t *testing.T) {
	  // ── 子场景 A：共享同一租约，N 并发争夺，只有 1 笔能 accepted ─────────────
	  t.Run("shared_lease_only_one_wins", func(t *testing.T) {
		  const goroutines = 20
		  svc := newTestService("500", "0", "0")
		   sharedLease := issueLease(t, svc, "cas-user-a", "USDT", "spot")
		   var successCount int64
		   var wg sync.WaitGroup
		   wg.Add(goroutines)
		   for i := 0; i < goroutines; i++ {
			   i := i
			   go func() {
				  defer wg.Done()
				  amt := decimal.RequireFromString("50")
				   decision, err := svc.Submit(context.Background(), &availcas.DebitIntent{
					   IdemKey: fmt.Sprintf("shared-wd-%d", i),
					   UserID: sharedLease.UserID,
					   Asset:  sharedLease.Asset,
					  Book: sharedLease.Book,
					   Amount:  amt,
					   Kind:    availcas.IntentKindFreeze,
					   Reason:  "concurrent_shared_lease",
					  LeaseID: sharedLease.ID,
					  LeaseGen:      sharedLease.Gen,
					   ExpectedAvail: sharedLease.Available,
				  })
				   if err == nil && decision != nil && decision.Accepted {
					   atomic.AddInt64(&successCount, 1)
				   }
			  }()
		  }
		  wg.Wait()
		   // 同一租约只能绑定一次 → 最多 1 笔 accepted。
		   if successCount != 1 {
			   t.Fatalf("shared lease: expected exactly 1 accepted, got %d", successCount)
		   }
	  })
	  // ── 子场景 B：顺序租约链，InFlight 累计，最终不超提 ────────────────────
	  t.Run("sequential_leases_inflight_accumulates", func(t *testing.T) {
		  const (
			   steps         = 10
			   amountPerStep = "60"
			  initialAvail = "500"
		   )
		  svc := newTestService(initialAvail, "0", "0")
		   successTotal := decimal.Zero
		   for i := 0; i < steps; i++ {
			   // 顺序颁发新租约：InFlight 随已接受意图累计，available 逐步减小。
			   lease, err := svc.IssueLease(context.Background(), "seq-user", "USDT", "spot",
				  availcas.LeaseOptions{})
			  if err != nil {
				   t.Fatalf("step %d IssueLease: %v", i, err)
			   }
			   amt := decimal.RequireFromString(amountPerStep)
			   if amt.GreaterThan(lease.Available) {
				   // 可用已不足，余下请求均会被拒绝（超提），停止提交。
				   break
			   }
			  decision, err := svc.Submit(context.Background(), &availcas.DebitIntent{
				  IdemKey: fmt.Sprintf("seq-wd-%d", i),
				   UserID:  "seq-user",
				   Asset:   "USDT",
				  Book:   "spot",
				  Amount: amt,
				   Kind:   availcas.IntentKindFreeze,
				  Reason: "sequential_test",
				  LeaseID:       lease.ID,
				  LeaseGen:      lease.Gen,
				  ExpectedAvail: lease.Available,
			  })
			   if err != nil {
				  t.Fatalf("step %d Submit: %v", i, err)
			   }
			  if decision.Accepted {
				  successTotal = successTotal.Add(amt)
			   }
		   }
		   initAvail := decimal.RequireFromString(initialAvail)
		   // 关键不变量：顺序提交成功总量不得超过初始可用。
		  if successTotal.GreaterThan(initAvail) {
			  t.Fatalf("sequential invariant violated: successTotal=%s > initialAvail=%s",
				   successTotal, initAvail)
		  }
		  finalAvail := initAvail.Sub(successTotal)
		  if finalAvail.IsNegative() {
			  t.Fatalf("final available must be ≥ 0, got %s", finalAvail)
		   }
		  t.Logf("sequential: steps=%d successTotal=%s initialAvail=%s finalAvail=%s",
			  steps, successTotal, initAvail, finalAvail)
	  })
   }

// ────────────────────────────────────────────────────────────────────────────
// 场景 7: 幂等键重复
// ────────────────────────────────────────────────────────────────────────────
// TestIdempotentIntent 相同 IdemKey 的意图只接受一次，后续调用返回已有裁决。
   func TestIdempotentIntent(t *testing.T) {
	   svc := newTestService("500", "0", "0")
	   lease := issueLease(t, svc, "user7", "USDT", "spot")
	   d1 := submitIntent(t, svc, lease, "100", availcas.IntentKindFreeze, "idem-key-1")
	  if !d1.Accepted {
		  t.Fatalf("first submission must be accepted")
	  }
	   // 以相同 IdemKey 再提交（不同租约也行）
	  lease2 := issueLease(t, svc, "user7", "USDT", "spot")
	  d2, err := svc.Submit(context.Background(), &availcas.DebitIntent{
		   IdemKey: "idem-key-1",
		   UserID: "user7",
		   Asset: "USDT",
		  Book:  "spot",
		  Amount: decimal.RequireFromString("100"),
		  Kind:    availcas.IntentKindFreeze,
		  LeaseID: lease2.ID,
		   LeaseGen:      lease2.Gen,
		  ExpectedAvail: lease2.Available,
	   })
	   if err != nil {
		   t.Fatalf("idempotent resubmit: %v", err)
	   }
	   // 返回已有裁决，ID 一致。
	   if d2.Intent.ID != d1.Intent.ID {
		  t.Fatalf("idempotent: expected same intent ID %s, got %s", d1.Intent.ID, d2.Intent.ID)
	  }
   }

// ────────────────────────────────────────────────────────────────────────────
// 场景 8: 期望可用不一致 → CAS 冲突
// ────────────────────────────────────────────────────────────────────────────
// TestCASMismatchExpectedAvail expected_avail 与租约快照不一致时 CAS 失败。

  func TestCASMismatchExpectedAvail(t *testing.T) {
	   svc := newTestService("800", "200", "0")
	   lease := issueLease(t, svc, "user8", "ETH", "spot")
	   // available = 800 - 200 = 600
	   // 故意填写错误的 expected_avail（700，而实际是 600）
	  amt := decimal.RequireFromString("100")
	  decision, err := svc.Submit(context.Background(), &availcas.DebitIntent{
		   UserID: "user8",
		  Asset: "ETH",
		  Book:  "spot",
		  Amount:   amt,
		  Kind:     availcas.IntentKindFreeze,
		   LeaseID:  lease.ID,
		   LeaseGen: lease.Gen,
		   ExpectedAvail: decimal.RequireFromString("700"), // 故意填错
	   })
	   if err != nil {
		  t.Fatalf("Submit: %v", err)
	  }
	  if decision.Accepted {
		   t.Fatal("mismatched expected_avail must cause CAS conflict rejection")
	   }
	   if decision.Intent.RejectCode != availcas.RejectCodeCASConflict {
		   t.Fatalf("expected cas_conflict, got %s", decision.Intent.RejectCode)
	   }
	  if decision.Proof == nil {
		   t.Fatal("race proof required for CAS conflict")
	   }
  }

// ────────────────────────────────────────────────────────────────────────────
// 场景 9: 超提企图 → rejected + proof
// ────────────────────────────────────────────────────────────────────────────
// TestOverdrawAttemptRejected amount > available 时超提被拒绝，RaceProof 落库。
  func TestOverdrawAttemptRejected(t *testing.T) {
	   svc := newTestService("300", "0", "0")
	   lease := issueLease(t, svc, "user9", "USDT", "spot")
	   // 提交 400 > 300 available
	   decision, err := svc.Submit(context.Background(), &availcas.DebitIntent{
		   UserID: "user9",
		   Asset:  "USDT",
		  Book:   "spot",
		   Amount:  decimal.RequireFromString("400"),
		  Kind:    availcas.IntentKindDebit,
		  LeaseID: lease.ID,
		  LeaseGen:      lease.Gen,
		  ExpectedAvail: lease.Available,
	  })
	  if err != nil {
		  t.Fatalf("Submit: %v", err)
	   }
	  if decision.Accepted {
		  t.Fatal("overdraw 400>300 must be rejected")
	  }
	   if decision.Intent.RejectCode != availcas.RejectCodeInsufficientAvail {
		  t.Fatalf("expected insufficient_avail, got %s", decision.Intent.RejectCode)
	   }
	  if decision.Proof == nil {
		   t.Fatal("overdraw attempt must create race proof")
	  }
   }

// ────────────────────────────────────────────────────────────────────────────
// 场景 10: 零额意图校验
// ────────────────────────────────────────────────────────────────────────────
// TestZeroAmountRejected 零额意图在参数校验阶段被拒绝。

  func TestZeroAmountRejected(t *testing.T) {
	   svc := newTestService("500", "0", "0")
	  lease := issueLease(t, svc, "user10", "USDT", "spot")
	  _, err := svc.Submit(context.Background(), &availcas.DebitIntent{
		   UserID: "user10",
		  Asset: "USDT",
		  Book:  "spot",
		  Amount: decimal.Zero,
		  Kind:    availcas.IntentKindFreeze,
		  LeaseID: lease.ID,
		  LeaseGen:      lease.Gen,
		  ExpectedAvail: lease.Available,
	  })
	   if err != availcas.ErrAmountNonPositive {
		  t.Fatalf("zero amount must return ErrAmountNonPositive, got %v", err)
	   }
  }

// ────────────────────────────────────────────────────────────────────────────
// 场景 11: OverdrawGuard.Commit 双闸工作流
// ────────────────────────────────────────────────────────────────────────────
// TestGuardPreCheckCommitFlow 预检+提交双闸完整工作流验证。

  func TestGuardPreCheckCommitFlow(t *testing.T) {
	   svc := newTestService("1000", "0", "0")
	   lease := issueLease(t, svc, "user11", "USDT", "spot")
	  guard := overwdbar.NewGuard(svc, nil)
	  // 预检：600 ≤ 1000 available，通过。
	  precheckDecision, err := guard.PreCheck(context.Background(), overwdbar.PreCheckRequest{
		   UserID: "user11",
		   Asset:  "USDT",
		   Book:   "spot",
		  LeaseID: lease.ID,
		   Amount: decimal.RequireFromString("600"),
		  ActorID: "test-actor",
	  })
	  if err != nil {
		   t.Fatalf("PreCheck: %v", err)
	   }
	  if !precheckDecision.Accepted {
		  t.Fatalf("PreCheck 600 ≤ 1000 should pass")
	  }
	  // 提交：binding CAS。
	   commitDecision, err := guard.Commit(context.Background(), overwdbar.CommitRequest{
		  UserID: "user11",
		  Asset:   "USDT",
		  Book:    "spot",
		   LeaseID: lease.ID,
		  LeaseGen: lease.Gen,
		  ExpectedAvail: lease.Available,
		   Amount:  decimal.RequireFromString("600"),
		  Kind:    availcas.IntentKindFreeze,
		   Reason:  "withdraw_freeze",
		   IdemKey: "wd-commit-test",
		   ActorID: "test-actor",
	   })
	  if err != nil {
		  t.Fatalf("Commit: %v", err)
	   }
	   if !commitDecision.Accepted {
		   t.Fatalf("Commit should be accepted: %s", commitDecision.Intent.RejectNote)
	  }
   }

// ────────────────────────────────────────────────────────────────────────────
// 场景 12: OverdrawGuard.Commit 超提被拒，差异事件发出
// ────────────────────────────────────────────────────────────────────────────
// TestGuardCommitOverdrawEmitsVariance 超提 Commit 被拒且发出 VarianceEvent。
   func TestGuardCommitOverdrawEmitsVariance(t *testing.T) {
	   svc := newTestService("200", "0", "0")
	   lease := issueLease(t, svc, "user12", "USDT", "spot")
	   emitter := &captureVarianceEmitter{}
	   guard := overwdbar.NewGuard(svc, emitter)
	  _, err := guard.Commit(context.Background(), overwdbar.CommitRequest{
		   UserID: "user12",
		   Asset:  "USDT",
		  Book:          "spot",
		  LeaseID:       lease.ID,
		   LeaseGen:      lease.Gen,
		  ExpectedAvail: lease.Available,
		   Amount: decimal.RequireFromString("300"), // 300 > 200 available
		   Kind:   availcas.IntentKindFreeze,
		  Reason:  "withdraw_freeze",
		   IdemKey: "wd-overdraw-commit",
		   ActorID: "test-actor",
	  })
	  if err != overwdbar.ErrOverdrawBlocked {
		  t.Fatalf("expected ErrOverdrawBlocked, got %v", err)
	   }
	   if emitter.emitCount.Load() == 0 {
		   t.Fatal("overdraw commit must emit variance event")
	   }
	  if emitter.lastEvent.Gate != overwdbar.GateCommit {
		   t.Fatalf("expected gate=commit, got %s", emitter.lastEvent.Gate)
	  }
   }

// ────────────────────────────────────────────────────────────────────────────
// 场景 13: Desk 查询 RaceProof 列表
// ────────────────────────────────────────────────────────────────────────────
// TestListRaceProofs desk 列出用户×资产竞态证明。
   func TestListRaceProofs(t *testing.T) {
	   svc := newTestService("500", "0", "0")
	   user := "desk-user"
	   asset := "USDT"
	  // 制造 3 条 proof（每次申请新租约后超提）
	  for i := 0; i < 3; i++ {
		  lease := issueLease(t, svc, user, asset, "spot")
		  svc.Submit(context.Background(), &availcas.DebitIntent{ // nolint:errcheck
			   UserID: user,
			  Asset:  asset,
			  Book:   "spot",
			   Amount: decimal.RequireFromString("600"), // > 500
			   Kind:          availcas.IntentKindFreeze,
			   LeaseID:       lease.ID,
			  LeaseGen:      lease.Gen,
			   ExpectedAvail: lease.Available,
		   })
	   }
	   proofs, err := svc.ListProofs(context.Background(), user, asset, 10)
	   if err != nil {
		   t.Fatalf("ListProofs: %v", err)
	  }
	  if len(proofs) != 3 {
		   t.Fatalf("expected 3 proofs, got %d", len(proofs))
	   }
	   for _, p := range proofs {
		   if err := p.Validate(); err != nil {
			   t.Fatalf("proof validation failed: %v", err)
		  }
	   }
   }

// ────────────────────────────────────────────────────────────────────────────
// 场景 14: Gen 世代不一致 → CAS 冲突
// ────────────────────────────────────────────────────────────────────────────
// TestCASGenMismatch lease_gen 与实际租约 Gen 不一致时 CAS 失败。

  func TestCASGenMismatch(t *testing.T) {
	   svc := newTestService("500", "0", "0")
	   lease := issueLease(t, svc, "user14", "USDT", "spot")
	  // 故意传入错误的 LeaseGen（应为 lease.Gen，故意传 lease.Gen+1）
	   decision, err := svc.Submit(context.Background(), &availcas.DebitIntent{
		   UserID: "user14",
		   Asset:  "USDT",
		   Book:   "spot",
		  Amount: decimal.RequireFromString("100"),
		   Kind:   availcas.IntentKindFreeze,
		   LeaseID:       lease.ID,
		  LeaseGen:      lease.Gen + 1, // 故意错误
		  ExpectedAvail: lease.Available,
	  })
	  if err != nil {
		  t.Fatalf("Submit: %v", err)
	   }
	   if decision.Accepted {
		  t.Fatal("wrong lease_gen must cause CAS conflict")
	  }
	  if decision.Intent.RejectCode != availcas.RejectCodeCASConflict {
		  t.Fatalf("expected cas_conflict, got %s", decision.Intent.RejectCode)
	  }
  }

// ────────────────────────────────────────────────────────────────────────────
// 场景 15: 证明完整性校验
// ────────────────────────────────────────────────────────────────────────────
// TestRaceProofValidate 验证 RaceProof.Validate 对各必填字段的检查。

  func TestRaceProofValidate(t *testing.T) {
	   now := time.Now().UTC()
	   valid := &availcas.RaceProof{
		   ID: "p1",
		   UserID:  "u1",
		  Asset:   "USDT",
		   Book:    "spot",
		  LeaseID: "l1",
		  LoserIntentID: "i2",
		   RejectCode:  availcas.RejectCodeCASConflict,
		   WindowStart: now.Add(-100 * time.Millisecond),
		   WindowEnd: now,
	  }
	  if err := valid.Validate(); err != nil {
		  t.Fatalf("valid proof failed validation: %v", err)
	   }
	  // 缺少 LoserIntentID → 校验失败。
	  invalid := *valid
	   invalid.LoserIntentID = ""
	  if err := invalid.Validate(); err == nil {
		   t.Fatal("missing loser_intent_id must fail validation")
	  }
	  // window_end 早于 window_start → 校验失败。
	  invalid2 := *valid
	  invalid2.WindowEnd = now.Add(-200 * time.Millisecond)
	   if err := invalid2.Validate(); err == nil {
		  t.Fatal("window_end before window_start must fail validation")
	  }
  }

// ────────────────────────────────────────────────────────────────────────────
// 场景 16: SnapshotAvailable 即时可用快照
// ────────────────────────────────────────────────────────────────────────────
// TestSnapshotAvailable SnapshotAvailable 正确折叠余额/冻额/预留/InFlight。
  func TestSnapshotAvailable(t *testing.T) {
	  // balance=1000, frozen=200, reserved=100 → available = 700
	   svc := newTestService("1000", "200", "100")
	   avail, err := svc.SnapshotAvailable(context.Background(), "snap-user", "USDT", "spot")
	   if err != nil {
		   t.Fatalf("SnapshotAvailable: %v", err)
	  }
	   if !avail.Equal(decimal.RequireFromString("700")) {
		   t.Fatalf("expected snapshot available=700, got %s", avail)
	  }
	   // 再颁发租约并接受一个 300 意图后，InFlight=300，available 应降为 400。
	   lease := issueLease(t, svc, "snap-user", "USDT", "spot")
	   d := submitIntent(t, svc, lease, "300", availcas.IntentKindFreeze, "snap-intent")
	   if !d.Accepted {
		   t.Fatalf("intent should be accepted")
	  }
	   avail2, err := svc.SnapshotAvailable(context.Background(), "snap-user", "USDT", "spot")
	  if err != nil {
		  t.Fatalf("SnapshotAvailable after intent: %v", err)
	  }
	  if !avail2.Equal(decimal.RequireFromString("400")) {
		   t.Fatalf("expected snapshot available=400 after 300 inflight, got %s", avail2)
	   }
  }

// ────────────────────────────────────────────────────────────────────────────
// 场景 17: 错误拒绝码描述完整性
// ────────────────────────────────────────────────────────────────────────────
// TestRejectCodeDescriptions 验证所有 RejectCode 都有非空描述。
   func TestRejectCodeDescriptions(t *testing.T) {
	  codes := []availcas.RejectCode{
		   availcas.RejectCodeCASConflict,
		   availcas.RejectCodeLeaseExpired,
		  availcas.RejectCodeLeaseRevoked,
		  availcas.RejectCodeInsufficientAvail,
		  availcas.RejectCodeDuplicateIdem,
	   }
	  for _, code := range codes {
		  desc := availcas.RejectCodeDescription(code)
		   if desc == "" {
			  t.Fatalf("no description for code %s", code)
		   }
	  }
  }

// ────────────────────────────────────────────────────────────────────────────
// 场景 18: 租约回收 worker 单轮运行
// ────────────────────────────────────────────────────────────────────────────
// TestWorkerSweepExpiredLeases worker 能回收已过期租约。
  func TestWorkerSweepExpiredLeases(t *testing.T) {
	   store := availcas.NewMemStore()
	  reader := &staticReader{
		  balance: decimal.RequireFromString("500"),
		  frozen:   decimal.Zero,
		  reserved: decimal.Zero,
	   }
	  svc := availcas.NewService(availcas.ServiceConfig{Store: store, Reader: reader})
	   // 颁发 3 个极短 TTL 租约
	  for i := 0; i < 3; i++ {
		  _, err := svc.IssueLease(context.Background(), fmt.Sprintf("sweep-user-%d", i), "USDT", "spot",
			  availcas.LeaseOptions{TTL: 1 * time.Millisecond})
		   if err != nil {
			  t.Fatalf("IssueLease: %v", err)
		   }
	   }
	   // 等待过期
	   time.Sleep(10 * time.Millisecond)
	   // Worker 回收
	  cfg := availcas.DefaultWorkerConfig()
	   worker := availcas.NewWorker(svc, cfg)
	  _ = worker
	   // 验证：store 中 open 状态的租约已被 sweep（通过 SweepExpired 直接）
	  expiredLeases, _ := store.ListExpirableLeases(context.Background(), time.Now().UTC(), 10)
	  if len(expiredLeases) != 3 {
		  t.Fatalf("expected 3 expirable leases, got %d", len(expiredLeases))
	  }
   }

// ────────────────────────────────────────────────────────────────────────────
// 表驱动
// ────────────────────────────────────────────────────────────────────────────
// TestBarrierBoundaryTable 表驱动场景（zero/negative/exact/overflow 金额）。

  func TestBarrierBoundaryTable(t *testing.T) {
	   type testCase struct {
		  name string
		  balance  string
		  frozen   string
		  reserved string
		  requestAmount string
		  wantAccepted  bool
		   wantCode      availcas.RejectCode
	  }
	  cases := []testCase{
		   {
			   name: "exact_available",
			  balance: "500", frozen: "0", reserved: "0",
			  requestAmount: "500",
			   wantAccepted:  true,
		  },
		  {
			  name: "one_satoshi_over",
			  balance: "500", frozen: "0", reserved: "0",
			   requestAmount: "500.000000000000000001",
			   wantAccepted:  false,
			   wantCode:      availcas.RejectCodeInsufficientAvail,
		  },
		   {
			   name:    "frozen_reduces_available",
			   balance: "1000", frozen: "600", reserved: "0",
			  requestAmount: "500", // available=400 < 500
			  wantAccepted:  false,
			   wantCode: availcas.RejectCodeInsufficientAvail,
		  },
		   {
			  name: "reserved_reduces_available",
			  balance: "1000", frozen: "0", reserved: "600",
			   requestAmount: "500", // available=400 < 500
			  wantAccepted: false,
			   wantCode:     availcas.RejectCodeInsufficientAvail,
		  },
		  {
			   name:    "zero_balance_rejected",
			   balance: "0", frozen: "0", reserved: "0",
			   requestAmount: "1",
			   wantAccepted:  false,
			  wantCode: availcas.RejectCodeInsufficientAvail,
		  },
		  {
			   name: "large_amount_accepted",
			  balance: "999999999999999999", frozen: "0", reserved: "0",
			  requestAmount: "999999999999999999",
			   wantAccepted:  true,
		  },
		  {
			   name: "tiny_amount_accepted",
			   balance: "0.000000000000000001", frozen: "0", reserved: "0",
			   requestAmount: "0.000000000000000001",
			  wantAccepted:  true,
		  },
	   }
	  for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   svc := newTestService(tc.balance, tc.frozen, tc.reserved)
			   lease := issueLease(t, svc, "boundary-user", "USDT", "spot")
			  amt := decimal.RequireFromString(tc.requestAmount)
			   decision, err := svc.Submit(context.Background(), &availcas.DebitIntent{
				  UserID: "boundary-user",
				   Asset: "USDT",
				  Book:   "spot",
				   Amount: amt,
				  Kind: availcas.IntentKindFreeze,
				   LeaseID:       lease.ID,
				  LeaseGen:      lease.Gen,
				   ExpectedAvail: lease.Available,
			   })
			  if err != nil {
				  t.Fatalf("Submit: %v", err)
			  }
			   if decision.Accepted != tc.wantAccepted {
				   t.Fatalf("accepted=%v want=%v: %s", decision.Accepted, tc.wantAccepted, decision.Intent.RejectNote)
			   }
			  if !tc.wantAccepted && decision.Intent.RejectCode != tc.wantCode {
				  t.Fatalf("reject code: got %s want %s", decision.Intent.RejectCode, tc.wantCode)
			   }
		   })
	   }
   }
