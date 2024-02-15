package overwdlease_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/overwdlease"
)

// ─── 辅助 ─────────────────────────────────────────────────────────────

type testClock struct {
	t time.Time
}

  func (tc *testClock) now() time.Time { return tc.t }

  func (tc *testClock) advance(d time.Duration) { tc.t = tc.t.Add(d) }

   func newTestEngine(t *testing.T) (*overwdlease.Engine, *overwdlease.MemStore, *testClock) {
	  t.Helper()
	  store := overwdlease.NewMemStore()
	  tc := &testClock{t: time.Date(2022, 8, 15, 0, 0, 0, 0, time.UTC)}
	   cfg := overwdlease.Config{
		  BindTTL:             60 * time.Second,
		  GenDriftTolerance:   0,
		   AvailDriftTolerance: false,
	   }
	   engine := overwdlease.NewEngine(store, cfg).WithClock(tc.now)
	  return engine, store, tc
  }

// makeDecision 构造一个通过的屏障决策快照。
  func makeDecision(leaseGen int64, avail string) overwdlease.BarrierDecision {
	   return overwdlease.BarrierDecision{
		  Allowed:   true,
		   LeaseGen:  leaseGen,
		   Available: avail,
	   }
   }

// makeObs 构造一个 availcas 只读观测。
  func makeObs(userID, asset string, leaseGen int64, avail string, at time.Time) overwdlease.LeaseObservation {
	  return overwdlease.LeaseObservation{
		  UserID: userID,
		  Asset:    asset,
		   LeaseGen: leaseGen,
		   Available: avail,
		   ObservedAt: at,
	  }
   }

// issueBind 签发预检绑定（辅助函数）。
  func issueBind(
	  t *testing.T,
	  engine *overwdlease.Engine,
	  userID, asset, amount string,
	  leaseGen int64,
	   avail string,
	   idemKey string,
  ) *overwdlease.IssueBindReply {
	  t.Helper()
	   reply, err := engine.IssueBind(context.Background(), overwdlease.IssueBindRequest{
		   UserID: userID,
		  Asset:  asset,
		   Amount: amount,
		  Decision: makeDecision(leaseGen,
			   avail),
		   IdemKey: idemKey,
	  })
	  if err != nil {
		  t.Fatalf("IssueBind: %v", err)
	  }
	  return reply
  }

// ─── 验收夹具 1：Commit 无 bind_id → ErrNoBind ────────────────────────────

   func TestCommitNoBind(t *testing.T) {
	  engine, _, tc := newTestEngine(t)
	   ctx := context.Background()
	  _, err := engine.CommitBind(ctx, overwdlease.CommitBindRequest{
		   BindID:    "", // 故意不提供 bind_id
		  UserID:    "u1",
		  CommitRef: "wd-001",
		  Current:   makeObs("u1", "BTC", 10, "1.0", tc.now()),
	  })
	  if !errors.Is(err, overwdlease.ErrNoBind) {
		  t.Fatalf("expect ErrNoBind, got %v", err)
	  }
  }

// ─── 验收夹具 2：Commit bind 已过期 → ErrBindExpired ─────────────────────

   func TestCommitExpiredBind(t *testing.T) {
	  engine, _, tc := newTestEngine(t)
	  ctx := context.Background()
	  reply := issueBind(t, engine, "u1", "BTC", "0.5", 10, "1.0", "idem-exp-1")
	  // 推进时钟超过 TTL
	  tc.advance(90 * time.Second)
	  _, err := engine.CommitBind(ctx, overwdlease.CommitBindRequest{
		   BindID:    reply.BindID,
		   UserID:    "u1",
		   CommitRef: "wd-002",
		   Current:   makeObs("u1", "BTC", 10, "1.0", tc.now()),
	  })
	  if !errors.Is(err, overwdlease.ErrBindExpired) {
		  t.Fatalf("expect ErrBindExpired, got %v", err)
	   }
  }

