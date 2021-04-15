package dualrelease_test

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/dualrelease"
)

// ──────────────────────────────────────────────────
// 辅助
// ──────────────────────────────────────────────────
// fixedClock 返回可控时钟。

   func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

// futureTicket 返回 TTL 远在未来的 TicketFacts。
   func futureTicket(requestID string) dualrelease.TicketFacts {
	   return dualrelease.TicketFacts{
		  TicketID:  "ticket-" + requestID,
		  RequestID: requestID,
		  EpochAt:   1,
		   TTL:       time.Now().UTC().Add(24 * time.Hour),
		  OpenedBy: "ops-alice",
	   }
  }

// expiredTicket 返回 TTL 已超期的 TicketFacts。
   func expiredTicket(requestID string) dualrelease.TicketFacts {
	  return dualrelease.TicketFacts{
		   TicketID:  "ticket-" + requestID,
		   RequestID: requestID,
		  EpochAt:   1,
		  TTL:       time.Now().UTC().Add(-1 * time.Hour),
		  OpenedBy:  "ops-alice",
	  }
   }

// buildEngine 快速构造带 FullMemStore 的 Engine。

   func buildEngine(t *testing.T, leaseTTL time.Duration) (*dualrelease.Engine, *dualrelease.FullMemStore) {
	   t.Helper()
	  store := dualrelease.NewFullMemStore()
	  engine := dualrelease.NewEngine(store, leaseTTL)
	  return engine, store
   }

// openAndLock 快速完成 OpenCase + AcquireLease + ConfirmUnlock（推进到 executing）。
   func openAndLock(t *testing.T, engine *dualrelease.Engine, requestID, actor, holder string) (*dualrelease.ReleaseCase, *dualrelease.ReleaseLease) {
	   t.Helper()
	   facts := futureTicket(requestID)
	  rc, err := engine.OpenCase(facts, actor)
	  if err != nil {
		  t.Fatalf("OpenCase: %v", err)
	  }
	  lease, err := engine.AcquireLease(requestID, holder)
	   if err != nil {
		  t.Fatalf("AcquireLease: %v", err)
	   }
	   rc, err = engine.ConfirmUnlock(requestID, lease.Gen, lease.LeaseToken)
	   if err != nil {
		   t.Fatalf("ConfirmUnlock: %v", err)
	  }
	  return rc, lease
   }

// ──────────────────────────────────────────────────
// 场景 1：Unlock 失败后 Execute 被拒，无资金副作用
// ──────────────────────────────────────────────────

  func TestUnlockFailExecuteRejected(t *testing.T) {
	  engine, store := buildEngine(t, 5*time.Minute)
	   requestID := "req-unlock-fail"
	   facts := futureTicket(requestID)
	   // 开案例。
	  rc, err := engine.OpenCase(facts, "ops-alice")
	   if err != nil {
		  t.Fatalf("OpenCase: %v", err)
	  }
	  if rc.State != dualrelease.StateReady {
		  t.Fatalf("expected ready, got %s", rc.State)
	  }
	  // 抢租约（ready→unlocking）。
	   lease, err := engine.AcquireLease(requestID, "worker-a")
	  if err != nil {
		  t.Fatalf("AcquireLease: %v", err)
	   }
	   if lease.Gen != 1 {
		  t.Fatalf("expected gen=1, got %d", lease.Gen)
	  }
	   // HSM Unlock 失败，回退。
	   rc, err = engine.RevertUnlock(requestID, lease.Gen)
	   if err != nil {
		   t.Fatalf("RevertUnlock: %v", err)
	   }
	   if rc.State != dualrelease.StateReady {
		   t.Fatalf("after revert: expected ready, got %s", rc.State)
	  }
	  // Execute 应被拒（State!=executing）。
	  _, _, execErr := engine.Execute(dualrelease.ExecuteParams{
		   RequestID: requestID,
		  Actor:       "ops-bob",
		  Gen:         1,
		  TicketFacts: facts,
		  FundObs: dualrelease.FundObservation{FundActionID: "fund-001"},
	   })
	  if execErr == nil {
		   t.Fatal("expected error, got nil")
	  }
	  if !errors.Is(execErr, dualrelease.ErrBadState) {
		   t.Fatalf("expected ErrBadState, got: %v", execErr)
	  }
	   // 验证无 EffectProof。
	  _, proofErr := store.GetProof(requestID, 1)
	  if proofErr == nil {
		   t.Fatal("proof should not exist after failed unlock")
	   }
	   // 验证案例未标 executed。
	   rc2, _ := store.GetCase(requestID)
	  if rc2.State == dualrelease.StateExecuted {
		   t.Fatal("case must not be executed after unlock fail")
	  }
	   if rc2.FundActionID != "" {
		  t.Fatalf("FundActionID should be empty, got %q", rc2.FundActionID)
	   }
   }

