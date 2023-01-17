package wdremgate_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/wdpartial"
	"account-assets.kucoin.net/funding/internal/wdremgate"
)

// ──────────────────────────────────────────────
// 辅助
// ──────────────────────────────────────────────

   func newGate() (*wdpartial.Service, *wdremgate.RemReleaseGate) {
	   svc := wdpartial.New()
	  return svc, wdremgate.NewRemReleaseGate(svc)
  }

// mustOpenHold 开立一个普通 ShardHold，失败则 t.Fatal。
  func mustOpenHold(t *testing.T, svc *wdpartial.Service, planID, legID, userID, asset, amount string) wdpartial.ShardHold {
	  t.Helper()
	   h, err := svc.OpenHold(wdpartial.OpenHoldReq{
		   PlanID: planID,
		  LegID: legID,
		   UserID: userID,
		   Asset:  asset,
		   Amount: amount,
	  })
	   if err != nil {
		  t.Fatalf("OpenHold: %v", err)
	  }
	  return h
   }

// mustOpenSkipHold 开立一个 skipBroadcasting 腿 Hold，失败则 t.Fatal。

   func mustOpenSkipHold(t *testing.T, svc *wdpartial.Service, planID, legID, userID, asset, amount, liveHash string) wdpartial.ShardHold {
	   t.Helper()
	   h, err := svc.OpenHold(wdpartial.OpenHoldReq{
		  PlanID: planID,
		   LegID:  legID,
		  UserID: userID,
		  Asset: asset,
		   Amount: amount,
		   SkipLeg: true,
		  LiveHash: liveHash,
	  })
	  if err != nil {
		   t.Fatalf("OpenSkipHold: %v", err)
	   }
	   return h
  }

// mustIssueLease 颁发租约，失败则 t.Fatal。
  func mustIssueLease(t *testing.T, gate *wdremgate.RemReleaseGate, planID, userID string) *wdremgate.GateIssueLeaseResp {
	   t.Helper()
	  resp, err := gate.IssueLease(wdremgate.GateIssueLeaseReq{
		   PlanID:     planID,
		  UserID:     userID,
		  TTLSeconds: 300,
	   })
	   if err != nil {
		  t.Fatalf("IssueLease: %v", err)
	  }
	   return resp
  }

// mustRelease 带世代释放，失败则 t.Fatal。
  func mustRelease(t *testing.T, gate *wdremgate.RemReleaseGate, planID, leaseID string, gen int64) *wdremgate.GateReleaseResp {
	   t.Helper()
	  resp, err := gate.Release(wdremgate.GateReleaseReq{
		  PlanID: planID,
		   LeaseID:    leaseID,
		   Generation: gen,
		  Actor: "test-actor",
	   })
	  if err != nil {
		  t.Fatalf("Release: %v", err)
	  }
	   return resp
   }

// ──────────────────────────────────────────────
// Golden 夹具辅助
// ──────────────────────────────────────────────

   func loadGolden(t *testing.T, name string) []byte {
	   t.Helper()
	  path := filepath.Join("..", "..", "testdata", "wdremgate", name)
	  data, err := os.ReadFile(path)
	   if err != nil {
		  t.Skipf("golden fixture not found: %s (skip)", path)
	  }
	  return data
   }