// ─── 验收夹具 3：Commit bind 已吊销 → ErrBindRevoked ─────────────────────

   func TestCommitRevokedBind(t *testing.T) {
	   engine, _, tc := newTestEngine(t)
	  ctx := context.Background()
	   reply := issueBind(t, engine, "u1", "BTC", "0.5", 10, "1.0", "idem-rev-1")
	  if err := engine.RevokeBind(ctx, reply.BindID, "manual revoke"); err != nil {
		  t.Fatalf("RevokeBind: %v", err)
	   }
	  _, err := engine.CommitBind(ctx, overwdlease.CommitBindRequest{
		  BindID: reply.BindID,
		  UserID: "u1",
		  CommitRef: "wd-003",
		   Current: makeObs("u1", "BTC", 10, "1.0", tc.now()),
	   })
	   if !errors.Is(err, overwdlease.ErrBindRevoked) {
		   t.Fatalf("expect ErrBindRevoked, got %v", err)
	  }
   }

// ─── 验收夹具 4：LeaseGen 漂移 → ErrOverdrawBlocked + VarianceProof ───────

   func TestCommitGenDrift(t *testing.T) {
	   engine, store, tc := newTestEngine(t)
	   ctx := context.Background()
	   // PreCheck 时 gen=10，Commit 时 gen=12（漂移 +2）
	   reply := issueBind(t, engine, "u2", "ETH", "1.0", 10, "5.0", "idem-gen-drift")
	   _, err := engine.CommitBind(ctx, overwdlease.CommitBindRequest{
		  BindID:    reply.BindID,
		  UserID:    "u2",
		   CommitRef: "wd-gen-drift",
		  Current:   makeObs("u2", "ETH", 12, "5.0", tc.now()), // gen 漂移到 12
	  })
	   if !errors.Is(err, overwdlease.ErrOverdrawBlocked) {
		   t.Fatalf("expect ErrOverdrawBlocked, got %v", err)
	   }
	   // 验证 VarianceProof 已开启（open 状态）
	   proofs, listErr := store.ListProofsByState(ctx, overwdlease.ProofStateOpen, 10)
	  if listErr != nil {
		   t.Fatalf("ListProofsByState: %v", listErr)
	  }
	  if len(proofs) == 0 {
		   t.Fatal("expect open VarianceProof after gen drift, got none")
	   }
	   p := proofs[0]
	   if p.Trigger != overwdlease.ProofTriggerGenDrift {
		  t.Fatalf("expect trigger=gen_drift, got %s", p.Trigger)
	   }
	  if p.DriftGen != 2 {
		  t.Fatalf("expect drift_gen=2, got %d", p.DriftGen)
	  }
	   if p.BindLeaseGen != 10 || p.CommitGen != 12 {
		  t.Fatalf("expect bind_lease_gen=10 commit_gen=12, got %d/%d", p.BindLeaseGen, p.CommitGen)
	   }
	   // 验证 bind 已被吊销
	   b, bErr := engine.GetBind(ctx, reply.BindID)
	   if bErr != nil {
		  t.Fatalf("GetBind: %v", bErr)
	  }
	  if b.State != overwdlease.BindStateRevoked {
		  t.Fatalf("expect bind state=revoked, got %s", b.State)
	   }
	   // 验证漂移案例已开启
	  cases, _ := store.ListDriftCasesByState(ctx, overwdlease.DriftCaseOpen, 10)
	  if len(cases) == 0 {
		  t.Fatal("expect open DriftCase after gen drift, got none")
	   }
	  if cases[0].Trigger != overwdlease.DriftTriggerGenDrift {
		   t.Fatalf("expect case trigger=gen_drift, got %s", cases[0].Trigger)
	  }
  }

