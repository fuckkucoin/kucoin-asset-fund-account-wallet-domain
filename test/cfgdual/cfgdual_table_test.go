package cfgdual_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/cfgcenter"
	"account-assets.kucoin.net/funding/internal/cfgdual"
	"github.com/google/uuid"
)

// ─────────────────────────────────────────────────────────────────────────────
// 内部工具函数
// ─────────────────────────────────────────────────────────────────────────────

   func newIDGen() func() string { return func() string { return uuid.NewString() } }

  func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

// buildCfgStore 构造装载了两版 kyc0 包的 cfgcenter.Store（内存模式）。
// v2=宽松 baseline（10000/50000），v3=收紧 canary（500/2000）。

  func buildCfgStore(t *testing.T) *cfgcenter.Store {
	   t.Helper()
	  st := cfgcenter.NewStore()
	  packs := []cfgcenter.LimitPack{
		  {Name: "kyc0", Version: 2, MaxSingle: "10000", DailyWithdraw: "50000", HourlyVel: 20, DailyVel: 50},
		   {Name: "kyc0", Version: 3, MaxSingle: "500", DailyWithdraw: "2000", HourlyVel: 5, DailyVel: 10},
		   {Name: "kyc1", Version: 4, MaxSingle: "8000", DailyWithdraw: "40000", HourlyVel: 12, DailyVel: 30},
		  {Name: "kyc1", Version: 5, MaxSingle: "1000", DailyWithdraw: "5000", HourlyVel: 3, DailyVel: 8},
		  {
			  Name: "kyc2", Version: 6, MaxSingle: "500", DailyWithdraw: "2000", HourlyVel: 5, DailyVel: 10,
			  AssetMax: map[string]string{"BTC": "0.05", "ETH": "2.0"},
		  },
	   }
	  for _, p := range packs {
		  if err := st.Put(p); err != nil {
			   t.Fatalf("buildCfgStore put %s v%d: %v", p.Name, p.Version, err)
		   }
	   }
	   if err := st.Pin("kyc0", 2); err != nil {
		   t.Fatalf("pin kyc0 v2: %v", err)
	   }
	  return st
  }

// buildManager 构造用 Manager（内存模式，指定切流发布）。
  func buildManager(t *testing.T, percent, policyDrift int) (*cfgdual.Manager, *cfgdual.GrayRelease) {
	  t.Helper()
	   ctx := context.Background()
	   cfgSt := buildCfgStore(t)
	   mgr := cfgdual.NewManager(cfgSt, nil, newIDGen())
	  rel, err := mgr.CreateRelease(ctx, cfgdual.CreateReleaseRequest{
		  PackName: "kyc0", BaselineVersion: 2, CanaryVersion: 3,
		  CohortPercent: percent, PolicyAllowDrift: policyDrift, Creator: "test",
	  })
	  if err != nil {
		  t.Fatalf("create release: %v", err)
	  }
	  if err := mgr.ActivateRelease(ctx, rel); err != nil {
		   t.Fatalf("activate release: %v", err)
	  }
	  return mgr, rel
  }

// ─────────────────────────────────────────────────────────────────────────────
// TestEvalPackTable EvalPack 各限额维度条件
// ─────────────────────────────────────────────────────────────────────────────

  func TestEvalPackTable(t *testing.T) {
	  pack := cfgcenter.LimitPack{
		   Name: "kyc0", Version: 3,
		  MaxSingle:     "500",
		   DailyWithdraw: "2000",
		  HourlyVel: 5,
		  DailyVel: 10,
		   AssetMax: map[string]string{"BTC": "0.05", "ETH": "2.0"},
	  }
	   cases := []struct {
		   name string
		   amount string
		  asset  string
		  todaySum string
		   hourly int
		  daily  int
		  killed bool
		   wantAllow  bool
		  wantReason string
	   }{
		   {"单笔等于上限_允许", "500", "USDT", "0", 0, 0, false, true, ""},
		   {"单笔超出上限", "500.000000000000000001", "USDT", "0", 0, 0, false, false, "max single"},
		  {"单笔远超上限", "9999", "USDT", "0", 0, 0, false, false, "max single"},
		  {"单笔极小_允许", "0.000000000000000001", "USDT", "0", 0, 0, false, true, ""},
		   {"日累计恰好等于上限_允许", "500", "USDT", "1500", 0, 0, false, true, ""},
		  {"日累计超出上限", "500", "USDT", "1501", 0, 0, false, false, "daily withdraw"},
		  {"日累计已满_拒绝", "1", "USDT", "2000", 0, 0, false, false, "daily withdraw"},
		  {"小时速率等于阈值_拒绝", "1", "USDT", "0", 5, 0, false, false, "hourly velocity"},
		  {"小时速率低于阈值_允许", "1", "USDT", "0", 4, 0, false, true, ""},
		   {"小时速率为零_允许", "1", "USDT", "0", 0, 0, false, true, ""},
		   {"日速率等于阈值_拒绝", "1", "USDT", "0", 0, 10, false, false, "daily velocity"},
		  {"日速率低于阈值_允许", "1", "USDT", "0", 0, 9, false, true, ""},
		   {"BTC超出资产专项上限_拒绝", "0.06", "BTC", "0", 0, 0, false, false, "asset cap BTC"},
		  {"BTC等于资产专项上限_允许", "0.05", "BTC", "0", 0, 0, false, true, ""},
		  {"ETH超出资产专项上限_拒绝", "2.000000000000000001", "ETH", "0", 0, 0, false, false, "asset cap ETH"},
		  {"ETH等于资产专项上限_允许", "2.0", "ETH", "0", 0, 0, false, true, ""},
		  {"TRX无专项限额_走单笔允许", "300", "TRX", "0", 0, 0, false, true, ""},
		   {"TRX无专项限额_超单笔拒绝", "600", "TRX", "0", 0, 0, false, false, "max single"},
		   {"kill_switch_打开_拒绝", "1", "USDT", "0", 0, 0, true, false, ""},
		   {"空资产走限额校验_允许", "200", "", "0", 0, 0, false, true, ""},
	  }
	   for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  p := pack
			   p.Killed = tc.killed
			   req := cfgcenter.Request{
				  Pack:  "kyc0",
				  Asset: tc.asset,
				   Amount:   tc.amount,
				   TodaySum: tc.todaySum,
				   Hourly:   tc.hourly,
				   Daily:    tc.daily,
			  }
			   dec, _ := cfgcenter.EvalPack(p, req)
			  if dec.Allow != tc.wantAllow {
				  t.Errorf("EvalPack Allow=%v, want %v (reason=%q)", dec.Allow, tc.wantAllow, dec.Reason)
			  }
			  if tc.wantReason != "" && dec.Reason != tc.wantReason {
				   t.Errorf("EvalPack Reason=%q, want %q", dec.Reason, tc.wantReason)
			   }
		   })
	  }
   }