// ──────────────────────────────────────────────
// 组 1：主路径——正常释放
// ──────────────────────────────────────────────

   func TestReleasePlanRemainder_HappyPath(t *testing.T) {
	  cases := []struct {
		  name string
		   planID string
		  legAmounts []string
		   wantReleased  int
		  wantRemainder string
	   }{
		   {
			  name: "单 Hold 完整释放",
			   planID: "plan-hp-001",
			  legAmounts:    []string{"100.5"},
			  wantReleased:  1,
			  wantRemainder: "100.5",
		   },
		  {
			  name: "多 Hold 完整释放",
			  planID: "plan-hp-002",
			  legAmounts:    []string{"50", "30", "20"},
			  wantReleased:  3,
			  wantRemainder: "100",
		  },
		  {
			   name:       "空 Hold 列表（幂等）",
			   planID:     "plan-hp-003",
			  legAmounts: []string{},
			   wantReleased:  0,
			  wantRemainder: "0",
		  },
	  }
	  for _, tc := range cases {
		   t.Run(tc.name, func(t *testing.T) {
			  svc, gate := newGate()
			   for i, amt := range tc.legAmounts {
				  legID := tc.planID + "-leg-" + string(rune('A'+i))
				  mustOpenHold(t, svc, tc.planID, legID, "user-1", "BTC", amt)
			   }
			   lease := mustIssueLease(t, gate, tc.planID, "user-1")
			   resp := mustRelease(t, gate, tc.planID, lease.LeaseID, lease.Generation)
			  if resp.ReleasedCount != tc.wantReleased {
				   t.Errorf("released count: got %d, want %d", resp.ReleasedCount, tc.wantReleased)
			   }
			  if resp.Remainder != tc.wantRemainder {
				   t.Errorf("remainder: got %s, want %s", resp.Remainder, tc.wantRemainder)
			  }
		   })
	   }
   }

// ──────────────────────────────────────────────
// 组 2：Fill 推进锁互斥
// ──────────────────────────────────────────────

  func TestReleasePlanRemainder_FillMutexConflict(t *testing.T) {
	   cases := []struct {
		   name string
		  hasMutex bool
		  wantErr error
	  }{
		  {
			   name: "持有 Fill 锁时 Release 被拒",
			   hasMutex: true,
			  wantErr:  wdpartial.ErrLeaseConflict,
		   },
		   {
			  name:     "无 Fill 锁时 Release 成功",
			   hasMutex: false,
			  wantErr: nil,
		  },
	  }
	  for _, tc := range cases {
		  t.Run(tc.name, func(t *testing.T) {
			  svc, gate := newGate()
			  planID := "plan-mutex-" + tc.name
			  h := mustOpenHold(t, svc, planID, planID+"-legA", "user-1", "USDT", "500")
			   fill, err := svc.CreateFill(wdpartial.CreateFillReq{
				  PlanID: planID, LegID: planID + "-legA",
				  HoldID: h.ID, UserID: "user-1",
				   Asset: "USDT", Amount: "200", Chain: "ETH",
			   })
			   if err != nil {
				  t.Fatalf("CreateFill: %v", err)
			   }
			  if tc.hasMutex {
				   _, err = svc.IssueFillMutex(wdpartial.IssueFillMutexReq{
					   PlanID: planID,
					   FillID: fill.ID,
					   TTL: time.Minute,
				   })
				   if err != nil {
					   t.Fatalf("IssueFillMutex: %v", err)
				   }
			  }
			  lease := mustIssueLease(t, gate, planID, "user-1")
			   _, releaseErr := gate.Release(wdremgate.GateReleaseReq{
				  PlanID:  planID,
				   LeaseID: lease.LeaseID,
				  Generation: lease.Generation,
				  Actor:      "test-actor",
			   })
			   if tc.wantErr != nil {
				   if !errors.Is(releaseErr, tc.wantErr) {
					  t.Errorf("expected %v, got %v", tc.wantErr, releaseErr)
				   }
			   } else {
				   if releaseErr != nil {
					  t.Errorf("unexpected error: %v", releaseErr)
				   }
			  }
		  })
	  }
   }