// ─── 验收夹具 5：Available 漂移 → ErrOverdrawBlocked（avail_drift）────────

   func TestCommitAvailDrift(t *testing.T) {
	   engine, store, tc := newTestEngine(t)
	   ctx := context.Background()
	  // PreCheck 时 avail="5.0"，Commit 时 avail="4.5"（漂移）
	   reply := issueBind(t, engine, "u3", "USDT", "0.1", 20, "5.0", "idem-avail-drift")
	  _, err := engine.CommitBind(ctx, overwdlease.CommitBindRequest{
		   BindID: reply.BindID,
		   UserID: "u3",
		   CommitRef: "wd-avail-drift",
		  Current:   makeObs("u3", "USDT", 20, "4.5", tc.now()), // avail 变化，gen 不变
	  })
	  if !errors.Is(err, overwdlease.ErrOverdrawBlocked) {
		  t.Fatalf("expect ErrOverdrawBlocked, got %v", err)
	   }
	   proofs, _ := store.ListProofsByState(ctx, overwdlease.ProofStateOpen, 10)
	  if len(proofs) == 0 {
		  t.Fatal("expect open VarianceProof after avail drift")
	   }
	   if proofs[0].Trigger != overwdlease.ProofTriggerAvailDrift {
		   t.Fatalf("expect trigger=avail_drift, got %s", proofs[0].Trigger)
	   }
  }

// ─── 验收夹具 6：Gen+Avail 同时漂移 → both_drift ─────────────────────────

   func TestCommitBothDrift(t *testing.T) {
	  engine, store, tc := newTestEngine(t)
	  ctx := context.Background()
	  reply := issueBind(t, engine, "u4", "BNB", "2.0", 5, "10.0", "idem-both-drift")
	   _, err := engine.CommitBind(ctx, overwdlease.CommitBindRequest{
		  BindID:    reply.BindID,
		   UserID:    "u4",
		  CommitRef: "wd-both-drift",
		   Current: makeObs("u4", "BNB", 7, "9.0", tc.now()), // gen=7(+2) avail=9.0(changed)
	   })
	  if !errors.Is(err, overwdlease.ErrOverdrawBlocked) {
		   t.Fatalf("expect ErrOverdrawBlocked, got %v", err)
	  }
	  proofs, _ := store.ListProofsByState(ctx, overwdlease.ProofStateOpen, 10)
	   if len(proofs) == 0 {
		  t.Fatal("expect open VarianceProof after both drift")
	  }
	  if proofs[0].Trigger != overwdlease.ProofTriggerBothDrift {
		  t.Fatalf("expect trigger=both_drift, got %s", proofs[0].Trigger)
	  }
   }

// ─── 验收夹具 7：VarianceProof open 期间再次 Commit → ErrProofBlocked ─────

   func TestCommitBlockedByOpenProof(t *testing.T) {
	  engine, _, tc := newTestEngine(t)
	  ctx := context.Background()
	   // 先触发漂移
	   r1 := issueBind(t, engine, "u5", "SOL", "1.0", 100, "50.0", "idem-proof-block-1")
	   _, _ = engine.CommitBind(ctx, overwdlease.CommitBindRequest{
		  BindID: r1.BindID,
		  UserID:  "u5",
		  Current: makeObs("u5", "SOL", 105, "50.0", tc.now()), // gen 漂移
	  })
	   // 重新签发一个 bind 并尝试 Commit（应被 open proof 挡写）
	   tc.advance(1 * time.Second)
	   r2 := issueBind(t, engine, "u5", "SOL", "1.0", 105, "50.0", "idem-proof-block-2")
	   _, err := engine.CommitBind(ctx, overwdlease.CommitBindRequest{
		   BindID: r2.BindID,
		   UserID: "u5",
		   CommitRef: "wd-block-2",
		   Current:   makeObs("u5", "SOL", 105, "50.0", tc.now()),
	   })
	   if !errors.Is(err, overwdlease.ErrProofBlocked) {
		   t.Fatalf("expect ErrProofBlocked, got %v", err)
	  }
   }

