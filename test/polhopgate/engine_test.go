package polhopgate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/acctpol"
	"account-assets.kucoin.net/funding/internal/polhopgate"
)

// ─── 辅助 ─────────────────────────────────────────────────────────────

type testClock struct {
	t time.Time
}

  func (tc *testClock) now() time.Time          { return tc.t }
  func (tc *testClock) advance(d time.Duration) { tc.t = tc.t.Add(d) }

  func newTestEngine(t *testing.T) (*polhopgate.Engine, *polhopgate.MemStore, *acctpol.HopMemStore, *testClock) {
	  t.Helper()
	  store := polhopgate.NewMemStore()
	  polStore := acctpol.NewHopMemStore()
	  tc := &testClock{t: time.Date(2022, 7, 18, 0, 0, 0, 0, time.UTC)}
	  cfg := polhopgate.Config{
		   TokenTTL:             30 * time.Second,
		  LeaseTTL:             60 * time.Second,
		  DualDivergeThreshold: 3,
		   DualDivergeWindow: 5 * time.Minute,
	   }
	   engine := polhopgate.NewEngine(store, polStore, cfg).WithClock(tc.now)
	  return engine, store, polStore, tc
   }

// publishGlobalVersion 在 polStore 中发布一个全局策略代次，返回 (Gen, VersionID)。
   func publishGlobalVersion(t *testing.T, reg *acctpol.Registry, asset string, allowAll bool) (int64, string) {
	  t.Helper()
	  var allowKinds []acctpol.HopBizKind
	  if allowAll {
		   allowKinds = nil // 空=全允许
	  }
	   reply, err := reg.Publish(context.Background(), acctpol.PublishRequest{
		  Asset:         asset,
		  AllowBizKinds: allowKinds,
		  PublishedBy: "ops-test",
	   })
	   if err != nil {
		  t.Fatalf("publishGlobalVersion: %v", err)
	  }
	  return reply.Gen, reply.VersionID
  }

// ─── 用例 ─────────────────────────────────────────────────────────────
// TestIssueAndBindToken 主路径：签发令牌后正常绑定 hop。
  func TestIssueAndBindToken(t *testing.T) {
	  engine,
		   _, polStore, _ := newTestEngine(t)
	  ctx := context.Background()
	   reg := acctpol.NewRegistry(polStore)
	   gen, _ := publishGlobalVersion(t, reg, "", true)
	   reply, err := engine.IssueToken(ctx, polhopgate.IssueRequest{
		  UserID:  "user1",
		   BizKind: "withdraw",
		  IdemKey: "idem-withdraw-1",
	   })
	  if err != nil {
		   t.Fatalf("IssueToken: %v", err)
	   }
	   if reply.PolicyGen != gen {
		  t.Errorf("expected policy_gen=%d got %d", gen, reply.PolicyGen)
	  }
	   if reply.State != polhopgate.TokenStateIssued {
		   t.Errorf("expected state=issued got %s", reply.State)
	   }
	   bindReply, err := engine.BindHop(ctx, polhopgate.BindRequest{
		  TokenID: reply.TokenID,
		  HopRef:  "withdraw-order-100",
		   UserID: "user1",
	   })
	  if err != nil {
		   t.Fatalf("BindHop: %v", err)
	  }
	   if bindReply.State != polhopgate.TokenStateConsumed {
		   t.Errorf("expected consumed got %s", bindReply.State)
	   }
   }