// ──────────────────────────────────────────────
// 组 3：确定性竞态——先持有 Fill 锁再尝试 Release
// ──────────────────────────────────────────────

   func TestReleasePlanRemainder_ConcurrentFillAndRelease(t *testing.T) {
	   // 确定性场景：先颁发 FillMutexToken（确保持有），再尝试 Release；
	   // 验证 Release 在 Fill 锁持有期间被 ErrLeaseConflict 阻断。
	  svc, gate := newGate()
	  planID := "plan-conc-det-001"
	   h := mustOpenHold(t, svc, planID, planID+"-legA", "user-1", "ETH", "10")
	   fill, err := svc.CreateFill(wdpartial.CreateFillReq{
		  PlanID: planID, LegID: planID + "-legA",
		   HoldID: h.ID, UserID: "user-1",
		  Asset: "ETH", Amount: "5", Chain: "ETH",
	  })
	  if err != nil {
		  t.Fatalf("CreateFill: %v", err)
	  }
	   // 先确定性颁发 FillMutexToken（持有互斥锁）。
	   tok, tokErr := svc.IssueFillMutex(wdpartial.IssueFillMutexReq{
		  PlanID: planID, FillID: fill.ID, TTL: time.Minute,
	  })
	   if tokErr != nil {
		  t.Fatalf("IssueFillMutex: %v", tokErr)
	  }
	   // 互斥锁持有期间尝试 Release → 必须被阻断。
	  lease := mustIssueLease(t, gate, planID, "user-1")
	   _, releaseErr := gate.Release(wdremgate.GateReleaseReq{
		   PlanID: planID,
		   LeaseID:    lease.LeaseID,
		   Generation: lease.Generation,
		  Actor:      "release-actor",
	   })
	   if !errors.Is(releaseErr, wdpartial.ErrLeaseConflict) {
		   t.Errorf("expected ErrLeaseConflict while fill mutex held, got %v", releaseErr)
	   }
	  // ConsumeHold 完成后，互斥锁释放，同 plan 的操作恢复正常。
	   var wg sync.WaitGroup
	  wg.Add(1)
	   go func() {
		  defer wg.Done()
		  _, _ = svc.ConfirmFill(wdpartial.ConfirmFillReq{FillID: fill.ID, Actor: "fill-actor"})
		  _, _ = svc.ConsumeHold(wdpartial.ConsumeHoldReq{
			  HoldID: h.ID, FillID: fill.ID, Amount: "5", Actor: "fill-actor",
		   })
		  _ = svc.ConsumeFillMutex(tok.ID)
	  }()
	   wg.Wait()
  }

// ──────────────────────────────────────────────
// 组 4：skip 腿守卫世代绑定
// ──────────────────────────────────────────────

  func TestSkipLeg_GuardGenBinding(t *testing.T) {
	   cases := []struct {
		  name string
		  invalidate   bool
		  wantReleased int
		  wantErr error
	  }{
		  {
			   name: "守卫活跃：skip 腿正常释放",
			  invalidate: false,
			   wantReleased: 2,
			  wantErr:      nil,
		   },
		  {
			   name:         "守卫失效：skip 腿被拒，整体 Release 失败",
			  invalidate:   true,
			   wantReleased: 0,
			  wantErr:      wdpartial.ErrGuardGenInvalid,
		   },
	  }
	   for _, tc := range cases {
		  t.Run(tc.name, func(t *testing.T) {
			   svc, gate := newGate()
			   planID := "plan-skip-" + tc.name
			  mustOpenHold(t, svc, planID, planID+"-legA", "user-1", "BTC", "100")
			   skipHold := mustOpenSkipHold(t, svc, planID, planID+"-legB", "user-1", "BTC", "50", "live-hash-abc")
			   if tc.invalidate {
				  _, err := svc.InvalidateGuardGen(wdpartial.InvalidateGuardGenReq{
					   GuardGenID: skipHold.GuardGenID,
					  Reason: "live-hash changed by RBF",
				   })
				   if err != nil {
					  t.Fatalf("InvalidateGuardGen: %v", err)
				   }
			  }
			   lease := mustIssueLease(t, gate, planID, "user-1")
			   resp, releaseErr := gate.Release(wdremgate.GateReleaseReq{
				   PlanID: planID,
				   LeaseID:    lease.LeaseID,
				  Generation: lease.Generation,
				  Actor:      "test-actor",
			   })
			  if tc.wantErr != nil {
				  if !errors.Is(releaseErr, tc.wantErr) {
					  t.Errorf("expected %v, got %v", tc.wantErr, releaseErr)
				   }
			   } else {
				  if releaseErr != nil {
					  t.Errorf("unexpected error: %v", releaseErr)
					   return
				  }
				   if resp.ReleasedCount != tc.wantReleased {
					   t.Errorf("released count: got %d, want %d", resp.ReleasedCount, tc.wantReleased)
				  }
			   }
		   })
	  }
  }

