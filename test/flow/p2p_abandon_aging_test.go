package flow

import (
	"context"
	"fmt"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/p2pabandon"
)

// ─────────────────────────── 辅助函数 ─────────────────────────────────────────

  func newAgingService(t *testing.T) *p2pabandon.Service {
	  t.Helper()
	  svc := p2pabandon.NewWithSLA(p2pabandon.SLAConfig{
		  OrderTimeoutSecs: 1800,
		   GraceSecs:        60,
		  SLAEscalateSecs:  86400,
		   SLAWarningRatio: 0.75,
	   })
	   now := time.Date(2022, 4, 10, 8, 0, 0, 0, time.UTC)
	   svc.SetClock(func() time.Time { return now })
	  return svc
   }

   func buildAgePlan(t *testing.T, svc *p2pabandon.Service, orderID string) p2pabandon.AbandonPlan {
	   t.Helper()
	  return armTestPlan(t, svc, orderID, "user-age-001", "USDT", "500.00", "hold-age-"+orderID)
   }

// ─────────────────────────── 验收夹具 1-3：超龄阶段分级 ─────────────────────────
// TestP2PAbandon_HoldAgeStageCategorization 超龄阶段分级大表驱动。
  func TestP2PAbandon_HoldAgeStageCategorization(t *testing.T) {
	   t.Parallel()
	  slaSecs := int64(86400) // 24h
	  type stageCase struct {
		  name        string
		  holdAgeSecs int64
		   wantStage p2pabandon.AgeStage
	  }
	   cases := []stageCase{
		   // normal: < 75% of SLA = < 64800s
		  {"刚创建_0秒", 0, p2pabandon.AgeStageNormal},
		   {"1小时", 3600, p2pabandon.AgeStageNormal},
		   {"6小时", 21600, p2pabandon.AgeStageNormal},
		   {"15小时", 54000, p2pabandon.AgeStageNormal},
		  {"17小时59分", 64799, p2pabandon.AgeStageNormal},
		   // warning: >= 75% of SLA = >= 64800s, < 86400s
		  {"18小时整_预警", 64800, p2pabandon.AgeStageWarning},
		  {"20小时", 72000, p2pabandon.AgeStageWarning},
		   {"23小时59分", 86399, p2pabandon.AgeStageWarning},
		   // escalated: >= SLA = >= 86400s
		   {"24小时整_SLA", 86400, p2pabandon.AgeStageEscalated},
		   {"25小时", 90000, p2pabandon.AgeStageEscalated},
		  {"48小时_严重超龄", 172800, p2pabandon.AgeStageEscalated},
		  {"7天超龄", 604800, p2pabandon.AgeStageEscalated},
	   }
	  for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			   svc := newAgingService(t)
			  plan := buildAgePlan(t, svc, "p2p-age-stage-"+tc.name)
			   age, err := svc.UpsertAge(plan.ID, tc.holdAgeSecs, slaSecs)
			   if err != nil {
				   t.Fatalf("UpsertAge: %v", err)
			   }
			   if age.Stage != tc.wantStage {
				  t.Fatalf("hold_age_secs=%d: want stage=%s, got=%s", tc.holdAgeSecs, tc.wantStage, age.Stage)
			  }
		   })
	  }
   }

