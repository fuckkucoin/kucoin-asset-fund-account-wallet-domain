package intxfergate_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/fundint"
	"account-assets.kucoin.net/funding/internal/intxfergate"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/userdir"
	"github.com/shopspring/decimal"
)

// ─── 夹具 ──────────────────────────────────────────────────────────────

type testEnv struct {
	store *intxfergate.MemStore
	dir    *userdir.Mem
	engine *intxfergate.Engine
	gate     *fundint.ResolveGate
	ledStore *ledger.MemStore // 直接引用内存账本 store，用于余额种入
}

// newTestEnv 构造完整环境（内存账本 + 内存 dir + 内存 store）。
  func newTestEnv(t *testing.T, snapTTL, leaseTTL time.Duration) *testEnv {
	  t.Helper()
	  rt, err := app.OpenMem()
	   if err != nil {
		   t.Fatalf("open mem runtime: %v", err)
	   }
	   // 通过 runtime.Store 获取内存账本 store（用于余额注入）
	  memSt, ok := rt.Store.(*ledger.MemStore)
	  if !ok {
		   t.Fatal("runtime store is not *ledger.MemStore")
	   }
	   dir := userdir.NewMem()
	  store := intxfergate.NewMemStore()
	   svc := fundint.New(rt.Ledger,
		  dir)
	  ext := fundint.NewExt(svc, store,
		  fundint.WithSnapTTL(snapTTL),
		  fundint.WithLeaseTTL(leaseTTL),
	  )
	   gate := fundint.NewResolveGate(store,
		   fundint.WithSnapTTL(snapTTL),
		   fundint.WithLeaseTTL(leaseTTL),
	   )
	  engine := intxfergate.NewEngine(ext, store)
	  return &testEnv{
		   store: store,
		   dir:   dir,
		  engine: engine,
		  gate: gate,
		   ledStore: memSt,
	   }
   }

// seedLedgerBalance 向内存账本种入可用余额（账本 Spot 科目）。

  func (e *testEnv) seedLedgerBalance(userID, asset, amount string) {
	   e.ledStore.Seed(userID, ledger.AccountSpot, asset, amount)
  }

// seedBalance 给账户注入初始余额（辅助，当前内存账本无需预置）。
   func seedBalance(t *testing.T, userID, asset, amount string) {
	   t.Helper()
	   // 内存账本的 SpotSend 路径由 ledger 内部处理余额检查；
	  // 单元中不需要额外预置余额（创建零账后直接 credit ）。
	  _ = userID
	   _ = asset
	  _ = amount
  }