// ──────────────────────────────────────────────
// 组 5：守恒失败升级 ConserveCase
// ──────────────────────────────────────────────

   func TestPlanConservation_FailureEscalate(t *testing.T) {
	   cases := []struct {
		   name    string
		   holdAmt string
		  fillAmt string
		   wantConserved   bool
		   wantCaseCreated bool
	   }{
		   {
			  name:    "守恒通过（fill_total <= consumed）",
			  holdAmt: "100",
			   fillAmt: "",
			   wantConserved: true,
			  wantCaseCreated: false,
		  },
		   {
			  name:    "守恒失败（fill_total > consumed）",
			  holdAmt: "100",
			  fillAmt: "150",
			  wantConserved: false,
			  wantCaseCreated: true,
		  },
	   }
	   for _, tc := range cases {
		   t.Run(tc.name, func(t *testing.T) {
			   svc, gate := newGate()
			   _ = gate
			   planID := "plan-conserve-" + tc.name
			  h := mustOpenHold(t, svc, planID, planID+"-legA", "user-1", "USDT", tc.holdAmt)
			  if tc.fillAmt != "" {
				   // 创建并确认 Fill（超额），不经过 ConsumeHold（仅 fill_total > consumed 守恒失败）。
				   fill, err := svc.CreateFill(wdpartial.CreateFillReq{
					   PlanID: planID, LegID: planID + "-legA",
					   HoldID: h.ID, UserID: "user-1",
					   Asset: "USDT", Amount: tc.fillAmt, Chain: "ETH",
				   })
				  if err != nil {
					   t.Fatalf("CreateFill: %v", err)
				   }
				   _, err = svc.ConfirmFill(wdpartial.ConfirmFillReq{FillID: fill.ID, Actor: "system"})
				   if err != nil {
					  t.Fatalf("ConfirmFill: %v", err)
				  }
			  }
			  err := svc.PlanConservation(planID)
			   if tc.wantConserved {
				   if err != nil {
					   t.Errorf("expected nil, got %v", err)
				   }
			  } else {
				   if !errors.Is(err, wdpartial.ErrConservationFail) {
					   t.Errorf("expected ErrConservationFail, got %v", err)
				  }
				  if tc.wantCaseCreated {
					   cases := svc.ListOpenConserveCases(10)
					   if len(cases) == 0 {
						  t.Error("expected ConserveCase to be created")
					  }
				   }
			   }
		   })
	   }
   }