// ─── 验收夹具 8：四眼确认 VarianceProof 后挡写解除 ───────────────────────

  func TestConfirmProofUnblocks(t *testing.T) {
	   engine, store, tc := newTestEngine(t)
	   ctx := context.Background()
	   // 触发漂移，开启 VarianceProof
	   r1 := issueBind(t, engine, "u6", "XRP", "100.0", 200, "500.0", "idem-confirm-1")
	  _, _ = engine.CommitBind(ctx, overwdlease.CommitBindRequest{
		   BindID:  r1.BindID,
		   UserID:  "u6",
		   Current: makeObs("u6", "XRP", 201, "500.0", tc.now()),
	   })
	  // 获取 open proof
	   proofs, _ := store.ListProofsByState(ctx, overwdlease.ProofStateOpen, 10)
	   if len(proofs) == 0 {
		  t.Fatal("expect open proof")
	   }
	  proofID := proofs[0].ID
	   // 四眼确认
	   if err := engine.ConfirmProof(ctx, overwdlease.ConfirmProofRequest{
		   ProofID:     proofID,
		  ConfirmedBy: "ops-admin-B",
		  CloseReason: "审计确认：此次漂移为正常更新，非超提",
	  }); err != nil {
		  t.Fatalf("ConfirmProof: %v", err)
	   }
	  // 证明应已关闭
	  p, _ := engine.GetProof(ctx, proofID)
	  if p.State != overwdlease.ProofStateClosed {
		   t.Fatalf("expect proof state=closed, got %s", p.State)
	   }
	   // 挡写解除：新的 Commit 应可通过
	  tc.advance(1 * time.Second)
	  r2 := issueBind(t, engine, "u6", "XRP", "100.0", 201, "500.0", "idem-confirm-2")
	  reply, err := engine.CommitBind(ctx, overwdlease.CommitBindRequest{
		  BindID: r2.BindID,
		   UserID: "u6",
		   CommitRef: "wd-unblocked",
		  Current:   makeObs("u6", "XRP", 201, "500.0", tc.now()),
	   })
	   if err != nil {
		   t.Fatalf("CommitBind after unblock: %v", err)
	  }
	  if reply.State != overwdlease.BindStateCommitted {
		   t.Fatalf("expect committed, got %s", reply.State)
	   }
   }

// ─── 验收夹具 9：正常路径（无漂移）→ committed ────────────────────────────

   func TestCommitNormalPath(t *testing.T) {
	   engine, _, tc := newTestEngine(t)
	  ctx := context.Background()
	   reply := issueBind(t, engine, "u7", "BTC", "0.01", 50, "3.5", "idem-normal-1")
	   commitReply, err := engine.CommitBind(ctx, overwdlease.CommitBindRequest{
		   BindID:    reply.BindID,
		  UserID:    "u7",
		  CommitRef: "wd-normal-1",
		  Current: makeObs("u7", "BTC", 50, "3.5", tc.now()), // gen 和 avail 完全一致
	  })
	   if err != nil {
		   t.Fatalf("CommitBind normal path: %v", err)
	  }
	  if commitReply.State != overwdlease.BindStateCommitted {
		  t.Fatalf("expect committed, got %s", commitReply.State)
	  }
	  if commitReply.LeaseGen != 50 {
		   t.Fatalf("expect lease_gen=50, got %d", commitReply.LeaseGen)
	  }
   }

// ─── 验收夹具 10：IssueBind 幂等 ─────────────────────────────────────────

   func TestIssueBindIdempotent(t *testing.T) {
	  engine, _, _ := newTestEngine(t)
	   ctx := context.Background()
	  idemKey := "idem-duplicate-1"
	  r1 := issueBind(t, engine, "u8", "ETH", "2.0", 30, "8.0", idemKey)
	   r2 := issueBind(t, engine, "u8", "ETH", "2.0", 30, "8.0", idemKey)
	   if r1.BindID != r2.BindID {
		  t.Fatalf("idempotent issue: expect same bind_id, got %s vs %s", r1.BindID, r2.BindID)
	   }
	   _ = ctx
  }