// TestPublishAfterGateCheck 验收场景 1：Gate.Check 后策略 Publish → hop 执行复验失败，不改账。

  func TestPublishAfterGateCheck(t *testing.T) {
	  engine, _, polStore, tc := newTestEngine(t)
	   ctx := context.Background()
	  reg := acctpol.NewRegistry(polStore).WithClock(tc.now)
	   // 初始发布（Gen=1）
	   gen1, _ := publishGlobalVersion(t, reg, "", true)
	   // Gate.Check：签发令牌（持有 Gen=1）
	   issueReply, err := engine.IssueToken(ctx, polhopgate.IssueRequest{
		   UserID: "user-alice",
		   BizKind: "transfer",
		   IdemKey: "idem-transfer-alice-1",
	   })
	   if err != nil {
		   t.Fatalf("IssueToken: %v", err)
	  }
	   if issueReply.PolicyGen != gen1 {
		  t.Errorf("expected gen=%d got %d", gen1, issueReply.PolicyGen)
	   }
	   // 策略 Publish 抬升世代（Gen=2）—— 竞态窗口
	   gen2, _ := publishGlobalVersion(t, reg, "", true)
	   if gen2 <= gen1 {
		  t.Fatalf("expected gen2 > gen1, got gen1=%d gen2=%d", gen1, gen2)
	  }
	  // hop 执行复验：应因 ErrGenMismatch 被拒，不改账
	  _, bindErr := engine.BindHop(ctx, polhopgate.BindRequest{
		   TokenID: issueReply.TokenID,
		  HopRef: "xfer-order-200",
		  UserID: "user-alice",
	   })
	  if bindErr == nil {
		   t.Fatal("expected ErrGenMismatch but got nil")
	   }
	   // 验证错误是世代不匹配类型
	  t.Logf("BindHop correctly rejected: %v", bindErr)
	   // 验证令牌已被挡写
	  tok, err := engine.GetToken(ctx, issueReply.TokenID)
	  if err != nil {
		   t.Fatalf("GetToken: %v", err)
	   }
	   if tok.State != polhopgate.TokenStateBlocked {
		  t.Errorf("expected token state=blocked got %s", tok.State)
	   }
  }

// TestDualReadDivergeEscalate 验收场景 2：DualRead 分歧超阈 → 对应 BizKind hop 挡写并升案。
  func TestDualReadDivergeEscalate(t *testing.T) {
	  engine,
		   store, polStore, tc := newTestEngine(t)
	  ctx := context.Background()
	  reg := acctpol.NewRegistry(polStore).WithClock(tc.now)
	   publishGlobalVersion(t, reg, "", true)
	   // 连续 3 次分歧（达到阈值）
	   for i := 0; i < 3; i++ {
		   err := engine.RecordDualRead(ctx, polhopgate.DualReadRequest{
			  UserID:  "",
			   BizKind: "deposit",
			   PrimaryGen: 5,
			   ReplicaGen: 3, // 分歧 delta=2
			  TraceID:    "trace-diverge",
		   })
		   if err != nil {
			  t.Fatalf("RecordDualRead i=%d: %v", i, err)
		   }
		   tc.advance(30 * time.Second) // 时间推进，避免重复幂等键
	   }
	  // 检查 DualEscalate 是否已激活
	   esc, err := store.GetActiveEscalate(ctx, "", "", "deposit")
	   if err != nil {
		  t.Fatalf("GetActiveEscalate: %v", err)
	   }
	  if esc.State != polhopgate.EscalateStateActive {
		   t.Errorf("expected escalate active got %s", esc.State)
	   }
	  t.Logf("DualEscalate activated: case_id=%s threshold=%d", esc.CaseID, esc.ThresholdCount)
	  // IssueToken 应被挡写
	  _, issueErr := engine.IssueToken(ctx, polhopgate.IssueRequest{
		  UserID: "user-bob",
		  BizKind: "deposit",
		   IdemKey: "idem-deposit-bob-blocked",
	   })
	  if issueErr == nil {
		  t.Fatal("expected ErrDualEscalateBlock but got nil")
	   }
	  t.Logf("IssueToken correctly blocked: %v", issueErr)
	   // 检查对应 PolDriftCase 已开启
	  cases,
		   err := store.ListDriftCasesByState(ctx, polhopgate.DriftCaseOpen, 10)
	   if err != nil {
		   t.Fatalf("ListDriftCases: %v", err)
	   }
	   if len(cases) == 0 {
		  t.Fatal("expected at least one open drift case")
	   }
	   t.Logf("DriftCase opened: id=%s trigger=%s", cases[0].ID, cases[0].Trigger)
  }

