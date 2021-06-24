package flow

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/destlife"
)

// TestWorker_RunActivate Worker 激活批量 cooling 条目。

  func TestWorker_RunActivate(t *testing.T) {
	  t.Parallel()
	  ctx := context.Background()
	   now := time.Date(2025, 4, 21, 8, 0, 0, 0, time.UTC)
	  clk := &fakeClock{t: now}
	  calStore := destlife.NewMemCalendarStore()
	  auditStore := destlife.NewMemAuditStore()
	  window := 1 * time.Hour
	   cal := destlife.NewCalendar(calStore,
		  auditStore, nil, window, clk.Now)
	  // 添加 5 个条目。
	   var entryIDs []string
	  for i := 0; i < 5; i++ {
		   addr := "bc1worker" + string(rune('A'+i))
		  e, _ := cal.InitAdded(ctx, "user40", "BTC", addr, "", int64(i))
		   entryIDs = append(entryIDs, e.EntryLifeID)
	   }
	  cfg := destlife.WorkerConfig{Batch: 10, CoolingOverstay: 2 * time.Hour}
	  w := destlife.NewLifecycleWorker(cal, nil, nil, cfg, clk.Now, nil)
	  // 冷却未满，激活数应为 0。
	  n, errs := w.RunActivate(ctx)
	  if n != 0 || len(errs) != 0 {
		  t.Errorf("expected 0 activations before deadline, got n=%d errs=%v", n, errs)
	   }
	   // 推进 61 分钟，全部条目冷却满足。
	  clk.Advance(61 * time.Minute)
	  n, errs = w.RunActivate(ctx)
	   if len(errs) != 0 {
		  t.Fatalf("RunActivate errors: %v", errs)
	  }
	  if n != 5 {
		  t.Errorf("expected 5 activations, got=%d", n)
	   }
	  // 再次激活：幂等，不重复计数。
	  n2, _ := w.RunActivate(ctx)
	  if n2 != 0 {
		   t.Errorf("expected 0 on second activate run (idempotent), got=%d", n2)
	  }
  }

// TestWorker_RunRevokeFlush Worker 撤销冲刷：引用清零后自动完成 revoking→revoked。
   func TestWorker_RunRevokeFlush(t *testing.T) {
	   t.Parallel()
	   ctx := context.Background()
	   clk := &fakeClock{t: time.Now().UTC()}
	   calStore := destlife.NewMemCalendarStore()
	  auditStore := destlife.NewMemAuditStore()
	   cal := destlife.NewCalendar(calStore,
		  auditStore, nil, 0, clk.Now)
	  inflightStore := destlife.NewMemInflightStore()
	   // lock 传 nil calendar，ReleaseRef 不会自动触发 FinalizeRevoked，
	  // 让 Worker 的 RunRevokeFlush 承担最终撤销职责。
	  lock := destlife.NewInflightLock(inflightStore, nil, clk.Now)
	  // 添加+激活+获取引用+转 revoking。
	  e, _ := cal.InitAdded(ctx, "user41", "ETH", "0xFlush1", "", 1)
	  clk.Advance(1 * time.Millisecond)
	  e, _ = cal.ActivateCooled(ctx, e.EntryLifeID, clk.Now(), 1)
	  ref, _ := lock.AcquireRef(ctx, e.EntryLifeID, "user41", "ETH", "0xFlush1", "mvt-flush-1")
	  _, _ = cal.BeginRevoking(ctx, e.EntryLifeID, "ops", "test_revoke", e.Generation)
	   // Worker 持有 lock（带 calendar），用于 RunRevokeFlush 中的 ActiveCount 和 FinalizeRevoked。
	  lockWithCal := destlife.NewInflightLock(inflightStore, cal, clk.Now)
	  cfg := destlife.WorkerConfig{Batch: 10, CoolingOverstay: time.Hour}
	  w := destlife.NewLifecycleWorker(cal, lockWithCal, nil, cfg, clk.Now, nil)
	  // 引用未释放，flush 无法完成。
	  n, _ := w.RunRevokeFlush(ctx)
	  if n != 0 {
		  t.Errorf("expected 0 revocations while ref held, got=%d", n)
	   }
	  state,
		   _ := cal.GetLifeByID(ctx, e.EntryLifeID)
	  if state.State != destlife.StateRevoking {
		   t.Errorf("expected still revoking while ref held, got=%s", state.State)
	  }
	   // 通过 lock（无 calendar）释放引用，不触发自动撤销，让 Worker 完成。
	   clk.Advance(100 * time.Millisecond)
	  _ = lock.ReleaseRef(ctx, ref.RefID, "test_worker", state.Generation)
	  // 再次 flush：Worker 应完成 revoking→revoked。
	  n, errs := w.RunRevokeFlush(ctx)
	   if len(errs) != 0 {
		   t.Fatalf("RunRevokeFlush errors: %v", errs)
	  }
	   if n != 1 {
		  t.Errorf("expected 1 revocation after ref release, got=%d", n)
	   }
  }