// ─────────────────────────────────────────────────────────────────────────────
// TestCohortTable 切流逻辑表驱动
// ─────────────────────────────────────────────────────────────────────────────

   func TestCohortTable(t *testing.T) {
	   t.Run("比例切流稳定性", func(t *testing.T) {
		  cohort := cfgdual.NewCohort("rel-001", "kyc0", 10, 1)
		   // 100 个用户，各自调用 50 次，结论必须完全相同
		  for i := 0; i < 100; i++ {
			   uid := fmt.Sprintf("user-%04d", i)
			   ref, gen := cohort.InCanary(uid, "USDT")
			   for j := 1; j < 50; j++ {
				   got, g := cohort.InCanary(uid, "USDT")
				  if g != gen || got != ref {
					  t.Errorf("user %s iter %d: got %v gen %d, want %v gen %d", uid, j, got, g, ref, gen)
				   }
			   }
		   }
	   })
	  t.Run("比例0%全走baseline", func(t *testing.T) {
		  cohort := cfgdual.NewCohort("rel-002", "kyc0", 0, 1)
		  for i := 0; i < 200; i++ {
			  uid := fmt.Sprintf("u-%d", i)
			   in, _ := cohort.InCanary(uid, "")
			   if in {
				   t.Errorf("user %s should be baseline with 0%% ratio, got canary", uid)
			   }
		  }
	   })
	  t.Run("比例100%全走canary", func(t *testing.T) {
		  cohort := cfgdual.NewCohort("rel-003", "kyc0", 100, 1)
		   for i := 0; i < 200; i++ {
			   uid := fmt.Sprintf("u-%d", i)
			   in, _ := cohort.InCanary(uid, "")
			   if !in {
				   t.Errorf("user %s should be canary with 100%% ratio, got baseline", uid)
			   }
		  }
	  })
	  t.Run("用户Pin优先于比例", func(t *testing.T) {
		  cases := []struct {
			  pinSide    string
			   wantCanary bool
		  }{
			   {"canary", true},
			  {"baseline", false},
		   }
		   for _, tc := range cases {
			   cohort := cfgdual.NewCohort("rel-004", "kyc0", 50, 1)
			  _ = cohort.PinUser("pinned-user", tc.pinSide)
			  got, _ := cohort.InCanary("pinned-user", "USDT")
			  if got != tc.wantCanary {
				  t.Errorf("pin_side=%s: got InCanary=%v, want %v", tc.pinSide, got, tc.wantCanary)
			  }
		   }
	   })
	  t.Run("资产Pin优先于比例", func(t *testing.T) {
		  cohort := cfgdual.NewCohort("rel-005", "kyc0", 0, 1) // 0% 全 baseline
		   _ = cohort.PinAsset("BTC", "canary")
		  got, _ := cohort.InCanary("any-user", "BTC")
		  if !got {
			  t.Error("BTC asset pinned to canary but got baseline")
		  }
		  got2, _ := cohort.InCanary("any-user", "USDT")
		  if got2 {
			  t.Error("USDT not pinned, 0% ratio, should be baseline")
		   }
	   })
	  t.Run("SetPercent世代自增", func(t *testing.T) {
		   cohort := cfgdual.NewCohort("rel-006", "kyc0", 10, 1)
		   gen1 := cohort.Generation()
		  newGen, err := cohort.SetPercent(50)
		   if err != nil {
			  t.Fatalf("SetPercent: %v", err)
		   }
		  if newGen != gen1+1 {
			  t.Errorf("expected gen %d, got %d", gen1+1, newGen)
		   }
		  if cohort.Generation() != newGen {
			   t.Errorf("Generation() = %d, want %d", cohort.Generation(), newGen)
		  }
	  })
	   t.Run("非法比例返回错误", func(t *testing.T) {
		   cohort := cfgdual.NewCohort("rel-007", "kyc0", 10, 1)
		   for _, bad := range []int{-1, 101, 200, -100} {
			   _, err := cohort.SetPercent(bad)
			  if err == nil {
				   t.Errorf("SetPercent(%d) should fail", bad)
			   }
		  }
	  })
	   t.Run("Pin非法side返回错误", func(t *testing.T) {
		   cohort := cfgdual.NewCohort("rel-008", "kyc0", 10, 1)
		  err := cohort.PinUser("user-1", "invalid-side")
		  if err == nil {
			  t.Error("PinUser with invalid side should fail")
		   }
		   err = cohort.PinUser("", "canary")
		   if err == nil {
			   t.Error("PinUser with empty userID should fail")
		   }
	   })
	  t.Run("Unpin后恢复比例切流", func(t *testing.T) {
		   cohort := cfgdual.NewCohort("rel-009", "kyc0", 0, 1) // 0% 全 baseline
		   uid := "test-user"
		  _ = cohort.PinUser(uid, "canary")
		   in1, _ := cohort.InCanary(uid, "")
		  if !in1 {
			  t.Error("pinned to canary: expected in_canary=true")
		   }
		   cohort.UnpinUser(uid)
		   in2, _ := cohort.InCanary(uid, "")
		  if in2 {
			   t.Error("unpinned with 0%: expected in_canary=false")
		   }
	   })
	   t.Run("Snapshot包含当前状态", func(t *testing.T) {
		  cohort := cfgdual.NewCohort("rel-010", "kyc0", 25, 3)
		   _ = cohort.PinUser("u1", "canary")
		  _ = cohort.PinAsset("ETH", "baseline")
		   snap := cohort.Snapshot()
		  if snap.Percent != 25 {
			  t.Errorf("Percent=%d, want 25", snap.Percent)
		  }
		   if snap.Generation != 3 {
			   t.Errorf("Generation=%d, want 3", snap.Generation)
		  }
		  if snap.UserPins["u1"] != "canary" {
			  t.Error("user pin not in snapshot")
		  }
		   if snap.AssetPins["ETH"] != "baseline" {
			  t.Error("asset pin not in snapshot")
		   }
	  })
   }

// ─────────────────────────────────────────────────────────────────────────────
// TestSealPolicyTable SealPolicy.ShouldEnforce 各组合
// ─────────────────────────────────────────────────────────────────────────────

   func TestSealPolicyTable(t *testing.T) {
	   // 与 golden/diff_aggregate_policy.json 对应
	   policy := cfgdual.SealPolicy{
		   MaxAllowDenyCount: 1,
		   MaxDiffRatio:      20.0,
	   }
	  cases := []struct {
		  name           string
		   allowDenyCount int64
		   denyAllowCount int64
		  limitDriftCount int64
		  killSwitch int64
		  diffRatio  float64
		   exceedsPolicy bool
		   wantEnforce bool
	  }{
		  {"allow_deny=1_触发", 1, 0, 0, 0, 10.0, false, true},
		   {"ratio=25_触发", 0, 5, 0, 0, 25.0, true, true},
		  {"kill_switch=1_触发", 0, 0, 0, 1, 0.0, false, true},
		  {"ratio=15_不触发", 0, 3, 2, 0, 15.0, false, false},
		  {"全零_不触发", 0, 0, 0, 0, 0.0, false, false},
		  {"exceeds_policy=true_触发", 0, 0, 100, 0, 19.9, true, true},
		   {"deny_allow=10_ratio=8_不触发", 0, 10, 0, 0, 8.0, false, false},
		   {"allow_deny=2_超阈值_触发", 2, 0, 0, 0, 5.0, false, true},
		  {"ratio=20_等于阈值_触发", 0, 0, 0, 0, 20.0, false, false}, // 20.0 恰好等于阈值，不超过
		  {"ratio=20.1_超出阈值_触发", 0, 0, 0, 0, 20.1, false, true},
		  {"allow_deny=0_ratio=19.99_不触发", 0, 0, 5, 0, 19.99, false, false},
	  }
	  for _, tc := range cases {
		   tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   diff := &cfgdual.SplitDiff{
				   AllowDenyCount:  tc.allowDenyCount,
				   DenyAllowCount:  tc.denyAllowCount,
				   LimitDriftCount: tc.limitDriftCount,
				   KillSwitchCount: tc.killSwitch,
				  DiffRatio:       tc.diffRatio,
				   ExceedsPolicy: tc.exceedsPolicy,
			  }
			  got := policy.ShouldEnforce(diff)
			  if got != tc.wantEnforce {
				   t.Errorf("ShouldEnforce=%v, want %v (allow_deny=%d ratio=%.2f ks=%d exceeds=%v)",
					   got, tc.wantEnforce, tc.allowDenyCount, tc.diffRatio, tc.killSwitch, tc.exceedsPolicy)
			   }
		  })
	  }
	  t.Run("nil_diff_不触发", func(t *testing.T) {
		   got := policy.ShouldEnforce(nil)
		  if got {
			  t.Error("ShouldEnforce(nil) should return false")
		   }
	   })
  }