// ──────────────────────────────────────────────────
// 场景 2：并发 Execute 世代竞态
// ──────────────────────────────────────────────────

  func TestConcurrentExecuteGenRace(t *testing.T) {
	   const concurrency = 5
	   engine,
		  _ := buildEngine(t, 5*time.Minute)
	  requestID := "req-concurrent-exec"
	   // 推进到 executing。
	  _, lease := openAndLock(t, engine, requestID, "ops-alice", "worker-main")
	  var (
		  winCount int64
		  staleCount int64
		   otherCount int64
		   wg         sync.WaitGroup
	   )
	   for i := 0; i < concurrency; i++ {
		  wg.Add(1)
		  go func(idx int) {
			   defer wg.Done()
			  actor := fmt.Sprintf("ops-worker-%d", idx)
			  _, _, err := engine.Execute(dualrelease.ExecuteParams{
				   RequestID: requestID,
				  Actor:     actor,
				   Gen: lease.Gen,
				   LeaseToken:  lease.LeaseToken,
				   TicketFacts: futureTicket(requestID),
				  FundObs: dualrelease.FundObservation{
					   FundActionID: fmt.Sprintf("fund-%d", idx),
					  ObservedAt:   time.Now().UTC(),
				  },
			  })
			   if err == nil {
				   atomic.AddInt64(&winCount, 1)
			  } else if errors.Is(err, dualrelease.ErrStaleGen) || errors.Is(err, dualrelease.ErrCaseClosed) {
				  atomic.AddInt64(&staleCount, 1)
			   } else {
				  atomic.AddInt64(&otherCount,
					  1)
			   }
		  }(i)
	   }
	   wg.Wait()
	   // 恰好一个赢家。
	   if winCount != 1 {
		   t.Fatalf("expected 1 winner, got %d (stale=%d, other=%d)", winCount, staleCount, otherCount)
	   }
	  if staleCount+winCount != concurrency {
		  t.Fatalf("winners+stale should equal concurrency %d, got win=%d stale=%d", concurrency, winCount, staleCount)
	   }
	   if otherCount != 0 {
		  t.Fatalf("unexpected errors: %d", otherCount)
	  }
   }

