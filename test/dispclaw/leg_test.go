package dispclaw_test

import (
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/dispclaw"
)

// ─────────────────────────── 腿状态机转换 ─────────────────────────────────────

  func TestLegStatus_Terminal(t *testing.T) {
	  t.Parallel()
	   cases := []struct {
		   status dispclaw.LegStatus
		  terminal bool
	  }{
		   {dispclaw.LegStatusPending, false},
		  {dispclaw.LegStatusPosting, false},
		   {dispclaw.LegStatusPosted, true},
		   {dispclaw.LegStatusFailed, true},
		   {dispclaw.LegStatusSkipped, true},
	  }
	  for _, tc := range cases {
		   if tc.status.Terminal() != tc.terminal {
			  t.Errorf("LegStatus(%q).Terminal()=%v want %v", tc.status, tc.status.Terminal(), tc.terminal)
		   }
	  }
   }

   func TestPlanStatus_Terminal(t *testing.T) {
	   t.Parallel()
	  cases := []struct {
		  status   dispclaw.PlanStatus
		  terminal bool
	   }{
		   {dispclaw.PlanStatusOpen, false},
		   {dispclaw.PlanStatusExecuting,
			  false},
		  {dispclaw.PlanStatusClosed, true},
		   {dispclaw.PlanStatusAborted, true},
	  }
	   for _, tc := range cases {
		  if tc.status.Terminal() != tc.terminal {
			  t.Errorf("PlanStatus(%q).Terminal()=%v want %v", tc.status, tc.status.Terminal(), tc.terminal)
		   }
	   }
  }

// TestParsePlanType 覆盖所有合法/非法计划类型解析。

  func TestParsePlanType(t *testing.T) {
	   t.Parallel()
	   cases := []struct {
		   input string
		   want dispclaw.PlanType
		  wantErr bool
	  }{
		   {"claw_back", dispclaw.PlanTypeClawBack, false},
		   {"refund", dispclaw.PlanTypeRefund, false},
		   {"restore", dispclaw.PlanTypeRestore, false},
		   {"invalid", "", true},
		  {"", "", true},
		  {"CLAW_BACK", "", true}, // 大写不合法
	  }
	  for _, tc := range cases {
		  tc := tc
		  t.Run(tc.input, func(t *testing.T) {
			  got, err := dispclaw.ParsePlanType(tc.input)
			  if tc.wantErr {
				  if err == nil {
					   t.Errorf("ParsePlanType(%q): expected error, got %v", tc.input, got)
				  }
				  return
			  }
			  if err != nil {
				  t.Errorf("ParsePlanType(%q): unexpected error: %v", tc.input, err)
			   }
			  if got != tc.want {
				  t.Errorf("ParsePlanType(%q)=%q want %q", tc.input, got, tc.want)
			   }
		  })
	  }
  }

// TestParseLegType 覆盖所有合法/非法腿类型解析。
   func TestParseLegType(t *testing.T) {
	  t.Parallel()
	  cases := []struct {
		  input string
		  want  dispclaw.LegType
		   wantErr bool
	   }{
		   {"freeze", dispclaw.LegTypeFreeze, false},
		   {"claw_debit", dispclaw.LegTypeClawDebit, false},
		   {"refund_credit", dispclaw.LegTypeRefundCredit, false},
		  {"restore_unfreeze", dispclaw.LegTypeRestoreUnfreeze, false},
		  {"skip_marker", dispclaw.LegTypeSkipMarker, false},
		   {"invalid", "", true},
		  {"", "", true},
	  }
	   for _, tc := range cases {
		  tc := tc
		   t.Run(tc.input, func(t *testing.T) {
			  got, err := dispclaw.ParseLegType(tc.input)
			   if tc.wantErr {
				   if err == nil {
					  t.Errorf("ParseLegType(%q): expected error, got %v", tc.input, got)
				  }
				   return
			  }
			   if err != nil {
				   t.Errorf("ParseLegType(%q): unexpected error: %v", tc.input, err)
			  }
			  if got != tc.want {
				   t.Errorf("ParseLegType(%q)=%q want %q", tc.input, got, tc.want)
			  }
		   })
	  }
  }

