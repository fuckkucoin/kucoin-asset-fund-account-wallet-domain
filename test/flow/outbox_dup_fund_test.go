/*
 * 出箱资金副作用双花封堵引擎 — 端对端集成流夹具。
 * 场景：
 * 1. 同一 business_key 连续两次成功 handler：仅一次 applied；第二次 DupProof 可查；
 * 2. Leader 切换导致 in-flight 重投：世代 CAS 拒绝旧世代；账本意图不重复；
 * 3. handler 抛错不 Mark：再 Tick 仍可见，但成功一次后不得双花；
 * 4. 毒消息进入隔离：不产生 credit/freeze 意图；desk 可双人放行重放；
 * 5. 非资金 kind 旁路：不写效果仓，不产生 DupProof；
 * 6. Webhook 重试双投：仅一个投递产生账本意图；
 * 7. 同 business_key 多 kind 隔离：credit_intent 与 freeze_intent 互不干扰。
 * 注意：本文件仅做内存级集成（不依赖数据库），验证引擎业务逻辑正确性。
 * 数据库级集成需在 CI 环境配置 DSN 后运行 TestIntegration_* 系列。
 * 作者：Kinsley、Draven、Knox、Blaise
 * 日期：2021.05.30
 */
package flow

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/fxonce"
	"account-assets.kucoin.net/funding/internal/outeffect"
)

// ─── 辅助 ────────────────────────────────────────────────────────────

type testHarness struct {
	store fxonce.Store
	gw      *fxonce.FundEffectGateway
	engine  *fxonce.CASEngine
	adapter *outeffect.Adapter
	worker *fxonce.Worker
	workerCfg fxonce.WorkerConfig
}

  func newHarness(t *testing.T) *testHarness {
	   t.Helper()
	   store := fxonce.NewMemStore()
	  registry := fxonce.NewPolicyRegistry()
	   gw := fxonce.NewFundEffectGateway(store, registry)
	  engine := fxonce.NewCASEngine(store, registry)
	   adapter := outeffect.NewAdapter(gw, nil)
	  cfg := fxonce.WorkerConfig{
		   InspectInterval:      5 * time.Millisecond,
		  ProposedTimeout:      2 * time.Second,
		   MaxRetryBeforePoison: 3,
		  GenLagThreshold: 5,
		   ReaperBatchSize:     100,
		   DupProofReviewLimit: 20,
	  }
	   worker := fxonce.NewWorker(store, engine, cfg, nil)
	   return &testHarness{
		  store: store, gw: gw, engine: engine,
		   adapter: adapter,
		   worker:  worker, workerCfg: cfg,
	  }
   }

// processSuccess handler 成功投递。

   func (h *testHarness) processSuccess(ctx context.Context, businessKey, kind string, gen int64) (outeffect.ProcessResult, error) {
	  return h.adapter.Process(ctx, outeffect.HandlerResult{
		  BusinessKey: businessKey,
		  Kind: kind,
		   Generation: gen,
		   Payload:    fmt.Sprintf(`{"gen":%d,"bk":%q}`, gen, businessKey),
		  Success: true,
	  })
  }

// processNack handler 失败（非 rate limit）。

   func (h *testHarness) processNack(ctx context.Context, businessKey, kind string, gen int64) error {
	   _, err := h.adapter.Process(ctx, outeffect.HandlerResult{
		  BusinessKey: businessKey,
		  Kind:       kind,
		   Generation: gen,
		   Success:    false,
		  ErrCode:    fxonce.RejectHandlerError,
	  })
	   return err
  }