// ──────────────────────────────────────────────────
// 场景 3：过期票证 Execute → ErrTicketExpired + 审计
// ──────────────────────────────────────────────────

  func TestExpiredTicketExecuteRejected(t *testing.T) {
	   engine, store := buildEngine(t, 5*time.Minute)
	   requestID := "req-expired-ticket"
	   // 开案例时用有效票证（TTL 未过期）。
	  validFacts := futureTicket(requestID)
	  _, _ = engine.OpenCase(validFacts, "ops-alice")
	   lease, _ := engine.AcquireLease(requestID, "worker-a")
	  engine.ConfirmUnlock(requestID, lease.Gen, lease.LeaseToken)
	   // Execute 时使用过期票证。
	   expiredFacts := expiredTicket(requestID)
	   _, _, execErr := engine.Execute(dualrelease.ExecuteParams{
		   RequestID: requestID,
		  Actor:      "ops-carol",
		   Gen:        lease.Gen,
		   LeaseToken: lease.LeaseToken,
		  TicketFacts: expiredFacts,
		   FundObs:     dualrelease.FundObservation{FundActionID: "fund-exp-001"},
	  })
	   if !errors.Is(execErr, dualrelease.ErrTicketExpired) {
		  t.Fatalf("expected ErrTicketExpired, got: %v", execErr)
	   }
	   // 验证 TicketExpiry 审计写入。
	  expiries := store.ListExpiries()
	  if len(expiries) == 0 {
		  t.Fatal("TicketExpiry audit record must be written")
	   }
	  exp := expiries[0]
	   if exp.TicketID != expiredFacts.TicketID {
		   t.Fatalf("expiry ticket_id mismatch: got %s want %s", exp.TicketID, expiredFacts.TicketID)
	  }
	  if exp.Reason != dualrelease.ReasonTTLExceeded {
		  t.Fatalf("expiry reason mismatch: got %s want %s", exp.Reason, dualrelease.ReasonTTLExceeded)
	  }
	   if exp.AuditHash == "" {
		  t.Fatal("audit_hash must be non-empty")
	   }
	   if exp.Actor != "ops-carol" {
		   t.Fatalf("expiry actor mismatch: got %s want %s", exp.Actor, "ops-carol")
	  }
	  // 案例应被标 expired。
	  rc, _ := store.GetCase(requestID)
	   if rc.State != dualrelease.StateExpired {
		  t.Fatalf("case state: expected expired, got %s", rc.State)
	  }
	   // 无 EffectProof。
	   _, proofErr := store.GetProof(requestID, lease.Gen)
	  if proofErr == nil {
		  t.Fatal("proof must not exist after expired ticket")
	   }
   }

// ──────────────────────────────────────────────────
// 场景 4：EffectProof 缺失不标 executed
// ──────────────────────────────────────────────────

  func TestMissingProofNotExecuted(t *testing.T) {
	   // 使用 errorStore PutProof 失败。
	  store := &errorProofStore{MemStore: dualrelease.NewMemStore()}
	  engine := dualrelease.NewEngine(store, 5*time.Minute)
	  requestID := "req-missing-proof"
	   // 推进到 executing。
	   facts := futureTicket(requestID)
	   engine.OpenCase(facts, "ops-alice")
	   lease, _ := engine.AcquireLease(requestID, "worker-a")
	  engine.ConfirmUnlock(requestID, lease.Gen, lease.LeaseToken)
	   // Execute 时 PutProof 失败。
	   _, _, execErr := engine.Execute(dualrelease.ExecuteParams{
		  RequestID: requestID,
		   Actor:     "ops-dave",
		  Gen:       lease.Gen,
		   TicketFacts: facts,
		  FundObs: dualrelease.FundObservation{FundActionID: "fund-mp-001"},
	   })
	  if execErr == nil {
		   t.Fatal("expected error when PutProof fails, got nil")
	   }
	   // 案例不应标 executed。
	   rc, _ := store.GetCase(requestID)
	  if rc == nil {
		   t.Fatal("case should exist")
	  }
	  if rc.State == dualrelease.StateExecuted {
		   t.Fatalf("case must not be executed when proof write fails, got %s", rc.State)
	   }
	   // EffectAlignWorker 因 GetProof 返回错误也不标 executed。
	   fullStore := dualrelease.NewFullMemStore()
	  // 复制案例到 fullStore 执行中状态。
	   rc.State = dualrelease.StateExecuting
	   fullStore.PutCase(rc)
	  alignWorker := dualrelease.NewEffectAlignWorker(
		   dualrelease.NewEngine(fullStore, 5*time.Minute),
		   fullStore,
		   dualrelease.WorkerConfig{Interval: time.Second, BatchSize: 10},
	  )
	  _ = alignWorker.RunOnce()
	   rc2, _ := fullStore.GetCase(requestID)
	   if rc2.State == dualrelease.StateExecuted {
		   t.Fatal("EffectAlignWorker must not mark executed when proof is missing")
	   }
   }

// errorProofStore 是 dualrelease.Store 的包装，PutProof 始终返回错误。