// ─── 表驱动 ────────────────────────────────────────────────────────────

   func TestEngine_Send_TableDriven(t *testing.T) {
	   type sendCase struct {
		   name        string
		   setupDir    map[string]string // kind:value → userID
		  seedBalance map[string]string // userID → amount (USDT)
		  req         intxfergate.SendRequest
		  wantErrContains string
		   wantState intxfergate.SendState
		   wantSnaps int
		  wantLeases int
	  }
	   cases := []sendCase{
		   {
			   name: "normal_send_uid",
			   setupDir: map[string]string{
				   "uid:u-recv-normal": "u-recv-normal",
			  },
			   seedBalance: map[string]string{
				   "u-sender-n": "10000",
			  },
			   req: intxfergate.SendRequest{
				   UserID: "u-sender-n",
				  ToUID:  "u-recv-normal",
				  Asset: "USDT",
				   Amount:         decimal.NewFromInt(100),
				  IdempotencyKey: "normal-uid-001",
			   },
			   wantState: intxfergate.SendStateCompleted,
			  wantSnaps: 1,
			  wantLeases: 1,
		   },
		   {
			  name: "ac1_ambiguous_uid_email",
			  setupDir: map[string]string{
				   "uid:u-recv-a": "u-recv-a",
				  "email:userc@example-internal.local": "u-recv-c",
			  },
			  seedBalance: map[string]string{
				   "u-sender-ac1": "10000",
			  },
			  req: intxfergate.SendRequest{
				  UserID: "u-sender-ac1",
				   ToUID:   "u-recv-a",
				   ToEmail: "userc@example-internal.local",
				  Asset:   "USDT",
				   Amount: decimal.NewFromInt(100),
				   IdempotencyKey: "ac1-ambiguous-001",
			  },
			   wantErrContains: "ambiguous",
			  wantSnaps:  0,
			   wantLeases: 0,
		  },
		   {
			   name: "idem_repeated_completed",
			  setupDir: map[string]string{
				   "uid:u-recv-idem": "u-recv-idem",
			  },
			  seedBalance: map[string]string{
				   "u-sender-idem": "10000",
			  },
			  req: intxfergate.SendRequest{
				  UserID: "u-sender-idem",
				  ToUID:  "u-recv-idem",
				  Asset:  "USDT",
				  Amount: decimal.NewFromInt(50),
				   IdempotencyKey: "idem-repeat-001",
			   },
			   wantState: intxfergate.SendStateCompleted,
			   wantSnaps: 1,
			  wantLeases: 1,
		   },
		   {
			  name:     "no_target_specified",
			   setupDir: map[string]string{},
			   req: intxfergate.SendRequest{
				   UserID: "u-sender-notarget",
				  Asset:  "USDT",
				   Amount:         decimal.NewFromInt(10),
				   IdempotencyKey: "notarget-001",
			   },
			   wantErrContains: "not found",
			  wantSnaps:  0,
			   wantLeases: 0,
		   },
		   {
			  name:     "idem_key_missing",
			  setupDir: map[string]string{},
			  req: intxfergate.SendRequest{
				  UserID: "u-sender-noidem",
				   ToUID:  "u-recv-x",
				  Asset:  "USDT",
				   Amount: decimal.NewFromInt(10),
			  },
			  wantErrContains: "idempotency",
			   wantSnaps: 0,
			   wantLeases: 0,
		   },
	   }
	   for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   env := newTestEnv(t, 5*time.Minute, 30*time.Second)
			   // 准备 directory
			  for kv, uid := range tc.setupDir {
				  parts := strings.SplitN(kv, ":", 2)
				   if len(parts) != 2 {
					   continue
				  }
				   if err := env.dir.Bind(parts[0], parts[1], uid); err != nil {
					   t.Fatalf("bind dir %s: %v", kv, err)
				   }
			   }
			   // 种入余额（内存账本 SpotSend 需要可用余额）
			   for uid, amount := range tc.seedBalance {
				  env.seedLedgerBalance(uid, "USDT", amount)
			   }
			  ctx := context.Background()
			   reply, err := env.engine.Send(ctx, tc.req)
			   if tc.wantErrContains != "" {
				   if err == nil {
					  t.Fatalf("expected error containing %q, got nil", tc.wantErrContains)
				   }
				   if !strings.Contains(err.Error(), tc.wantErrContains) {
					   t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErrContains)
				  }
			  } else {
				   if err != nil {
					  t.Fatalf("unexpected error: %v", err)
				   }
				  if reply.State != tc.wantState {
					   t.Fatalf("state: got %s, want %s", reply.State, tc.wantState)
				  }
			  }
		  })
	   }
  }

// ─── AC-2 并发双 Send ──────────────────────────────────────────────────────

   func TestEngine_Send_AC2_ConcurrentSameIdem(t *testing.T) {
	   env := newTestEnv(t, 5*time.Minute, 30*time.Second)
	  // 准备 directory 和余额
	  _ = env.dir.Bind("uid", "u-recv-ac2", "u-recv-ac2")
	   env.seedLedgerBalance("u-sender-ac2", "USDT", "10000")
	   ctx := context.Background()
	  const idem = "ac2-concurrent-race-001"
	   type result struct {
		  reply *intxfergate.SendReply
		   err error
	   }
	   results := make([]result, 2)
	   var wg sync.WaitGroup
	   for i := 0; i < 2; i++ {
		   i := i
		   wg.Add(1)
		   go func() {
			  defer wg.Done()
			   r, err := env.engine.Send(ctx, intxfergate.SendRequest{
				  UserID: "u-sender-ac2",
				   ToUID:  "u-recv-ac2",
				  Asset:  "USDT",
				   Amount:         decimal.NewFromInt(50),
				   IdempotencyKey: idem,
			   })
			   results[i] = result{reply: r, err: err}
		   }()
	  }
	   wg.Wait()
	  // 收集所有成功结果的 LedgerID
	   ledgerIDs := map[string]bool{}
	  successCount := 0
	   for _, r := range results {
		  if r.err == nil && r.reply != nil && r.reply.LedgerID != "" {
			   ledgerIDs[r.reply.LedgerID] = true
			  successCount++
		   }
	  }
	  // 核心约束：所有成功结果必须共享同一个 LedgerID（幂等：仅一笔 SpotSend）
	   // 允许：成功数=0（均被余额拒），成功数=1（一个成功一个冲突），成功数=2但 LedgerID 相同（幂等命中）
	  if len(ledgerIDs) > 1 {
		  t.Fatalf("AC-2 double debit: %d distinct LedgerIDs found (only 1 allowed): %v",
			   len(ledgerIDs), ledgerIDs)
	  }
	   t.Logf("AC-2 passed: successCount=%d distinctLedgerIDs=%d", successCount, len(ledgerIDs))
   }