// ─────────────────────────────────────────────────────────────────────────────
// TestSealGateFSMTable SealGate 状态机完整迁移
// ─────────────────────────────────────────────────────────────────────────────

  func TestSealGateFSMTable(t *testing.T) {
	  // 与 golden/seal_gate_fsm.json 对应
	   cases := []struct {
		   name string
		   setup  func(g *cfgdual.SealGate)
		  action string
		   wantState cfgdual.SealState
		  wantError bool
		   idempotent bool
	   }{
		   {
			   name:  "open→enforce→enforced",
			  setup: func(g *cfgdual.SealGate) {},
			   action: "enforce", wantState: cfgdual.SealEnforced,
		  },
		  {
			  name: "enforced→enforce幂等",
			  setup: func(g *cfgdual.SealGate) { _, _ = g.Enforce("d1", "r1", "sys") },
			  action: "enforce", wantState: cfgdual.SealEnforced, idempotent: true,
		   },
		   {
			  name:   "enforced→waive→waived",
			  setup:  func(g *cfgdual.SealGate) { _, _ = g.Enforce("d1", "r1", "sys") },
			  action: "waive", wantState: cfgdual.SealWaived,
		   },
		   {
			   name: "waived→waive幂等",
			   setup:  func(g *cfgdual.SealGate) { _, _ = g.Waive("r1", "ops") },
			   action: "waive", wantState: cfgdual.SealWaived, idempotent: true,
		  },
		   {
			   name:  "waived→reset→open",
			  setup: func(g *cfgdual.SealGate) { _, _ = g.Waive("r1", "ops") },
			  action: "reset", wantState: cfgdual.SealOpen,
		  },
		   {
			   name: "open→reset幂等",
			  setup:  func(g *cfgdual.SealGate) {},
			  action: "reset", wantState: cfgdual.SealOpen, idempotent: true,
		   },
		   {
			   name:   "open→waive（预防性豁免）",
			  setup:  func(g *cfgdual.SealGate) {},
			   action: "waive", wantState: cfgdual.SealWaived,
		   },
		  {
			   name: "waived→enforce非法",
			  setup: func(g *cfgdual.SealGate) { _, _ = g.Waive("r1", "ops") },
			  action: "enforce", wantError: true,
		   },
	  }
	   for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			  gate := cfgdual.NewSealGate("rel-test", newIDGen())
			  tc.setup(gate)
			   before := gate.State()
			  var act interface{ nonnil() } // 仅用于类型断言检查
			  var err error
			  switch tc.action {
			  case "enforce":
				  a, e := gate.Enforce("diff-x", "test reason", "test-actor")
				  err = e
				  if !tc.wantError && !tc.idempotent && a != nil {
					  _ = a
				   }
				   if tc.idempotent && a != nil {
					   t.Errorf("idempotent enforce should return nil action, got non-nil")
				   }
			  case "waive":
				   a, e := gate.Waive("test reason", "ops")
				   err = e
				   if tc.idempotent && a != nil {
					  t.Errorf("idempotent waive should return nil action, got non-nil")
				   }
				  _ = a
			   case "reset":
				  a, e := gate.Reset("test reason", "ops")
				  err = e
				  if tc.idempotent && a != nil {
					  t.Errorf("idempotent reset should return nil action, got non-nil")
				  }
				  _ = a
			  }
			   _ = act
			  if tc.wantError {
				   if err == nil {
					   t.Errorf("expected error for %s from state %s, got nil", tc.action, before)
				   }
				   return
			  }
			  if err != nil {
				   t.Errorf("unexpected error: %v", err)
			  }
			  if gate.State() != tc.wantState {
				   t.Errorf("state=%s, want %s", gate.State(), tc.wantState)
			   }
		   })
	  }
  }

// ─────────────────────────────────────────────────────────────────────────────
// TestClassifyDiffTable 差分分类完整覆盖
// ─────────────────────────────────────────────────────────────────────────────
// classifyDiffExported 通过 Engine.Eval 的返回值反查差分分类
// （classifyDiff 是包内私有，通过 DualResult 验证）

  func runEvalAndGetDiff(
	   t *testing.T, mgr *cfgdual.Manager, releaseID string,
	  userID, amount string,
   ) cfgdual.DualResult {
	  t.Helper()
	  result, _ := mgr.Engine.Eval(context.Background(), releaseID, cfgdual.EvalRequest{
		   UserID: userID, Pack: "kyc0", Asset: "USDT", Amount: amount,
	  })
	  return result
   }

  func TestClassifyDiffTable(t *testing.T) {
	  cases := []struct {
		  name string
		   amount  string
		  pinSide string
		   wantHasDiff bool
		  wantDiffKind cfgdual.DiffKind
	   }{
		   {
			   name: "limit_drift: 300 < both limits, versions differ",
			  amount: "300", pinSide: "canary",
			  wantHasDiff: true, wantDiffKind: cfgdual.DiffLimit,
		  },
		  {
			   name: "no_diff: baseline_only, not in canary cohort",
			   amount: "300", pinSide: "baseline",
			   wantHasDiff: false, wantDiffKind: "",
		   },
		   {
			  name:   "allow_deny: 800 > canary 500",
			  amount: "800", pinSide: "canary",
			  wantHasDiff: true, wantDiffKind: cfgdual.DiffAllow,
		   },
		   {
			  name:   "limit_drift: 400 < 500 canary, both allow but different versions",
			  amount: "400", pinSide: "canary",
			  wantHasDiff: true, wantDiffKind: cfgdual.DiffLimit,
		  },
	  }
	  ctx := context.Background()
	   cfgSt := buildCfgStore(t)
	   mgr := cfgdual.NewManager(cfgSt, nil, newIDGen())
	   rel, err := mgr.CreateRelease(ctx, cfgdual.CreateReleaseRequest{
		   PackName: "kyc0", BaselineVersion: 2, CanaryVersion: 3,
		   CohortPercent: 0, // 全走 baseline，通过 Pin 控制
		  PolicyAllowDrift: 5, Creator: "test",
	   })
	  if err != nil {
		   t.Fatalf("create release: %v", err)
	  }
	   if err := mgr.ActivateRelease(ctx, rel); err != nil {
		  t.Fatalf("activate: %v", err)
	   }
	  for _, tc := range cases {
		   tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   cohort, _ := mgr.Cohorts.Get(rel.ID)
			  uid := uuid.NewString()
			  _ = cohort.PinUser(uid, tc.pinSide)
			  result := runEvalAndGetDiff(t, mgr, rel.ID, uid, tc.amount)
			  if result.HasDiff != tc.wantHasDiff {
				   t.Errorf("HasDiff=%v, want %v (amount=%s pin=%s)",
					  result.HasDiff, tc.wantHasDiff, tc.amount, tc.pinSide)
			   }
			  if result.DiffKind != tc.wantDiffKind {
				  t.Errorf("DiffKind=%s, want %s", result.DiffKind, tc.wantDiffKind)
			  }
		  })
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// TestGoldenAllowDeny golden 夹具驱动：allow_deny_splitbrain
// ─────────────────────────────────────────────────────────────────────────────