// ─── 验收夹具 11：BindExpiryWorker 过期扫描 ──────────────────────────────

  func TestBindExpiryWorker(t *testing.T) {
	   _, store, tc := newTestEngine(t)
	  ctx := context.Background()
	  wcfg := overwdlease.WorkerConfig{Interval: time.Hour, BatchSize: 100}
	   // 手动注入一个 open bind（直接插入 store）
	  now := tc.now()
	   b := &overwdlease.PrecheckBind{
		  ID:     "bind-exp-1",
		  UserID: "u9",
		   Asset:  "LTC",
		  Amount:   "5.0",
		   LeaseGen: 1,
		  ExpectedAvail: "100.0",
		  State: overwdlease.BindStateOpen,
		   IdemKey: "idem-exp-worker-1",
		  PreCheckAt: now.Add(-120 * time.Second),
		  ExpiresAt: now.Add(-30 * time.Second), // 已过期
		   CreatedAt: now.Add(-120 * time.Second),
		  UpdatedAt: now.Add(-120 * time.Second),
	  }
	   if err := store.InsertBind(ctx, b); err != nil {
		  t.Fatalf("InsertBind: %v", err)
	  }
	   worker := overwdlease.NewBindExpiryWorker(store, wcfg)
	  if err := worker.RunOnce(ctx); err != nil {
		  t.Fatalf("BindExpiryWorker.RunOnce: %v", err)
	   }
	   got, _ := store.GetBindByID(ctx, "bind-exp-1")
	   if got.State != overwdlease.BindStateExpired {
		  t.Fatalf("expect expired, got %s", got.State)
	  }
  }

// ─── 验收夹具 12：DriftPatrolWorker 推进 DriftCase proven ─────────────────

   func TestDriftPatrolWorker(t *testing.T) {
	  engine, store, tc := newTestEngine(t)
	   ctx := context.Background()
	  wcfg := overwdlease.WorkerConfig{Interval: time.Hour, BatchSize: 100}
	   // 触发漂移，产出 open DriftCase（含 ProofID）
	  r := issueBind(t, engine, "u10", "DOT", "3.0", 10, "20.0", "idem-patrol-1")
	   _, _ = engine.CommitBind(ctx, overwdlease.CommitBindRequest{
		  BindID:  r.BindID,
		   UserID:  "u10",
		  Current: makeObs("u10", "DOT", 12, "20.0", tc.now()),
	   })
	  // 执行巡检
	   worker := overwdlease.NewDriftPatrolWorker(store, wcfg)
	   if err := worker.RunOnce(ctx); err != nil {
		   t.Fatalf("DriftPatrolWorker.RunOnce: %v", err)
	   }
	  // 验证 DriftCase 已推进至 proven
	  cases, _ := store.ListDriftCasesByState(ctx, overwdlease.DriftCaseProven, 10)
	   if len(cases) == 0 {
		  t.Fatal("expect proven DriftCase after patrol")
	  }
   }

// ─── 验收夹具 13：ProofChaseWorker 扫描 open 证明（不改状态）──────────────

  func TestProofChaseWorker(t *testing.T) {
	   engine, store, tc := newTestEngine(t)
	   ctx := context.Background()
	   wcfg := overwdlease.WorkerConfig{Interval: time.Hour, BatchSize: 100}
	   // 触发漂移，产出 open VarianceProof
	   r := issueBind(t, engine, "u11", "ATOM", "0.5", 100, "100.0", "idem-chase-1")
	  _, _ = engine.CommitBind(ctx, overwdlease.CommitBindRequest{
		   BindID: r.BindID,
		  UserID:  "u11",
		   Current: makeObs("u11", "ATOM", 103, "100.0", tc.now()),
	  })
	  proofsBefore, _ := store.ListProofsByState(ctx, overwdlease.ProofStateOpen, 10)
	   if len(proofsBefore) == 0 {
		  t.Fatal("expect open proof before chase worker")
	  }
	  notified := 0
	  worker := overwdlease.NewProofChaseWorker(store, wcfg, func(p *overwdlease.VarianceProof) error {
		  notified++
		  return nil
	  })
	  if err := worker.RunOnce(ctx); err != nil {
		  t.Fatalf("ProofChaseWorker.RunOnce: %v", err)
	   }
	  if notified == 0 {
		  t.Fatal("expect notify callback to be called")
	  }
	  // 证明状态应不变（still open；chase 不关闭证明）
	   proofsAfter,
		  _ := store.ListProofsByState(ctx, overwdlease.ProofStateOpen, 10)
	  if len(proofsAfter) != len(proofsBefore) {
		  t.Fatalf("proof count changed unexpectedly: before=%d after=%d", len(proofsBefore), len(proofsAfter))
	  }
  }