// ─────────────────────────── 腿骨架结构验证 ───────────────────────────────────

  func TestLegBones_Structure(t *testing.T) {
	  t.Parallel()
	   cases := []struct {
		  planType      dispclaw.PlanType
		  wantLegCount  int
		  wantSeqs      []int
		   wantFirstType dispclaw.LegType
		   wantLastType dispclaw.LegType
	  }{
		  {
			  planType: dispclaw.PlanTypeClawBack,
			  wantLegCount: 2,
			   wantSeqs:     []int{1, 2},
			  wantFirstType: dispclaw.LegTypeFreeze,
			   wantLastType:  dispclaw.LegTypeClawDebit,
		   },
		  {
			  planType: dispclaw.PlanTypeRefund,
			   wantLegCount: 2,
			   wantSeqs:      []int{1, 2},
			   wantFirstType: dispclaw.LegTypeFreeze,
			   wantLastType:  dispclaw.LegTypeRefundCredit,
		  },
		  {
			  planType: dispclaw.PlanTypeRestore,
			  wantLegCount:  1,
			   wantSeqs:      []int{1},
			   wantFirstType: dispclaw.LegTypeRestoreUnfreeze,
			   wantLastType: dispclaw.LegTypeRestoreUnfreeze,
		   },
	   }
	   for _, tc := range cases {
		  tc := tc
		  t.Run(string(tc.planType), func(t *testing.T) {
			   t.Parallel()
			  svc := dispclaw.New()
			   caseID := "case-bones-" + string(tc.planType)
			  _, legs, err := svc.GeneratePlan(dispclaw.GeneratePlanReq{
				  CaseID:         caseID,
				  PlanType:       tc.planType,
				   Actor:          "ops-alice",
				  IdempotencyKey: "idem-bones-" + string(tc.planType),
			   }, "user-10001", "fiat", "USDT", "100.00", "ops-alice")
			   if err != nil {
				  t.Fatal(err)
			  }
			   if len(legs) != tc.wantLegCount {
				  t.Fatalf("leg count=%d want %d", len(legs), tc.wantLegCount)
			  }
			  for i, seq := range tc.wantSeqs {
				   if legs[i].Seq != seq {
					   t.Errorf("legs[%d].Seq=%d want %d", i, legs[i].Seq, seq)
				  }
			  }
			  if legs[0].LegType != tc.wantFirstType {
				   t.Errorf("first leg type=%q want %q", legs[0].LegType, tc.wantFirstType)
			  }
			   if legs[len(legs)-1].LegType != tc.wantLastType {
				   t.Errorf("last leg type=%q want %q", legs[len(legs)-1].LegType, tc.wantLastType)
			  }
			   // 所有腿应有唯一幂等键。
			   idemSet := map[string]struct{}{}
			  for _, l := range legs {
				   if l.IdempotencyKey == "" {
					  t.Errorf("leg seq=%d has empty idempotency_key", l.Seq)
				   }
				   if _, dup := idemSet[l.IdempotencyKey]; dup {
					  t.Errorf("duplicate idempotency_key: %s", l.IdempotencyKey)
				   }
				   idemSet[l.IdempotencyKey] = struct{}{}
			   }
		   })
	  }
   }