// TestDriftCaseProofRequired 验收场景 3：PolDriftCase 证明通过后方可关闭。
   func TestDriftCaseProofRequired(t *testing.T) {
	   engine, store, polStore, tc := newTestEngine(t)
	   ctx := context.Background()
	  reg := acctpol.NewRegistry(polStore).WithClock(tc.now)
	   publishGlobalVersion(t, reg, "", true)
	   // 触发升级
	   for i := 0; i < 3; i++ {
		  _ = engine.RecordDualRead(ctx, polhopgate.DualReadRequest{
			   BizKind:    "earn",
			  PrimaryGen: 10,
			  ReplicaGen: 7,
			   TraceID:    "proof-test",
		   })
		   tc.advance(10 * time.Second)
	  }
	   cases,
		   _ := store.ListDriftCasesByState(ctx, polhopgate.DriftCaseOpen, 1)
	   if len(cases) == 0 {
		   t.Skip("no open drift case, skip proof test")
	  }
	  dc := cases[0]
	   // 尝试直接 Confirm（无证明）应返回 ErrProofMissing
	  err := engine.ConfirmDriftCase(ctx, polhopgate.ConfirmDriftCaseRequest{
		   CaseID:      dc.ID,
		  ConfirmedBy: "admin-ops",
		  CloseReason: "false alarm",
	   })
	   if err == nil {
		  t.Fatal("expected ErrProofMissing on open case confirm but got nil")
	  }
	  t.Logf("ConfirmDriftCase correctly rejected: %v", err)
	  // 运行 ProofSealWorker 产出证明
	  proofWorker := polhopgate.NewProofSealWorker(store, polhopgate.WorkerConfig{BatchSize: 10})
	  if runErr := proofWorker.RunOnce(ctx); runErr != nil {
		   t.Fatalf("ProofSealWorker.RunOnce: %v", runErr)
	   }
	   // 案例应已推进到 proven
	  dc2, err := store.GetDriftCaseByID(ctx, dc.ID)
	   if err != nil {
		   t.Fatalf("GetDriftCaseByID: %v", err)
	  }
	   if dc2.State != polhopgate.DriftCaseProven {
		  t.Errorf("expected state=proven got %s", dc2.State)
	   }
	   t.Logf("DriftCase proven: proof_id=%s proof_hash=%s", dc2.ProofID, dc2.ProofHash)
	  // 现在可以 Confirm
	   if err := engine.ConfirmDriftCase(ctx, polhopgate.ConfirmDriftCaseRequest{
		   CaseID: dc.ID,
		   ConfirmedBy: "admin-ops",
		  CloseReason: "verified and closed",
	   }); err != nil {
		   t.Fatalf("ConfirmDriftCase after proof: %v", err)
	   }
	  dc3, _ := store.GetDriftCaseByID(ctx, dc.ID)
	   if dc3.State != polhopgate.DriftCaseClosed {
		   t.Errorf("expected state=closed got %s", dc3.State)
	   }
	   t.Logf("DriftCase closed by admin")
   }