// ─── 验收夹具 15：ConfirmDriftCase open 时返回 ErrProofMissing ────────────

   func TestConfirmDriftCaseRequiresProven(t *testing.T) {
	   engine, store, tc := newTestEngine(t)
	   ctx := context.Background()
	  // 手动插入一个 no_bind 类型的 open 案例（没有对应 proof）
	  now := tc.now()
	   dc := &overwdlease.OverwdDriftCase{
		   ID: "case-no-proof-1",
		  UserID: "u12",
		  Asset:  "BTC",
		   Trigger: overwdlease.DriftTriggerNoBind,
		   State: overwdlease.DriftCaseOpen,
		   BlockedAt: now,
		  IdemKey:   "idem-no-proof-case-1",
		  CreatedAt: now,
		  UpdatedAt: now,
	   }
	  if err := store.InsertDriftCase(ctx, dc); err != nil {
		   t.Fatalf("InsertDriftCase: %v", err)
	   }
	  err := engine.ConfirmDriftCase(ctx, overwdlease.ConfirmDriftCaseRequest{
		   CaseID: "case-no-proof-1",
		  ConfirmedBy: "ops-admin",
		  CloseReason: "test",
	  })
	  if !errors.Is(err, overwdlease.ErrProofMissing) {
		   t.Fatalf("expect ErrProofMissing for open case, got %v", err)
	  }
   }

// ─── 验收夹具 16：DismissDriftCase 无需证明直接关闭 ──────────────────────

  func TestDismissDriftCase(t *testing.T) {
	   engine,
		  store, tc := newTestEngine(t)
	   ctx := context.Background()
	  now := tc.now()
	  dc := &overwdlease.OverwdDriftCase{
		   ID:     "case-dismiss-1",
		   UserID: "u13",
		  Asset:   "ETH",
		  Trigger: overwdlease.DriftTriggerNoBind,
		   State:   overwdlease.DriftCaseOpen,
		  BlockedAt: now,
		   IdemKey:   "idem-dismiss-case-1",
		   CreatedAt: now,
		  UpdatedAt: now,
	  }
	   if err := store.InsertDriftCase(ctx, dc); err != nil {
		  t.Fatalf("InsertDriftCase: %v", err)
	  }
	   if err := engine.DismissDriftCase(ctx, "case-dismiss-1", "ops-admin", "false alarm"); err != nil {
		   t.Fatalf("DismissDriftCase: %v", err)
	  }
	   got, _ := engine.GetDriftCase(ctx, "case-dismiss-1")
	   if got.State != overwdlease.DriftCaseClosed {
		   t.Fatalf("expect closed, got %s", got.State)
	  }
  }

// ─── 验收夹具 17：CommitBind 已消费后重复调用幂等 ─────────────────────────

  func TestCommitBindIdempotentConsumed(t *testing.T) {
	  engine, _, tc := newTestEngine(t)
	   ctx := context.Background()
	  reply := issueBind(t, engine, "u14", "BTC", "0.1", 1, "10.0", "idem-consumed-1")
	  _, err := engine.CommitBind(ctx, overwdlease.CommitBindRequest{
		   BindID: reply.BindID,
		   UserID: "u14",
		  CommitRef: "wd-consumed-1",
		   Current:   makeObs("u14", "BTC", 1, "10.0", tc.now()),
	  })
	  if err != nil {
		   t.Fatalf("first commit: %v", err)
	   }
	  // 第二次 commit 同一 bind_id 应返回幂等 committed（非 error）
	  r2, err := engine.CommitBind(ctx, overwdlease.CommitBindRequest{
		   BindID: reply.BindID,
		   UserID:    "u14",
		  CommitRef: "wd-consumed-1",
		   Current:   makeObs("u14", "BTC", 1, "10.0", tc.now()),
	   })
	  if err != nil {
		   t.Fatalf("second commit (idempotent): %v", err)
	   }
	   if r2.State != overwdlease.BindStateCommitted {
		  t.Fatalf("expect committed, got %s", r2.State)
	   }
  }