type errorProofStore struct {
	*dualrelease.MemStore
}

  func (e *errorProofStore) PutProof(_ *dualrelease.EffectProof) error {
	   return fmt.Errorf("simulated proof write failure")
  }

// ──────────────────────────────────────────────────
// 场景 5：ForceAbort 双人校验
// ──────────────────────────────────────────────────

  func TestForceAbortTwoActors(t *testing.T) {
	   engine, store := buildEngine(t, 5*time.Minute)
	  requestID := "req-force-abort"
	  engine.OpenCase(futureTicket(requestID), "ops-alice")
	  // 相同 actor 拒绝。
	   _, err := engine.ForceAbort(requestID, "ops-alice", "ops-alice", "test")
	   if err == nil {
		  t.Fatal("same actor force abort should fail")
	  }
	  // 不同 actor 成功。
	   rc, err := engine.ForceAbort(requestID, "ops-alice", "ops-bob", "emergency")
	  if err != nil {
		  t.Fatalf("ForceAbort: %v", err)
	   }
	  if rc.State != dualrelease.StateAborted {
		  t.Fatalf("expected aborted, got %s", rc.State)
	   }
	   if rc.AbortReason == "" {
		   t.Fatal("abort_reason must be set")
	  }
	  // 再次中止已终结案例 → ErrCaseClosed。
	  _, err = engine.ForceAbort(requestID, "ops-carol", "ops-dave", "again")
	  if !errors.Is(err, dualrelease.ErrCaseClosed) {
		   t.Fatalf("expected ErrCaseClosed, got: %v", err)
	   }
	   // 验证租约被回收。
	  l, _ := store.GetLease(requestID)
	   if l != nil && time.Now().Before(l.ExpiresAt) {
		   t.Fatal("lease should be expired after force abort")
	   }
   }

// ──────────────────────────────────────────────────
// 场景 6：状态机合法/非法转换
// ──────────────────────────────────────────────────

   func TestFSMTransitionTable(t *testing.T) {
	  cases := []struct {
		   from dualrelease.ReleaseState
		   to   dualrelease.ReleaseState
		   wantOK bool
	  }{
		   {dualrelease.StateReady,
			   dualrelease.StateUnlocking, true},
		  {dualrelease.StateUnlocking, dualrelease.StateExecuting, true},
		   {dualrelease.StateUnlocking, dualrelease.StateReady, true},
		   {dualrelease.StateExecuting, dualrelease.StateExecuted, true},
		   {dualrelease.StateExecuting,
			   dualrelease.StateExpired, true},
		  {dualrelease.StateReady, dualrelease.StateAborted, true},
		   {dualrelease.StateUnlocking, dualrelease.StateAborted, true},
		   {dualrelease.StateExecuting, dualrelease.StateAborted, true},
		   {dualrelease.StateReady, dualrelease.StateExpired, true},
		  // 非法转换。
		  {dualrelease.StateExecuted, dualrelease.StateReady, false},
		   {dualrelease.StateAborted, dualrelease.StateExecuting, false},
		   {dualrelease.StateExpired,
			   dualrelease.StateExecuted, false},
		   {dualrelease.StateReady, dualrelease.StateExecuted, false},
		   {dualrelease.StateExecuting,
			  dualrelease.StateUnlocking, false},
	   }
	  for _, tc := range cases {
		  got := dualrelease.ValidTransition(tc.from, tc.to)
		   if got != tc.wantOK {
			   t.Errorf("ValidTransition(%s→%s): got %v, want %v", tc.from, tc.to, got, tc.wantOK)
		   }
	   }
   }

