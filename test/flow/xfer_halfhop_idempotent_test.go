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

// TestIdempotent_RollbackTwice 同 hop 两次 rollback 幂等（只执行一次 rollback）。

   func TestIdempotent_RollbackTwice(t *testing.T) {
	  store := xferclose.NewMemStore()
	   sla := xferclose.NewSLABoard()
	  sla.Replace([]xferclose.XferSLA{{
		  ID: "sla-rb2", Timeout: time.Millisecond, DefaultAction: xferclose.ActionRollbackEgress,
		  AutoCloseout: true, Enabled: true, Priority: 1,
	   }})
	   rollbackCount := 0
	  rollback := func(_ context.Context, hopID, idem string) (string, error) {
		   rollbackCount++
		   return "rb-" + idem, nil
	  }
	  orch := xferclose.NewOrchestrator(store, sla,
		  func(_ context.Context,
			  _ string) error {
			  return nil
		  },
		   rollback,
		  func(_ context.Context, _, _, _, _, _ string) (string, error) { return "bf", nil },
		  func(_ context.Context, _, _ string) (string, error) { return "inc", nil },
	  )
	   detector := xferclose.NewDetector(store, &stubXferSource{}, sla)
	   detector.WithMinAge(0)
	   ctx := context.Background()
	   hopID := uuid.NewString()
	  hop, err := detector.InjectHalfHop(ctx, hopID, "user-rb-idem", "USDT",
		   "funding", "trading", decimal.NewFromInt(100), xferclose.HopKindStranded)
	  if err != nil {
		  t.Fatalf("inject: %v", err)
	  }
	   hop.SLABreached = true
	   _ = store.UpdateHop(ctx, hop)
	  hop, _ = store.GetByHopID(ctx, hopID)
	   // 第一次收口。
	   r1, err1 := orch.Closeout(ctx, hop, xferclose.ActionRollbackEgress, "system")
	   if err1 != nil {
		  t.Fatalf("first closeout: %v", err1)
	  }
	   if !r1.Success {
		   t.Error("first closeout should succeed")
	   }
	  // 第二次：hop 已 closed，应返回 ErrAlreadyTerminal。
	  hop2, _ := store.GetByHopID(ctx, hopID)
	  _, err2 := orch.Closeout(ctx, hop2, xferclose.ActionRollbackEgress, "system")
	   if !errors.Is(err2, xferclose.ErrAlreadyTerminal) {
		   t.Errorf("expected ErrAlreadyTerminal on second closeout, got %v", err2)
	  }
	  // rollback 函数只应被调用 1 次。
	   if rollbackCount != 1 {
		  t.Errorf("rollback called %d times, expected 1", rollbackCount)
	   }
   }

// TestIdempotent_BackfillTwice 同 hop 两次 backfill_ingress 幂等（只执行一次补入）。
  func TestIdempotent_BackfillTwice(t *testing.T) {
	   store := xferclose.NewMemStore()
	   sla := xferclose.NewSLABoard()
	  sla.Replace([]xferclose.XferSLA{{
		  ID: "sla-bf2", Timeout: time.Millisecond, DefaultAction: xferclose.ActionBackfillIngress,
		   AutoCloseout: true,
		  Enabled:      true, Priority: 1,
	  }})
	  backfillCount := 0
	  backfill := func(_ context.Context, _, _, _, _, idem string) (string, error) {
		  backfillCount++
		  return "bf-" + idem, nil
	   }
	  orch := xferclose.NewOrchestrator(store, sla,
		   func(_ context.Context, _ string) error { return nil },
		   func(_ context.Context, _, _ string) (string, error) { return "rb", nil },
		   backfill,
		   func(_ context.Context, _, _ string) (string, error) { return "inc", nil },
	  )
	   detector := xferclose.NewDetector(store, &stubXferSource{}, sla)
	  detector.WithMinAge(0)
	  ctx := context.Background()
	  hopID := uuid.NewString()
	  hop, _ := detector.InjectHalfHop(ctx, hopID, "user-bf-idem", "USDT",
		  "funding", "earn", decimal.NewFromInt(300), xferclose.HopKindOpen)
	  hop.SLABreached = true
	  _ = store.UpdateHop(ctx, hop)
	   hop, _ = store.GetByHopID(ctx, hopID)
	   // 第一次收口（成功补入）。
	   r1, err1 := orch.Closeout(ctx, hop, xferclose.ActionBackfillIngress, "system")
	  if err1 != nil {
		   t.Fatalf("first closeout: %v", err1)
	  }
	  if !r1.Success {
		   t.Error("first closeout should succeed")
	   }
	   // 第二次：hop 已 closed。
	  hop2, _ := store.GetByHopID(ctx, hopID)
	   _, err2 := orch.Closeout(ctx, hop2, xferclose.ActionBackfillIngress, "system")
	  if !errors.Is(err2, xferclose.ErrAlreadyTerminal) {
		  t.Logf("second closeout returned: %v", err2)
	  }
	  // backfill 函数只被调用 1 次。
	   if backfillCount != 1 {
		  t.Errorf("backfill called %d times, expected 1", backfillCount)
	   }
   }