// ─── 验收夹具 18：RevokeBind 手动吊销 ────────────────────────────────────

  func TestRevokeBind(t *testing.T) {
	   engine, _, _ := newTestEngine(t)
	   ctx := context.Background()
	   reply := issueBind(t, engine, "u15", "SOL", "5.0", 77, "200.0", "idem-revoke-1")
	  if err := engine.RevokeBind(ctx, reply.BindID, "admin manual revoke"); err != nil {
		   t.Fatalf("RevokeBind: %v", err)
	   }
	   b, _ := engine.GetBind(ctx, reply.BindID)
	   if b.State != overwdlease.BindStateRevoked {
		  t.Fatalf("expect revoked, got %s", b.State)
	  }
	   if b.RevokeReason == "" {
		   t.Fatal("expect revoke reason non-empty")
	  }
  }

// ─── 验收夹具 19：ExportProof 摘要字段完整性 ─────────────────────────────

  func TestExportProof(t *testing.T) {
	   engine, _, tc := newTestEngine(t)
	   ctx := context.Background()
	   r := issueBind(t, engine, "u16", "AVAX", "3.0", 50, "300.0", "idem-export-1")
	   _, _ = engine.CommitBind(ctx, overwdlease.CommitBindRequest{
		   BindID: r.BindID,
		   UserID:  "u16",
		  Current: makeObs("u16", "AVAX", 53, "300.0", tc.now()),
	  })
	   // 获取 proof id
	  p, _ := engine.GetOpenProofByUser(ctx, "u16", "AVAX")
	  if p == nil {
		   t.Fatal("expect open proof")
	  }
	  export, err := engine.ExportProof(ctx, p.ID)
	   if err != nil {
		  t.Fatalf("ExportProof: %v", err)
	  }
	   if export.ProofID == "" {
		  t.Fatal("export.ProofID empty")
	  }
	  if export.BindID != r.BindID {
		  t.Fatalf("export.BindID mismatch: want %s got %s", r.BindID, export.BindID)
	  }
	   if export.DriftGen != 3 {
		  t.Fatalf("export.DriftGen: want 3, got %d", export.DriftGen)
	   }
	  if export.ProofHash == "" {
		   t.Fatal("export.ProofHash empty")
	  }
	  if export.State != "open" {
		  t.Fatalf("export.State: want open, got %s", export.State)
	   }
   }

// ─── 验收夹具 20：竞态场景（并发 PreCheck 后扣减 Available 再 Commit）──────
// 两个并发请求都通过 PreCheck（看到 avail=10.0），但在 Commit 前
// 某个 Commit 已先将 available 更新到 9.0；后续 Commit 应被漂移检测拒绝。

   func TestConcurrentPreCheckAvailDrift(t *testing.T) {
	  engine, _, tc := newTestEngine(t)
	  ctx := context.Background()
	   // 两个请求在相同时间点 PreCheck（都看到 avail=10.0, gen=100）
	  r1 := issueBind(t, engine, "u17", "BTC", "1.0", 100, "10.0", "idem-conc-1")
	  tc.advance(10 * time.Millisecond)
	   r2 := issueBind(t, engine, "u17", "BTC", "1.0", 100, "10.0", "idem-conc-2")
	   // r1 Commit 通过（gen 和 avail 一致）
	  tc.advance(100 * time.Millisecond)
	  _, err1 := engine.CommitBind(ctx, overwdlease.CommitBindRequest{
		   BindID:    r1.BindID,
		   UserID:    "u17",
		  CommitRef: "wd-conc-1",
		   Current:   makeObs("u17", "BTC", 100, "10.0", tc.now()),
	  })
	   if err1 != nil {
		  t.Fatalf("r1 commit should succeed: %v", err1)
	   }
	   // r1 Commit 后 availcas Available 已变（gen 未必变，avail 减少）
	  // r2 Commit 时 avail=9.0（r1 已占用 1.0）
	  tc.advance(50 * time.Millisecond)
	  _, err2 := engine.CommitBind(ctx, overwdlease.CommitBindRequest{
		  BindID:    r2.BindID,
		  UserID:    "u17",
		  CommitRef: "wd-conc-2",
		  Current:   makeObs("u17", "BTC", 100, "9.0", tc.now()), // avail 已被 r1 占用减少
	   })
	  if !errors.Is(err2, overwdlease.ErrOverdrawBlocked) {
		   t.Fatalf("r2 commit should be blocked by avail drift, got %v", err2)
	   }
   }

