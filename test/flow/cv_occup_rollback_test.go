package flow

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/cvoccup"
	"account-assets.kucoin.net/funding/internal/cvrollback"
)

// TestRollbackPlanIdempotent 回滚计划幂等：同 occID 重复调用只生成一条计划。
   func TestRollbackPlanIdempotent(t *testing.T) {
	   hold := newStubHoldAPI()
	   avail := newStubAvailReader()
	   occSvc, _ := buildOccSvc(hold, avail)
	   ctx := context.Background()
	   res, _ := occSvc.Occupy(ctx, cvoccup.OccupyReq{
		   UserID: "u8",
		  OrderID: "ord-8",
		   ClientOID: "coid-8",
		   PathKind:  cvoccup.PathLimit,
		  FromAsset: "USDT",
		   ToAsset: "BTC",
		  TotalAmount: "100",
		   AccountType: "MAIN",
		  ExpireAt: time.Now().Add(time.Hour),
	   })
	   rbStore := cvrollback.NewMemStore()
	  orch := cvrollback.NewOrchestrator(rbStore, &testOccReleaser{svc: occSvc}, nil, nil)
	  req := cvrollback.PlanReq{
		  OccID: res.Occ.ID, OrderID: "ord-8", UserID: "u8",
		   Trigger: cvrollback.TriggerFail, RemainAmount: "100",
	   }
	  plan1, err := orch.Plan(ctx, req)
	   if err != nil {
		  t.Fatalf("Plan1 failed: %v", err)
	   }
	   plan2, err := orch.Plan(ctx, req)
	   if err != nil {
		   t.Fatalf("Plan2 failed: %v", err)
	  }
	  if plan1.ID != plan2.ID {
		   t.Fatalf("expected same plan ID on duplicate, got %s vs %s", plan1.ID, plan2.ID)
	  }
   }

// TestRollbackPlanTerminalNotRedo 终态计划不重复执行。
   func TestRollbackPlanTerminalNotRedo(t *testing.T) {
	  hold := newStubHoldAPI()
	   avail := newStubAvailReader()
	  occSvc, _ := buildOccSvc(hold, avail)
	  ctx := context.Background()
	   res, _ := occSvc.Occupy(ctx, cvoccup.OccupyReq{
		  UserID:    "u9",
		   OrderID:   "ord-9",
		   ClientOID: "coid-9",
		  PathKind:  cvoccup.PathLimit,
		  FromAsset: "USDT",
		  ToAsset:   "BTC",
		  TotalAmount: "200",
		  AccountType: "MAIN",
		   ExpireAt:    time.Now().Add(time.Hour),
	  })
	   rbStore := cvrollback.NewMemStore()
	   orch := cvrollback.NewOrchestrator(rbStore, &testOccReleaser{svc: occSvc}, nil, nil)
	   req := cvrollback.PlanReq{
		   OccID: res.Occ.ID, OrderID: "ord-9", UserID: "u9",
		   Trigger: cvrollback.TriggerCancel, RemainAmount: "200",
	  }
	   plan,
		   err := orch.PlanAndExecute(ctx, req)
	  if err != nil {
		  t.Fatalf("PlanAndExecute failed: %v", err)
	  }
	   if plan.Status != cvrollback.RollbackDone {
		   t.Fatalf("expected DONE: %s %s", plan.Status, plan.Error)
	  }
	   _, err = orch.Execute(ctx, plan.ID)
	   if err != cvrollback.ErrPlanTerminal {
		  t.Fatalf("expected ErrPlanTerminal on re-execute, got: %v", err)
	  }
   }