// ──────────────────────────────────────────────────
// 场景 7：TicketAgingWorker 批量标 expired
// ──────────────────────────────────────────────────

   func TestTicketAgingWorkerRunOnce(t *testing.T) {
	   store := dualrelease.NewFullMemStore()
	  now := time.Now().UTC()
	   // 注入过去时刻时钟，使所有租约立即过期。
	  engine := dualrelease.NewEngine(store, 1*time.Nanosecond).WithClock(fixedClock(now))
	  for i := 0; i < 3; i++ {
		   reqID := fmt.Sprintf("req-aging-%d", i)
		   facts := dualrelease.TicketFacts{
			  TicketID:  "ticket-" + reqID,
			  RequestID: reqID,
			   EpochAt:   1,
			   TTL:       now.Add(24 * time.Hour),
			   OpenedBy: "ops-alice",
		  }
		  engine.OpenCase(facts, "ops-alice")
		  engine.AcquireLease(reqID, "worker-aging")
	   }
	  // 推进时钟超过租约 TTL（1ns）。
	   futureEngine := dualrelease.NewEngine(store, 1*time.Nanosecond).
		  WithClock(fixedClock(now.Add(time.Hour)))
	   worker := dualrelease.NewTicketAgingWorker(futureEngine, dualrelease.WorkerConfig{
		   Interval:  time.Second,
		  BatchSize: 10,
	   })
	  if err := worker.RunOnce(); err != nil {
		  t.Fatalf("RunOnce: %v", err)
	  }
	   // 所有案例应已标 expired。
	  expiries := store.ListExpiries()
	   if len(expiries) < 3 {
		  t.Fatalf("expected at least 3 expiry audits, got %d", len(expiries))
	  }
   }

// ──────────────────────────────────────────────────
// 场景 8：MutexReclaimWorker 回收过期租约
// ──────────────────────────────────────────────────

  func TestMutexReclaimWorkerRunOnce(t *testing.T) {
	  store := dualrelease.NewFullMemStore()
	  now := time.Now().UTC()
	  engine := dualrelease.NewEngine(store, 1*time.Nanosecond).WithClock(fixedClock(now))
	   reqID := "req-reclaim"
	  facts := futureTicket(reqID)
	  engine.OpenCase(facts, "ops-alice")
	   engine.AcquireLease(reqID, "worker-stale")
	  // 此时 State=unlocking，租约立即过期（TTL=1ns）。
	  futureEngine := dualrelease.NewEngine(store, 1*time.Nanosecond).
		  WithClock(fixedClock(now.Add(time.Hour)))
	  worker := dualrelease.NewMutexReclaimWorker(futureEngine, store, dualrelease.WorkerConfig{
		   Interval: time.Second,
		   BatchSize: 10,
	  })
	   if err := worker.RunOnce(); err != nil {
		  t.Fatalf("RunOnce: %v", err)
	   }
	   rc, _ := store.GetCase(reqID)
	   if rc.State != dualrelease.StateReady {
		   t.Fatalf("expected ready after reclaim, got %s", rc.State)
	   }
   }

// ──────────────────────────────────────────────────
// 场景 9：EffectAlignWorker 补偿半截状态
// ──────────────────────────────────────────────────

  func TestEffectAlignWorkerCompensates(t *testing.T) {
	   store := dualrelease.NewFullMemStore()
	   engine := dualrelease.NewEngine(store, 5*time.Minute)
	  reqID := "req-align"
	   facts := futureTicket(reqID)
	  engine.OpenCase(facts, "ops-alice")
	   lease, _ := engine.AcquireLease(reqID, "worker-align")
	  engine.ConfirmUnlock(reqID, lease.Gen, lease.LeaseToken)
	  // 手动写 EffectProof 但不翻案例状态（半截写）。
	  proof := &dualrelease.EffectProof{
		   ID: "proof-align-001",
		  RequestID: reqID,
		   TicketID:  facts.TicketID,
		  Gen:          lease.Gen,
		  FundActionID: "fund-align-001",
		   Actor:        "ops-alice",
		   ProofHash: dualrelease.ComputeProofHash(reqID, facts.TicketID, "fund-align-001", lease.Gen, "ops-alice"),
		  SignedAt:  time.Now().UTC(),
		  CreatedAt: time.Now().UTC(),
	   }
	  store.PutProof(proof)
	   // 案例仍为 executing（未翻）。
	  rc, _ := store.GetCase(reqID)
	  if rc.State != dualrelease.StateExecuting {
		   t.Fatalf("expected executing, got %s", rc.State)
	  }
	  // 运行 EffectAlignWorker。
	   worker := dualrelease.NewEffectAlignWorker(engine, store, dualrelease.WorkerConfig{
		   Interval: time.Second,
		  BatchSize: 10,
	  })
	  if err := worker.RunOnce(); err != nil {
		   t.Fatalf("RunOnce: %v", err)
	   }
	  // 案例应被补偿为 executed。
	   rc, _ = store.GetCase(reqID)
	   if rc.State != dualrelease.StateExecuted {
		   t.Fatalf("expected executed after align, got %s", rc.State)
	   }
	   if rc.FundActionID != "fund-align-001" {
		  t.Fatalf("FundActionID mismatch: got %s", rc.FundActionID)
	  }
   }