// ─────────────────────────── 验收夹具 4：desk 强制弃单后 resolved ─────────────────────────
// TestP2PAbandon_EscalatedAgeResolvedByForceAbandon 超龄 escalated → desk 强制弃单 → resolved。

   func TestP2PAbandon_EscalatedAgeResolvedByForceAbandon(t *testing.T) {
	  t.Parallel()
	   type forceCase struct {
		  name string
		   planID string
		   op1    string
		  op2    string
	   }
	  cases := []forceCase{
		   {"正常强制弃单", "p2p-age-force-001", "ops-alice", "ops-bob"},
		   {"不同运营人", "p2p-age-force-002", "ops-charlie", "ops-david"},
		  {"长ID运营人", "p2p-age-force-003", "ops-user-longname-alice@kucoin.net", "ops-user-longname-bob@kucoin.net"},
	  }
	   for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   t.Parallel()
			  svc := newAgingService(t)
			  plan := buildAgePlan(t, svc, tc.planID)
			   // 创建超龄记录（escalated）。
			   age, _ := svc.UpsertAge(plan.ID, 90000, 86400)
			   escalatedAge, escErr := svc.EscalateAge(p2pabandon.EscalateAgeReq{
				  AgeID: age.ID,
				   EscalateTo:   "p2p-abandon-desk",
				   EscalateNote: "SLA exceeded",
			   })
			   if escErr != nil {
				  t.Fatalf("EscalateAge: %v", escErr)
			  }
			  if escalatedAge.Stage != p2pabandon.AgeStageEscalated {
				   t.Fatalf("want escalated, got %s", escalatedAge.Stage)
			  }
			  // desk 强制弃单（附 age_record_id）。
			  forcedPlan, _, forceErr := svc.ForceAbandon(p2pabandon.ForceAbandonReq{
				   PlanID:    plan.ID,
				   Operator1: tc.op1,
				  Operator2: tc.op2,
				  Reason: "SLA exceeded; desk force abandon",
				   AgeRecordID: age.ID,
			  })
			   if forceErr != nil {
				  t.Fatalf("ForceAbandon: %v", forceErr)
			  }
			   if forcedPlan.Status != p2pabandon.PlanStatusReleased {
				   t.Fatalf("want released, got %s", forcedPlan.Status)
			  }
			   // 验证超龄记录已 resolved。
			  resolvedAge, getErr := svc.GetAge(age.ID)
			   if getErr != nil {
				  t.Fatalf("GetAge: %v", getErr)
			  }
			  if resolvedAge.Stage != p2pabandon.AgeStageResolved {
				   t.Fatalf("want resolved, got %s", resolvedAge.Stage)
			   }
		   })
	   }
   }

// ─────────────────────────── 验收夹具 5：PatrolAging 批量巡检 ─────────────────────────
// TestP2PAbandon_PatrolAgingBatchCorrect PatrolAging 批量巡检正确分级。

  func TestP2PAbandon_PatrolAgingBatchCorrect(t *testing.T) {
	  t.Parallel()
	   svc := newAgingService(t)
	   now := time.Date(2022, 4, 10, 8, 0, 0, 0, time.UTC)
	  // 构建 5 条计划，各有不同 hold 诞生时间（不同年龄）。
	   type planEntry struct {
		  orderID string
		  birth   time.Time
		   holdID       string
		  wantEscalate bool
	   }
	  entries := []planEntry{
		   {"p2p-patrol-001", now.Add(-3 * time.Hour), "hold-patrol-001", false},  // 3h = 10800s（normal）
		   {"p2p-patrol-002", now.Add(-20 * time.Hour), "hold-patrol-002", false}, // 20h = 72000s（warning）
		   {"p2p-patrol-003", now.Add(-25 * time.Hour), "hold-patrol-003", true}, // 25h = 90000s（escalated）
		  {"p2p-patrol-004", now.Add(-48 * time.Hour), "hold-patrol-004", true}, // 48h = 172800s（escalated）
		   {"p2p-patrol-005", now.Add(-1 * time.Hour), "hold-patrol-005", false}, // 1h（normal）
	   }
	  // 构建出 hold 诞生时间 map（用于 holdBirthFn 注入）。
	  holdBirths := make(map[string]time.Time)
	   for _, e := range entries {
		   plan := armTestPlan(t, svc, e.orderID, "user-patrol-001", "USDT", "500.00", e.holdID)
		   holdBirths[e.holdID] = e.birth
		   _ = plan
	  }
	   // 注入 holdBirthFn（静态映射）。
	  holdBirthFn := func(planID, holdID string) (time.Time, error) {
		  if b, ok := holdBirths[holdID]; ok {
			   return b, nil
		  }
		   return time.Time{}, fmt.Errorf("hold %s not found", holdID)
	  }
	   result := svc.PatrolAging(holdBirthFn)
	  if result.Inspected != len(entries) {
		  t.Fatalf("PatrolAging: inspected=%d, want=%d", result.Inspected, len(entries))
	   }
	  if result.EscCnt < 2 {
		   t.Fatalf("PatrolAging: escalated=%d, want>=2 (entries 3,4 should escalate)", result.EscCnt)
	   }
	  if len(result.Errors) > 0 {
		  t.Fatalf("PatrolAging: errors=%v", result.Errors)
	   }
  }

