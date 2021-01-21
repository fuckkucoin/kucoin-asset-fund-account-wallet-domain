package wrfreeze_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/wrfreeze"
)

// ─── TestAddWave_OK ───────────────────────────────────────────────────────────

  func TestAddWave_OK(t *testing.T) {
	  svc, store, f := makePlanSvc(t)
	  plan := openUserSetPlan(t, svc, []string{"uid-001", "uid-002", "uid-003"})
	  waveSvc := wrfreeze.NewWaveService(store, f, 4)
	   wave, err := waveSvc.AddWave(context.Background(), plan.ID, "tier1", []string{"uid-001", "uid-002"}, "ops_alice")
	  if err != nil {
		  t.Fatalf("AddWave: %v", err)
	   }
	   if wave.SeqNo != 1 {
		   t.Errorf("want seq_no=1, got %d", wave.SeqNo)
	   }
	   if len(wave.TargetUsers) != 2 {
		   t.Errorf("want 2 target users, got %d", len(wave.TargetUsers))
	   }
   }

// ─── TestAddWave_SeqIncrement ─────────────────────────────────────────────────

   func TestAddWave_SeqIncrement(t *testing.T) {
	  svc, store, f := makePlanSvc(t)
	  plan := openUserSetPlan(t, svc, []string{"uid-001", "uid-002", "uid-003"})
	  waveSvc := wrfreeze.NewWaveService(store,
		  f, 4)
	  w1, _ := waveSvc.AddWave(context.Background(), plan.ID, "t1", []string{"uid-001"}, "ops_alice")
	   w2, _ := waveSvc.AddWave(context.Background(), plan.ID, "t2", []string{"uid-002"}, "ops_alice")
	  w3, _ := waveSvc.AddWave(context.Background(), plan.ID, "t3", []string{"uid-003"}, "ops_alice")
	  if w1.SeqNo != 1 || w2.SeqNo != 2 || w3.SeqNo != 3 {
		   t.Errorf("seq_no want 1,2,3 got %d,%d,%d", w1.SeqNo, w2.SeqNo, w3.SeqNo)
	   }
  }

