package flow

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/destlife"
)

// TestBypassProof_BlocksCooldownSkip 冷却未满、闸逻辑放行时 BypassGuard 检出并写入证明。
  func TestBypassProof_BlocksCooldownSkip(t *testing.T) {
	   t.Parallel()
	  ctx := context.Background()
	   now := time.Date(2025, 4, 21, 9, 0, 0, 0, time.UTC)
	  clk := &fakeClock{t: now}
	  calStore := destlife.NewMemCalendarStore()
	  auditStore := destlife.NewMemAuditStore()
	  window := 24 * time.Hour
	  cal := destlife.NewCalendar(calStore, auditStore, nil, window, clk.Now)
	  e, err := cal.InitAdded(ctx, "user20", "BTC", "bc1bypass", "", 1)
	   if err != nil {
		  t.Fatalf("InitAdded: %v", err)
	  }
	   bypassStore := destlife.NewMemBypassStore()
	   guard := destlife.NewBypassGuard(bypassStore, cal, clk.Now, 30*time.Second)
	  // 冷却未满（刚添加），gatePassedAt=now → 冷却未结束，应被 BypassGuard 挡。
	  err = guard.CheckAndProve(ctx, e.EntryLifeID, "user20", "BTC", "bc1bypass", clk.Now())
	   if !errors.Is(err, destlife.ErrBypassBlocked) {
		  t.Fatalf("expected ErrBypassBlocked, got=%v", err)
	  }
	  // 证明已写入 bypass_proofs。
	   proofs,
		  err := guard.ListProofs(ctx, e.EntryLifeID, 10)
	   if err != nil {
		   t.Fatalf("ListProofs: %v", err)
	  }
	   if len(proofs) == 0 {
		  t.Fatal("expected bypass proof to be recorded")
	   }
	  if !proofs[0].Blocked {
		  t.Error("expected proof to be blocked=true")
	  }
   }

// TestBypassProof_AllowsAfterCooldown 冷却已满时 BypassGuard 不阻断。

  func TestBypassProof_AllowsAfterCooldown(t *testing.T) {
	   t.Parallel()
	   ctx := context.Background()
	   now := time.Date(2025,
		   4, 21, 10, 0, 0, 0, time.UTC)
	  clk := &fakeClock{t: now}
	   calStore := destlife.NewMemCalendarStore()
	  auditStore := destlife.NewMemAuditStore()
	  window := 1 * time.Hour
	   cal := destlife.NewCalendar(calStore, auditStore, nil, window, clk.Now)
	   e, _ := cal.InitAdded(ctx, "user21", "ETH", "0xAllow1", "", 1)
	  // 推进 61 分钟，激活。
	  clk.Advance(61 * time.Minute)
	  _, _ = cal.ActivateCooled(ctx, e.EntryLifeID, clk.Now(), 1)
	   bypassStore := destlife.NewMemBypassStore()
	  guard := destlife.NewBypassGuard(bypassStore, cal, clk.Now, 30*time.Second)
	  // 冷却已满，不应阻断。
	  err := guard.CheckAndProve(ctx, e.EntryLifeID, "user21", "ETH", "0xAllow1", clk.Now())
	  if err != nil {
		  t.Errorf("expected nil, got=%v", err)
	   }
	   proofs,
		   _ := guard.ListProofs(ctx, e.EntryLifeID, 10)
	  if len(proofs) != 0 {
		   t.Errorf("expected no proofs for legitimately passed check, got=%d", len(proofs))
	   }
  }

// TestBypassProof_ClockDriftDetection 伪造时钟漂移：gate_passed_at 远超服务器时间。

   func TestBypassProof_ClockDriftDetection(t *testing.T) {
	  t.Parallel()
	  ctx := context.Background()
	  now := time.Date(2025,
		   4, 21, 11, 0, 0, 0, time.UTC)
	  clk := &fakeClock{t: now}
	   calStore := destlife.NewMemCalendarStore()
	  auditStore := destlife.NewMemAuditStore()
	   window := 24 * time.Hour
	  cal := destlife.NewCalendar(calStore, auditStore, nil, window, clk.Now)
	   e, _ := cal.InitAdded(ctx, "user22", "SOL", "SolDrift1", "", 1)
	   bypassStore := destlife.NewMemBypassStore()
	   driftThreshold := 30 * time.Second
	   guard := destlife.NewBypassGuard(bypassStore, cal, clk.Now, driftThreshold)
	   // 伪造 gatePassedAt：比服务器时间快 2 小时（超过阈值）。
	  fakePassedAt := clk.Now().Add(2 * time.Hour)
	  err := guard.CheckAndProve(ctx, e.EntryLifeID, "user22", "SOL", "SolDrift1", fakePassedAt)
	   if !errors.Is(err, destlife.ErrBypassBlocked) {
		   t.Fatalf("expected ErrBypassBlocked for clock drift, got=%v", err)
	  }
	   proofs, _ := guard.ListProofs(ctx, e.EntryLifeID, 10)
	   if len(proofs) == 0 {
		   t.Fatal("expected proof to be recorded for clock drift")
	   }
	   p := proofs[0]
	  if p.Reason != destlife.ReasonClockDrift && p.Reason != destlife.ReasonCooldownSkip {
		   t.Errorf("expected reason clock_drift or cooldown_skip, got=%s", p.Reason)
	  }
	  if p.ClockDrift < int64(driftThreshold.Milliseconds()) {
		   t.Errorf("expected drift >= threshold, got drift_ms=%d threshold_ms=%d",
			   p.ClockDrift, driftThreshold.Milliseconds())
	  }
  }

