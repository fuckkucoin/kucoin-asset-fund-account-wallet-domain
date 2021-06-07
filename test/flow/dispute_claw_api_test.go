package flow

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/dispclaw"
	"account-assets.kucoin.net/funding/internal/disputedesk"
)

  func init() {
	   gin.SetMode(gin.TestMode)
   }

// ─────────────────────────── HTTP 路由 ────────────────────────────────────

  func newTestRouter(svc *dispclaw.Service) *gin.Engine {
	   r := gin.New()
	  v1 := r.Group("/v1")
	   dispclaw.Register(v1, nil)
	  dispclaw.RegisterOps(v1, nil, svc)
	  return r
   }

  func doJSONReq(t *testing.T, r *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	  t.Helper()
	   var req *http.Request
	  if body != nil {
		   b, err := json.Marshal(body)
		  if err != nil {
			  t.Fatal(err)
		   }
		   req = httptest.NewRequest(method, path, bytes.NewReader(b))
		   req.Header.Set("Content-Type", "application/json")
	   } else {
		  req = httptest.NewRequest(method, path, nil)
	   }
	  req.Header.Set("X-Role", "ops")
	  w := httptest.NewRecorder()
	  r.ServeHTTP(w, req)
	  return w
   }

// ─────────────────────────── Ops 运营台 API ─────────────────────────────

   func TestOpsAPI_Stats(t *testing.T) {
	  t.Parallel()
	   svc := dispclaw.New()
	  r := newTestRouter(svc)
	  w := doJSONReq(t, r, http.MethodGet, "/v1/ops/dispute-claw/stats", nil)
	   if w.Code != 200 {
		  t.Errorf("status=%d want 200", w.Code)
	   }
	   var resp map[string]interface{}
	  if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		   t.Fatal(err)
	   }
	   if _, ok := resp["counts"]; !ok {
		   t.Error("response missing 'counts' field")
	  }
   }

   func TestOpsAPI_Plans(t *testing.T) {
	  t.Parallel()
	  svc := dispclaw.New()
	   // 插入 3 个计划。
	  for i := 0; i < 3; i++ {
		  suffix := intToLetter(i)
		   svc.GeneratePlan(dispclaw.GeneratePlanReq{
			  CaseID: "case-opsapi-" + suffix,
			   PlanType: dispclaw.PlanTypeRefund,
			  Actor:          "ops-alice",
			   IdempotencyKey: "idem-opsapi-" + suffix,
		  }, "user-10001", "fiat", "USDT", "50.00", "ops-alice")
	  }
	   r := newTestRouter(svc)
	  w := doJSONReq(t, r, http.MethodGet, "/v1/ops/dispute-claw/plans", nil)
	   if w.Code != 200 {
		   t.Errorf("status=%d want 200, body: %s", w.Code, w.Body.String())
	  }
	  var resp map[string]interface{}
	  json.NewDecoder(w.Body).Decode(&resp)
	  count, _ := resp["count"].(float64)
	   if int(count) != 3 {
		   t.Errorf("count=%v want 3", count)
	  }
   }

  func TestOpsAPI_PlanSnapshot(t *testing.T) {
	   t.Parallel()
	  svc := dispclaw.New()
	  caseID := "case-snapshot-001"
	  svc.GeneratePlan(dispclaw.GeneratePlanReq{
		  CaseID: caseID,
		  PlanType: dispclaw.PlanTypeRefund,
		  Actor:    "ops-alice",
		   IdempotencyKey: "idem-snapshot-001",
	  }, "user-10001", "fiat", "USDT", "100.00", "ops-alice")
	  r := newTestRouter(svc)
	  w := doJSONReq(t, r, http.MethodGet, "/v1/ops/dispute-claw/plan/snapshot?case_id="+caseID, nil)
	   if w.Code != 200 {
		  t.Errorf("status=%d want 200, body: %s", w.Code, w.Body.String())
	  }
	  var resp map[string]interface{}
	  json.NewDecoder(w.Body).Decode(&resp)
	   if _, ok := resp["plan"]; !ok {
		   t.Error("snapshot missing 'plan'")
	   }
	   if _, ok := resp["legs"]; !ok {
		   t.Error("snapshot missing 'legs'")
	  }
	   if _, ok := resp["gate"]; !ok {
		  t.Error("snapshot missing 'gate'")
	  }
   }

  func TestOpsAPI_ProofPatrolAll(t *testing.T) {
	  t.Parallel()
	   svc := dispclaw.New()
	  caseID := "case-patrol-all"
	  plan, legs, _ := svc.GeneratePlan(dispclaw.GeneratePlanReq{
		   CaseID: caseID,
		   PlanType: dispclaw.PlanTypeRestore,
		   Actor: "ops-alice",
		   IdempotencyKey: "idem-patrol-all",
	   }, "user-10001", "fiat", "USDT", "100.00", "ops-alice")
	   for _, l := range legs {
		   svc.ExecuteLeg(dispclaw.ExecuteLegReq{PlanID: plan.ID, LegID: l.ID, Actor: "ops-bob"})
	   }
	   svc.CheckCloseGate(dispclaw.CloseGateCheckReq{CaseID: caseID, Actor: "ops-bob"})
	  svc.GenerateCloseProof(caseID, "ops-carol", "resolve")
	  r := newTestRouter(svc)
	   w := doJSONReq(t, r, http.MethodPost, "/v1/ops/dispute-claw/proof/patrol-all", nil)
	   if w.Code != 200 {
		   t.Errorf("status=%d want 200, body: %s", w.Code, w.Body.String())
	  }
	   var resp map[string]interface{}
	   json.NewDecoder(w.Body).Decode(&resp)
	   total, _ := resp["total"].(float64)
	   if int(total) != 1 {
		  t.Errorf("total=%v want 1", total)
	  }
	  tampered, _ := resp["tampered_count"].(float64)
	  if int(tampered) != 0 {
		  t.Errorf("tampered_count=%v want 0", tampered)
	   }
  }