// TestRollbackExpireTrigger 过期触发器流程：ExpireRollbackWorker 发起回滚编排。
   func TestRollbackExpireTrigger(t *testing.T) {
	   hold := newStubHoldAPI()
	  avail := newStubAvailReader()
	  occSvc, _ := buildOccSvc(hold, avail)
	  rbStore := cvrollback.NewMemStore()
	   orch := cvrollback.NewOrchestrator(rbStore, &testOccReleaser{svc: occSvc}, nil, nil)
	   rollbackFn := func(ctx context.Context, occID, orderID, userID, trigger, remain string) error {
		   _, err := orch.PlanAndExecute(ctx, cvrollback.PlanReq{
			  OccID: occID, OrderID: orderID, UserID: userID,
			   Trigger: cvrollback.RollbackTrigger(trigger),
			  RemainAmount: remain,
		  })
		  return err
	  }
	  ctx := context.Background()
	  res, _ := occSvc.Occupy(ctx, cvoccup.OccupyReq{
		   UserID:  "u10",
		  OrderID: "ord-10",
		   ClientOID: "coid-10",
		  PathKind: cvoccup.PathLimit,
		   FromAsset:   "USDT",
		   ToAsset:     "BTC",
		   TotalAmount: "150",
		   AccountType: "MAIN",
		   ExpireAt:    time.Now().Add(-time.Second), // 已过期
	  })
	   worker := cvoccup.NewExpireRollbackWorker(occSvc, rollbackFn, time.Minute)
	  n, err := worker.RunOnce(ctx)
	  if err != nil {
		  t.Fatalf("RunOnce failed: %v", err)
	   }
	  if n != 1 {
		  t.Fatalf("expected 1 expired, got %d", n)
	   }
	  occ, _ := occSvc.GetOcc(ctx, res.Occ.ID)
	   if occ.Status != cvoccup.OccRolledBack {
		   t.Fatalf("expected ROLLED_BACK after expire+rollback, got %s", occ.Status)
	  }
	  // 回滚计划应已完成
	   plan,
		   err := orch.GetPlanByOcc(ctx, res.Occ.ID)
	  if err != nil {
		   t.Fatalf("GetPlanByOcc failed: %v", err)
	   }
	  if plan.Status != cvrollback.RollbackDone {
		   t.Fatalf("expected plan DONE, got %s: %s", plan.Status, plan.Error)
	   }
   }

// TestBilateralOccImbalanceDetected 双边资产占用不一致时巡检检出（穿仓扫描）。
   func TestBilateralOccImbalanceDetected(t *testing.T) {
	  hold := newStubHoldAPI()
	   // 可用余额 100，但我们要占用 200（超额）
	  avail := newStubAvailReader("u11:USDT", "100")
	  occSvc, _ := buildOccSvc(hold, avail)
	   ctx := context.Background()
	  // 直接注入一条 remain=200 但 avail=100 的路径（跳过 Occupy 校验以恶意场景）
	  store := cvoccup.NewMemStore()
	   occSvc2 := cvoccup.NewService(store, hold, avail)
	   // 直接写入 store（绕过 Occupy 的余额校验，数据库被人为篡改）
	   fakeOcc := &cvoccup.PathOcc{
		  ID: "fake-occ-11",
		   UserID:  "u11",
		  OrderID: "ord-11",
		  PathKind:  cvoccup.PathLimit,
		  FromAsset: "USDT",
		   ToAsset:      "BTC",
		  TotalAmount:  "200",
		  FilledAmount: "0",
		   RemainAmount: "200",
		   Status:       cvoccup.OccFrozen,
		   HoldToken: "hold-fake",
		  ExpireAt:  time.Now().Add(time.Hour),
		   CreatedAt: time.Now(),
		  UpdatedAt: time.Now(),
	   }
	  _ = store.SaveOcc(ctx, fakeOcc)
	   // 穿仓扫描应检出
	   worker := cvoccup.NewBreachScanWorker(occSvc2, time.Minute)
	  n, err := worker.RunOnce(ctx)
	   if err != nil {
		   t.Fatalf("BreachScan failed: %v", err)
	  }
	   if n != 1 {
		   t.Fatalf("expected 1 breach detected, got %d", n)
	   }
	   _ = occSvc // 仅用于 hold 存根共享，避免编译警告
  }