// TestIdempotent_LegIdemKey 腿幂等键唯一性验证——同 idem_key 的腿 PutLeg 返回 ErrIdempotent。
  func TestIdempotent_LegIdemKey(t *testing.T) {
	  store := xferclose.NewMemStore()
	  ctx := context.Background()
	  planID := xferclose.GeneratePlanID()
	   idemKey := "leg:plan-test:rollback_egress:1"
	  leg := xferclose.CloseoutLeg{
		  ID:     uuid.NewString(),
		   PlanID: planID,
		   HopID:  "hop-test",
		  SeqNo:  1,
		   Action: xferclose.ActionRollbackEgress,
		   State:  xferclose.LegPending,
		   IdemKey: idemKey,
	   }
	  if err := store.PutLeg(ctx, leg); err != nil {
		  t.Fatalf("first PutLeg: %v", err)
	   }
	   // 第二次相同 idem_key。
	  leg2 := leg
	  leg2.ID = uuid.NewString()
	  err := store.PutLeg(ctx, leg2)
	   if !errors.Is(err, xferclose.ErrIdempotent) {
		   t.Errorf("expected ErrIdempotent, got %v", err)
	  }
   }

// TestIdempotent_MirrorCreditedTwice 镜像 MarkCredited 幂等（第二次调用返回已有记录）。
   func TestIdempotent_MirrorCreditedTwice(t *testing.T) {
	  machine,
		  _ := newTestMirrorMachine()
	  ctx := context.Background()
	   hopID := uuid.NewString()
	  _, _ = machine.Create(ctx, hopID, "u1", "u2", "USDT", "funding", "earn",
		  decimal.NewFromInt(100))
	  mv1, err1 := machine.MarkCredited(ctx, hopID, "e1", "plan1")
	  if err1 != nil {
		  t.Fatalf("first MarkCredited: %v", err1)
	   }
	  // 第二次：已 credited，应幂等返回（不报错）。
	  mv2, err2 := machine.MarkCredited(ctx, hopID, "e1", "plan1")
	  if err2 != nil {
		   t.Fatalf("second MarkCredited: %v", err2)
	   }
	  // 两次代次相同（幂等，代次不再递增）。
	   if mv1.Version != mv2.Version {
		   t.Errorf("version should not change on idempotent call: v1=%d v2=%d", mv1.Version, mv2.Version)
	  }
	  _ = mv1
   }

// TestIdempotent_PlanStore_ConflictHopID 同一 hopID 不能有两个 open/running 计划。
  func TestIdempotent_PlanStore_ConflictHopID(t *testing.T) {
	   store := xferclose.NewMemStore()
	  ctx := context.Background()
	  hopID := uuid.NewString()
	  p1 := xferclose.CloseoutPlan{
		   ID: uuid.NewString(),
		   HopID:     hopID,
		   HalfHopID: uuid.NewString(),
		   State:     xferclose.PlanOpen,
		  SelectedAction: xferclose.ActionRollbackEgress,
		  Actor: "system",
	  }
	   if err := store.PutPlan(ctx, p1); err != nil {
		   t.Fatalf("first PutPlan: %v", err)
	  }
	  p2 := p1
	   p2.ID = uuid.NewString()
	   err := store.PutPlan(ctx, p2)
	   if !errors.Is(err, xferclose.ErrPlanRunning) {
		   t.Errorf("expected ErrPlanRunning, got %v", err)
	  }
   }