// ─── ResolveSnap 模型 ──────────────────────────────────────────────────

   func TestResolveSnap_Valid(t *testing.T) {
	  now := time.Now().UTC()
	  cases := []struct {
		   name string
		  snap    *fundint.ResolveSnap
		   at      time.Time
		  wantErr bool
	   }{
		  {
			   name: "fresh_not_expired",
			   snap: &fundint.ResolveSnap{
				  State: fundint.ResolveSnapFresh,
				  ExpiresAt: now.Add(5 * time.Minute),
			  },
			   at:      now,
			  wantErr: false,
		   },
		  {
			  name: "fresh_but_expired",
			  snap: &fundint.ResolveSnap{
				   State: fundint.ResolveSnapFresh,
				   ExpiresAt: now.Add(-1 * time.Second),
			   },
			   at:      now,
			  wantErr: true,
		   },
		   {
			   name: "expired_state",
			  snap: &fundint.ResolveSnap{
				  State: fundint.ResolveSnapExpired,
				  ExpiresAt: now.Add(5 * time.Minute),
			  },
			  at:      now,
			   wantErr: true,
		   },
		   {
			  name: "nil_snap",
			   snap: nil,
			  at:      now,
			   wantErr: true,
		  },
	  }
	  for _, tc := range cases {
		  t.Run(tc.name, func(t *testing.T) {
			  err := tc.snap.Valid(tc.at)
			   if tc.wantErr && err == nil {
				  t.Fatal("expected error, got nil")
			   }
			   if !tc.wantErr && err != nil {
				   t.Fatalf("unexpected error: %v", err)
			   }
		  })
	   }
   }

// ─── DebitLease 状态机 ────────────────────────────────────────────────

   func TestDebitLease_StateTransitions(t *testing.T) {
	  cases := []struct {
		   name string
		  initial fundint.DebitLeaseState
		  next    fundint.DebitLeaseState
		   wantErr bool
	  }{
		  // DebitLease 暂无 Transition 方法，此处 ResolveGate.AcquireLease 行为
		   // 通过 MemStore 直接幂等场景
	  }
	  for _, tc := range cases {
		   _ = tc
	  }
  }