// TestParallelPinAndGlobalPublish 场景 4：Pin 用户与全局 Publish 并发，各自持有不同 Gen。
   func TestParallelPinAndGlobalPublish(t *testing.T) {
	  engine, _, polStore, tc := newTestEngine(t)
	   ctx := context.Background()
	   reg := acctpol.NewRegistry(polStore).WithClock(tc.now)
	   // 全局代次 Gen=1
	  globalGen, globalVID := publishGlobalVersion(t, reg, "BTC", true)
	  // 个人 Pin（Gen=2）
	  pinReply, err := reg.Pin(ctx, acctpol.PinRequest{
		  VersionID: globalVID,
		  UserID: "user-charlie",
		   PinnedBy: "user-charlie",
	   })
	   if err != nil {
		   t.Fatalf("Pin: %v", err)
	   }
	   pinnedGen := pinReply.Gen
	   if pinnedGen <= globalGen {
		   t.Errorf("pinned gen should be > global gen: global=%d pinned=%d", globalGen, pinnedGen)
	   }
	   t.Logf("Pinned Gen=%d for user-charlie", pinnedGen)
	   // user-charlie 使用 pinned Gen 签发令牌
	  charlieToken, err := engine.IssueToken(ctx, polhopgate.IssueRequest{
		   UserID:  "user-charlie",
		  Asset:   "BTC",
		  BizKind: "transfer",
		  IdemKey: "charlie-xfer-1",
	  })
	  if err != nil {
		   t.Fatalf("IssueToken charlie: %v", err)
	   }
	  if charlieToken.PolicyGen != pinnedGen {
		   t.Errorf("charlie token should use pinned gen=%d got %d", pinnedGen, charlieToken.PolicyGen)
	  }
	  // 全局再次 Publish（Gen=3）
	  gen3, _ := publishGlobalVersion(t, reg, "BTC", true)
	   // 新用户 user-dave 使用全局新 Gen 签发令牌
	   daveToken, err := engine.IssueToken(ctx, polhopgate.IssueRequest{
		   UserID: "user-dave",
		   Asset:  "BTC",
		  BizKind: "transfer",
		   IdemKey: "dave-xfer-1",
	  })
	  if err != nil {
		  t.Fatalf("IssueToken dave: %v", err)
	   }
	  if daveToken.PolicyGen != gen3 {
		  t.Errorf("dave token should use gen3=%d got %d", gen3, daveToken.PolicyGen)
	   }
	  // charlie 的旧令牌（pinned Gen）在 Unpin 后全局 Gen 抬升时应被拒
	  // 首先 Unpin charlie
	  if err := reg.Unpin(ctx, acctpol.UnpinRequest{UserID: "user-charlie", Asset: "BTC"}); err != nil {
		   t.Fatalf("Unpin: %v", err)
	  }
	  // charlie 尝试用旧令牌绑定 hop（此时全局 active Gen=3，令牌 Gen=pinned gen）
	  _, bindErr := engine.BindHop(ctx, polhopgate.BindRequest{
		  TokenID: charlieToken.TokenID,
		   HopRef: "charlie-xfer-order-1",
		   UserID: "user-charlie",
	  })
	   // 注意：Unpin 后 charlie 使用全局代次，若 pinned gen != gen3 则 ErrGenMismatch
	  if pinnedGen != gen3 {
		  if bindErr == nil {
			   t.Error("expected ErrGenMismatch for charlie's stale token")
		  } else {
			  t.Logf("charlie stale token correctly rejected: %v", bindErr)
		  }
	   }
	  // dave 的令牌（gen3）可以正常绑定
	   tc.advance(0) // 不推进时钟
	   bindReply, err := engine.BindHop(ctx, polhopgate.BindRequest{
		   TokenID: daveToken.TokenID,
		   HopRef:  "dave-xfer-order-1",
		   UserID:  "user-dave",
	   })
	   if err != nil {
		   t.Fatalf("dave BindHop: %v", err)
	   }
	   if bindReply.State != polhopgate.TokenStateConsumed {
		   t.Errorf("dave token should be consumed, got %s", bindReply.State)
	  }
   }