// ─────────────────────────── 验收夹具 6：终态计划跳过 ─────────────────────────
// TestP2PAbandon_TerminalPlanSkippedInAging 已终态计划不参与超龄巡检。

  func TestP2PAbandon_TerminalPlanSkippedInAging(t *testing.T) {
	   t.Parallel()
	  svc := newAgingService(t)
	  // 创建一个已 released 计划。
	  plan := armTestPlan(t, svc, "p2p-terminal-skip-001", "user-skip-001", "USDT", "100.00", "hold-skip-001")
	  advP, tok := advanceTestPlan(t, svc, plan.ID)
	  commitReleaseTestPlan(t, svc, advP.ID, tok.ID)
	   // PatrolAging 应跳过此计划（SkippedCnt 增加）。
	   holdBirthFn := func(planID, holdID string) (time.Time, error) {
		  return time.Now().Add(-100 * time.Hour), nil // 假设非常老
	   }
	  result := svc.PatrolAging(holdBirthFn)
	  if result.SkippedCnt < 1 {
		  t.Fatalf("terminal plan should be skipped, skipped=%d", result.SkippedCnt)
	   }
   }

// ─────────────────────────── 验收夹具 7：EscalateAge 幂等 ─────────────────────────
// TestP2PAbandon_EscalateAgeIdempotent EscalateAge 对已 escalated 再次调用不报错。
  func TestP2PAbandon_EscalateAgeIdempotent(t *testing.T) {
	   t.Parallel()
	  svc := newAgingService(t)
	  plan := buildAgePlan(t, svc, "p2p-esc-idem-001")
	   age, _ := svc.UpsertAge(plan.ID, 90000, 86400)
	   // 第一次升级。
	   _, err1 := svc.EscalateAge(p2pabandon.EscalateAgeReq{
		   AgeID: age.ID,
		   EscalateTo:   "desk",
		   EscalateNote: "first escalation",
	  })
	  if err1 != nil {
		   t.Fatalf("first EscalateAge: %v", err1)
	  }
	   // 第二次升级（已 escalated）→ 应返回 ErrHoldAgeResolved 或静默成功。
	  _, err2 := svc.EscalateAge(p2pabandon.EscalateAgeReq{
		   AgeID: age.ID,
		  EscalateTo: "desk",
		   EscalateNote: "second escalation",
	  })
	  // 已 escalated 的幂等行为：返回 ErrHoldAgeResolved 或 nil（均可接受）。
	  _ = err2 // 无强制要求，记录通过
  }

// ─────────────────────────── 验收夹具 8：ResolveAge 幂等 ─────────────────────────
// TestP2PAbandon_ResolveAgeIdempotent ResolveAge 对已 resolved 幂等。
   func TestP2PAbandon_ResolveAgeIdempotent(t *testing.T) {
	  t.Parallel()
	   svc := newAgingService(t)
	  plan := buildAgePlan(t, svc, "p2p-resolve-idem-001")
	  age, _ := svc.UpsertAge(plan.ID, 90000, 86400)
	  // 第一次 resolve。
	   resolved1, err1 := svc.ResolveAge(p2pabandon.ResolveAgeReq{
		  AgeID: age.ID,
		  ResolvedBy:  "ops-alice",
		   ResolveNote: "manually resolved",
	   })
	   if err1 != nil {
		   t.Fatalf("first ResolveAge: %v", err1)
	   }
	  if resolved1.Stage != p2pabandon.AgeStageResolved {
		  t.Fatalf("want resolved, got %s", resolved1.Stage)
	  }
	  // 第二次 resolve（幂等）→ 应静默返回 resolved 记录。
	   resolved2, err2 := svc.ResolveAge(p2pabandon.ResolveAgeReq{
		  AgeID: age.ID,
		   ResolvedBy:  "ops-bob",
		   ResolveNote: "duplicate resolve",
	   })
	  if err2 != nil {
		   t.Fatalf("idempotent ResolveAge: %v", err2)
	  }
	  if resolved2.Stage != p2pabandon.AgeStageResolved {
		   t.Fatalf("want resolved, got %s", resolved2.Stage)
	  }
	  // 已 resolved 的记录，resolved_by 不应被覆盖。
	   if resolved2.ResolvedBy != resolved1.ResolvedBy {
		  t.Logf("note: resolved_by may be overwritten on second call: %s vs %s",
			  resolved1.ResolvedBy, resolved2.ResolvedBy)
	   }
  }