// ────────────────────────────────────────────────────────────────────────────
// 场景 1: 同一 business_key 连续两次成功 handler
// ────────────────────────────────────────────────────────────────────────────

  func TestFlow_DupTickSameKey_OnlyOneApplied(t *testing.T) {
	  t.Parallel()
	  h := newHarness(t)
	  ctx := context.Background()
	  // 第一次投递（gen=1）→ applied
	   r1, err := h.processSuccess(ctx, "dep-flow-dup-001", "credit_intent", 1)
	  if err != nil {
		   t.Fatalf("first delivery: %v", err)
	   }
	  if !r1.Applied {
		   t.Fatalf("first delivery: want Applied=true, got %+v", r1)
	  }
	   // 第二次投递（gen=1 重投）→ idempotent
	  r2, err := h.processSuccess(ctx, "dep-flow-dup-001", "credit_intent", 1)
	   if err != nil {
		  t.Fatalf("second delivery (idempotent): %v", err)
	  }
	   if !r2.Idempotent {
		   t.Errorf("second delivery: want Idempotent=true, got %+v", r2)
	   }
	  // 不应有 DupProof（同世代幂等不产生双花证明）
	   dups, _ := h.store.ListDupProofsForBusiness(ctx, "dep-flow-dup-001", 10)
	  if len(dups) != 0 {
		  t.Errorf("idempotent delivery: must not produce DupProof, got %d", len(dups))
	  }
	  t.Logf("scenario 1 passed: r1.Applied=%v r2.Idempotent=%v dup_count=%d", r1.Applied, r2.Idempotent, len(dups))
  }

// ────────────────────────────────────────────────────────────────────────────
// 场景 2: Leader 切换 — 旧世代 stale_gen，账本意图不重复
// ────────────────────────────────────────────────────────────────────────────

   func TestFlow_LeaderFailover_Stalegen_NoDuplicate(t *testing.T) {
	   t.Parallel()
	  h := newHarness(t)
	  ctx := context.Background()
	   // Leader A 以 gen=2 成功 apply
	   r_leaderA, err := h.processSuccess(ctx, "dep-flow-leader-001", "credit_intent", 2)
	  if err != nil || !r_leaderA.Applied {
		  t.Fatalf("leader A delivery: applied=%v err=%v", r_leaderA.Applied, err)
	  }
	  // Leader B 切换后，仍持有旧的 gen=1 的出箱事件，再次投递
	  r_leaderB, err := h.processSuccess(ctx, "dep-flow-leader-001", "credit_intent", 1)
	  if err != fxonce.ErrStalegen {
		  t.Fatalf("leader B (stale gen=1): want ErrStalegen, got %v", err)
	  }
	  if !r_leaderB.Rejected {
		   t.Error("leader B (stale): want Rejected=true")
	   }
	   // DupProof 可查
	  dups, _ := h.store.ListDupProofsForBusiness(ctx, "dep-flow-leader-001", 10)
	   if len(dups) == 0 {
		  t.Error("leader failover: DupProof should be created")
	   }
	  if dups[0].WinnerGen != 2 || dups[0].LoserGen != 1 {
		  t.Errorf("DupProof gens: winner=%d loser=%d", dups[0].WinnerGen, dups[0].LoserGen)
	  }
	  // 验证账本意图不重复产生（Applied 只发生一次）
	   effects, _ := h.store.GetEffectsByBusiness(ctx, "dep-flow-leader-001")
	  appliedCount := 0
	  for _, e := range effects {
		   if e.State == fxonce.EffectApplied {
			   appliedCount++
		  }
	   }
	   if appliedCount > 1 {
		   t.Errorf("leader failover: at most 1 applied effect, got %d", appliedCount)
	  }
	  t.Logf("scenario 2 passed: winner_gen=%d loser_gen=%d dup_count=%d", dups[0].WinnerGen, dups[0].LoserGen, len(dups))
  }

// ────────────────────────────────────────────────────────────────────────────
// 场景 3: handler 抛错不 Mark — 再 Tick 可见，成功后不得双花
// ────────────────────────────────────────────────────────────────────────────

  func TestFlow_HandlerError_RetryOK_NoDuplicate(t *testing.T) {
	  t.Parallel()
	   h := newHarness(t)
	  ctx := context.Background()
	   // 首次投递失败（Nack）
	  _ = h.processNack(ctx, "dep-flow-err-001", "credit_intent", 1)
	   // 验证 gen 状态（nacked 或 inflight，依实现）
	   // 再次投递成功（gen=1 重试）
	   r, err := h.processSuccess(ctx, "dep-flow-err-001", "credit_intent", 1)
	   if err != nil {
		  t.Fatalf("retry after error: %v", err)
	   }
	  // 应该 applied（首次 nack 后再成功不违反双花）
	  if !r.Applied && !r.Idempotent {
		   t.Errorf("retry after error: want Applied or Idempotent, got %+v", r)
	   }
	  // 再次 apply（同世代）→ idempotent，不双花
	  r2, err := h.processSuccess(ctx, "dep-flow-err-001", "credit_intent", 1)
	  if err != nil {
		   t.Fatalf("idempotent after success: %v", err)
	  }
	  if !r2.Idempotent {
		  t.Errorf("after success, re-apply: want Idempotent=true, got %+v", r2)
	  }
	   t.Logf("scenario 3 passed: r.Applied=%v r2.Idempotent=%v", r.Applied, r2.Idempotent)
   }