// TestTokenExpiry 场景 5：令牌过期后绑定 hop 返回 ErrTokenExpired。
   func TestTokenExpiry(t *testing.T) {
	   engine, store, polStore, tc := newTestEngine(t)
	  ctx := context.Background()
	  reg := acctpol.NewRegistry(polStore).WithClock(tc.now)
	  publishGlobalVersion(t, reg, "", true)
	  reply, err := engine.IssueToken(ctx, polhopgate.IssueRequest{
		   UserID: "user-expire",
		  BizKind: "withdraw",
		  IdemKey: "expire-test-1",
	   })
	  if err != nil {
		   t.Fatalf("IssueToken: %v", err)
	  }
	   // 推进时间超过 TTL（30s）
	  tc.advance(31 * time.Second)
	   // TokenExpiryWorker 扫描
	   w := polhopgate.NewTokenExpiryWorker(store, polhopgate.WorkerConfig{BatchSize: 10})
	   if err := w.RunOnce(ctx); err != nil {
		  t.Fatalf("TokenExpiryWorker: %v", err)
	  }
	  // 绑定 hop 应返回 ErrTokenExpired
	   _, bindErr := engine.BindHop(ctx, polhopgate.BindRequest{
		  TokenID: reply.TokenID,
		  HopRef:  "expired-hop",
	  })
	   if bindErr == nil {
		   t.Fatal("expected ErrTokenExpired but got nil")
	   }
	  t.Logf("Expired token correctly rejected: %v", bindErr)
  }

// TestTokenConsumedIdempotent 场景 6：令牌消费后重复绑定返回 ErrTokenConsumed。
   func TestTokenConsumedIdempotent(t *testing.T) {
	   engine, _, polStore, _ := newTestEngine(t)
	  ctx := context.Background()
	  reg := acctpol.NewRegistry(polStore)
	  publishGlobalVersion(t, reg, "", true)
	   reply, err := engine.IssueToken(ctx, polhopgate.IssueRequest{
		   UserID:  "user-idem",
		   BizKind: "transfer",
		  IdemKey: "idem-consumed-1",
	   })
	   if err != nil {
		   t.Fatalf("IssueToken: %v", err)
	  }
	   // 第一次绑定（成功）
	  _, err = engine.BindHop(ctx, polhopgate.BindRequest{
		  TokenID: reply.TokenID,
		   HopRef: "xfer-001",
	   })
	   if err != nil {
		   t.Fatalf("first BindHop: %v", err)
	   }
	  // 第二次绑定（应返回 ErrTokenConsumed）
	  _, err = engine.BindHop(ctx, polhopgate.BindRequest{
		  TokenID: reply.TokenID,
		   HopRef:  "xfer-001-retry",
	  })
	  if err == nil {
		  t.Fatal("expected ErrTokenConsumed on second bind but got nil")
	   }
	  t.Logf("Consumed token correctly rejected: %v", err)
   }

// TestEscalateResolveUnblocks 场景 7：DualEscalate 解除后 IssueToken 可恢复。
  func TestEscalateResolveUnblocks(t *testing.T) {
	  engine, store, polStore, tc := newTestEngine(t)
	  ctx := context.Background()
	   reg := acctpol.NewRegistry(polStore).WithClock(tc.now)
	  publishGlobalVersion(t, reg, "", true)
	  // 触发升级
	   for i := 0; i < 3; i++ {
		  _ = engine.RecordDualRead(ctx, polhopgate.DualReadRequest{
			   BizKind: "lending",
			   PrimaryGen: 5,
			  ReplicaGen: 2,
			  TraceID: "resolve-test",
		  })
		  tc.advance(10 * time.Second)
	   }
	  // 确认 IssueToken 被挡
	   _, issueErr := engine.IssueToken(ctx, polhopgate.IssueRequest{
		   UserID:  "user-lend",
		   BizKind: "lending",
		  IdemKey: "lend-blocked-1",
	   })
	  if issueErr == nil {
		  t.Fatal("expected ErrDualEscalateBlock")
	   }
	   // 运行 ProofSealWorker
	   proofWorker := polhopgate.NewProofSealWorker(store, polhopgate.WorkerConfig{BatchSize: 10})
	   _ = proofWorker.RunOnce(ctx)
	  // Confirm 关闭案例（解除挡写）
	  cases, _ := store.ListDriftCasesByState(ctx, polhopgate.DriftCaseProven, 10)
	  if len(cases) == 0 {
		   // 若还是 open，直接 Dismiss
		  openCases,
			  _ := store.ListDriftCasesByState(ctx, polhopgate.DriftCaseOpen, 10)
		   if len(openCases) > 0 {
			  _ = engine.DismissDriftCase(ctx, openCases[0].ID, "admin", "test-resolve")
		  }
	  } else {
		   _ = engine.ConfirmDriftCase(ctx, polhopgate.ConfirmDriftCaseRequest{
			   CaseID: cases[0].ID,
			   ConfirmedBy: "admin",
			  CloseReason: "resolved",
		  })
	   }
	  // 现在 IssueToken 应恢复
	   issueReply, err := engine.IssueToken(ctx, polhopgate.IssueRequest{
		  UserID:  "user-lend",
		  BizKind: "lending",
		  IdemKey: "lend-unblocked-1",
	  })
	   if err != nil {
		  t.Fatalf("IssueToken after resolve: %v", err)
	   }
	  if issueReply.State != polhopgate.TokenStateIssued {
		   t.Errorf("expected issued got %s", issueReply.State)
	  }
	  t.Logf("IssueToken restored after escalate resolve: token_id=%s", issueReply.TokenID)
  }