// TestWorker_AgingAlert 冷却超期老化告警（超过 overstay 阈值）。

  func TestWorker_AgingAlert(t *testing.T) {
	  t.Parallel()
	   ctx := context.Background()
	  now := time.Date(2025, 4, 21, 6, 0, 0, 0, time.UTC)
	  clk := &fakeClock{t: now}
	  calStore := destlife.NewMemCalendarStore()
	   auditStore := destlife.NewMemAuditStore()
	   window := 1 * time.Hour // 冷却 1h
	   cal := destlife.NewCalendar(calStore,
		  auditStore, nil, window, clk.Now)
	   // 添加 2 个条目。
	   cal.InitAdded(ctx, "user42", "BTC", "bc1aging1", "", 1)
	   cal.InitAdded(ctx, "user42", "BTC", "bc1aging2", "", 2)
	  // 推进 4 小时：超过 overstay 阈值（2h）。
	   clk.Advance(4 * time.Hour)
	   cfg := destlife.WorkerConfig{Batch: 50, CoolingOverstay: 2 * time.Hour}
	   w := destlife.NewLifecycleWorker(cal, nil, nil, cfg, clk.Now, nil)
	  n, errs := w.RunAgingAlert(ctx)
	   if len(errs) != 0 {
		  t.Fatalf("RunAgingAlert errors: %v", errs)
	   }
	   if n < 2 {
		   t.Errorf("expected >= 2 aging alerts, got=%d", n)
	  }
  }

// TestWorker_BypassPatrol Worker 绕过巡检补挡未挡写证明。
   func TestWorker_BypassPatrol(t *testing.T) {
	  t.Parallel()
	  ctx := context.Background()
	   clk := &fakeClock{t: time.Now().UTC()}
	   calStore := destlife.NewMemCalendarStore()
	   auditStore := destlife.NewMemAuditStore()
	   cal := destlife.NewCalendar(calStore, auditStore, nil, 0, clk.Now)
	   bypassStore := destlife.NewMemBypassStore()
	   guard := destlife.NewBypassGuard(bypassStore, cal, clk.Now, 0)
	   // 注入 2 条未挡写证明。
	  e, _ := cal.InitAdded(ctx, "user43", "BTC", "bc1patrol1", "", 1)
	   e2, _ := cal.InitAdded(ctx, "user43", "BTC", "bc1patrol2", "", 2)
	   _ = bypassStore.InsertProof(ctx, destlife.BypassProof{
		  ProofID: "pp-1", EntryLifeID: e.EntryLifeID,
		   UserID: "user43", Chain: "BTC", Address: "bc1patrol1",
		  Reason: destlife.ReasonCooldownSkip, Blocked: false, DetectedAt: clk.Now(),
	   })
	   _ = bypassStore.InsertProof(ctx, destlife.BypassProof{
		   ProofID: "pp-2", EntryLifeID: e2.EntryLifeID,
		   UserID: "user43", Chain: "BTC", Address: "bc1patrol2",
		   Reason: destlife.ReasonClockDrift, Blocked: false, DetectedAt: clk.Now(),
	   })
	   cfg := destlife.WorkerConfig{Batch: 10, CoolingOverstay: time.Hour}
	   w := destlife.NewLifecycleWorker(cal, nil, guard, cfg, clk.Now, nil)
	   processed, errs := w.RunBypassPatrol(ctx)
	   if len(errs) != 0 {
		  t.Fatalf("RunBypassPatrol errors: %v", errs)
	   }
	   if processed != 2 {
		  t.Errorf("expected processed=2, got=%d", processed)
	  }
	  // 再次巡检：无未挡写证明。
	   processed2, _ := w.RunBypassPatrol(ctx)
	  if processed2 != 0 {
		   t.Errorf("expected 0 on second patrol, got=%d", processed2)
	   }
  }