// ─── TestAddWave_NonDraftPlan ─────────────────────────────────────────────────

   func TestAddWave_NonDraftPlan(t *testing.T) {
	  svc, store, f := makePlanSvc(t)
	   plan := openUserSetPlan(t, svc, []string{"uid-001"})
	   waveSvc := wrfreeze.NewWaveService(store, f, 4)
	  // 武装计划后不得再添加波次
	  _, _ = svc.ArmPlan(context.Background(), wrfreeze.ArmPlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	   _, err := waveSvc.AddWave(context.Background(), plan.ID, "t1", []string{"uid-001"}, "ops_alice")
	   if err != wrfreeze.ErrPlanState {
		   t.Errorf("want ErrPlanState, got %v", err)
	  }
   }

// ─── TestRunWave_AllSuccess ───────────────────────────────────────────────────

   func TestRunWave_AllSuccess(t *testing.T) {
	   svc, store, f := makePlanSvc(t)
	   plan := openUserSetPlan(t, svc, []string{"uid-001", "uid-002"})
	  waveSvc := wrfreeze.NewWaveService(store, f, 4)
	  wave, _ := waveSvc.AddWave(context.Background(), plan.ID, "wave1", []string{"uid-001", "uid-002"}, "ops_alice")
	   result,
		   err := waveSvc.RunWave(context.Background(), wave.ID)
	   if err != nil {
		   t.Fatalf("RunWave: %v", err)
	   }
	   if result.Status != wrfreeze.WaveDone {
		  t.Errorf("want done, got %s", result.Status)
	  }
	   if result.FrozenCount != 2 {
		   t.Errorf("want frozen=2, got %d", result.FrozenCount)
	   }
	  // 验证账本冻结位实际设置
	  for _, uid := range []string{"uid-001", "uid-002"} {
		   frozen, _ := f.IsFrozen(context.Background(), uid)
		  if !frozen {
			  t.Errorf("user %s should be frozen", uid)
		  }
	  }
  }

// ─── TestRunWave_IdempotentSkip ───────────────────────────────────────────────

   func TestRunWave_IdempotentSkip(t *testing.T) {
	  svc, store, f := makePlanSvc(t)
	  plan := openUserSetPlan(t, svc, []string{"uid-001"})
	  waveSvc := wrfreeze.NewWaveService(store, f, 4)
	  // 预先冻结 uid-001（幂等）
	  f.setFrozen("uid-001")
	  wave, _ := waveSvc.AddWave(context.Background(), plan.ID, "w1", []string{"uid-001"}, "ops_alice")
	  result, err := waveSvc.RunWave(context.Background(), wave.ID)
	   if err != nil {
		   t.Fatalf("RunWave: %v", err)
	   }
	   if result.Status != wrfreeze.WaveDone {
		   t.Errorf("want done (idempotent skip), got %s", result.Status)
	   }
   }

// ─── TestRunWave_CannotRunTerminalWave ────────────────────────────────────────

  func TestRunWave_CannotRunTerminalWave(t *testing.T) {
	   svc, store, f := makePlanSvc(t)
	  plan := openUserSetPlan(t, svc, []string{"uid-001"})
	  waveSvc := wrfreeze.NewWaveService(store, f, 4)
	  wave, _ := waveSvc.AddWave(context.Background(), plan.ID, "w1", []string{"uid-001"}, "ops_alice")
	   _, _ = waveSvc.RunWave(context.Background(), wave.ID) // first run → done
	   _, err := waveSvc.RunWave(context.Background(), wave.ID) // second run → error
	   if err != wrfreeze.ErrWaveState {
		   t.Errorf("want ErrWaveState, got %v", err)
	  }
   }

// ─── TestAllWavesDone ─────────────────────────────────────────────────────────

  func TestAllWavesDone(t *testing.T) {
	   svc, store, f := makePlanSvc(t)
	   plan := openUserSetPlan(t, svc, []string{"uid-001", "uid-002"})
	  waveSvc := wrfreeze.NewWaveService(store, f, 4)
	  w1, _ := waveSvc.AddWave(context.Background(), plan.ID, "w1", []string{"uid-001"}, "ops_alice")
	  w2, _ := waveSvc.AddWave(context.Background(), plan.ID, "w2", []string{"uid-002"}, "ops_alice")
	  done, _ := waveSvc.AllWavesDone(context.Background(), plan.ID)
	   if done {
		  t.Error("want not done before waves run")
	  }
	   _, _ = waveSvc.RunWave(context.Background(), w1.ID)
	  _, _ = waveSvc.RunWave(context.Background(), w2.ID)
	   done, _ = waveSvc.AllWavesDone(context.Background(), plan.ID)
	  if !done {
		   t.Error("want done after all waves run")
	  }
  }

// ─── TestAdvancePlan_Serial ───────────────────────────────────────────────────

   func TestAdvancePlan_Serial(t *testing.T) {
	   svc, store, f := makePlanSvc(t)
	  users := []string{"uid-001", "uid-002"}
	  plan := openUserSetPlan(t, svc, users)
	   // 在武装前添加波次（draft 状态才允许）
	  waveSvc := wrfreeze.NewWaveService(store,
		  f, 4)
	  _, _ = waveSvc.AddWave(context.Background(), plan.ID, "w1", []string{"uid-001"}, "ops_alice")
	  _, _ = waveSvc.AddWave(context.Background(), plan.ID, "w2", []string{"uid-002"}, "ops_alice")
	   _, _ = svc.ArmPlan(context.Background(), wrfreeze.ArmPlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	  _, _ = svc.ExecutePlan(context.Background(), wrfreeze.ExecutePlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	  // 第一次推进 → 触发 w1
	   advanced, err := waveSvc.AdvancePlan(context.Background(), plan.ID)
	   if err != nil {
		  t.Fatalf("AdvancePlan 1: %v", err)
	   }
	   if !advanced {
		  t.Error("want advanced=true for first advance")
	  }
	  // 第二次推进 → w1 already running, skip
	   // （在内存中 w1 已 done，因为 RunWave 同步返回）
	  // 故直接再推进 → 触发 w2
	   advanced, err = waveSvc.AdvancePlan(context.Background(), plan.ID)
	  if err != nil {
		   t.Fatalf("AdvancePlan 2: %v", err)
	  }
	  if !advanced {
		   t.Error("want advanced=true for second advance")
	   }
  }

// ─── TestAddWave_DuplicateUserDedup ───────────────────────────────────────────

   func TestAddWave_DuplicateUserDedup(t *testing.T) {
	   svc, store, f := makePlanSvc(t)
	   plan := openUserSetPlan(t, svc, []string{"uid-001"})
	   waveSvc := wrfreeze.NewWaveService(store,
		   f, 4)
	   wave, err := waveSvc.AddWave(context.Background(), plan.ID, "w1",
		   []string{"uid-001", "uid-001", "uid-001"}, "ops_alice")
	   if err != nil {
		  t.Fatalf("AddWave: %v", err)
	   }
	  if len(wave.TargetUsers) != 1 {
		   t.Errorf("want 1 deduped user, got %d", len(wave.TargetUsers))
	  }
   }