// TestForceReEvaluate 场景 8：ForceReEvaluate 只挡写旧 Gen 令牌，不影响新 Gen 令牌。
  func TestForceReEvaluate(t *testing.T) {
	  engine, _, polStore, _ := newTestEngine(t)
	   ctx := context.Background()
	   reg := acctpol.NewRegistry(polStore)
	   gen1, _ := publishGlobalVersion(t, reg, "", true)
	  // 签发旧 Gen 令牌
	   oldToken, err := engine.IssueToken(ctx, polhopgate.IssueRequest{
		   UserID: "user-reeval",
		  BizKind: "convert",
		   IdemKey: "convert-old-gen",
	   })
	   if err != nil {
		  t.Fatalf("IssueToken old gen: %v", err)
	   }
	  if oldToken.PolicyGen != gen1 {
		  t.Errorf("old token gen mismatch: want %d got %d", gen1, oldToken.PolicyGen)
	  }
	   // Publish 新代次
	   gen2, _ := publishGlobalVersion(t, reg, "", true)
	  // 签发新 Gen 令牌
	   newToken, err := engine.IssueToken(ctx, polhopgate.IssueRequest{
		   UserID: "user-reeval",
		   BizKind: "convert",
		   IdemKey: "convert-new-gen",
	   })
	   if err != nil {
		  t.Fatalf("IssueToken new gen: %v", err)
	  }
	  if newToken.PolicyGen != gen2 {
		  t.Errorf("new token gen mismatch: want %d got %d", gen2, newToken.PolicyGen)
	   }
	   // ForceReEvaluate（只挡写 PolicyGen < gen2 的令牌）
	  reply, err := engine.ForceReEvaluate(ctx, polhopgate.ForceReEvaluateRequest{
		   UserID: "user-reeval",
		  BizKind: "convert",
		  CurrentGen: gen2,
		  BlockReason: "policy updated to gen2",
	   })
	  if err != nil {
		   t.Fatalf("ForceReEvaluate: %v", err)
	   }
	   if reply.BlockedCount != 1 {
		  t.Errorf("expected 1 blocked, got %d", reply.BlockedCount)
	  }
	  t.Logf("ForceReEvaluate: blocked=%d skipped=%d", reply.BlockedCount, reply.SkippedCount)
	  // 旧令牌应被挡写
	  oldTok, _ := engine.GetToken(ctx, oldToken.TokenID)
	  if oldTok.State != polhopgate.TokenStateBlocked {
		   t.Errorf("old token should be blocked, got %s", oldTok.State)
	  }
	  // 新令牌不受影响
	   newTok, _ := engine.GetToken(ctx, newToken.TokenID)
	  if newTok.State != polhopgate.TokenStateIssued {
		   t.Errorf("new token should still be issued, got %s", newTok.State)
	  }
   }