// ─────────────────────────── 腿哈希一致性 ─────────────────────────────────────

  func TestLegHash_Determinism(t *testing.T) {
	  t.Parallel()
	   testCases := []struct {
		   name string
		   in   dispclaw.LegHashInput
	   }{
		   {
			   name: "freeze leg",
			  in: dispclaw.LegHashInput{
				   PlanID:  "plan-123",
				   Seq:     1,
				   LegType: dispclaw.LegTypeFreeze,
				  UserID: "user-abc",
				   Asset:  "USDT",
				   Amount: "100.00",
				  Status: dispclaw.LegStatusPending,
			  },
		   },
		  {
			   name: "claw debit leg",
			  in: dispclaw.LegHashInput{
				  PlanID: "plan-456",
				  Seq:     2,
				   LegType: dispclaw.LegTypeClawDebit,
				   UserID:  "user-def",
				   Asset:  "BTC",
				  Amount: "0.05",
				   Status: dispclaw.LegStatusPosted,
			   },
		  },
		  {
			   name: "refund credit leg",
			   in: dispclaw.LegHashInput{
				   PlanID: "plan-789",
				   Seq:    2,
				   LegType: dispclaw.LegTypeRefundCredit,
				  UserID: "user-ghi",
				  Asset:  "ETH",
				   Amount: "1.234567",
				  Status: dispclaw.LegStatusFailed,
			   },
		  },
	   }
	  for _, tc := range testCases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			   h1 := dispclaw.ComputeLegHash(tc.in)
			  h2 := dispclaw.ComputeLegHash(tc.in)
			  if h1 != h2 {
				  t.Error("hash not deterministic")
			  }
			  if len(h1) != 64 {
				   t.Errorf("hash length=%d want 64 (SHA256 hex)", len(h1))
			  }
			  // 任意字段变化应导致哈希不同。
			  modified := tc.in
			  modified.Amount = tc.in.Amount + "1"
			  h3 := dispclaw.ComputeLegHash(modified)
			   if h1 == h3 {
				  t.Error("hash same after amount change, expected different")
			   }
		  })
	  }
  }

  func TestLegHash_FieldSensitivity(t *testing.T) {
	   t.Parallel()
	   base := dispclaw.LegHashInput{
		  PlanID:  "plan-base",
		   Seq:     1,
		   LegType: dispclaw.LegTypeFreeze,
		  UserID:  "user-base",
		   Asset:  "USDT",
		   Amount: "100.00",
		   Status: dispclaw.LegStatusPending,
	  }
	  baseHash := dispclaw.ComputeLegHash(base)
	  modifications := []struct {
		   name   string
		  modify func(*dispclaw.LegHashInput)
	   }{
		  {"plan_id", func(in *dispclaw.LegHashInput) { in.PlanID = "plan-modified" }},
		   {"seq", func(in *dispclaw.LegHashInput) { in.Seq = 2 }},
		  {"leg_type", func(in *dispclaw.LegHashInput) { in.LegType = dispclaw.LegTypeClawDebit }},
		  {"user_id", func(in *dispclaw.LegHashInput) { in.UserID = "user-modified" }},
		   {"asset", func(in *dispclaw.LegHashInput) { in.Asset = "BTC" }},
		  {"amount", func(in *dispclaw.LegHashInput) { in.Amount = "200.00" }},
		   {"status", func(in *dispclaw.LegHashInput) { in.Status = dispclaw.LegStatusPosted }},
	   }
	  for _, mod := range modifications {
		  mod := mod
		   t.Run(mod.name, func(t *testing.T) {
			  t.Parallel()
			  changed := base
			  mod.modify(&changed)
			  h := dispclaw.ComputeLegHash(changed)
			  if h == baseHash {
				   t.Errorf("hash unchanged after modifying %s", mod.name)
			   }
		   })
	   }
   }

// ─────────────────────────── 收口证明序列化 ─────────────────────────────────────

  func TestMarshalUnmarshalLegHashes(t *testing.T) {
	   t.Parallel()
	   cases := []struct {
		  name string
		  hashes []string
	   }{
		  {"empty", nil},
		  {"single", []string{"abc123"}},
		   {"multiple", []string{"h1", "h2", "h3", "h4"}},
		  {"with_special", []string{"hash-with-dash", "hash_with_under"}},
	   }
	   for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			   marshaled, err := dispclaw.MarshalLegHashes(tc.hashes)
			  if err != nil {
				  t.Fatal(err)
			  }
			   got, err := dispclaw.UnmarshalLegHashes(marshaled)
			  if err != nil {
				  t.Fatal(err)
			  }
			  if len(got) != len(tc.hashes) {
				   t.Errorf("count=%d want %d", len(got), len(tc.hashes))
			   }
			   for i := range tc.hashes {
				   if got[i] != tc.hashes[i] {
					  t.Errorf("[%d]=%q want %q", i, got[i], tc.hashes[i])
				  }
			  }
		   })
	   }
  }