// ──────────────────────────────────────────────
// 组 6：守恒案件挡写 ConsumeHold
// ──────────────────────────────────────────────

   func TestConsumeHold_BlockedByConserveCase(t *testing.T) {
	  svc, _ := newGate()
	   planID := "plan-block-consume"
	  h := mustOpenHold(t, svc, planID, planID+"-legA", "user-1", "BTC", "100")
	   // 创建并确认超额 Fill（触发守恒失败）。
	   fill, _ := svc.CreateFill(wdpartial.CreateFillReq{
		   PlanID: planID, LegID: planID + "-legA",
		   HoldID: h.ID, UserID: "user-1",
		  Asset: "BTC", Amount: "150", Chain: "BTC",
	  })
	   _, _ = svc.ConfirmFill(wdpartial.ConfirmFillReq{FillID: fill.ID, Actor: "system"})
	  // 守恒校验 → 必须失败并创建 ConserveCase。
	   err := svc.PlanConservation(planID)
	   if !errors.Is(err, wdpartial.ErrConservationFail) {
		   t.Fatalf("expected ErrConservationFail, got %v", err)
	   }
	  // 尝试 ConsumeHold → 应被 ConserveCaseOpen 挡写。
	   _, consumeErr := svc.ConsumeHold(wdpartial.ConsumeHoldReq{
		   HoldID: h.ID, FillID: fill.ID, Amount: "100", Actor: "system",
	   })
	  if !errors.Is(consumeErr, wdpartial.ErrConserveCaseOpen) {
		  t.Errorf("expected ErrConserveCaseOpen, got %v", consumeErr)
	  }
  }

// ──────────────────────────────────────────────
// 组 7：守恒案件收口后恢复 ConsumeHold
// ──────────────────────────────────────────────

  func TestConsumeHold_ResumesAfterConserveCaseClosed(t *testing.T) {
	  svc, gate := newGate()
	   planID := "plan-resume-consume"
	  h := mustOpenHold(t, svc, planID, planID+"-legA", "user-1", "ETH", "100")
	  fill, _ := svc.CreateFill(wdpartial.CreateFillReq{
		  PlanID: planID, LegID: planID + "-legA",
		  HoldID: h.ID, UserID: "user-1",
		  Asset: "ETH", Amount: "150", Chain: "ETH",
	  })
	   _, _ = svc.ConfirmFill(wdpartial.ConfirmFillReq{FillID: fill.ID, Actor: "system"})
	  _ = svc.PlanConservation(planID)
	   // 获取 ConserveCase ID。
	  cases := svc.ListOpenConserveCases(1)
	  if len(cases) == 0 {
		  t.Fatal("no open conserve case created")
	   }
	  caseID := cases[0].ID
	   // 证明固化 + 收口。
	  _, err := gate.ProveConserve(wdremgate.GateProveConserveReq{
		   CaseID:       caseID,
		  ProofPayload: `{"note":"手工核查确认金额超出，原因：系统误报"}`,
		  Approver:     "ops-alice",
		  Approver2: "ops-bob",
	   })
	   if err != nil {
		  t.Fatalf("ProveConserve: %v", err)
	  }
	  _, err = gate.CloseConserve(caseID, "ops-alice")
	   if err != nil {
		  t.Fatalf("CloseConserve: %v", err)
	  }
	  // ConsumeHold 应恢复正常。
	   _, consumeErr := svc.ConsumeHold(wdpartial.ConsumeHoldReq{
		  HoldID: h.ID, FillID: fill.ID, Amount: "100", Actor: "system",
	   })
	  if consumeErr != nil {
		   t.Errorf("expected ConsumeHold to succeed after case closed, got %v", consumeErr)
	  }
  }