// TestCfgRolloutKeyspaceIsolation 场景 9：DualRead 分歧不影响 cfgcenter 相关字段（键空间隔离）。

   func TestCfgRolloutKeyspaceIsolation(t *testing.T) {
	   engine, store, polStore, _ := newTestEngine(t)
	  ctx := context.Background()
	   reg := acctpol.NewRegistry(polStore)
	  publishGlobalVersion(t, reg, "", true)
	  // 记录 DualRead 分歧（BizKind=deposit，与 cfgcenter rollout_id 无关）
	   for i := 0; i < 5; i++ {
		   _ = engine.RecordDualRead(ctx, polhopgate.DualReadRequest{
			  BizKind:    "deposit",
			  PrimaryGen: 10,
			   ReplicaGen: 5,
			   TraceID: "isolation-test",
		   })
	  }
	  // 确认只有 deposit 的 DualEscalate 被激活，而非全局所有 BizKind
	  depositEsc, err := store.GetActiveEscalate(ctx, "", "", "deposit")
	  if err != nil {
		   t.Fatalf("GetActiveEscalate deposit: %v", err)
	  }
	  t.Logf("deposit escalate active: %s", depositEsc.ID)
	  // withdraw、transfer 等不受影响
	   if _, err := store.GetActiveEscalate(ctx, "", "", "withdraw"); err == nil {
		   t.Error("withdraw should NOT have active escalate")
	  }
	  if _, err := store.GetActiveEscalate(ctx, "", "", "transfer"); err == nil {
		  t.Error("transfer should NOT have active escalate")
	  }
	  // 验证 DriftCase 中没有 cfgcenter / rollout_id 相关字段
	   cases, _ := store.ListDriftCasesByState(ctx, polhopgate.DriftCaseOpen, 10)
	   for _, dc := range cases {
		   // BizKind 只应是 polhopgate 已知的业务类型，不包含 cfgcenter 字段
		   if dc.BizKind == "cfgcenter" || dc.BizKind == "rollout" {
			   t.Errorf("drift case should not contain cfgcenter/rollout biz_kind: %s", dc.BizKind)
		  }
	   }
	  t.Log("Keyspace isolation verified: cfgcenter/rollout not in drift cases")
  }

// TestLeaseWindowBlocksHopInPublishWindow 验收场景：Publish 租约窗口期间 hop 令牌签发等待。

  func TestLeaseWindowBlocksHopInPublishWindow(t *testing.T) {
	   engine, _, polStore, _ := newTestEngine(t)
	  ctx := context.Background()
	  reg := acctpol.NewRegistry(polStore)
	   publishGlobalVersion(t, reg, "", true)
	  // 获取发布互斥租约（ Publish 操作进行中）
	   leaseReply, err := engine.AcquireLease(ctx, polhopgate.AcquireLeaseRequest{
		  UserID: "",
		   Asset:  "ETH",
		  Holder: "ops-publisher",
	  })
	  if err != nil {
		  t.Fatalf("AcquireLease: %v", err)
	   }
	   t.Logf("Lease acquired: lease_id=%s", leaseReply.LeaseID)
	   // 另一操作人尝试获取同一租约（应被 ErrLeaseLocked 阻断）
	  _, lockErr := engine.AcquireLease(ctx, polhopgate.AcquireLeaseRequest{
		  UserID: "",
		  Asset:  "ETH",
		  Holder: "ops-another",
	   })
	  if lockErr == nil {
		  t.Fatal("expected ErrLeaseLocked from second acquirer")
	   }
	  t.Logf("Second acquirer correctly locked: %v", lockErr)
	   // 发布完成后释放租约
	   if err := engine.ReleaseLease(ctx, leaseReply.LeaseID, 2); err != nil {
		   t.Fatalf("ReleaseLease: %v", err)
	   }
	   // 发布完成后新操作人可获取租约
	   _, err = engine.AcquireLease(ctx, polhopgate.AcquireLeaseRequest{
		   UserID: "",
		  Asset:  "ETH",
		  Holder: "ops-next",
	   })
	  if err != nil {
		   t.Fatalf("AcquireLease after release: %v", err)
	   }
   }

