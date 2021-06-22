package flow

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/xferclose"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// buildTestOrchestrator 构造携带成功执行函数的编排器。

   func buildTestOrchestrator(t *testing.T, store xferclose.Store, sla *xferclose.SLABoard) *xferclose.Orchestrator {
	   t.Helper()
	   advance := func(_ context.Context, clientOid string) error { return nil }
	  rollback := func(_ context.Context, hopID, idem string) (string, error) {
		  return "rb-" + idem, nil
	   }
	   backfill := func(_ context.Context, hopID, asset, toBook, userID, idem string) (string, error) {
		  return "bf-" + idem, nil
	  }
	  incident := func(_ context.Context, hopID, reason string) (string, error) {
		   return "inc-" + hopID, nil
	  }
	   return xferclose.NewOrchestrator(store, sla, advance, rollback, backfill, incident)
   }

// injectAndBreach 注入工单并立刻标记 SLA 超限。

   func injectAndBreach(t *testing.T, detector *xferclose.Detector, store *xferclose.MemStore,
	   hopID, userID, asset, from, to string, amt decimal.Decimal, kind xferclose.HopKind) xferclose.HalfHop {
	   t.Helper()
	   ctx := context.Background()
	   hop,
		   err := detector.InjectHalfHop(ctx, hopID, userID, asset, from, to, amt, kind)
	   if err != nil {
		  t.Fatalf("inject: %v", err)
	   }
	   // 标记已超限。
	   hop.SLABreached = true
	   _ = store.UpdateHop(ctx, hop)
	  updated, _ := store.GetByHopID(ctx, hopID)
	   return updated
  }

// TestCloseout_RollbackEgress 回滚出腿收口路径。
  func TestCloseout_RollbackEgress(t *testing.T) {
	   store := xferclose.NewMemStore()
	  sla := xferclose.NewSLABoard()
	   sla.Replace([]xferclose.XferSLA{
		  {
			   ID:           "sla-rollback",
			   Timeout:      100 * time.Millisecond,
			   AutoCloseout: true,
			  DefaultAction: xferclose.ActionRollbackEgress,
			   Enabled:       true,
			  Priority: 1,
		  },
	  })
	  detector := xferclose.NewDetector(store, &stubXferSource{}, sla)
	  detector.WithMinAge(0)
	  orch := buildTestOrchestrator(t, store, sla)
	   ctx := context.Background()
	   hopID := uuid.NewString()
	  hop := injectAndBreach(t, detector, store, hopID, "user-r1", "USDT",
		  "funding", "trading", decimal.NewFromInt(200), xferclose.HopKindStranded)
	   result, err := orch.Closeout(ctx, hop, xferclose.ActionRollbackEgress, "system")
	   if err != nil {
		  t.Fatalf("Closeout rollback: %v", err)
	   }
	   if !result.Success {
		   t.Errorf("expected success, got: %s", result.Error)
	   }
	  if result.Action != xferclose.ActionRollbackEgress {
		  t.Errorf("action mismatch: %s", result.Action)
	   }
	  // hop 状态应为 closed。
	   updated, _ := store.GetByHopID(ctx, hopID)
	  if updated.State != xferclose.HopClosed {
		  t.Errorf("expected hop closed, got %s", updated.State)
	   }
	   // plan 状态应为 done。
	   plan, found, _ := store.GetPlanByHopID(ctx, hopID)
	  if !found {
		  t.Fatal("plan not found")
	  }
	  if plan.State != xferclose.PlanDone {
		  t.Errorf("expected plan done, got %s", plan.State)
	  }
   }