// ─────────────────────────── 审计日志 ─────────────────────────────────────

  func TestAuditLog_Basic(t *testing.T) {
	  t.Parallel()
	   al := dispclaw.NewAuditLog()
	  al.Record("plan-001", "case-001", "ops-alice", dispclaw.AuditKindPlanGenerated, "plan created")
	  al.Record("plan-001", "case-001", "ops-bob", dispclaw.AuditKindLegExecuted, "leg-1 posted")
	  events := al.Query("plan-001", "")
	  if len(events) != 2 {
		  t.Errorf("events count=%d want 2", len(events))
	   }
	   if events[0].Kind != dispclaw.AuditKindPlanGenerated {
		   t.Errorf("events[0].Kind=%q want plan_generated", events[0].Kind)
	  }
  }

   func TestAuditLog_FilterByCaseID(t *testing.T) {
	  t.Parallel()
	   al := dispclaw.NewAuditLog()
	   al.Record("plan-A", "case-X", "ops-alice", dispclaw.AuditKindPlanGenerated, "")
	  al.Record("plan-B", "case-Y", "ops-alice", dispclaw.AuditKindPlanGenerated, "")
	  al.Record("plan-A", "case-X", "ops-bob", dispclaw.AuditKindLegExecuted, "")
	  byX := al.Query("", "case-X")
	   if len(byX) != 2 {
		   t.Errorf("case-X events=%d want 2", len(byX))
	   }
	  byY := al.Query("", "case-Y")
	  if len(byY) != 1 {
		  t.Errorf("case-Y events=%d want 1", len(byY))
	  }
  }

   func TestAuditLog_MarshalJSON(t *testing.T) {
	   t.Parallel()
	  al := dispclaw.NewAuditLog()
	  al.Record("plan-1", "case-1", "ops-alice", dispclaw.AuditKindGatePassed, "")
	  b, err := al.MarshalJSON()
	  if err != nil {
		   t.Fatal(err)
	   }
	  if !strings.Contains(string(b), "gate_passed") {
		   t.Errorf("JSON missing gate_passed: %s", b)
	  }
  }

  func TestAuditLog_Nil(t *testing.T) {
	  t.Parallel()
	  var al *dispclaw.AuditLog
	   // 不 panic。
	   al.Record("", "", "", dispclaw.AuditKindPlanGenerated, "")
	   events := al.Query("", "")
	   if events != nil {
		   t.Error("nil audit log should return nil")
	  }
	  all := al.All()
	  if all != nil {
		  t.Error("nil audit log all should return nil")
	   }
  }

// ─────────────────────────── 观察器适配器 ────────────────────────────────

   func TestObserveLeg_Default(t *testing.T) {
	   t.Parallel()
	  obs := dispclaw.DefaultObservers()
	   leg := dispclaw.RecoveryLeg{
		   UserID: "user-10001",
		  Asset:  "USDT",
		  HoldRef:      "hold-xyz",
		   RailBreakRef: "rb-001",
	  }
	  result := dispclaw.ObserveLeg(nil, obs, leg)
	  // DefaultObservers 返回空实现，所有值为默认零值。
	   if result.HoldAmount != "0" {
		   t.Errorf("HoldAmount=%q want 0", result.HoldAmount)
	  }
	  if result.HoldExists {
		  t.Error("HoldExists should be false for noop")
	  }
	  if result.RailBreakActive {
		  t.Error("RailBreakActive should be false for noop")
	   }
   }