// ─── 多场景表驱动 ─────────────────────────────────────────────────────
// TestCommitBindTable 多场景表驱动（覆盖各种 bind 状态与漂移组合）。
   func TestCommitBindTable(t *testing.T) {
	   type tcase struct {
		  name    string
		  bindGen int64
		   bindAvail string
		   commitGen   int64
		   commitAvail string
		  ttlElapsed  time.Duration
		   revoke      bool
		  wantErr   error
		  wantState overwdlease.BindState
	  }
	   cases := []tcase{
		   {
			   name: "正常提交（无漂移）",
			  bindGen: 10, bindAvail: "5.0",
			  commitGen: 10, commitAvail: "5.0",
			  wantErr:   nil,
			   wantState: overwdlease.BindStateCommitted,
		  },
		  {
			   name: "gen 漂移 +1",
			   bindGen: 10, bindAvail: "5.0",
			   commitGen: 11, commitAvail: "5.0",
			  wantErr: overwdlease.ErrOverdrawBlocked,
			  wantState: overwdlease.BindStateRevoked,
		  },
		   {
			   name: "gen 漂移 -1（回退）",
			   bindGen: 10, bindAvail: "5.0",
			  commitGen: 9, commitAvail: "5.0",
			   wantErr: overwdlease.ErrOverdrawBlocked, wantState: overwdlease.BindStateRevoked,
		   },
		  {
			  name: "avail 微小漂移",
			  bindGen: 10, bindAvail: "5.0",
			   commitGen: 10, commitAvail: "4.99",
			   wantErr: overwdlease.ErrOverdrawBlocked, wantState: overwdlease.BindStateRevoked,
		  },
		   {
			  name:    "bind TTL 超时",
			   bindGen: 10, bindAvail: "5.0",
			  commitGen: 10, commitAvail: "5.0",
			  ttlElapsed: 90 * time.Second,
			  wantErr: overwdlease.ErrBindExpired, wantState: overwdlease.BindStateExpired,
		  },
		   {
			  name:    "bind 已手动吊销",
			  bindGen: 10, bindAvail: "5.0",
			  commitGen: 10, commitAvail: "5.0",
			  revoke: true,
			  wantErr: overwdlease.ErrBindRevoked, wantState: overwdlease.BindStateRevoked,
		   },
	   }
	  for i, tc := range cases {
		   tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  engine, _, clock := newTestEngine(t)
			   ctx := context.Background()
			   idemKey := fmt.Sprintf("idem-table-%d", i)
			  reply := issueBind(t, engine, "utbl", "BTC", "1.0",
				   tc.bindGen,
				   tc.bindAvail, idemKey)
			  if tc.ttlElapsed > 0 {
				  clock.advance(tc.ttlElapsed)
			   }
			  if tc.revoke {
				  _ = engine.RevokeBind(ctx, reply.BindID, "table test revoke")
			   }
			  commitReply, err := engine.CommitBind(ctx, overwdlease.CommitBindRequest{
				  BindID:    reply.BindID,
				  UserID:    "utbl",
				   CommitRef: "wd-table-" + tc.name,
				   Current:   makeObs("utbl", "BTC", tc.commitGen, tc.commitAvail, clock.now()),
			   })
			   if tc.wantErr != nil {
				  if !errors.Is(err, tc.wantErr) {
					  t.Fatalf("want err %v, got %v", tc.wantErr, err)
				  }
			   } else {
				  if err != nil {
					   t.Fatalf("unexpected error: %v", err)
				   }
				   if commitReply.State != tc.wantState {
					  t.Fatalf("want state %s, got %s", tc.wantState, commitReply.State)
				  }
			   }
			   // 校验 bind 最终状态
			   b, _ := engine.GetBind(ctx, reply.BindID)
			  if b.State != tc.wantState {
				  t.Fatalf("bind state: want %s, got %s", tc.wantState, b.State)
			  }
		  })
	  }
  }