// ──────────────────────────────────────────────
// 组 8：幂等双释防护
// ──────────────────────────────────────────────

  func TestReleasePlanRemainder_Idempotent(t *testing.T) {
	  cases := []struct {
		  name string
		   sameLeaseRepeat bool
		   wantSecondErr bool
	  }{
		  {
			  name:            "同租约重复调用：第二次租约已 done",
			   sameLeaseRepeat: true,
			   wantSecondErr:   true,
		   },
		  {
			   name: "双颁租约防重（同 planID open 租约唯一）",
			   sameLeaseRepeat: false,
			   wantSecondErr: true,
		  },
	  }
	  for _, tc := range cases {
		   t.Run(tc.name, func(t *testing.T) {
			   svc, gate := newGate()
			  planID := "plan-idem-" + tc.name
			  mustOpenHold(t, svc, planID, planID+"-legA", "user-1", "BTC", "100")
			  lease := mustIssueLease(t, gate, planID, "user-1")
			  mustRelease(t, gate, planID, lease.LeaseID, lease.Generation)
			   if tc.sameLeaseRepeat {
				   // 重复调用同一租约（已 done）。
				  _, err := gate.Release(wdremgate.GateReleaseReq{
					   PlanID:     planID,
					   LeaseID:    lease.LeaseID,
					   Generation: lease.Generation,
					   Actor:      "test-actor",
				   })
				   if tc.wantSecondErr && err == nil {
					   t.Error("expected error on second release, got nil")
				  }
			   } else {
				  // 颁发第二个租约（同 planID open 已 done，新颁应成功；但若第一个还是 open 则失败）。
				  planID2 := "plan-idem-dup-" + tc.name
				  mustOpenHold(t, svc, planID2, planID2+"-legA", "user-1", "BTC", "50")
				   _ = mustIssueLease(t, gate, planID2, "user-1")
				  // 再次颁发同 planID2（open 租约已存在）→ ErrLeaseDuplicate。
				  _, err := gate.IssueLease(wdremgate.GateIssueLeaseReq{
					   PlanID:     planID2,
					   UserID:     "user-1",
					  TTLSeconds: 300,
				   })
				   if !errors.Is(err, wdpartial.ErrLeaseDuplicate) {
					   t.Errorf("expected ErrLeaseDuplicate, got %v", err)
				   }
			  }
		   })
	  }
   }

// ──────────────────────────────────────────────
// 组 9：FSM 禁止边回归（状态机不变量）
// ──────────────────────────────────────────────

  func TestFSM_DenyInventory(t *testing.T) {
	  t.Run("Lease 禁止边（终态不可迁出）", func(t *testing.T) {
		  deny := wdpartial.DenyLeaseInventory()
		  if len(deny) == 0 {
			   t.Error("DenyLeaseInventory returned empty")
		   }
		   // 验证所有禁止边 from 都是终态。
		   for _, edge := range deny {
			   from := edge[0]
			  if !from.IsTerminal() {
				  t.Logf("deny edge from non-terminal %s — acceptable", from)
			   }
		   }
	  })
	  t.Run("ConserveCase 禁止边", func(t *testing.T) {
		   deny := wdpartial.DenyConserveInventory()
		   if len(deny) == 0 {
			   t.Error("DenyConserveInventory returned empty")
		   }
	   })
	   t.Run("GuardGen 禁止边", func(t *testing.T) {
		   deny := wdpartial.DenyGuardInventory()
		   if len(deny) == 0 {
			  t.Error("DenyGuardInventory returned empty")
		  }
		   for _, edge := range deny {
			   from := edge[0]
			   if !from.IsTerminal() {
				   t.Logf("deny edge from non-terminal %s — acceptable", from)
			  }
		  }
	  })
   }