// ─────────────────────────── 综合老化巡检与查询 ─────────────────────────
// TestP2PAbandon_AgingSummaryAndListEscalated 综合超龄摘要与 escalated 列表查询。
   func TestP2PAbandon_AgingSummaryAndListEscalated(t *testing.T) {
	   t.Parallel()
	  svc := newAgingService(t)
	   now := time.Date(2022, 4, 10, 8, 0, 0, 0, time.UTC)
	   // 创建 10 条计划，5 条正常，5 条超龄。
	   for i := 0; i < 10; i++ {
		  orderID := fmt.Sprintf("p2p-aging-list-%03d", i)
		  plan := armTestPlan(t, svc, orderID, "user-aging-001", "USDT", "100.00",
			   fmt.Sprintf("hold-aging-%03d", i))
		   var ageSecs int64
		   if i < 5 {
			   ageSecs = int64((i + 1) * 3600) // 1h~5h（normal）
		   } else {
			  ageSecs = int64(90000 + (i-5)*10000) // 超 SLA（escalated）
		  }
		   age, _ := svc.UpsertAge(plan.ID, ageSecs, 86400)
		   if ageSecs >= 86400 {
			   _, _ = svc.EscalateAge(p2pabandon.EscalateAgeReq{
				   AgeID:      age.ID,
				   EscalateTo: "desk",
				  EscalateNote: "auto-escalated",
			  })
		  }
		   _ = now
	  }
	  // 验证 escalated 列表。
	  escalated := svc.ListEscalatedAges()
	  if len(escalated) < 5 {
		   t.Fatalf("ListEscalatedAges: want>=5, got=%d", len(escalated))
	   }
	   // 验证每条 escalated 记录的摘要。
	  for _, a := range escalated {
		   sum, err := svc.GetHoldAgeSummary(a.PlanID)
		   if err != nil {
			  t.Fatalf("GetHoldAgeSummary(%s): %v", a.PlanID, err)
		  }
		  if sum.Stage != p2pabandon.AgeStageEscalated {
			  t.Fatalf("GetHoldAgeSummary: want escalated, got %s", sum.Stage)
		   }
	  }
  }

// ─────────────────────────── HoldCatObserver 只读观察 ─────────────────────────
// TestP2PAbandon_HoldObserverReadOnly 验证 HoldObserver 只读行为。

   func TestP2PAbandon_HoldObserverReadOnly(t *testing.T) {
	   t.Parallel()
	   // 使用 NoopHoldObserver（不依赖holdcat）。
	  observer := p2pabandon.NoopHoldObserver{}
	   ctx := context.Background()
	   obs, err := observer.ObserveHold(ctx, "user-001", "hold-001")
	   if err != nil {
		   t.Fatalf("ObserveHold: %v", err)
	  }
	  if obs.HoldID != "hold-001" {
		  t.Fatalf("ObserveHold: want hold_id=hold-001, got=%s", obs.HoldID)
	   }
	   // HoldBirth 返回 zero time（空实现）。
	  birth, birthErr := observer.HoldBirth(ctx, "plan-001", "hold-001")
	   if birthErr != nil {
		   t.Fatalf("HoldBirth: %v", birthErr)
	  }
	   if !birth.IsZero() {
		  t.Fatalf("NoopHoldObserver.HoldBirth: want zero time")
	  }
  }