// TestCloseout_BackfillIngress 补入腿收口路径。

   func TestCloseout_BackfillIngress(t *testing.T) {
	  store := xferclose.NewMemStore()
	   sla := xferclose.NewSLABoard()
	  sla.Replace([]xferclose.XferSLA{
		   {
			  ID:           "sla-backfill",
			   Timeout:      100 * time.Millisecond,
			   AutoCloseout: true,
			   DefaultAction: xferclose.ActionBackfillIngress,
			   Enabled: true,
			  Priority: 1,
		  },
	  })
	  detector := xferclose.NewDetector(store, &stubXferSource{}, sla)
	  detector.WithMinAge(0)
	  orch := buildTestOrchestrator(t, store, sla)
	  ctx := context.Background()
	   hopID := uuid.NewString()
	   hop := injectAndBreach(t, detector, store, hopID, "user-b1", "USDT",
		   "funding", "earn", decimal.NewFromInt(500), xferclose.HopKindOpen)
	   result, err := orch.Closeout(ctx, hop, xferclose.ActionBackfillIngress, "ops-user")
	  if err != nil {
		  t.Fatalf("Closeout backfill: %v", err)
	   }
	  if !result.Success {
		   t.Errorf("expected success, got: %s", result.Error)
	  }
	  // 验证腿状态。
	  plan, _, _ := store.GetPlanByHopID(ctx, hopID)
	  legs, _ := store.ListLegs(ctx, plan.ID)
	   if len(legs) == 0 {
		  t.Fatal("no legs found")
	  }
	   var backfillLeg *xferclose.CloseoutLeg
	  for i := range legs {
		   if legs[i].Action == xferclose.ActionBackfillIngress {
			   backfillLeg = &legs[i]
		   }
	  }
	   if backfillLeg == nil {
		  t.Fatal("backfill leg not found")
	   }
	  if backfillLeg.State != xferclose.LegDone {
		  t.Errorf("expected leg done, got %s", backfillLeg.State)
	  }
	  if backfillLeg.EntryRef == "" {
		   t.Error("entry_ref should be set after backfill")
	   }
   }

// TestCloseout_AlreadyTerminal 已终态 hop 重入收口返回 ErrAlreadyTerminal。

   func TestCloseout_AlreadyTerminal(t *testing.T) {
	   store := xferclose.NewMemStore()
	   sla := xferclose.NewSLABoard()
	   sla.Replace([]xferclose.XferSLA{{
		   ID: "sla-x", Timeout: time.Second, DefaultAction: xferclose.ActionRollbackEgress,
		  Enabled: true, Priority: 1,
	  }})
	  detector := xferclose.NewDetector(store, &stubXferSource{}, sla)
	  detector.WithMinAge(0)
	  orch := buildTestOrchestrator(t, store, sla)
	  ctx := context.Background()
	  hopID := uuid.NewString()
	  hop := injectAndBreach(t, detector, store, hopID, "user-t1", "USDT",
		  "funding", "trading", decimal.NewFromInt(100), xferclose.HopKindTimeout)
	  // 第一次收口成功。
	   _, err := orch.Closeout(ctx, hop, xferclose.ActionRollbackEgress, "system")
	  if err != nil {
		  t.Fatalf("first closeout: %v", err)
	   }
	  // 重新取 hop（状态已变更）。
	   closedHop, _ := store.GetByHopID(ctx, hopID)
	   _, err2 := orch.Closeout(ctx, closedHop, xferclose.ActionRollbackEgress, "system")
	   if !errors.Is(err2, xferclose.ErrAlreadyTerminal) {
		   t.Errorf("expected ErrAlreadyTerminal, got %v", err2)
	  }
   }