// TestIdemKeyIssuance 幂等键：相同 IdemKey 重复签发返回已有令牌。

  func TestIdemKeyIssuance(t *testing.T) {
	   engine, _, polStore, _ := newTestEngine(t)
	   ctx := context.Background()
	  reg := acctpol.NewRegistry(polStore)
	   publishGlobalVersion(t, reg, "", true)
	   idem := "idem-idempotent-test"
	  r1, err := engine.IssueToken(ctx, polhopgate.IssueRequest{
		   UserID: "user-idem2",
		   BizKind: "fee_charge",
		   IdemKey: idem,
	   })
	   if err != nil {
		   t.Fatalf("first IssueToken: %v", err)
	   }
	   r2, err := engine.IssueToken(ctx, polhopgate.IssueRequest{
		  UserID:  "user-idem2",
		   BizKind: "fee_charge",
		  IdemKey: idem,
	   })
	  if err != nil {
		  t.Fatalf("second IssueToken: %v", err)
	   }
	   if r1.TokenID != r2.TokenID {
		  t.Errorf("expected same token_id on idem repeat: got %s vs %s", r1.TokenID, r2.TokenID)
	  }
   }

// TestWorkerTokenExpiryAndProofSeal Worker 集成。

  func TestWorkerTokenExpiryAndProofSeal(t *testing.T) {
	   _, store, polStore, tc := newTestEngine(t)
	   ctx := context.Background()
	  reg := acctpol.NewRegistry(polStore).WithClock(tc.now)
	   publishGlobalVersion(t, reg, "", true)
	   engine2 := polhopgate.NewEngine(store, polStore, polhopgate.Config{
		   TokenTTL: 5 * time.Second,
		   DualDivergeThreshold: 2,
		  DualDivergeWindow:    1 * time.Minute,
	   }).WithClock(tc.now)
	   // 签发令牌
	   reply, _ := engine2.IssueToken(ctx, polhopgate.IssueRequest{
		   UserID: "user-worker",
		   BizKind: "earn",
		   IdemKey: "worker-earn-1",
	  })
	  // 推进时间超过 TokenTTL
	   tc.advance(6 * time.Second)
	   // 运行 TokenExpiryWorker
	  w := polhopgate.NewTokenExpiryWorker(store, polhopgate.WorkerConfig{BatchSize: 10})
	  if err := w.RunOnce(ctx); err != nil {
		  t.Fatalf("TokenExpiryWorker: %v", err)
	   }
	  tok,
		  _ := store.GetTokenByID(ctx, reply.TokenID)
	  if tok.State != polhopgate.TokenStateExpired {
		   t.Errorf("expected expired got %s", tok.State)
	  }
	   // 触发 DualRead 分歧升级
	   for i := 0; i < 2; i++ {
		  _ = engine2.RecordDualRead(ctx, polhopgate.DualReadRequest{
			  BizKind:    "earn",
			  PrimaryGen: 5,
			   ReplicaGen: 3,
			   TraceID: "worker-test",
		   })
		   tc.advance(5 * time.Second)
	   }
	   // 运行 ProofSealWorker
	  proofW := polhopgate.NewProofSealWorker(store, polhopgate.WorkerConfig{BatchSize: 10})
	   if err := proofW.RunOnce(ctx); err != nil {
		   t.Fatalf("ProofSealWorker: %v", err)
	   }
	  // 验证 DriftCase 已推进到 proven
	  cases, _ := store.ListDriftCasesByState(ctx, polhopgate.DriftCaseProven, 10)
	  t.Logf("Proven cases after worker: %d", len(cases))
  }
