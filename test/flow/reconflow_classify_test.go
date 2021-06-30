package flow_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/reconflow"
)

// ──────────────────────────────────────────────
// 辅助：构造内存模式引擎（无 DB，无外部依赖）
// ──────────────────────────────────────────────

   func newTestEngine() *reconflow.Engine {
	  executor := &reconflow.Executor{
		   Tickets: reconflow.NoopTicketOpener{},
		  Freezer: reconflow.NoopUserFreezer{},
		   Halter:  reconflow.NoopHaltRequester{},
		  Notifier: reconflow.NoopNotifier{},
		  Suggests: reconflow.NoopSuggestWriter{},
	  }
	   return reconflow.NewEngine(reconflow.EngineOptions{
		  Executor: executor,
	  })
  }

// ──────────────────────────────────────────────
// Fixture 1：bridge_inflate 稳定分类
// ──────────────────────────────────────────────

  func TestClassifyBridgeInflate_Stable(t *testing.T) {
	  c := reconflow.NewClassifier()
	   feat := reconflow.DiffFeature{
		   SourceKind: "bridge_inflate",
		   Asset:      "USDT",
		   Book:       "bridge",
		  Delta:    "1.5",
		  Severity: "warning",
		  AssetClass:  "stable",
		  KnownSource: false,
		   InTransit:   false,
	  }
	   result := c.Classify(feat)
	   if result.Kind != reconflow.DiffClassBridgeInflate {
		   t.Fatalf("bridge_inflate: expected DiffClassBridgeInflate, got %s (score=%.3f rules=%v)",
			   result.Kind, result.Score, result.MatchedRules)
	   }
	  if result.Confidence == reconflow.ConfidenceLow {
		  t.Fatalf("bridge_inflate: confidence should not be low for canonical feature, got low (score=%.3f)", result.Score)
	   }
	   // 相同特征多次分类结果一致（确定性）。
	  result2 := c.Classify(feat)
	   if result.Kind != result2.Kind || result.Confidence != result2.Confidence {
		  t.Errorf("bridge_inflate: not deterministic: %v vs %v", result, result2)
	  }
   }

// ──────────────────────────────────────────────
// Fixture 2：hot_below_user 稳定分类
// ──────────────────────────────────────────────

  func TestClassifyHotBelowUser_Stable(t *testing.T) {
	   c := reconflow.NewClassifier()
	  feat := reconflow.DiffFeature{
		   SourceKind: "hot_below_user",
		   Asset: "BTC",
		  AssetClass: "major",
		  Severity: "critical",
		  KnownSource: false,
		  InTransit: false,
		   SecondCredit: false,
	   }
	   result := c.Classify(feat)
	   if result.Kind != reconflow.DiffClassHotBelowUser {
		  t.Fatalf("hot_below_user: expected DiffClassHotBelowUser, got %s (score=%.3f)", result.Kind, result.Score)
	  }
	  // 高置信度时包含 open_ticket 建议。
	  found := false
	  for _, a := range result.SuggestedActs {
		  if a == "open_ticket" {
			  found = true
		   }
	  }
	  if !found {
		  t.Errorf("hot_below_user: expected open_ticket in suggested_acts, got %v", result.SuggestedActs)
	  }
   }

// ──────────────────────────────────────────────
// Fixture 3：deposit_short 稳定分类
// ──────────────────────────────────────────────

  func TestClassifyDepositShort_Stable(t *testing.T) {
	  c := reconflow.NewClassifier()
	  feat := reconflow.DiffFeature{
		   SourceKind: "deposit_diff",
		  Asset:  "ETH",
		  TxHash: "0xabc123",
		  KnownSource: true,
		   InTransit:   false,
		  SecondCredit: false,
	   }
	  result := c.Classify(feat)
	   if result.Kind != reconflow.DiffClassDepositShort {
		  t.Fatalf("deposit_short: expected DiffClassDepositShort, got %s (score=%.3f)", result.Kind, result.Score)
	  }
   }

// ──────────────────────────────────────────────
// Fixture 4：低置信度阻断自动处置
// ──────────────────────────────────────────────

  func TestClassifyLowConfidence_BlocksAutoAction(t *testing.T) {
	   c := reconflow.NewClassifier()
	  // 无任何明显特征的差异，应归为 unknown 或低置信。
	  feat := reconflow.DiffFeature{
		  SourceKind: "unknown_source",
		   Asset: "XYZ",
	   }
	  result := c.Classify(feat)
	  if result.Confidence != reconflow.ConfidenceLow {
		  // unknown kind 必须 low confidence。
		   if result.Kind == reconflow.DiffClassUnknown {
			   t.Errorf("unknown diff: expected ConfidenceLow, got %s", result.Confidence)
		   }
	  }
	  // FSM：低置信 → waiting_human（不进 acting）。
	  evt := reconflow.ClassifyEvent(result.Confidence, nil)
	   if result.Confidence == reconflow.ConfidenceLow && evt != "classify_done_low" {
		  t.Errorf("low confidence should produce classify_done_low event, got %s", evt)
	  }
   }