// ─────────────────────────── GateChecker 接口 ──────────────────────────────
// TestGateCheckerFunc NewGateCheckerFunc 适配器。

  func TestGateCheckerFunc(t *testing.T) {
	  t.Parallel()
	   cases := []struct {
		   name      string
		   retStatus disputedesk.GateStatus
		  retErr    error
		  caseID string
	  }{
		   {"passed", disputedesk.GatePassed, nil, "case-fn-001"},
		  {"blocking", disputedesk.GateBlocking, nil, "case-fn-002"},
		   {"bypassed", disputedesk.GateBypassed, nil, "case-fn-003"},
	  }
	  for _, tc := range cases {
		   tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   t.Parallel()
			  checker := disputedesk.NewGateCheckerFunc(func(caseID string) (disputedesk.GateStatus, error) {
				   if caseID == tc.caseID {
					  return tc.retStatus, tc.retErr
				  }
				   return disputedesk.GatePassed, nil
			   })
			  got, err := checker.GateStatusForCase(tc.caseID)
			   if err != tc.retErr {
				  t.Errorf("err=%v want %v", err, tc.retErr)
			  }
			   if got != tc.retStatus {
				  t.Errorf("status=%q want %q", got, tc.retStatus)
			  }
		  })
	   }
  }

// TestGateChecker_BlockingDueToDispclaw 当 dispclaw.CloseGate 为 blocking 时，
// GateChecker 返回 GateBlocking；ResolveFourEye 应失败。
   func TestGateChecker_BlockingDueToDispclaw(t *testing.T) {
	   t.Parallel()
	  desk, clawSvc := buildDeskWithGate(t)
	  opener := "ops-a"
	   userID := "user-checker-001"
	  caseID := openCaseAndEvidence(t, desk, opener, userID, "idem-checker-001")
	   // 生成 refund 计划（腿 pending → gate blocking）。
	  clawSvc.GeneratePlan(dispclaw.GeneratePlanReq{
		   CaseID: caseID,
		  PlanType: dispclaw.PlanTypeRefund,
		  Actor:    opener,
		   IdempotencyKey: "idem-checker-001-plan",
	  }, userID, "fiat", "USDT", "100.00", opener)
	  // gate = blocking → Resolve 必须失败。
	   _, err := desk.ResolveFourEye(caseID, "ops-b", "resolve", "")
	   if err == nil {
		  t.Fatal("expected blocking error, got nil")
	   }
	  if !strings.Contains(err.Error(), "gate blocking") && !strings.Contains(err.Error(), "fund gate") {
		   t.Errorf("unexpected error: %v", err)
	  }
   }

// TestGateChecker_PassedAfterAllLegs 所有腿完成后 Resolve 应成功。

   func TestGateChecker_PassedAfterAllLegs(t *testing.T) {
	   t.Parallel()
	   desk, clawSvc := buildDeskWithGate(t)
	  opener := "ops-a2"
	  userID := "user-checker-002"
	  caseID := openCaseAndEvidence(t, desk, opener, userID, "idem-checker-002")
	   plan, legs, err := clawSvc.GeneratePlan(dispclaw.GeneratePlanReq{
		  CaseID: caseID,
		   PlanType:       dispclaw.PlanTypeRestore,
		  Actor:          opener,
		  IdempotencyKey: "idem-checker-002-plan",
	  }, userID, "fiat", "USDT", "50.00", opener)
	   if err != nil {
		   t.Fatal(err)
	  }
	   for _, leg := range legs {
		   clawSvc.ExecuteLeg(dispclaw.ExecuteLegReq{PlanID: plan.ID, LegID: leg.ID, Actor: "ops-b2"})
	   }
	  clawSvc.CheckCloseGate(dispclaw.CloseGateCheckReq{CaseID: caseID, Actor: "ops-b2"})
	   clawSvc.GenerateCloseProof(caseID, "ops-c2", "resolve")
	  // 现在 Resolve 应成功。
	   if _, err := desk.ResolveFourEye(caseID, "ops-c2", "resolve", ""); err != nil {
		   t.Fatalf("resolve should succeed: %v", err)
	  }
   }

// ─────────────────────────── 并发安全流程 ─────────────────────────────────
// TestDispClaw_ConcurrentPlanGeneration 并发生成多个计划不产生数据竞争。
  func TestDispClaw_ConcurrentPlanGeneration(t *testing.T) {
	   t.Parallel()
	   _, clawSvc := buildDeskWithGate(t)
	  const N = 20
	  done := make(chan error, N)
	   for i := 0; i < N; i++ {
		   i := i
		   go func() {
			  suffix := intToLetter(i % 26)
			   num := i / 26
			  caseID := "case-conc-flow-" + suffix + strconv.Itoa(num)
			   _, _, err := clawSvc.GeneratePlan(dispclaw.GeneratePlanReq{
				  CaseID:   caseID,
				   PlanType: dispclaw.PlanTypeRefund,
				   Actor:          "ops-alice",
				  IdempotencyKey: "idem-conc-flow-" + caseID,
			   }, "user-10001", "fiat", "USDT", "10.00", "ops-alice")
			  done <- err
		  }()
	  }
	  for i := 0; i < N; i++ {
		  if err := <-done; err != nil {
			   t.Errorf("concurrent generate: %v", err)
		  }
	   }
	  counts := clawSvc.Counts()
	  if counts["plans_open"] != N {
		   t.Errorf("plans_open=%d want %d", counts["plans_open"], N)
	  }
  }