// ─────────────────────────── VerifyProof ──────────────────────────────────

   func TestVerifyProof_EmptyLegs(t *testing.T) {
	   t.Parallel()
	  // 空腿集合的证明哈希与空输入哈希匹配。
	   emptyProofHash := dispclaw.ComputeProofHash(nil)
	  proof := dispclaw.CloseProof{
		  ProofHash: emptyProofHash,
		  LegHashes: "[]",
	  }
	  err := dispclaw.VerifyProof(proof, nil)
	   if err != nil {
		   t.Errorf("empty legs should verify: %v", err)
	  }
   }

  func TestVerifyProof_SingleLeg(t *testing.T) {
	  t.Parallel()
	   svc := dispclaw.New()
	   req := dispclaw.GeneratePlanReq{
		  CaseID:   "case-verify-single",
		  PlanType: dispclaw.PlanTypeRestore,
		   Actor:          "ops-alice",
		   IdempotencyKey: "idem-verify-single",
	   }
	  plan, legs, err := svc.GeneratePlan(req, "user-10001", "fiat", "USDT", "100.00", "ops-alice")
	   if err != nil {
		   t.Fatal(err)
	   }
	  for _, leg := range legs {
		   svc.ExecuteLeg(dispclaw.ExecuteLegReq{PlanID: plan.ID, LegID: leg.ID, Actor: "ops-bob"})
	   }
	  svc.CheckCloseGate(dispclaw.CloseGateCheckReq{CaseID: "case-verify-single", Actor: "ops-bob"})
	   proof, err := svc.GenerateCloseProof("case-verify-single", "ops-carol", "resolve")
	   if err != nil {
		   t.Fatal(err)
	  }
	  currentLegs := svc.ListLegs(plan.ID)
	   if err := dispclaw.VerifyProof(proof, currentLegs); err != nil {
		   t.Errorf("should verify: %v", err)
	  }
  }

// ─────────────────────────── 腿执行幂等性 ──────────────────────────────────────

  func TestExecuteLeg_PostedIsIdempotent(t *testing.T) {
	  t.Parallel()
	  svc := dispclaw.New()
	  req := dispclaw.GeneratePlanReq{
		   CaseID:   "case-exec-idem",
		  PlanType: dispclaw.PlanTypeRestore,
		  Actor:    "ops-alice",
		   IdempotencyKey: "idem-exec-idem",
	   }
	   plan, legs, err := svc.GeneratePlan(req, "user-10001", "fiat", "USDT", "50.00", "ops-alice")
	  if err != nil {
		  t.Fatal(err)
	  }
	  // 第一次执行成功。
	  execReq := dispclaw.ExecuteLegReq{PlanID: plan.ID, LegID: legs[0].ID, Actor: "ops-bob"}
	  result1, err := svc.ExecuteLeg(execReq)
	   if err != nil {
		  t.Fatal(err)
	  }
	  // 第二次执行同一腿应报 ErrLegNotPending。
	  _, err = svc.ExecuteLeg(execReq)
	  if err == nil {
		  t.Fatal("expected error for re-execute posted leg")
	   }
	   if !strings.Contains(err.Error(), "not in pending") {
		  t.Errorf("unexpected error: %v", err)
	  }
	  // 第一次结果的哈希应保持不变（幂等不改变已终态腿）。
	   result2Legs := svc.ListLegs(plan.ID)
	   for _, l := range result2Legs {
		   if l.ID == result1.ID && l.LegHash != result1.LegHash {
			  t.Error("hash changed on re-query")
		   }
	  }
  }