// ────────────────────────────────────────────────────────────────────────────
// 场景 4: 毒消息隔离 — 不产生 credit/freeze 意图；desk 双人放行
// ────────────────────────────────────────────────────────────────────────────

  func TestFlow_PoisonIsolation_NoIntent_DualApprovalRelease(t *testing.T) {
	  t.Parallel()
	  h := newHarness(t)
	  ctx := context.Background()
	  // 先 apply gen=1 → applied
	   r, _ := h.processSuccess(ctx, "dep-flow-poison-001", "credit_intent", 1)
	   if r.Effect == nil {
		   t.Fatal("expected effect after apply")
	  }
	  // 手动毒丸
	   _ = h.store.MarkPoisoned(ctx, r.Effect.ID, 5, time.Now().UTC())
	   // 毒丸状态下再次 apply → 拒绝，不产生账本意图
	  r2, err2 := h.processSuccess(ctx, "dep-flow-poison-001", "credit_intent", 1)
	  if err2 != fxonce.ErrPoisoned {
		   t.Fatalf("poisoned apply: want ErrPoisoned, got %v", err2)
	   }
	   if !r2.Rejected {
		  t.Error("poisoned apply: want Rejected=true")
	   }
	   // Desk 双人放行
	   if err := h.engine.ForceReplay(ctx, r.Effect.ID, "alice+bob"); err != nil {
		  t.Fatalf("ForceReplay: %v", err)
	  }
	   // 放行后 effect 应为 proposed
	  eff, _ := h.store.GetEffectByID(ctx, r.Effect.ID)
	  if eff.State != fxonce.EffectProposed {
		  t.Errorf("after ForceReplay: want proposed, got %q", eff.State)
	   }
	   t.Logf("scenario 4 passed: poison isolated + dual approval released")
   }

// ────────────────────────────────────────────────────────────────────────────
// 场景 5: 非资金 kind 旁路 — 不写效果仓，不产生 DupProof
// ────────────────────────────────────────────────────────────────────────────

   func TestFlow_NonFundKind_BypassNoDupProof(t *testing.T) {
	  t.Parallel()
	   h := newHarness(t)
	   ctx := context.Background()
	  kinds := []string{"audit.log", "notify.email", "deposit.credited", "user.login"}
	  for _, kind := range kinds {
		  r, err := h.processSuccess(ctx, "flow-bypass-001", kind, 1)
		   if err != nil {
			  t.Fatalf("non-fund kind %q: %v", kind, err)
		   }
		   if !r.Bypassed {
			  t.Errorf("non-fund kind %q: want Bypassed=true, got %+v", kind, r)
		   }
		   // 无效果记录
		   effects, _ := h.store.GetEffectsByBusiness(ctx, "flow-bypass-001")
		  if len(effects) != 0 {
			   t.Errorf("non-fund kind %q: must not write effects, got %d", kind, len(effects))
		   }
		  // 无双花证明
		   dups, _ := h.store.ListDupProofsForBusiness(ctx, "flow-bypass-001", 10)
		   if len(dups) != 0 {
			   t.Errorf("non-fund kind %q: must not produce DupProof, got %d", kind, len(dups))
		  }
	  }
	   t.Logf("scenario 5 passed: %d non-fund kinds all bypassed", len(kinds))
   }