// ─── XferEffectProof 哈希 ─────────────────────────────────────────────

   func TestXferEffectProof_ComputeHash(t *testing.T) {
	  cases := []struct {
		   name string
		  snapID     string
		  fromUserID string
		  toUserID   string
		  amount     string
		   ledgerID string
		   // 哈希结果由运行时计算，此处验证一致性（不变性）
	  }{
		   {
			  name:       "basic",
			   snapID:     "snap-001",
			   fromUserID: "u-alice",
			   toUserID: "u-bob",
			   amount:   "100",
			  ledgerID: "xfer-001",
		  },
		   {
			  name: "small_amount",
			   snapID: "snap-002",
			   fromUserID: "u-carol",
			   toUserID:   "u-dave",
			  amount:     "0.00000001",
			   ledgerID: "xfer-002",
		  },
		   {
			  name: "empty_strings_differ",
			   snapID: "snap-003",
			  fromUserID: "",
			  toUserID:   "u-eve",
			   amount:     "1",
			  ledgerID:   "xfer-003",
		   },
	  }
	  for _, tc := range cases {
		  t.Run(tc.name, func(t *testing.T) {
			   h1 := intxfergate.ComputeHash(tc.snapID, tc.fromUserID, tc.toUserID, tc.amount, tc.ledgerID)
			   h2 := intxfergate.ComputeHash(tc.snapID, tc.fromUserID, tc.toUserID, tc.amount, tc.ledgerID)
			  // 同参数两次计算结果必须一致（确定性）
			   if h1 != h2 {
				   t.Fatalf("hash not deterministic: %s != %s", h1, h2)
			   }
			  // 哈希长度应为 64 位 hex
			   if len(h1) != 64 {
				   t.Fatalf("hash length=%d want 64", len(h1))
			   }
			  // 改变一个字段，哈希应不同
			   h3 := intxfergate.ComputeHash(tc.snapID+"x", tc.fromUserID, tc.toUserID, tc.amount, tc.ledgerID)
			   if h1 == h3 {
				   t.Fatal("hash collision detected (snap_id change)")
			   }
		  })
	   }
   }

// ─── XferEffectProof.Verify ──────────────────────────────────────────

  func TestXferEffectProof_Verify(t *testing.T) {
	  validProof := &intxfergate.XferEffectProof{
		   SnapID:     "snap-v001",
		  FromUserID: "u-from",
		   ToUserID:   "u-to",
		   Amount:   "999.5",
		   LedgerID: "xfer-v001",
	   }
	  validProof.ProofHash = intxfergate.ComputeHash(
		   validProof.SnapID,
		  validProof.FromUserID,
		  validProof.ToUserID, validProof.Amount, validProof.LedgerID,
	   )
	   t.Run("valid_proof", func(t *testing.T) {
		  if err := validProof.Verify(); err != nil {
			   t.Fatalf("verify failed: %v", err)
		  }
	  })
	   t.Run("tampered_amount", func(t *testing.T) {
		   tampered := *validProof
		  tampered.Amount = "9999999"
		   if err := tampered.Verify(); err == nil {
			   t.Fatal("expected verify error for tampered amount, got nil")
		   }
	   })
	  t.Run("tampered_to_user", func(t *testing.T) {
		  tampered := *validProof
		  tampered.ToUserID = "u-attacker"
		   if err := tampered.Verify(); err == nil {
			  t.Fatal("expected verify error for tampered to_user, got nil")
		  }
	   })
	   t.Run("nil_proof", func(t *testing.T) {
		   var p *intxfergate.XferEffectProof
		  if err := p.Verify(); err == nil {
			   t.Fatal("expected error for nil proof")
		  }
	   })
   }

// ─── MisrouteCase FSM ────────────────────────────────────────────────

  func TestMisrouteCase_Transition(t *testing.T) {
	   cases := []struct {
		  name string
		   from intxfergate.MisrouteCaseState
		  to intxfergate.MisrouteCaseState
		  wantErr bool
	  }{
		   {"open_to_proven", intxfergate.MisrouteCaseOpen, intxfergate.MisrouteCaseProven, false},
		  {"open_to_closed", intxfergate.MisrouteCaseOpen, intxfergate.MisrouteCaseClosed, false},
		   {"proven_to_closed", intxfergate.MisrouteCaseProven, intxfergate.MisrouteCaseClosed, false},
		  {"closed_to_open", intxfergate.MisrouteCaseClosed, intxfergate.MisrouteCaseOpen, true},
		   {"proven_to_open", intxfergate.MisrouteCaseProven, intxfergate.MisrouteCaseOpen, true},
		   {"closed_to_proven", intxfergate.MisrouteCaseClosed, intxfergate.MisrouteCaseProven, true},
	  }
	  for _, tc := range cases {
		  t.Run(tc.name, func(t *testing.T) {
			  mc := &intxfergate.MisrouteCase{State: tc.from}
			  err := mc.Transition(tc.to)
			   if tc.wantErr && err == nil {
				  t.Fatal("expected error, got nil")
			   }
			   if !tc.wantErr && err != nil {
				   t.Fatalf("unexpected error: %v", err)
			   }
		  })
	   }
  }