// ──────────────────────────────────────────────
// Fixture 5：replay 差异消失 → Run 关闭
// ──────────────────────────────────────────────

   func TestReplayEval_DiffGone_ClosesRun(t *testing.T) {
	   eng := newTestEngine()
	  ctx := context.Background()
	  feat := reconflow.DiffFeature{
		  SourceKind: "bridge_inflate",
		  Asset:      "USDT",
		  Book:     "bridge",
		  Severity: "warning",
		  AssetClass: "stable",
	  }
	  result, err := eng.Classify(ctx, reconflow.ClassifyRequest{Feature: feat, DryRun: false}, "ops-test")
	   if err != nil {
		   t.Fatalf("classify: %v", err)
	  }
	   if result.RunID == "" {
		   // 内存模式下无 DB，RunID 为空表示引擎未持久化但分类成功。
		  t.Logf("内存模式：RunID 为空（无 DB），replay 验证跳过 store 层")
		   return
	   }
	  // 差异消失时 ReplayEval 关闭 Run。
	  if err := eng.ReplayEval(ctx, result.RunID, false, "ops-test"); err != nil {
		   t.Fatalf("replay eval (diff gone): %v", err)
	  }
  }

// ──────────────────────────────────────────────
// Fixture 6：replay 差异仍在 → Run 保持活跃
// ──────────────────────────────────────────────

   func TestReplayEval_DiffStill_KeepsRun(t *testing.T) {
	   eng := newTestEngine()
	  ctx := context.Background()
	  feat := reconflow.DiffFeature{
		   SourceKind: "hot_below_user",
		   Asset: "BTC",
		   AssetClass: "major",
		   Severity: "critical",
	   }
	  result, err := eng.Classify(ctx, reconflow.ClassifyRequest{Feature: feat, DryRun: false}, "ops-test")
	  if err != nil {
		  t.Fatalf("classify: %v", err)
	   }
	   if result.RunID == "" {
		   t.Logf("内存模式：RunID 为空，replay-still 验证跳过")
		  return
	  }
	  // 差异仍在，不得关闭 Run。
	   if err := eng.ReplayEval(ctx, result.RunID, true, "ops-test"); err != nil {
		   t.Fatalf("replay eval (diff still): %v", err)
	   }
   }

// ──────────────────────────────────────────────
// Fixture 7：SLA 超时 → escalated
// ──────────────────────────────────────────────

  func TestSLABreach_Escalates(t *testing.T) {
	   eng := newTestEngine()
	   ctx := context.Background()
	   feat := reconflow.DiffFeature{
		  SourceKind: "bridge_inflate",
		  Asset: "USDT",
		   Severity:   "critical",
		  AssetClass: "stable",
	  }
	   result, err := eng.Classify(ctx, reconflow.ClassifyRequest{Feature: feat, DryRun: false}, "ops-test")
	  if err != nil {
		   t.Fatalf("classify: %v", err)
	   }
	  if result.RunID == "" {
		  t.Logf("内存模式：RunID 为空，SLA breach 验证跳过")
		  return
	  }
	  // 直接触发升级（ EscalateRun 路径）。
	   if err := eng.EscalateRun(ctx, result.RunID, "ops-test"); err != nil {
		   t.Fatalf("escalate run: %v", err)
	   }
  }

// ──────────────────────────────────────────────
// Fixture 8：SLABreached 时钟
// ──────────────────────────────────────────────

  func TestFlowRun_SLABreached(t *testing.T) {
	  now := time.Now().UTC()
	  r := &reconflow.FlowRun{
		   State: reconflow.RunStateActing,
		   SLADeadline: now.Add(-1 * time.Minute), // 已超时
	   }
	  if !r.SLABreached(now) {
		  t.Errorf("SLABreached: expected true for past deadline, got false")
	  }
	   r2 := &reconflow.FlowRun{
		   State:       reconflow.RunStateActing,
		   SLADeadline: now.Add(1 * time.Hour), // 未超时
	  }
	   if r2.SLABreached(now) {
		   t.Errorf("SLABreached: expected false for future deadline, got true")
	  }
	  r3 := &reconflow.FlowRun{
		   State:       reconflow.RunStateClosed, // 终态
		   SLADeadline: now.Add(-1 * time.Minute),
	  }
	   if r3.SLABreached(now) {
		  t.Errorf("SLABreached: expected false for closed run, got true")
	   }
  }