type goldenAllowDenyFixture struct {
	Name string `json:"name"`
	Description string `json:"description"`
	Release     struct {
		PackName        string `json:"pack_name"`
		BaselineVersion int    `json:"baseline_version"`
		CanaryVersion   int    `json:"canary_version"`
		CohortPercent    int    `json:"cohort_percent"`
		PolicyAllowDrift int    `json:"policy_allow_drift"`
		Creator          string `json:"creator"`
	} `json:"release"`
	Cases []struct {
		ID   string `json:"id"`
		Desc string `json:"desc"`
		UserID string `json:"user_id"`
		Asset  string `json:"asset"`
		Amount         string `json:"amount"`
		WantHasDiff    bool   `json:"want_has_diff"`
		WantDiffKind   string `json:"want_diff_kind"`
		WantFinalAllow bool   `json:"want_final_allow"`
		WantSealed     bool   `json:"want_sealed"`
	} `json:"cases"`
}

  func TestGoldenAllowDeny(t *testing.T) {
	   data, err := os.ReadFile("../../testdata/golden/cfgdual/allow_deny_splitbrain.json")
	  if err != nil {
		  t.Skipf("golden fixture not found: %v", err)
	  }
	  var fix goldenAllowDenyFixture
	   if err := json.Unmarshal(data, &fix); err != nil {
		  t.Fatalf("parse fixture: %v", err)
	  }
	  ctx := context.Background()
	   cfgSt := buildCfgStore(t)
	   mgr := cfgdual.NewManager(cfgSt,
		  nil, newIDGen())
	  rel, err := mgr.CreateRelease(ctx, cfgdual.CreateReleaseRequest{
		  PackName:        fix.Release.PackName,
		   BaselineVersion: fix.Release.BaselineVersion,
		   CanaryVersion:   fix.Release.CanaryVersion,
		   CohortPercent:   fix.Release.CohortPercent,
		  PolicyAllowDrift: fix.Release.PolicyAllowDrift,
		  Creator:          fix.Release.Creator,
	   })
	  if err != nil {
		  t.Fatalf("create release: %v", err)
	   }
	  if err := mgr.ActivateRelease(ctx, rel); err != nil {
		  t.Fatalf("activate: %v", err)
	  }
	   cohort,
		   _ := mgr.Cohorts.Get(rel.ID)
	  for _, tc := range fix.Cases {
		   tc := tc
		  t.Run(tc.ID+"_"+tc.Desc, func(t *testing.T) {
			   _ = cohort.PinUser(tc.UserID, "canary")
			  result, _ := mgr.Engine.Eval(ctx, rel.ID, cfgdual.EvalRequest{
				   UserID: tc.UserID,
				  Pack:   fix.Release.PackName,
				  Asset:  tc.Asset, Amount: tc.Amount,
			  })
			  if result.HasDiff != tc.WantHasDiff {
				  t.Errorf("[%s] HasDiff=%v, want %v", tc.ID, result.HasDiff, tc.WantHasDiff)
			   }
			  if tc.WantDiffKind != "" && string(result.DiffKind) != tc.WantDiffKind {
				   t.Errorf("[%s] DiffKind=%s, want %s", tc.ID, result.DiffKind, tc.WantDiffKind)
			   }
			   if result.Sealed != tc.WantSealed {
				   t.Errorf("[%s] Sealed=%v, want %v", tc.ID, result.Sealed, tc.WantSealed)
			   }
		   })
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// TestGoldenSealEnforced golden 夹具驱动：seal_enforced
// ─────────────────────────────────────────────────────────────────────────────

type goldenSealFixture struct {
	Name string `json:"name"`
	PreSeal bool `json:"pre_seal"`
	Release struct {
		PackName string `json:"pack_name"`
		BaselineVersion int `json:"baseline_version"`
		CanaryVersion int `json:"canary_version"`
		CohortPercent int `json:"cohort_percent"`
		PolicyAllowDrift int `json:"policy_allow_drift"`
	} `json:"release"`
	Cases []struct {
		ID   string `json:"id"`
		Desc string `json:"desc"`
		UserID string `json:"user_id"`
		Asset  string `json:"asset"`
		Amount string `json:"amount"`
		WantHasDiff    bool `json:"want_has_diff"`
		WantFinalAllow bool `json:"want_final_allow"`
		WantSealed bool `json:"want_sealed"`
		WantError string `json:"want_error"`
	} `json:"cases"`
}

  func TestGoldenSealEnforced(t *testing.T) {
	  data, err := os.ReadFile("../../testdata/golden/cfgdual/seal_enforced.json")
	   if err != nil {
		  t.Skipf("golden fixture not found: %v", err)
	  }
	  var fix goldenSealFixture
	  if err := json.Unmarshal(data, &fix); err != nil {
		  t.Fatalf("parse: %v", err)
	  }
	   ctx := context.Background()
	   cfgSt := buildCfgStore(t)
	  mgr := cfgdual.NewManager(cfgSt, nil, newIDGen())
	  rel, err := mgr.CreateRelease(ctx, cfgdual.CreateReleaseRequest{
		   PackName:        fix.Release.PackName,
		   BaselineVersion: fix.Release.BaselineVersion,
		   CanaryVersion: fix.Release.CanaryVersion, CohortPercent: fix.Release.CohortPercent,
		  PolicyAllowDrift: fix.Release.PolicyAllowDrift,
	   })
	  if err != nil {
		   t.Fatalf("create release: %v", err)
	  }
	   if err := mgr.ActivateRelease(ctx, rel); err != nil {
		  t.Fatalf("activate: %v", err)
	  }
	  if fix.PreSeal {
		   gate := mgr.Seals.GetOrCreate(rel.ID)
		  _, _ = gate.Enforce("d0", "pre-seal for test", "system")
	  }
	  cohort, _ := mgr.Cohorts.Get(rel.ID)
	   for _, tc := range fix.Cases {
		  tc := tc
		  t.Run(tc.ID, func(t *testing.T) {
			   _ = cohort.PinUser(tc.UserID, "canary")
			  result, evalErr := mgr.Engine.Eval(ctx, rel.ID, cfgdual.EvalRequest{
				  UserID: tc.UserID, Pack: fix.Release.PackName,
				  Asset: tc.Asset, Amount: tc.Amount,
			  })
			   if tc.WantError != "" && evalErr == nil {
				  t.Errorf("[%s] expected error containing %q, got nil", tc.ID, tc.WantError)
			  }
			  if tc.WantFinalAllow != result.FinalAllow {
				   t.Errorf("[%s] FinalAllow=%v, want %v (err=%v)", tc.ID, result.FinalAllow, tc.WantFinalAllow, evalErr)
			   }
			  if tc.WantSealed != result.Sealed {
				  t.Errorf("[%s] Sealed=%v, want %v", tc.ID, result.Sealed, tc.WantSealed)
			   }
		   })
	   }
  }

// ─────────────────────────────────────────────────────────────────────────────
// TestGoldenKillSwitch golden 夹具驱动：kill_switch
// ─────────────────────────────────────────────────────────────────────────────

type goldenKillSwitchFixture struct {
	Release struct {
		PackName        string `json:"pack_name"`
		BaselineVersion int    `json:"baseline_version"`
		CanaryVersion int `json:"canary_version"`
		CohortPercent int `json:"cohort_percent"`
		PolicyAllowDrift int `json:"policy_allow_drift"`
	} `json:"release"`
	Cases []struct {
		ID   string `json:"id"`
		Desc string `json:"desc"`
		UserID string `json:"user_id"`
		Asset  string `json:"asset"`
		Amount string `json:"amount"`
		KillGlobal bool   `json:"kill_global"`
		KillPack   string `json:"kill_pack"`
		WantFinalAllow      bool   `json:"want_final_allow"`
		WantError           string `json:"want_error"`
		WantAppliedNonempty bool   `json:"want_applied_nonempty"`
	} `json:"cases"`
}

   func TestGoldenKillSwitch(t *testing.T) {
	   data, err := os.ReadFile("../../testdata/golden/cfgdual/kill_switch.json")
	  if err != nil {
		   t.Skipf("fixture not found: %v", err)
	   }
	  var fix goldenKillSwitchFixture
	   if err := json.Unmarshal(data, &fix); err != nil {
		   t.Fatalf("parse: %v", err)
	   }
	   ctx := context.Background()
	  for _, tc := range fix.Cases {
		   tc := tc
		   t.Run(tc.ID, func(t *testing.T) {
			  // 每个 case 独立构造 Store，避免 kill switch 状态泄漏
			   cfgSt := buildCfgStore(t)
			  mgr := cfgdual.NewManager(cfgSt, nil, newIDGen())
			  rel, err := mgr.CreateRelease(ctx, cfgdual.CreateReleaseRequest{
				   PackName: fix.Release.PackName,
				   BaselineVersion: fix.Release.BaselineVersion,
				  CanaryVersion:    fix.Release.CanaryVersion,
				   CohortPercent:    fix.Release.CohortPercent,
				   PolicyAllowDrift: fix.Release.PolicyAllowDrift,
			  })
			   if err != nil {
				   t.Fatalf("create: %v", err)
			   }
			  if err := mgr.ActivateRelease(ctx, rel); err != nil {
				   t.Fatalf("activate: %v", err)
			  }
			  cohort, _ := mgr.Cohorts.Get(rel.ID)
			   _ = cohort.PinUser(tc.UserID, "canary")
			   if tc.KillGlobal {
				   cfgSt.Kill("*")
			   }
			  if tc.KillPack != "" {
				   cfgSt.Kill(tc.KillPack)
			   }
			   result, evalErr := mgr.Engine.Eval(ctx, rel.ID, cfgdual.EvalRequest{
				  UserID: tc.UserID, Pack: fix.Release.PackName,
				  Asset: tc.Asset, Amount: tc.Amount,
			   })
			  if tc.WantError != "" && evalErr == nil {
				   t.Errorf("[%s] expected error, got nil", tc.ID)
			   }
			   if result.FinalAllow != tc.WantFinalAllow {
				  t.Errorf("[%s] FinalAllow=%v, want %v", tc.ID, result.FinalAllow, tc.WantFinalAllow)
			   }
			   if tc.WantAppliedNonempty && result.Applied == "" {
				   t.Errorf("[%s] Applied should not be empty", tc.ID)
			   }
		   })
	  }
   }

// ─────────────────────────────────────────────────────────────────────────────
// TestGoldenRollbackSequence golden 夹具驱动：rollback_sequence
// ─────────────────────────────────────────────────────────────────────────────

type rollbackStep struct {
	Step   int    `json:"step"`
	Action string `json:"action"`
	UserID string `json:"user_id"`
	Asset         string `json:"asset"`
	Amount        string `json:"amount"`
	ExpectHasDiff bool   `json:"expect_has_diff"`
	ExpectDiffKind string `json:"expect_diff_kind"`
	Actor string `json:"actor"`
	Reason             string `json:"reason"`
	TriggerSplitDiffID string `json:"trigger_split_diff_id"`
	ExpectRolledBackVersion int `json:"expect_rolled_back_version"`
	ExpectRestoredVersion       int  `json:"expect_restored_version"`
	ExpectGenerationIncremented bool `json:"expect_generation_incremented"`
	ExpectInCohort   *bool  `json:"expect_in_cohort"`
	ExpectApplied    string `json:"expect_applied"`
	ExpectFinalAllow *bool  `json:"expect_final_allow"`
	ExpectError string `json:"expect_error"`
}

type goldenRollbackFixture struct {
	Release struct {
		PackName string `json:"pack_name"`
		BaselineVersion int `json:"baseline_version"`
		CanaryVersion int `json:"canary_version"`
		CohortPercent    int `json:"cohort_percent"`
		PolicyAllowDrift int `json:"policy_allow_drift"`
	} `json:"release"`
	RollbackSequence []rollbackStep `json:"rollback_sequence"`
}

   func TestGoldenRollbackSequence(t *testing.T) {
	   data, err := os.ReadFile("../../testdata/golden/cfgdual/rollback_sequence.json")
	   if err != nil {
		   t.Skipf("fixture not found: %v", err)
	   }
	   var fix goldenRollbackFixture
	   if err := json.Unmarshal(data, &fix); err != nil {
		  t.Fatalf("parse: %v", err)
	   }
	  ctx := context.Background()
	   cfgSt := buildCfgStore(t)
	   mgr := cfgdual.NewManager(cfgSt, nil, newIDGen())
	   rel, err := mgr.CreateRelease(ctx, cfgdual.CreateReleaseRequest{
		   PackName: fix.Release.PackName,
		  BaselineVersion: fix.Release.BaselineVersion,
		  CanaryVersion: fix.Release.CanaryVersion,
		   CohortPercent:    fix.Release.CohortPercent,
		  PolicyAllowDrift: fix.Release.PolicyAllowDrift,
	  })
	   if err != nil {
		   t.Fatalf("create: %v", err)
	  }
	  if err := mgr.ActivateRelease(ctx, rel); err != nil {
		   t.Fatalf("activate: %v", err)
	  }
	  cohort,
		   _ := mgr.Cohorts.Get(rel.ID)
	   genBefore := rel.Generation
	  for _, step := range fix.RollbackSequence {
		  step := step
		  t.Run(fmt.Sprintf("step%d_%s", step.Step, step.Action), func(t *testing.T) {
			  switch step.Action {
			   case "eval_before_rollback":
				   _ = cohort.PinUser(step.UserID, "canary")
				  result, _ := mgr.Engine.Eval(ctx, rel.ID, cfgdual.EvalRequest{
					   UserID: step.UserID, Pack: fix.Release.PackName,
					   Asset: step.Asset, Amount: step.Amount,
				  })
				  if result.HasDiff != step.ExpectHasDiff {
					  t.Errorf("HasDiff=%v, want %v", result.HasDiff, step.ExpectHasDiff)
				   }
			   case "rollback":
				   proof, err := mgr.Rollback.Rollback(ctx, rel, step.TriggerSplitDiffID, step.Actor, step.Reason)
				   if err != nil {
					  t.Fatalf("rollback: %v", err)
				   }
				  if proof == nil {
					  t.Fatal("proof is nil")
				  }
			  case "verify_proof":
				  proof,
					  err := mgr.Rollback.GetProof(ctx, rel.ID)
				   if err != nil {
					   t.Fatalf("get proof: %v", err)
				   }
				   if step.ExpectRolledBackVersion > 0 && proof.RolledBackVersion != step.ExpectRolledBackVersion {
					   t.Errorf("RolledBackVersion=%d, want %d", proof.RolledBackVersion, step.ExpectRolledBackVersion)
				   }
				   if step.ExpectRestoredVersion > 0 && proof.RestoredVersion != step.ExpectRestoredVersion {
					   t.Errorf("RestoredVersion=%d, want %d", proof.RestoredVersion, step.ExpectRestoredVersion)
				  }
				  if step.ExpectGenerationIncremented && rel.Generation <= genBefore {
					   t.Errorf("generation should have incremented: before=%d after=%d", genBefore, rel.Generation)
				   }
			   case "eval_after_rollback":
				  result, _ := mgr.Engine.Eval(ctx, rel.ID, cfgdual.EvalRequest{
					   UserID: step.UserID, Pack: fix.Release.PackName,
					  Asset: step.Asset, Amount: step.Amount,
				  })
				   if step.ExpectApplied != "" && result.Applied != step.ExpectApplied {
					  t.Errorf("Applied=%s, want %s", result.Applied, step.ExpectApplied)
				  }
				  if step.ExpectFinalAllow != nil && result.FinalAllow != *step.ExpectFinalAllow {
					  t.Errorf("FinalAllow=%v, want %v", result.FinalAllow, *step.ExpectFinalAllow)
				  }
			  case "duplicate_rollback_forbidden":
				   _, err := mgr.Rollback.Rollback(ctx, rel, "", "ops", "dup")
				   if step.ExpectError != "" && err == nil {
					   t.Errorf("expected error %q, got nil", step.ExpectError)
				  }
				  if err != nil && !errors.Is(err, cfgdual.ErrRollbackProofExists) {
					  if step.ExpectError == "" {
						   t.Errorf("unexpected error: %v", err)
					  }
				   }
			   }
		   })
	  }
  }

// ─────────────────────────────────────────────────────────────────────────────
// TestReplayEngine 审计回放引擎
// ─────────────────────────────────────────────────────────────────────────────

  func TestReplayEngine(t *testing.T) {
	   ctx := context.Background()
	   cfgSt := buildCfgStore(t)
	   mgr := cfgdual.NewManager(cfgSt, nil, newIDGen())
	   rel, err := mgr.CreateRelease(ctx, cfgdual.CreateReleaseRequest{
		  PackName: "kyc0", BaselineVersion: 2, CanaryVersion: 3,
		   CohortPercent: 100, PolicyAllowDrift: 5,
	   })
	  if err != nil {
		  t.Fatalf("create: %v", err)
	   }
	   if err := mgr.ActivateRelease(ctx, rel); err != nil {
		  t.Fatalf("activate: %v", err)
	  }
	  // 写入历史决策：3 条 allow_deny 差分
	  for i := 0; i < 3; i++ {
		  d := &cfgdual.DualDecision{
			   ID:            uuid.NewString(),
			   GrayReleaseID: rel.ID,
			  Generation: rel.Generation,
			  UserID:     fmt.Sprintf("replay-user-%d", i),
			   Asset: "USDT",
			  Amount: "800",
			   PackName: "kyc0",
			   BaselineAllow: true,
			   CanaryAllow: false,
			  HasDiff:     true,
			   DiffKind:    cfgdual.DiffAllow,
			  Applied: "baseline",
			   CreatedAt: time.Now().UTC(),
		  }
		   if err := mgr.Store.SaveDecision(ctx, d); err != nil {
			   t.Fatalf("save decision: %v", err)
		   }
	   }
	  // 执行回放
	   replay := cfgdual.NewReplayEngine(mgr.Store, cfgSt)
	   report, err := replay.Replay(ctx, rel, 0, false, 100)
	  if err != nil {
		  t.Fatalf("replay: %v", err)
	  }
	   if report.TotalReplayed != 3 {
		   t.Errorf("TotalReplayed=%d, want 3", report.TotalReplayed)
	   }
	   // 回放与原始结论应一致（包配置未变）
	   if !report.Consistent {
		   t.Errorf("expected consistent report, mismatch=%d missing=%d",
			  report.MismatchCount, report.MissingPackCount)
	   }
	   // diffOnly 模式
	   reportDiff, err := replay.Replay(ctx, rel, 0, true, 100)
	  if err != nil {
		  t.Fatalf("replay diffOnly: %v", err)
	   }
	   if reportDiff.TotalReplayed != 3 {
		  t.Errorf("diffOnly TotalReplayed=%d, want 3", reportDiff.TotalReplayed)
	  }
   }

// ─────────────────────────────────────────────────────────────────────────────
// TestValidator Validator 前置校验
// ─────────────────────────────────────────────────────────────────────────────

  func TestValidator(t *testing.T) {
	  cfgSt := buildCfgStore(t)
	   v := cfgdual.NewValidator(cfgSt)
	   t.Run("valid_request", func(t *testing.T) {
		   r := v.ValidateCreateRequest(cfgdual.CreateReleaseRequest{
			   PackName: "kyc0", BaselineVersion: 2, CanaryVersion: 3,
			  CohortPercent:    10,
			   PolicyAllowDrift: 5,
		   })
		  if !r.Valid {
			  t.Errorf("should be valid, errors: %v", r.Errors)
		   }
	  })
	  t.Run("empty_pack_name", func(t *testing.T) {
		   r := v.ValidateCreateRequest(cfgdual.CreateReleaseRequest{
			  PackName: "", BaselineVersion: 2, CanaryVersion: 3, CohortPercent: 10,
		   })
		   if r.Valid {
			   t.Error("should fail with empty pack_name")
		  }
	   })
	  t.Run("canary_le_baseline", func(t *testing.T) {
		  r := v.ValidateCreateRequest(cfgdual.CreateReleaseRequest{
			  PackName: "kyc0", BaselineVersion: 3, CanaryVersion: 3, CohortPercent: 10,
		   })
		   if r.Valid {
			   t.Error("should fail when canary_version <= baseline_version")
		  }
	  })
	   t.Run("invalid_percent", func(t *testing.T) {
		   for _, bad := range []int{-1, 101} {
			  r := v.ValidateCreateRequest(cfgdual.CreateReleaseRequest{
				  PackName: "kyc0", BaselineVersion: 2, CanaryVersion: 3, CohortPercent: bad,
			   })
			  if r.Valid {
				  t.Errorf("percent %d should fail", bad)
			  }
		  }
	  })
	   t.Run("high_percent_warning", func(t *testing.T) {
		  r := v.ValidateCreateRequest(cfgdual.CreateReleaseRequest{
			   PackName: "kyc0", BaselineVersion: 2, CanaryVersion: 3, CohortPercent: 75,
		  })
		   if !r.Valid {
			  t.Error("should be valid (high percent is warning only)")
		  }
		  if len(r.Warnings) == 0 {
			  t.Error("should have warning for high percent")
		   }
	  })
	   t.Run("activation_tightening_warning", func(t *testing.T) {
		   r := v.ValidateActivation(&cfgdual.GrayRelease{
			   PackName: "kyc0", BaselineVersion: 2, CanaryVersion: 3,
		  })
		  // kyc0 v3 收紧了 v2 的限额，应有 warning
		   if !r.Valid {
			  t.Errorf("should be valid, errors: %v", r.Errors)
		  }
		  if len(r.Warnings) == 0 {
			  t.Error("should have tightening warning")
		   }
		  if r.PackDiff == nil {
			  t.Error("PackDiff should not be nil")
		   }
		  if !r.PackDiff.TighteningDetected {
			   t.Error("TighteningDetected should be true")
		   }
	  })
	   t.Run("activation_missing_pack", func(t *testing.T) {
		   r := v.ValidateActivation(&cfgdual.GrayRelease{
			  PackName: "kyc0", BaselineVersion: 99, CanaryVersion: 100,
		   })
		   if r.Valid {
			   t.Error("should fail for missing pack versions")
		  }
	   })
  }

// ─────────────────────────────────────────────────────────────────────────────
// TestConflictDetector 高危差分模式检测
// ─────────────────────────────────────────────────────────────────────────────

   func TestConflictDetector(t *testing.T) {
	   detector := cfgdual.DefaultConflictDetector()
	  t.Run("allow_deny_burst", func(t *testing.T) {
		  diffs := []*cfgdual.SplitDiff{
			  {GrayReleaseID: "r1", AllowDenyCount: 3, DiffRatio: 10.0, Generation: 1},
		   }
		   alerts := detector.Detect("r1", diffs, 1)
		  found := false
		   for _, a := range alerts {
			   if a.Pattern == cfgdual.ConflictAllowDenyBurst {
				   found = true
			  }
		   }
		  if !found {
			   t.Error("expected allow_deny_burst alert")
		  }
	  })
	   t.Run("high_diff_ratio_sustained", func(t *testing.T) {
		  diffs := []*cfgdual.SplitDiff{
			   {DiffRatio: 35.0, Generation: 1},
			   {DiffRatio: 32.0, Generation: 1},
			   {DiffRatio: 28.0,
				  Generation: 1},
		   }
		  alerts := detector.Detect("r2", diffs, 1)
		  found := false
		  for _, a := range alerts {
			   if a.Pattern == cfgdual.ConflictHighDiffRatio {
				  found = true
			   }
		   }
		   if !found {
			   t.Error("expected high_diff_ratio alert")
		   }
	   })
	   t.Run("generation_leak", func(t *testing.T) {
		  diffs := []*cfgdual.SplitDiff{
			  {Generation: 2}, // 当前世代 3 但记录是 2
		  }
		  alerts := detector.Detect("r3", diffs, 3)
		   found := false
		   for _, a := range alerts {
			   if a.Pattern == cfgdual.ConflictGenerationLeak {
				   found = true
			  }
		  }
		  if !found {
			   t.Error("expected generation_leak alert")
		  }
	   })
	   t.Run("no_alerts", func(t *testing.T) {
		  diffs := []*cfgdual.SplitDiff{
			   {AllowDenyCount: 0, DiffRatio: 5.0, Generation: 1},
		  }
		   alerts := detector.Detect("r4", diffs, 1)
		  if len(alerts) != 0 {
			   t.Errorf("expected no alerts, got %d", len(alerts))
		   }
	  })
	   t.Run("empty_diffs", func(t *testing.T) {
		   alerts := detector.Detect("r5", nil, 1)
		   if alerts != nil {
			   t.Error("empty diffs should return nil alerts")
		   }
	  })
  }

// ─────────────────────────────────────────────────────────────────────────────
// TestAdminOps 管理员操作集
// ─────────────────────────────────────────────────────────────────────────────

  func TestAdminOps(t *testing.T) {
	  ctx := context.Background()
	   mgr, rel := buildManager(t, 0, 5)
	  admin := cfgdual.NewAdminOps(mgr)
	   t.Run("BatchPin_succeeds", func(t *testing.T) {
		  users := []string{"user-1", "user-2", "user-3", "user-4", "user-5"}
		   result, err := admin.BatchPin(rel.ID, "ops-admin", "canary", users)
		   if err != nil {
			  t.Fatalf("BatchPin: %v", err)
		  }
		   if result.Succeeded != 5 {
			  t.Errorf("Succeeded=%d, want 5", result.Succeeded)
		  }
		   if result.Failed != 0 {
			   t.Errorf("Failed=%d, want 0", result.Failed)
		   }
		   if result.AuditID == "" {
			  t.Error("AuditID should not be empty")
		  }
		   // 验证 Pin 生效
		   cohort, _ := mgr.Cohorts.Get(rel.ID)
		   for _, uid := range users {
			   in, _ := cohort.InCanary(uid, "USDT")
			   if !in {
				   t.Errorf("user %s should be in canary after batch pin", uid)
			  }
		   }
	   })
	  t.Run("BatchUnpin_succeeds", func(t *testing.T) {
		  users := []string{"user-1", "user-2"}
		  n, err := admin.BatchUnpin(rel.ID, "ops-admin", users)
		  if err != nil {
			  t.Fatalf("BatchUnpin: %v", err)
		   }
		  if n != 2 {
			   t.Errorf("unpin count=%d, want 2", n)
		  }
	   })
	   t.Run("EmergencySeal", func(t *testing.T) {
		  act, err := admin.EmergencySeal(ctx, rel.ID, "diff-001", "ops-admin", "emergency block")
		   if err != nil {
			   t.Fatalf("EmergencySeal: %v", err)
		  }
		  if act == nil {
			   t.Fatal("expected seal action")
		  }
		  if act.ToState != cfgdual.SealEnforced {
			  t.Errorf("ToState=%s, want enforced", act.ToState)
		   }
		  // 重复封堵应幂等
		  act2, err2 := admin.EmergencySeal(ctx, rel.ID, "diff-002", "ops-admin", "dup seal")
		   if err2 != nil {
			  t.Fatalf("idempotent EmergencySeal: %v", err2)
		  }
		   if act2 != nil {
			  t.Error("idempotent seal should return nil action")
		   }
	   })
	  t.Run("SetPercent_with_audit", func(t *testing.T) {
		  genBefore := rel.Generation
		   newGen, err := admin.SetPercent(rel.ID, "ops-admin", 30)
		  if err != nil {
			   t.Fatalf("SetPercent: %v", err)
		   }
		   _ = genBefore // 世代自增由 Cohort 管理，不依赖 rel.Generation
		  if newGen <= 0 {
			   t.Errorf("newGen=%d, want >0", newGen)
		  }
	  })
	  t.Run("ExportSnapshot", func(t *testing.T) {
		   snap, err := admin.ExportSnapshot(ctx, rel.ID)
		  if err != nil {
			   t.Fatalf("ExportSnapshot: %v", err)
		   }
		  if snap.Release == nil {
			   t.Error("Release should not be nil in snapshot")
		  }
		  if snap.SealState != cfgdual.SealEnforced {
			  t.Errorf("SealState=%s, want enforced (set by EmergencySeal above)", snap.SealState)
		   }
		  if snap.SnapshotAt.IsZero() {
			  t.Error("SnapshotAt should not be zero")
		   }
	  })
	  t.Run("Stats", func(t *testing.T) {
		  stats := admin.Stats(ctx)
		  if stats == nil {
			  t.Fatal("stats should not be nil")
		  }
		  if stats.ActiveReleaseCount < 1 {
			   t.Errorf("expected >=1 active release, got %d", stats.ActiveReleaseCount)
		   }
	  })
	   t.Run("AuditLog_contains_ops", func(t *testing.T) {
		   log := admin.AuditLog()
		   if len(log) == 0 {
			  t.Error("audit log should not be empty after operations")
		   }
		   opsSeen := map[cfgdual.AdminOpKind]bool{}
		  for _, r := range log {
			   opsSeen[r.Op] = true
		   }
		  for _, wantOp := range []cfgdual.AdminOpKind{
			  cfgdual.AdminOpBatchPin, cfgdual.AdminOpBatchUnpin,
			  cfgdual.AdminOpEmergencySeal, cfgdual.AdminOpSetPercent,
			  cfgdual.AdminOpExportSnapshot,
		  } {
			   if !opsSeen[wantOp] {
				  t.Errorf("op %s not found in audit log", wantOp)
			   }
		   }
	  })
   }

// ─────────────────────────────────────────────────────────────────────────────
// TestGenerationConsistencyProof 世代切流一致性证明
// ─────────────────────────────────────────────────────────────────────────────

   func TestGenerationConsistencyProof(t *testing.T) {
	   cohort := cfgdual.NewCohort("rel-proof", "kyc0", 30, 1)
	   clock := fixedClock(time.Date(2026,
		  9, 21, 12, 0, 0, 0, time.UTC))
	   // 生成 50 个用户 ID
	   users := make([]string, 50)
	  for i := range users {
		  users[i] = fmt.Sprintf("proof-user-%03d", i)
	   }
	   proof := cfgdual.VerifyGenerationConsistency(cohort, users, "USDT", 20, clock)
	   if proof == nil {
		  t.Fatal("proof should not be nil")
	  }
	  if !proof.AllConsistent {
		   t.Errorf("expected AllConsistent=true, got false. Entries: %v", proof.Entries)
	   }
	   if proof.SampleCount != 50 {
		   t.Errorf("SampleCount=%d, want 50", proof.SampleCount)
	  }
	   if proof.VerifyRounds != 20 {
		  t.Errorf("VerifyRounds=%d, want 20", proof.VerifyRounds)
	  }
  }

// ─────────────────────────────────────────────────────────────────────────────
// TestMemStoreCompleteness MemStore CRUD 完整性
// ─────────────────────────────────────────────────────────────────────────────

  func TestMemStoreCompleteness(t *testing.T) {
	   ctx := context.Background()
	  st := cfgdual.NewMemStore()
	   now := time.Now().UTC()
	  rel := &cfgdual.GrayRelease{
		  ID: uuid.NewString(), PackName: "kyc0",
		  BaselineVersion: 2, CanaryVersion: 3,
		  State: cfgdual.StateCanary, CohortPercent: 10, Generation: 1,
		   PolicyAllowDrift: 5, Creator: "test",
		  CreatedAt: now, UpdatedAt: now,
	   }
	   if err := st.SaveRelease(ctx, rel); err != nil {
		  t.Fatalf("SaveRelease: %v", err)
	   }
	  got, err := st.GetRelease(ctx, rel.ID)
	  if err != nil || got.ID != rel.ID {
		  t.Fatalf("GetRelease: %v / %v", err, got)
	   }
	  list, err := st.ListReleases(ctx)
	   if err != nil || len(list) != 1 {
		   t.Fatalf("ListReleases: %v len=%d", err, len(list))
	   }
	  active, err := st.ListActiveReleases(ctx)
	   if err != nil || len(active) != 1 {
		  t.Fatalf("ListActiveReleases: %v len=%d", err, len(active))
	  }
	  // Decision
	   d := &cfgdual.DualDecision{
		   ID:            uuid.NewString(),
		   GrayReleaseID: rel.ID,
		   Generation:    1, UserID: "u1", Asset: "USDT", Amount: "100",
		   PackName: "kyc0", BaselineAllow: true, CanaryAllow: false,
		  HasDiff: true, DiffKind: cfgdual.DiffAllow,
		   Applied: "baseline", CreatedAt: now,
	   }
	  if err := st.SaveDecision(ctx, d); err != nil {
		  t.Fatalf("SaveDecision: %v", err)
	   }
	  decs,
		   err := st.ListDecisions(ctx, rel.ID, 10)
	   if err != nil || len(decs) != 1 {
		   t.Fatalf("ListDecisions: %v len=%d", err, len(decs))
	  }
	   // SplitDiff
	  diff := &cfgdual.SplitDiff{
		  ID: uuid.NewString(), GrayReleaseID: rel.ID,
		   Generation: 1, WindowStart: now.Add(-5 * time.Minute), WindowEnd: now,
		  TotalDecisions: 10, DiffCount: 3, DiffRatio: 30.0,
		   AllowDenyCount: 2, ExceedsPolicy: true, CreatedAt: now,
	  }
	  if err := st.SaveSplitDiff(ctx, diff); err != nil {
		  t.Fatalf("SaveSplitDiff: %v", err)
	   }
	   gotDiff,
		  err := st.GetSplitDiff(ctx, diff.ID)
	  if err != nil || gotDiff.ID != diff.ID {
		   t.Fatalf("GetSplitDiff: %v", err)
	  }
	   // SealAction
	  act := &cfgdual.SealAction{
		   ID: uuid.NewString(), GrayReleaseID: rel.ID,
		   FromState: cfgdual.SealOpen, ToState: cfgdual.SealEnforced,
		  Reason: "test", Actor: "system", CreatedAt: now,
	  }
	  if err := st.SaveSealAction(ctx, act); err != nil {
		   t.Fatalf("SaveSealAction: %v", err)
	  }
	   actions, err := st.ListSealActions(ctx, rel.ID)
	  if err != nil || len(actions) != 1 {
		  t.Fatalf("ListSealActions: %v len=%d", err, len(actions))
	   }
	   // RollbackProof
	  proof := &cfgdual.RollbackProof{
		  ID: uuid.NewString(), GrayReleaseID: rel.ID,
		  PackName: "kyc0", RolledBackVersion: 3, RestoredVersion: 2,
		   GenerationBefore: 1, GenerationAfter: 2,
		   Actor: "ops", Reason: "test rollback", CreatedAt: now,
	  }
	   if err := st.SaveRollbackProof(ctx, proof); err != nil {
		  t.Fatalf("SaveRollbackProof: %v", err)
	  }
	   gotProof, err := st.GetRollbackProof(ctx, rel.ID)
	   if err != nil || gotProof.ID != proof.ID {
		   t.Fatalf("GetRollbackProof: %v", err)
	   }
	  t.Log("PASS: MemStore CRUD completeness verified")
  }
// temporary guard removed after the production check