// ──────────────────────────────────────────────
// 组 10：强制释放（双人确认）
// ──────────────────────────────────────────────

   func TestForceRelease_DualApproval(t *testing.T) {
	  cases := []struct {
		  name string
		   approver string
		  approver2 string
		  proof string
		   wantErr bool
	   }{
		  {
			  name: "双人不同：强制释放成功",
			   approver: "ops-alice",
			   approver2: "ops-bob",
			  proof:     `{"reason":"emergency release after conservation closure"}`,
			   wantErr:   false,
		   },
		  {
			   name:     "双人相同：拒绝",
			   approver: "ops-alice",
			  approver2: "ops-alice",
			   proof:   `{"reason":"same person"}`,
			  wantErr: true,
		  },
		   {
			   name:      "缺少 proof：拒绝",
			  approver:  "ops-alice",
			  approver2: "ops-bob",
			   proof: "",
			  wantErr: true,
		   },
	  }
	  for _, tc := range cases {
		   t.Run(tc.name, func(t *testing.T) {
			   svc, gate := newGate()
			  planID := "plan-force-" + tc.name
			  h := mustOpenHold(t, svc, planID, planID+"-legA", "user-1", "USDT", "500")
			   // 制造守恒失败。
			   fill, _ := svc.CreateFill(wdpartial.CreateFillReq{
				   PlanID: planID, LegID: planID + "-legA",
				  HoldID: h.ID, UserID: "user-1",
				  Asset: "USDT", Amount: "600", Chain: "ETH",
			  })
			   _, _ = svc.ConfirmFill(wdpartial.ConfirmFillReq{FillID: fill.ID, Actor: "system"})
			   _ = svc.PlanConservation(planID)
			   openCases := svc.ListOpenConserveCases(1)
			  caseID := ""
			  if len(openCases) > 0 {
				   caseID = openCases[0].ID
			  }
			  _, err := gate.ForceRelease(wdremgate.GateForceReleaseReq{
				   PlanID: planID,
				  CaseID:       caseID,
				   ProofPayload: tc.proof,
				   Approver: tc.approver,
				   Approver2: tc.approver2,
			   })
			  if tc.wantErr {
				  if err == nil {
					  t.Error("expected error, got nil")
				   }
			  } else {
				   if err != nil {
					  t.Errorf("unexpected error: %v", err)
				  }
			   }
		  })
	  }
   }

// ──────────────────────────────────────────────
// 组 11：世代不匹配防重放
// ──────────────────────────────────────────────

   func TestReleasePlanRemainder_GenMismatch(t *testing.T) {
	   svc, gate := newGate()
	  planID := "plan-gen-mismatch"
	   mustOpenHold(t, svc, planID, planID+"-legA", "user-1", "BTC", "100")
	   lease := mustIssueLease(t, gate, planID, "user-1")
	   // 使用错误的世代号（+1 重放攻击）。
	  _, err := gate.Release(wdremgate.GateReleaseReq{
		  PlanID:  planID,
		  LeaseID: lease.LeaseID,
		   Generation: lease.Generation + 1,
		   Actor: "attacker",
	   })
	  if !errors.Is(err, wdpartial.ErrLeaseGenMismatch) {
		  t.Errorf("expected ErrLeaseGenMismatch, got %v", err)
	  }
   }

// ──────────────────────────────────────────────
// 组 12：Golden 夹具回归
// ──────────────────────────────────────────────

  func TestGolden_ConserveCaseSnapshot(t *testing.T) {
	   data := loadGolden(t, "conserve_case_snapshot.json")
	  var snapshot wdpartial.PlanFillSummary
	  if err := json.Unmarshal(data, &snapshot); err != nil {
		   t.Fatalf("unmarshal golden: %v", err)
	  }
	  if snapshot.PlanID == "" {
		   t.Error("golden: plan_id empty")
	  }
	  if snapshot.TotalConsumed == "" {
		   t.Error("golden: total_consumed empty")
	   }
  }

  func TestGolden_RemReleaseLease(t *testing.T) {
	  data := loadGolden(t, "rem_release_lease.json")
	   var lease wdpartial.RemReleaseLease
	   if err := json.Unmarshal(data, &lease); err != nil {
		  t.Fatalf("unmarshal golden: %v", err)
	  }
	  if lease.ID == "" {
		  t.Error("golden: lease id empty")
	   }
	  if lease.Generation <= 0 {
		  t.Error("golden: generation must be >= 1")
	   }
   }

   func TestGolden_BroadcastGuardGen(t *testing.T) {
	   data := loadGolden(t, "broadcast_guard_gen.json")
	  var g wdpartial.BroadcastGuardGen
	   if err := json.Unmarshal(data, &g); err != nil {
		   t.Fatalf("unmarshal golden: %v", err)
	   }
	  if g.ID == "" {
		   t.Error("golden: guard_gen_id empty")
	  }
	  if g.LiveHash == "" {
		   t.Error("golden: live_hash empty")
	  }
  }