// ──────────────────────────────────────────────────
// 场景 10：OpenCase 幂等性
// ──────────────────────────────────────────────────

   func TestOpenCaseIdempotent(t *testing.T) {
	   engine, _ := buildEngine(t, 5*time.Minute)
	   reqID := "req-idempotent"
	  facts := futureTicket(reqID)
	  rc1, err := engine.OpenCase(facts, "ops-alice")
	   if err != nil {
		   t.Fatalf("first OpenCase: %v", err)
	   }
	   rc2, err := engine.OpenCase(facts, "ops-alice")
	  if err != nil {
		  t.Fatalf("second OpenCase: %v", err)
	   }
	  if rc1.ID != rc2.ID {
		   t.Fatalf("idempotent OpenCase must return same ID: %s != %s", rc1.ID, rc2.ID)
	   }
  }

// ──────────────────────────────────────────────────
// 场景 11：ProofHash 确定性
// ──────────────────────────────────────────────────

   func TestProofHashDeterministic(t *testing.T) {
	   h1 := dualrelease.ComputeProofHash("req-1", "ticket-1", "fund-1", 42, "actor-x")
	   h2 := dualrelease.ComputeProofHash("req-1", "ticket-1", "fund-1", 42, "actor-x")
	   if h1 != h2 {
		  t.Fatalf("proof hash must be deterministic: %s != %s", h1, h2)
	  }
	   // 不同输入产生不同哈希。
	  h3 := dualrelease.ComputeProofHash("req-2", "ticket-1", "fund-1", 42, "actor-x")
	   if h1 == h3 {
		   t.Fatal("different inputs must produce different proof hash")
	  }
  }

// ──────────────────────────────────────────────────
// 场景 12：actor 校验
// ──────────────────────────────────────────────────

   func TestActorValidation(t *testing.T) {
	  engine,
		  _ := buildEngine(t, 5*time.Minute)
	  facts := futureTicket("req-actor-val")
	  invalidActors := []string{"", "system", "  ", "SYSTEM"}
	   for _, a := range invalidActors {
		   _, err := engine.OpenCase(facts, a)
		   if !errors.Is(err, dualrelease.ErrActorInvalid) {
			   t.Errorf("actor %q: expected ErrActorInvalid, got %v", a, err)
		   }
	   }
   }