// ────────────────────────────────────────────────────────────────────────────
// 场景 6: Webhook 重试双投 — 仅一个产生账本意图
// ────────────────────────────────────────────────────────────────────────────

  func TestFlow_WebhookRetryDualDelivery_OneIntent(t *testing.T) {
	   t.Parallel()
	  h := newHarness(t)
	  ctx := context.Background()
	   const deliveries = 10
	  var applied, idempotent int64
	   // webhook 重试：同一 business_key + gen 被投递 10 次
	   var wg sync.WaitGroup
	  for i := 0; i < deliveries; i++ {
		  wg.Add(1)
		  go func() {
			  defer wg.Done()
			   r, _ := h.processSuccess(ctx, "dep-flow-webhook-001", "freeze_intent", 1)
			   if r.Applied {
				  atomic.AddInt64(&applied, 1)
			   }
			   if r.Idempotent {
				  atomic.AddInt64(&idempotent,
					  1)
			  }
		   }()
	   }
	   wg.Wait()
	  total := applied + idempotent
	  if total != deliveries {
		  t.Errorf("webhook retry: applied+idempotent should equal %d, got %d", deliveries, total)
	   }
	   if applied > 1 {
		  t.Errorf("webhook retry: only 1 should be applied, got %d", applied)
	   }
	   if idempotent != deliveries-1 {
		   t.Errorf("webhook retry: expected %d idempotent, got %d", deliveries-1, idempotent)
	   }
	  t.Logf("scenario 6 passed: applied=%d idempotent=%d", applied, idempotent)
   }

// ────────────────────────────────────────────────────────────────────────────
// 场景 7: 同 business_key 多 kind 隔离
// ────────────────────────────────────────────────────────────────────────────

  func TestFlow_SameBusinessKey_MultiKind_Isolated(t *testing.T) {
	   t.Parallel()
	  h := newHarness(t)
	  ctx := context.Background()
	   // 同一业务键（如出金单 wd-0001）产生 credit_intent + freeze_intent 两种副作用
	  // 两者应独立 applied，互不干扰
	  r_credit, err := h.processSuccess(ctx, "wd-flow-mk-001", "credit_intent", 1)
	   if err != nil || !r_credit.Applied {
		   t.Fatalf("credit_intent: applied=%v err=%v", r_credit.Applied, err)
	   }
	  r_freeze, err := h.processSuccess(ctx, "wd-flow-mk-001", "freeze_intent", 1)
	  if err != nil || !r_freeze.Applied {
		   t.Fatalf("freeze_intent: applied=%v err=%v", r_freeze.Applied, err)
	  }
	   // 两个 effect 应为不同 ID、不同 effect_key
	   if r_credit.MarkResult.Effect == nil || r_freeze.MarkResult.Effect == nil {
		  t.Fatal("both effects should be non-nil")
	   }
	  if r_credit.MarkResult.Effect.ID == r_freeze.MarkResult.Effect.ID {
		   t.Error("credit and freeze effects should have different IDs")
	   }
	   if r_credit.MarkResult.Effect.EffectKey == r_freeze.MarkResult.Effect.EffectKey {
		  t.Error("credit and freeze effects should have different effect_keys")
	   }
	  // 重投 credit_intent → idempotent，不影响 freeze_intent
	  r_credit2, _ := h.processSuccess(ctx, "wd-flow-mk-001", "credit_intent", 1)
	  if !r_credit2.Idempotent {
		  t.Errorf("re-deliver credit_intent: want Idempotent=true, got %+v", r_credit2)
	   }
	   // 无 DupProof（两种 kind 都只有一次成功投递）
	  dups, _ := h.store.ListDupProofsForBusiness(ctx, "wd-flow-mk-001", 10)
	  if len(dups) != 0 {
		   t.Errorf("multi-kind isolated: unexpected DupProof count=%d", len(dups))
	  }
	  t.Logf("scenario 7 passed: credit and freeze effects isolated, no dup proofs")
  }