// TestCloseout_BackfillIdempotent 同一 hop 重复 backfill_ingress 不得双补入。
   func TestCloseout_BackfillIdempotent(t *testing.T) {
	   store := xferclose.NewMemStore()
	   sla := xferclose.NewSLABoard()
	   sla.Replace([]xferclose.XferSLA{{
		  ID: "sla-bf2", Timeout: time.Millisecond, DefaultAction: xferclose.ActionBackfillIngress,
		  Enabled: true, Priority: 1,
	  }})
	   detector := xferclose.NewDetector(store, &stubXferSource{}, sla)
	  detector.WithMinAge(0)
	   backfillCount := 0
	   backfill := func(_ context.Context, hopID, asset, toBook, userID, idem string) (string, error) {
		   backfillCount++
		  return "bf-" + idem, nil
	  }
	  orch := xferclose.NewOrchestrator(store, sla,
		  func(_ context.Context, _ string) error { return nil },
		  func(_ context.Context, _, _ string) (string, error) { return "rb", nil },
		  backfill,
		  func(_ context.Context, _, _ string) (string, error) { return "inc", nil },
	   )
	  ctx := context.Background()
	  hopID := uuid.NewString()
	   hop := injectAndBreach(t, detector, store, hopID, "user-idem1", "USDT",
		   "funding", "earn", decimal.NewFromInt(100), xferclose.HopKindOpen)
	   // 第一次收口。
	  r1, err := orch.Closeout(ctx, hop, xferclose.ActionBackfillIngress, "system")
	   if err != nil {
		   t.Fatalf("first closeout: %v", err)
	  }
	   if !r1.Success {
		  t.Error("first closeout should succeed")
	  }
	  // 第二次对同一个已 closed hop 尝试收口。
	   closedHop, _ := store.GetByHopID(ctx, hopID)
	  _, err2 := orch.Closeout(ctx, closedHop, xferclose.ActionBackfillIngress, "system")
	  if !errors.Is(err2, xferclose.ErrAlreadyTerminal) {
		   t.Logf("second closeout err: %v", err2)
	   }
	  // 补入只应发生 1 次。
	   if backfillCount != 1 {
		   t.Errorf("expected backfill called 1 time, got %d", backfillCount)
	  }
  }

// TestCloseout_ForceCloseout 运维强制收口（空 actor 被拒绝）。

  func TestCloseout_ForceCloseout(t *testing.T) {
	  store := xferclose.NewMemStore()
	   sla := xferclose.NewSLABoard()
	  sla.Replace([]xferclose.XferSLA{{
		   ID: "sla-fc", Timeout: time.Millisecond, DefaultAction: xferclose.ActionRollbackEgress,
		   Enabled: true, Priority: 1,
	   }})
	   detector := xferclose.NewDetector(store, &stubXferSource{}, sla)
	   detector.WithMinAge(0)
	   orch := buildTestOrchestrator(t, store, sla)
	   ctx := context.Background()
	  hopID := uuid.NewString()
	  hop := injectAndBreach(t, detector, store, hopID, "user-fc1", "USDT",
		   "funding", "trading", decimal.NewFromInt(300), xferclose.HopKindStranded)
	  _ = hop
	   // 空 actor 被拒绝。
	  _, err := orch.ForceCloseout(ctx, hopID, "", xferclose.ActionRollbackEgress)
	  if !errors.Is(err, xferclose.ErrActorRequired) {
		  t.Errorf("expected ErrActorRequired, got %v", err)
	   }
	   // 有效 actor 成功。
	  result, err2 := orch.ForceCloseout(ctx, hopID, "ops-admin", xferclose.ActionRollbackEgress)
	   if err2 != nil {
		   t.Fatalf("force closeout: %v", err2)
	  }
	   if !result.Success {
		  t.Error("expected success")
	  }
   }

// TestCloseout_BatchAutoCloseout 批量自动收口扫描 SLA 超限工单。

  func TestCloseout_BatchAutoCloseout(t *testing.T) {
	   store := xferclose.NewMemStore()
	  sla := xferclose.NewSLABoard()
	  sla.Replace([]xferclose.XferSLA{{
		   ID: "sla-batch", Timeout: time.Millisecond, AutoCloseout: true,
		  DefaultAction: xferclose.ActionRollbackEgress, Enabled: true, Priority: 1,
	   }})
	  detector := xferclose.NewDetector(store, &stubXferSource{}, sla)
	  detector.WithMinAge(0)
	   orch := buildTestOrchestrator(t,
		   store, sla)
	  ctx := context.Background()
	   for i := 0; i < 3; i++ {
		   hopID := uuid.NewString()
		   hop, _ := detector.InjectHalfHop(ctx, hopID, "user-batch", "USDT",
			   "funding", "trading", decimal.NewFromInt(int64(i+10)), xferclose.HopKindOpen)
		   hop.SLABreached = true
		  _ = store.UpdateHop(ctx, hop)
	  }
	   succ, fail, err := orch.RunBatchCloseout(ctx, 10)
	  if err != nil {
		   t.Fatalf("batch closeout: %v", err)
	   }
	  if succ+fail != 3 {
		   t.Errorf("expected 3 processed, got succ=%d fail=%d", succ, fail)
	   }
  }