// ──────────────────────────────────────────────────
// 场景 13：gen CAS 连续抢占（多轮）
// ──────────────────────────────────────────────────

   func TestMultiRoundGenCAS(t *testing.T) {
	   engine,
		  store := buildEngine(t, 5*time.Minute)
	   reqID := "req-multi-round"
	   facts := futureTicket(reqID)
	  engine.OpenCase(facts, "ops-alice")
	  for round := int64(1); round <= 3; round++ {
		  // 抢租约（gen 递增）。
		  lease, err := engine.AcquireLease(reqID, fmt.Sprintf("worker-%d", round))
		  if err != nil {
			  t.Fatalf("round %d AcquireLease: %v", round, err)
		  }
		  if lease.Gen != round {
			  t.Fatalf("round %d: expected gen=%d, got %d", round, round, lease.Gen)
		   }
		   // Unlock 成功→Execute→Executed。
		   engine.ConfirmUnlock(reqID, lease.Gen, lease.LeaseToken)
		  _, rc, err := engine.Execute(dualrelease.ExecuteParams{
			  RequestID: reqID,
			  Actor: fmt.Sprintf("ops-worker-%d", round),
			  Gen:         lease.Gen,
			  LeaseToken:  lease.LeaseToken,
			   TicketFacts: futureTicket(reqID),
			  FundObs: dualrelease.FundObservation{
				  FundActionID: fmt.Sprintf("fund-%d", round),
				   ObservedAt:   time.Now().UTC(),
			   },
		   })
		  if round == 1 {
			  // 第一轮应成功。
			  if err != nil {
				   t.Fatalf("round 1 Execute: %v", err)
			   }
			   if rc.State != dualrelease.StateExecuted {
				   t.Fatalf("round 1: expected executed, got %s", rc.State)
			  }
			  // 后续轮次：案例已 closed，OpenCase 返回 ErrCaseClosed。
			   _, openErr := engine.OpenCase(facts, "ops-alice")
			   if !errors.Is(openErr, dualrelease.ErrCaseClosed) {
				   t.Fatalf("round 2+: expected ErrCaseClosed on re-open, got: %v", openErr)
			   }
			  break
		   }
	   }
	  // 最终 proof 存在。
	   proof, err := store.GetProof(reqID, 1)
	  if err != nil {
		  t.Fatalf("GetProof: %v", err)
	   }
	  if proof.FundActionID == "" {
		   t.Fatal("proof FundActionID must not be empty")
	  }
   }

// ──────────────────────────────────────────────────
// 场景 14：AuditHash 确定性
// ──────────────────────────────────────────────────

  func TestAuditHashDeterministic(t *testing.T) {
	   now := time.Date(2026, 9, 21, 20, 0, 0, 0, time.UTC)
	   h1 := dualrelease.ComputeAuditHash("t1", "r1", dualrelease.ReasonTTLExceeded, 42, now)
	   h2 := dualrelease.ComputeAuditHash("t1", "r1", dualrelease.ReasonTTLExceeded, 42, now)
	   if h1 != h2 {
		  t.Fatal("audit hash must be deterministic")
	  }
	  h3 := dualrelease.ComputeAuditHash("t2", "r1", dualrelease.ReasonTTLExceeded, 42, now)
	  if h1 == h3 {
		   t.Fatal("different ticket must produce different audit hash")
	   }
  }

// ──────────────────────────────────────────────────
// 场景 15：StateSummary 覆盖所有状态
// ──────────────────────────────────────────────────

  func TestStateSummaryAll(t *testing.T) {
	   states := []dualrelease.ReleaseState{
		   dualrelease.StateReady,
		   dualrelease.StateUnlocking,
		  dualrelease.StateExecuting,
		   dualrelease.StateExecuted,
		  dualrelease.StateExpired,
		   dualrelease.StateAborted,
	  }
	   for _, s := range states {
		   summary := dualrelease.StateSummary(s)
		  if summary == "" {
			   t.Errorf("StateSummary(%s) returned empty", s)
		   }
	   }
  }

// ──────────────────────────────────────────────────
// 场景 16：Closed 状态不可操作
// ──────────────────────────────────────────────────

   func TestClosedStatePreventsOps(t *testing.T) {
	  closedStates := []dualrelease.ReleaseState{
		   dualrelease.StateExecuted,
		  dualrelease.StateExpired,
		   dualrelease.StateAborted,
	   }
	   for _, s := range closedStates {
		  if !s.Closed() {
			   t.Errorf("state %s should be closed", s)
		  }
	   }
	   openStates := []dualrelease.ReleaseState{
		   dualrelease.StateReady,
		  dualrelease.StateUnlocking,
		   dualrelease.StateExecuting,
	  }
	  for _, s := range openStates {
		   if s.Closed() {
			  t.Errorf("state %s should not be closed", s)
		   }
	   }
  }