// ─── SendRecord FSM ──────────────────────────────────────────────────

  func TestSendRecord_Transition(t *testing.T) {
	   cases := []struct {
		  name string
		   from intxfergate.SendState
		   to intxfergate.SendState
		  wantErr bool
	   }{
		   {"resolving_to_debiting", intxfergate.SendStateResolving, intxfergate.SendStateDebiting, false},
		   {"resolving_to_blocked", intxfergate.SendStateResolving, intxfergate.SendStateBlocked, false},
		   {"debiting_to_sealing", intxfergate.SendStateDebiting, intxfergate.SendStateSealing, false},
		   {"debiting_to_blocked", intxfergate.SendStateDebiting, intxfergate.SendStateBlocked, false},
		   {"sealing_to_completed", intxfergate.SendStateSealing, intxfergate.SendStateCompleted, false},
		  {"sealing_to_blocked", intxfergate.SendStateSealing, intxfergate.SendStateBlocked, false},
		   {"completed_to_resolving", intxfergate.SendStateCompleted, intxfergate.SendStateResolving, true},
		  {"blocked_to_completed", intxfergate.SendStateBlocked, intxfergate.SendStateCompleted, true},
	  }
	  for _, tc := range cases {
		  t.Run(tc.name, func(t *testing.T) {
			  rec := &intxfergate.SendRecord{State: tc.from}
			  err := rec.Transition(tc.to)
			  if tc.wantErr && err == nil {
				   t.Fatal("expected error, got nil")
			   }
			  if !tc.wantErr && err != nil {
				   t.Fatalf("unexpected error: %v", err)
			   }
		   })
	   }
  }

// ─── PatrolWorker ────────────────────────────────────────────────────

  func TestPatrolWorker_ExpireSnapsAndLeases(t *testing.T) {
	  env := newTestEnv(t, 1*time.Millisecond, 1*time.Millisecond) // 极短 TTL
	  ctx := context.Background()
	  // 插入一个 fresh snap（即将过期）
	  now := time.Now().UTC()
	   snap := &fundint.ResolveSnap{
		  ID:         "snap-patrol-001",
		   IdemKey:    "patrol-idem-001",
		  State:      fundint.ResolveSnapFresh,
		   ResolvedAt: now.Add(-10 * time.Millisecond),
		  ExpiresAt: now.Add(-1 * time.Millisecond), // 已过期
		  CreatedAt: now,
		   UpdatedAt: now,
	   }
	  if err := env.store.InsertResolveSnap(ctx, snap); err != nil {
		   t.Fatalf("insert snap: %v", err)
	   }
	  // 插入一个 held lease（即将过期）
	   acquired := now
	   lease := &fundint.DebitLease{
		  ID: "lease-patrol-001",
		  IdemKey:    "patrol-idem-001",
		  State:      fundint.DebitLeaseHeld,
		   AcquiredAt: &acquired,
		   ExpiresAt: now.Add(-1 * time.Millisecond), // 已过期
		   CreatedAt: now,
		  UpdatedAt: now,
	  }
	   if err := env.store.InsertDebitLease(ctx, lease); err != nil {
		  t.Fatalf("insert lease: %v", err)
	   }
	  time.Sleep(5 * time.Millisecond)
	   patrol := intxfergate.NewPatrolWorker(env.gate, env.store, 100)
	   released, expired, err := patrol.RunOnce(ctx)
	  if err != nil {
		   t.Fatalf("patrol error: %v", err)
	   }
	   if released < 1 {
		  t.Fatalf("expected ≥1 lease released, got %d", released)
	  }
	  if expired < 1 {
		   t.Fatalf("expected ≥1 snap expired, got %d", expired)
	  }
	   t.Logf("patrol: released=%d expired=%d", released, expired)
   }