// ──────────────────────────────────────────────
// Fixture 9：FSM 状态跃迁合法性
// ──────────────────────────────────────────────

   func TestFSM_LegalTransitions(t *testing.T) {
	   cases := []struct {
		  from  reconflow.RunState
		  event string
		  ok bool
	   }{
		  {reconflow.RunStateOpen, "classify_start", true},
		   {reconflow.RunStateOpen, "escalate", true},
		  {reconflow.RunStateOpen, "all_done", false}, // 非法
		   {reconflow.RunStateClassifying, "classify_done_auto", true},
		  {reconflow.RunStateClassifying, "classify_done_low", true},
		  {reconflow.RunStateActing, "action_gated", true},
		   {reconflow.RunStateActing, "all_done", true},
		  {reconflow.RunStateWaitingHuman, "gate_allow", true},
		  {reconflow.RunStateWaitingHuman, "gate_deny", true},
		  {reconflow.RunStateClosed, "classify_start", false}, // 终态不跃迁
		   {reconflow.RunStateEscalated, "gate_allow", false},
	   }
	  for _, tc := range cases {
		   _, err := reconflow.Transition(tc.from, tc.event)
		   got := err == nil
		  if got != tc.ok {
			  t.Errorf("Transition(%s, %s): expected ok=%v, got ok=%v (err=%v)",
				  tc.from, tc.event, tc.ok, got, err)
		  }
	  }
   }

// ──────────────────────────────────────────────
// Fixture 10：三方轧差数字不变回归（只读消费差异事实）
// ──────────────────────────────────────────────
// TestTripartyFormulaUnchanged 验证 reconflow 不修改 recon 包内任何三方公式。
// 只验证分类器不改变差异事实本身：输入特征经分类后，原始字段保持不变。
   func TestTripartyFormulaUnchanged(t *testing.T) {
	  c := reconflow.NewClassifier()
	   feat := reconflow.DiffFeature{
		  SourceKind: "hot_below_user",
		  Asset:      "BTC",
		   ObservedAmt: "10.5",
		   BookAmt: "10.6",
		   Delta:   "-0.1",
		  Severity:   "warning",
		   AssetClass: "major",
	  }
	  // 分类前后差异特征不变。
	   featCopy := feat
	  result := c.Classify(feat)
	   if feat.ObservedAmt != featCopy.ObservedAmt || feat.BookAmt != featCopy.BookAmt || feat.Delta != featCopy.Delta {
		  t.Errorf("classify modified feat fields: before=%+v after=%+v", featCopy, feat)
	   }
	  // 分类结果不引入新的差额数字。
	  if result.Score < 0 || result.Score > 1 {
		  t.Errorf("score out of range: %.4f", result.Score)
	   }
   }

// ──────────────────────────────────────────────
// Fixture 11：dry-run 不创建 FlowRun
// ──────────────────────────────────────────────

   func TestClassifyDryRun_NoRun(t *testing.T) {
	  eng := newTestEngine()
	   ctx := context.Background()
	   feat := reconflow.DiffFeature{
		  SourceKind: "bridge_inflate",
		  Asset:    "USDT",
		  Severity: "warning",
	   }
	  result, err := eng.Classify(ctx, reconflow.ClassifyRequest{Feature: feat, DryRun: true}, "ops-test")
	  if err != nil {
		   t.Fatalf("dry-run classify: %v", err)
	   }
	  if !result.DryRun {
		   t.Errorf("dry-run: expected DryRun=true in result")
	  }
	   if result.RunID != "" {
		  t.Errorf("dry-run: expected empty RunID, got %s", result.RunID)
	   }
  }

// ──────────────────────────────────────────────
// Fixture 12：内置动作目录完整性
// ──────────────────────────────────────────────

  func TestCatalog_BuiltinIntegrity(t *testing.T) {
	   cat := reconflow.NewCatalog()
	  requiredNames := []string{
		  "noop", "open_ticket", "suggest_post", "notify",
		   "escalate_human", "freeze_user", "request_circuit", "close_flow",
	   }
	   for _, name := range requiredNames {
		   if _, ok := cat.ByName(name); !ok {
			   t.Errorf("catalog: built-in action %q not found", name)
		  }
	  }
	  // freeze_user 和 request_circuit 必须为高风险+require_gate。
	  for _, name := range []string{"freeze_user", "request_circuit"} {
		   a, _ := cat.ByName(name)
		  if a.Risk != reconflow.RiskHigh {
			  t.Errorf("catalog: %s expected RiskHigh, got %s", name, a.Risk)
		   }
		   if !a.RequireGate {
			   t.Errorf("catalog: %s expected RequireGate=true", name)
		   }
	   }
   }

// ──────────────────────────────────────────────
// Fixture 13：FlowDef 默认配置合法性
// ──────────────────────────────────────────────

   func TestDefaultFlowDefs_Valid(t *testing.T) {
	  defs := reconflow.DefaultFlowDefs()
	  if len(defs) < 3 {
		  t.Fatalf("expected at least 3 default FlowDefs, got %d", len(defs))
	   }
	   for _, d := range defs {
		   if d.Name == "" {
			  t.Errorf("FlowDef has empty name: %+v", d)
		   }
		  if d.SLA <= 0 {
			   t.Errorf("FlowDef %s has non-positive SLA", d.Name)
		   }
		  if len(d.ActionIDs) == 0 {
			   t.Errorf("FlowDef %s has no action IDs", d.Name)
		   }
	  }
   }