// ────────────────────────────────────────────────────────────────────────────
// 场景 8: 毒消息隔离 — http_rate_limit 不触发双花路径
// ────────────────────────────────────────────────────────────────────────────

  func TestFlow_HTTPRateLimit_NotFundExactlyOnce(t *testing.T) {
	   t.Parallel()
	   h := newHarness(t)
	  ctx := context.Background()
	   // rate limit 失败
	   r, _ := h.adapter.Process(ctx, outeffect.HandlerResult{
		  BusinessKey: "dep-flow-rl-001",
		  Kind: "credit_intent",
		  Generation: 1,
		  Success: false,
		  ErrCode: fxonce.RejectHTTPRateLimit,
	   })
	   if !r.Rejected {
		  t.Error("rate limit: want Rejected=true")
	   }
	  // 随后成功投递 → applied（rate limit 不阻塞后续重试）
	   r2, err := h.processSuccess(ctx, "dep-flow-rl-001", "credit_intent", 1)
	   if err != nil {
		   t.Fatalf("after rate limit, success: %v", err)
	   }
	  if !r2.Applied {
		   t.Errorf("after rate limit, success: want Applied=true, got %+v", r2)
	  }
	   // 无 DupProof（rate limit 不是双花）
	   dups, _ := h.store.ListDupProofsForBusiness(ctx, "dep-flow-rl-001", 10)
	   if len(dups) != 0 {
		  t.Errorf("rate limit: must not produce DupProof, got %d", len(dups))
	   }
	  t.Logf("scenario 8 passed: rate limit not treated as fund double-spend")
  }

// ────────────────────────────────────────────────────────────────────────────
// 场景 9: Worker 巡检 + DupProof 审阅完整流
// ────────────────────────────────────────────────────────────────────────────

  func TestFlow_WorkerInspect_DupReview_FullCycle(t *testing.T) {
	   t.Parallel()
	   ctx := context.Background()
	  store := fxonce.NewMemStore()
	  registry := fxonce.NewPolicyRegistry()
	   gw := fxonce.NewFundEffectGateway(store,
		  registry)
	   engine := fxonce.NewCASEngine(store, registry)
	   adapter := outeffect.NewAdapter(gw,
		   nil)
	   var warnCount int64
	  var mu sync.Mutex
	   alertSink := func(a fxonce.WorkerAlert) {
		   if a.Kind == "dup_proof_unreviewed" {
			  mu.Lock()
			   warnCount++
			   mu.Unlock()
		  }
	  }
	   cfg := fxonce.WorkerConfig{
		  InspectInterval: 5 * time.Millisecond,
		  ProposedTimeout: 2 * time.Second,
		   MaxRetryBeforePoison: 3,
		  GenLagThreshold: 5,
		   ReaperBatchSize: 100,
		   DupProofReviewLimit: 20,
		   AlertSink: alertSink,
	   }
	  worker := fxonce.NewWorker(store, engine, cfg, nil)
	  // gen=2 先 apply
	  r2, _ := adapter.Process(ctx, outeffect.HandlerResult{
		  BusinessKey: "dep-flow-cycle-001",
		  Kind:        "credit_intent",
		   Generation:  2,
		  Success: true,
	   })
	  _ = r2
	   // gen=1 (stale) → DupProof
	   r1, err := adapter.Process(ctx, outeffect.HandlerResult{
		   BusinessKey: "dep-flow-cycle-001",
		   Kind:        "credit_intent",
		  Generation:  1,
		   Success: true,
	  })
	  if err != fxonce.ErrStalegen || r1.MarkResult.Dup == nil {
		   t.Fatalf("stale gen: err=%v dup=%v", err, r1.MarkResult.Dup)
	   }
	  dupID := r1.MarkResult.Dup.ID
	  // Worker 巡检
	  report, _ := worker.Run(ctx)
	   if report.DupProofsUnreviewed == 0 {
		   t.Error("worker: expected DupProofsUnreviewed > 0")
	   }
	   // Desk 标记已审阅
	   dupSvc := fxonce.NewDupProofService(store)
	   if err := dupSvc.MarkReviewed(ctx, dupID, "desk-reviewer"); err != nil {
		   t.Fatalf("MarkReviewed: %v", err)
	  }
	   // 再次 Worker 巡检：已审阅的不再触发告警
	  mu.Lock()
	  warnCount = 0
	   mu.Unlock()
	  worker.Run(ctx)
	  mu.Lock()
	  wc := warnCount
	  mu.Unlock()
	   if wc != 0 {
		   t.Errorf("after review: expected 0 dup_proof_unreviewed alerts, got %d", wc)
	   }
	   t.Logf("scenario 9 passed: worker inspected, dup reviewed, no more alerts")
	  _ = engine
   }