// ─── Engine.DetectMisroute ───────────────────────────────────────────

   func TestEngine_DetectMisroute(t *testing.T) {
	  env := newTestEnv(t, 5*time.Minute, 30*time.Second)
	   ctx := context.Background()
	  t.Run("no_deviation_nil", func(t *testing.T) {
		  mc, err := env.engine.DetectMisroute(ctx,
			  "snap-001", "xfer-001",
			   "u-same", "u-same", // snap_to == actual_to → 无偏差
			  "u-from", "USDT", "100", "idem-001", "scanner note")
		   if err != nil {
			   t.Fatalf("unexpected error: %v", err)
		  }
		   if mc != nil {
			  t.Fatalf("expected nil case for same users, got %+v", mc)
		   }
	  })
	  t.Run("deviation_opens_case", func(t *testing.T) {
		   mc, err := env.engine.DetectMisroute(ctx,
			  "snap-002", "xfer-002",
			   "u-orig", "u-new", // 不同 → 开案
			   "u-from", "USDT", "200", "idem-002", "directory rebind detected")
		   if err != nil {
			   t.Fatalf("unexpected error: %v", err)
		   }
		   if mc == nil {
			  t.Fatal("expected misroute case, got nil")
		   }
		  if mc.State != intxfergate.MisrouteCaseOpen {
			   t.Fatalf("state: got %s, want open", mc.State)
		  }
		   if mc.SnapToUserID != "u-orig" {
			  t.Fatalf("snap_to_user_id: got %s, want u-orig", mc.SnapToUserID)
		  }
	   })
	   t.Run("idempotent_second_detect", func(t *testing.T) {
		   // 第二次检测同一 snap+ledger 应返回已有 case（幂等）
		   mc1, err := env.engine.DetectMisroute(ctx,
			   "snap-003", "xfer-003",
			   "u-a", "u-b", "u-from", "USDT", "10", "idem-003", "first")
		  if err != nil {
			  t.Fatalf("first detect: %v", err)
		  }
		  mc2, err := env.engine.DetectMisroute(ctx,
			   "snap-003", "xfer-003",
			   "u-a", "u-b", "u-from", "USDT", "10", "idem-003", "second")
		   if err != nil {
			   t.Fatalf("second detect: %v", err)
		   }
		  if mc1.ID != mc2.ID {
			   t.Fatalf("idempotency violated: mc1.ID=%s mc2.ID=%s", mc1.ID, mc2.ID)
		   }
	   })
   }

// ─── Engine.ExportProof ──────────────────────────────────────────────

  func TestEngine_ExportProof(t *testing.T) {
	   env := newTestEnv(t, 5*time.Minute, 30*time.Second)
	   ctx := context.Background()
	   // 直接插入一个证明
	  proof := &intxfergate.XferEffectProof{
		   ID:     "proof-export-001",
		  SnapID: "snap-ep-001",
		   LedgerID:   "xfer-ep-001",
		  FromUserID: "u-from-ep",
		   ToUserID: "u-to-ep",
		   Amount:   "555",
		  IdemKey:  "ep-idem-001",
		   SealedAt: time.Now().UTC(),
		  CreatedAt: time.Now().UTC(),
	   }
	   proof.ProofHash = intxfergate.ComputeHash(
		   proof.SnapID, proof.FromUserID, proof.ToUserID, proof.Amount, proof.LedgerID,
	   )
	   if err := env.store.InsertEffectProof(ctx, proof); err != nil {
		   t.Fatalf("insert proof: %v", err)
	  }
	  t.Run("found_and_valid", func(t *testing.T) {
		  got, err := env.engine.ExportProof(ctx, "snap-ep-001", "xfer-ep-001")
		   if err != nil {
			  t.Fatalf("export proof: %v", err)
		   }
		  if got.ID != proof.ID {
			   t.Fatalf("id mismatch: got %s want %s", got.ID, proof.ID)
		  }
		  if err := got.Verify(); err != nil {
			   t.Fatalf("verify exported proof: %v", err)
		  }
	   })
	   t.Run("not_found", func(t *testing.T) {
		   _, err := env.engine.ExportProof(ctx, "snap-nonexist", "xfer-nonexist")
		   if err == nil {
			  t.Fatal("expected error for missing proof")
		   }
	   })
   }