// ─────────────────────────── 计划状态推进验证 ──────────────────────────────────

   func TestPlan_StatusTransition(t *testing.T) {
	   t.Parallel()
	  svc := dispclaw.New()
	   req := dispclaw.GeneratePlanReq{
		  CaseID:         "case-status-trans",
		   PlanType:       dispclaw.PlanTypeRefund,
		  Actor:          "ops-alice",
		   IdempotencyKey: "idem-status-trans",
	   }
	   plan, legs, err := svc.GeneratePlan(req, "user-10001", "fiat", "USDT", "100.00", "ops-alice")
	  if err != nil {
		   t.Fatal(err)
	   }
	  // open 状态。
	   if plan.Status != dispclaw.PlanStatusOpen {
		   t.Errorf("initial status=%q want open", plan.Status)
	   }
	  // 执行第一条腿后计划应变为 executing。
	  svc.ExecuteLeg(dispclaw.ExecuteLegReq{PlanID: plan.ID, LegID: legs[0].ID, Actor: "ops-bob"})
	   p2, _ := svc.GetPlan(plan.ID)
	  if p2.Status != dispclaw.PlanStatusExecuting {
		  t.Errorf("after first leg: status=%q want executing", p2.Status)
	   }
	  // 执行第二条腿后生成证明，计划应变为 closed。
	   svc.ExecuteLeg(dispclaw.ExecuteLegReq{PlanID: plan.ID, LegID: legs[1].ID, Actor: "ops-bob"})
	  svc.CheckCloseGate(dispclaw.CloseGateCheckReq{CaseID: "case-status-trans", Actor: "ops-bob"})
	   svc.GenerateCloseProof("case-status-trans", "ops-carol", "resolve")
	  p3, _ := svc.GetPlan(plan.ID)
	   if p3.Status != dispclaw.PlanStatusClosed {
		  t.Errorf("after proof: status=%q want closed", p3.Status)
	   }
   }

// ─────────────────────────── 重试退避 ─────────────────────────────────────────

   func TestMarkLegFailed_Retry(t *testing.T) {
	  t.Parallel()
	   svc := dispclaw.New()
	  req := dispclaw.GeneratePlanReq{
		  CaseID: "case-retry",
		  PlanType: dispclaw.PlanTypeRefund,
		   Actor:          "ops-alice",
		   IdempotencyKey: "idem-retry",
	   }
	   _, legs, err := svc.GeneratePlan(req, "user-10001", "fiat", "USDT", "100.00", "ops-alice")
	   if err != nil {
		  t.Fatal(err)
	   }
	   // 第一次失败：pending→failed，retry_count=1。
	   nextRetry := time.Now().UTC().Add(2 * time.Minute)
	   result, err := svc.MarkLegFailed(legs[0].ID, "transient error attempt 1", &nextRetry)
	  if err != nil {
		  t.Fatalf("fail attempt 1: %v", err)
	  }
	  if result.Status != dispclaw.LegStatusFailed {
		  t.Errorf("status=%q want failed", result.Status)
	   }
	  if result.RetryCount != 1 {
		   t.Errorf("retry_count=%d want 1", result.RetryCount)
	  }
	   // 第二次对同一已 failed 腿调用 MarkLegFailed 应幂等返回，不再递增。
	  result2, err := svc.MarkLegFailed(legs[0].ID, "already failed", &nextRetry)
	  if err != nil {
		  t.Fatalf("fail attempt 2: %v", err)
	   }
	  if result2.RetryCount != 1 {
		  t.Errorf("idempotent: retry_count=%d want 1 (already terminal)", result2.RetryCount)
	   }
  }

// ─────────────────────────── 空服务防护 ───────────────────────────────────────

   func TestNilService_Safety(t *testing.T) {
	  t.Parallel()
	   var svc *dispclaw.Service
	  // 所有方法对 nil 服务应返回 ErrUnavailable 而不是 panic。
	   _, _, err := svc.GeneratePlan(dispclaw.GeneratePlanReq{}, "", "", "", "", "")
	  if err != dispclaw.ErrUnavailable {
		   t.Errorf("GeneratePlan nil: %v", err)
	  }
	   _, err = svc.ExecuteLeg(dispclaw.ExecuteLegReq{})
	   if err != dispclaw.ErrUnavailable {
		  t.Errorf("ExecuteLeg nil: %v", err)
	   }
	  _, err = svc.CheckCloseGate(dispclaw.CloseGateCheckReq{})
	  if err != dispclaw.ErrUnavailable {
		  t.Errorf("CheckCloseGate nil: %v", err)
	  }
	  _, err = svc.GenerateCloseProof("", "", "")
	  if err != dispclaw.ErrUnavailable {
		  t.Errorf("GenerateCloseProof nil: %v", err)
	   }
	   _, err = svc.PatrolProof("")
	  if err != dispclaw.ErrUnavailable {
		   t.Errorf("PatrolProof nil: %v", err)
	  }
	   if svc.ListLegs("") != nil {
		   t.Error("ListLegs nil: expected nil slice")
	  }
	   if svc.Counts() != nil {
		  t.Error("Counts nil: expected nil map")
	   }
   }