// TestBypassProof_GenMismatch 世代不一致时 BypassGuard 写入 gen_mismatch 证明。

  func TestBypassProof_GenMismatch(t *testing.T) {
	  t.Parallel()
	  ctx := context.Background()
	  clk := &fakeClock{t: time.Now().UTC()}
	  calStore := destlife.NewMemCalendarStore()
	   auditStore := destlife.NewMemAuditStore()
	  cal := destlife.NewCalendar(calStore,
		  auditStore, nil, 0, clk.Now)
	   e, _ := cal.InitAdded(ctx, "user23", "BTC", "bc1gen", "", 5) // book_gen=5
	   bypassStore := destlife.NewMemBypassStore()
	  guard := destlife.NewBypassGuard(bypassStore, cal, clk.Now, 0)
	   // 用 book_gen=10（不匹配 life_gen=5）触发世代错误。
	   mismatch := &destlife.GenMismatch{
		  EntryLifeID: e.EntryLifeID,
		   LifeGen:     5,
		   BookGen:     10,
		   Discrepancy: 5,
	   }
	  err := guard.ProveGenMismatch(ctx, mismatch, "user23", "BTC", "bc1gen")
	  if !errors.Is(err, destlife.ErrBypassBlocked) {
		  t.Fatalf("expected ErrBypassBlocked, got=%v", err)
	  }
	  proofs, _ := guard.ListProofs(ctx, e.EntryLifeID, 10)
	   if len(proofs) == 0 {
		   t.Fatal("expected gen_mismatch proof to be recorded")
	  }
	   if proofs[0].Reason != destlife.ReasonGenMismatch {
		  t.Errorf("expected reason=gen_mismatch, got=%s", proofs[0].Reason)
	   }
  }

// TestBypassProof_PatrolUnblocked Worker 巡检补挡所有未挡写证明。
  func TestBypassProof_PatrolUnblocked(t *testing.T) {
	   t.Parallel()
	  ctx := context.Background()
	   clk := &fakeClock{t: time.Now().UTC()}
	  calStore := destlife.NewMemCalendarStore()
	   auditStore := destlife.NewMemAuditStore()
	  window := 24 * time.Hour
	  cal := destlife.NewCalendar(calStore, auditStore, nil, window, clk.Now)
	  // 注入 3 条未挡写证明（直接写 store，绕过 guard 检测）。
	   bypassStore := destlife.NewMemBypassStore()
	   for i := 0; i < 3; i++ {
		  e, _ := cal.InitAdded(ctx, "user24", "ETH", "0xPatrol"+string(rune('A'+i)), "", int64(i))
		  _ = bypassStore.InsertProof(ctx, destlife.BypassProof{
			  ProofID:     "proof-patrol-" + string(rune('A'+i)),
			   EntryLifeID: e.EntryLifeID,
			  UserID:      "user24",
			   Chain:       "ETH",
			   Address: e.Address,
			   Reason:  destlife.ReasonCooldownSkip,
			   Blocked: false,
			  DetectedAt: clk.Now(),
		   })
	  }
	  guard := destlife.NewBypassGuard(bypassStore, cal, clk.Now, 0)
	  processed, errs := guard.PatrolUnblocked(ctx, 100)
	  if len(errs) != 0 {
		   t.Fatalf("patrol errors: %v", errs)
	  }
	   if processed != 3 {
		   t.Errorf("expected processed=3, got=%d", processed)
	   }
	   // 再次巡检：无未挡写证明。
	   processed2, _ := guard.PatrolUnblocked(ctx, 100)
	  if processed2 != 0 {
		  t.Errorf("expected 0 on second patrol, got=%d", processed2)
	   }
   }