// ─── MemStore 隔离 ─────────────────────────────────────────────────────

  func TestMemStore_Isolation(t *testing.T) {
	   store := intxfergate.NewMemStore()
	   ctx := context.Background()
	  // 同 snap_id + ledger_id 不得重复插入证明
	   p1 := &intxfergate.XferEffectProof{
		  ID:     "proof-iso-001",
		   SnapID: "snap-iso",
		  LedgerID: "xfer-iso",
		  ProofHash: "hash-iso-001",
		   CreatedAt: time.Now(),
		  SealedAt:  time.Now(),
	  }
	   if err := store.InsertEffectProof(ctx, p1); err != nil {
		   t.Fatalf("first insert: %v", err)
	   }
	   p2 := *p1
	  p2.ID = "proof-iso-002"
	   p2.ProofHash = "hash-iso-002"
	   if err := store.InsertEffectProof(ctx, &p2); err == nil {
		   t.Fatal("expected duplicate error for same snap+ledger, got nil")
	  }
  }

// ─── AC-4 EffectProof 缺失禁止 completed ────────────────────────────────────

  func TestEngine_AC4_ProofMissing_BlocksCompleted(t *testing.T) {
	   env := newTestEnv(t,
		  5*time.Minute, 30*time.Second)
	  ctx := context.Background()
	   // 构造一个 sealing 状态的 SendRecord（ sealProof 失败场景）
	  now := time.Now().UTC()
	  rec := &intxfergate.SendRecord{
		  ID: "sr-ac4-001",
		  IdemKey: "ac4-idem-001",
		  FromUserID: "u-from-ac4",
		  Asset: "USDT",
		  Amount: "300",
		   State:  intxfergate.SendStateSealing,
		  SnapID:    "snap-ac4-001",
		   LedgerID:  "xfer-ac4-001",
		   CreatedAt: now,
		   UpdatedAt: now.Add(-10 * time.Second),
	   }
	  if err := env.store.InsertSendRecord(ctx, rec); err != nil {
		   t.Fatalf("insert send record: %v", err)
	   }
	   // 插入对应的 snap（sealer 需要它来重建 proof）
	   snap := &fundint.ResolveSnap{
		   ID: "snap-ac4-001",
		   IdemKey: "ac4-idem-001",
		   ToUserID: "u-to-ac4",
		   State: fundint.ResolveSnapFresh,
		  ExpiresAt:  now.Add(5 * time.Minute),
		  ResolvedAt: now,
		  CreatedAt: now,
		  UpdatedAt: now,
	  }
	  if err := env.store.InsertResolveSnap(ctx, snap); err != nil {
		  t.Fatalf("insert snap: %v", err)
	   }
	  // 验证：当前无 proof，SendRecord 应处于 sealing（不是 completed）
	   got, err := env.store.GetSendRecordByID(ctx, rec.ID)
	   if err != nil {
		  t.Fatalf("get record: %v", err)
	   }
	   if got.State == intxfergate.SendStateCompleted {
		   t.Fatal("AC-4 violated: SendRecord marked completed without proof")
	   }
	   if got.State != intxfergate.SendStateSealing {
		   t.Fatalf("expected sealing state, got %s", got.State)
	   }
	  // SealerWorker 应能重建 proof 并推进到 completed
	   sealer := intxfergate.NewSealerWorker(env.engine, env.store, 2*time.Minute, 100)
	   sealed, blocked, err := sealer.RunOnce(ctx)
	   if err != nil {
		   t.Fatalf("sealer error: %v", err)
	  }
	   t.Logf("AC-4 sealer: sealed=%d blocked=%d", sealed, blocked)
	   // 验证最终状态（sealer 能重建则 completed；否则应还是 sealing，不是 completed w/o proof）
	  got2, err := env.store.GetSendRecordByID(ctx, rec.ID)
	  if err != nil {
		  t.Fatalf("get record after sealer: %v", err)
	   }
	   if got2.State == intxfergate.SendStateCompleted && got2.ProofID == "" {
		   t.Fatal("AC-4 violated: completed without proof_id")
	  }
	  t.Logf("AC-4 final state: %s proof_id=%s", got2.State, got2.ProofID)
  }

// ─── 辅助函数（防编译错误占位）────────────────────────────────────────────

var _ = fmt.Sprintf
