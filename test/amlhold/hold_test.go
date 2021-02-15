package amlhold_test

import (
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/amlhold"
)

// ─── 用账本观察者 ──────────────────────────────────────────────────────────

type mockLedger struct {
	frozen map[string]bool // "userID:asset" → frozen
}

  func newMockLedger() *mockLedger {
	   return &mockLedger{frozen: make(map[string]bool)}
  }

  func (m *mockLedger) SetFrozen(userID, asset string, frozen bool) {
	  m.frozen[userID+":"+asset] = frozen
  }

  func (m *mockLedger) FrozenBit(userID, asset string) (bool, string) {
	   f := m.frozen[userID+":"+asset]
	   state := "unfrozen"
	  if f {
		  state = "frozen"
	  }
	   return f, state
  }

// ─── 辅助：推进时钟 ────────────────────────────────────────────────────────────

   func fixedClock(t time.Time) func() time.Time {
	   return func() time.Time { return t }
  }

// ─── OpenHold 表驱动 ──────────────────────────────────────────────────────

  func TestOpenHold(t *testing.T) {
	   t.Parallel()
	   base := time.Date(2024, 6, 17, 12, 0, 0, 0, time.UTC)
	  ledger := newMockLedger()
	  ledger.SetFrozen("user-001", "USDT", true)
	   cases := []struct {
		   name   string
		  caseID string
		  userID string
		  asset  string
		   amount string
		   actor    string
		  slaLimit int64
		  wantErr bool
		   errContains string
	   }{
		  {
			  name: "正常开启",
			   caseID: "case-001", userID: "user-001", asset: "USDT",
			   amount: "5000.00", actor: "ops-alice", slaLimit: 86400,
		   },
		   {
			  name: "空 actor 失败",
			  caseID: "case-002", userID: "user-001", asset: "USDT",
			   amount: "100", actor: "", slaLimit: 86400,
			  wantErr: true, errContains: "actor",
		   },
		  {
			   name: "空 asset 失败",
			   caseID: "case-003", userID: "user-001", asset: "",
			   amount: "100", actor: "ops-alice", slaLimit: 86400,
			   wantErr: true, errContains: "asset",
		   },
		  {
			   name:   "空金额失败",
			   caseID: "case-004", userID: "user-001", asset: "BTC",
			   amount: "", actor: "ops-alice", slaLimit: 86400,
			  wantErr: true, errContains: "amount",
		   },
		  {
			  name: "零金额失败",
			   caseID: "case-005", userID: "user-001", asset: "BTC",
			   amount: "0", actor: "ops-alice", slaLimit: 86400,
			   wantErr: true, errContains: "amount",
		  },
		   {
			  name: "空 caseID 失败",
			   caseID: "", userID: "user-001", asset: "BTC",
			  amount: "1.0", actor: "ops-alice", slaLimit: 86400,
			   wantErr: true, errContains: "case",
		   },
		  {
			   name:   "默认 SLA（零值使用默认）",
			   caseID: "case-006", userID: "user-001", asset: "ETH",
			   amount: "2.5", actor: "ops-alice", slaLimit: 0,
		   },
	  }
	  for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			   svc := amlhold.NewWithLedger(ledger)
			   svc.WithClock(fixedClock(base))
			   h,
				  err := svc.OpenHold(tc.caseID, tc.userID, tc.asset, tc.amount, tc.actor, tc.slaLimit)
			   if tc.wantErr {
				   if err == nil {
					  t.Fatalf("期望错误，但得到 hold=%+v", h)
				   }
				  if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					  t.Fatalf("错误应含 %q，得到: %v", tc.errContains, err)
				  }
				   return
			   }
			   if err != nil {
				  t.Fatalf("不期望错误: %v", err)
			   }
			   if h.ID == "" {
				   t.Fatal("hold.ID 为空")
			  }
			   if h.Status != amlhold.HoldStatusOpen {
				  t.Fatalf("初始状态应为 open，得到: %s", h.Status)
			  }
			   if h.AgingTier != amlhold.AgingTierNone {
				   t.Fatalf("初始老化级别应为 none，得到: %s", h.AgingTier)
			  }
		  })
	  }
  }

// ─── 幂等 ─────────────────────────────────────────────────────────────────

  func TestOpenHoldIdempotent(t *testing.T) {
	  t.Parallel()
	   svc := amlhold.New()
	   h1, err := svc.OpenHold("case-idem", "user-idem", "USDT", "1000", "ops-x", 86400)
	   if err != nil {
		  t.Fatal(err)
	  }
	   h2, err := svc.OpenHold("case-idem", "user-idem", "USDT", "2000", "ops-x", 86400)
	  if err != nil {
		  t.Fatal(err)
	  }
	  if h1.ID != h2.ID {
		  t.Fatalf("幂等开启应返回同一 hold，h1=%s h2=%s", h1.ID, h2.ID)
	  }
   }

// ─── 老化升级序列（表驱动）────────────────────────────────────────────────────

   func TestAgingSequence(t *testing.T) {
	  t.Parallel()
	   base := time.Date(2024, 6, 17, 0, 0, 0, 0, time.UTC)
	  cases := []struct {
		  name string
		  ageSeconds int64
		  wantStatus amlhold.HoldStatus
		  wantTier   amlhold.AgingTier
		  advanceCount int // 需要调用 AdvanceAging 的次数
	   }{
		   {
			   name: "初始状态_open",
			  ageSeconds:   0,
			  wantStatus:   amlhold.HoldStatusOpen,
			  wantTier:     amlhold.AgingTierNone,
			  advanceCount: 0,
		  },
		   {
			   name: "warn_阶段",
			   ageSeconds: 86401,
			  wantStatus: amlhold.HoldStatusAging,
			   wantTier:   amlhold.AgingTierWarn,
			  advanceCount: 1,
		  },
		   {
			   name: "force_hold_阶段",
			   ageSeconds: 129601,
			  wantStatus: amlhold.HoldStatusAging,
			  wantTier:   amlhold.AgingTierForceHold,
			   advanceCount: 2,
		  },
		  {
			  name:         "escalated_阶段",
			   ageSeconds:   172801,
			  wantStatus:   amlhold.HoldStatusEscalated,
			   wantTier:     amlhold.AgingTierEscalated,
			   advanceCount: 3,
		  },
		   {
			   name: "compel_review_阶段",
			  ageSeconds: 259201,
			   wantStatus: amlhold.HoldStatusCompelReview,
			  wantTier: amlhold.AgingTierCompelReview,
			   advanceCount: 4,
		  },
	   }
	   for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			   t.Parallel()
			   svc := amlhold.New()
			  svc.WithClock(fixedClock(base))
			   h, err := svc.OpenHold("case-aging", "user-aging", "USDT", "1000", "ops-agent", 86400)
			  if err != nil {
				   t.Fatal(err)
			  }
			   // 推进到目标老化级别
			  for i := 0; i < tc.advanceCount; i++ {
				  cur, _ := svc.GetHold(h.ID)
				   h, _, err = svc.AdvanceAging(h.ID, "system:aging", "test", cur.FenceEpoch, cur.FenceToken)
				   if err != nil {
					   t.Fatalf("第 %d 次 AdvanceAging 失败: %v", i+1, err)
				  }
			   }
			  cur, err := svc.GetHold(h.ID)
			   if err != nil {
				  t.Fatal(err)
			  }
			  if cur.Status != tc.wantStatus {
				   t.Errorf("hold status: 期望 %s，得到 %s", tc.wantStatus, cur.Status)
			  }
			   if cur.AgingTier != tc.wantTier {
				   t.Errorf("aging tier: 期望 %s，得到 %s", tc.wantTier, cur.AgingTier)
			  }
			  // 关键：老化升级后 hold 必须仍为非终态（不得自动 Resolved）
			  if cur.Terminal() {
				  t.Errorf("aging 升级后 hold 不得进入终态: status=%s", cur.Status)
			   }
		  })
	   }
  }

// ─── SLA 超时不得自动结案 ──────────────────────────────────────────────────────

   func TestSLATimeoutMustNotAutoResolve(t *testing.T) {
	   t.Parallel()
	   base := time.Date(2024, 6, 17, 0, 0, 0, 0, time.UTC)
	   // 时钟推进到 SLA 超时后
	   slaExpired := base.Add(25 * time.Hour)
	   svc := amlhold.New()
	  svc.WithClock(fixedClock(base))
	   _, err := svc.OpenHold("case-sla", "user-sla", "ETH", "10", "ops-agent", 86400)
	  if err != nil {
		  t.Fatal(err)
	  }
	  // 推进时钟到 SLA 超时
	  svc.WithClock(fixedClock(slaExpired))
	  // 执行老化扫描（ worker 定时触发）
	   result := svc.RunAgingScan()
	  if result.Advanced == 0 {
		  t.Error("SLA 超时后应该有 hold 被升级")
	  }
	   // 验证：所有 hold 仍为非终态（不得自动 resolved）
	  holds := svc.ListByCase("case-sla", false)
	   for _, h := range holds {
		  if h.Terminal() {
			  t.Errorf("SLA 超时后 hold 不得自动进入终态: hold_id=%s status=%s", h.ID, h.Status)
		   }
		  if h.Status == "closed" || h.Status == "resolved" {
			  t.Errorf("SLA 超时不得自动关闭/解决 hold: status=%s", h.Status)
		   }
	   }
  }

// ─── Release↔Close 互斥：结案前 hold 必须已 released ─────────────────────────

  func TestCloseBlockedByOpenHold(t *testing.T) {
	  t.Parallel()
	  svc := amlhold.New()
	  caseID := "case-mutex-001"
	  // 开启 hold
	   _, err := svc.OpenHold(caseID, "user-x", "USDT", "1000", "ops-alice", 86400)
	   if err != nil {
		   t.Fatal(err)
	   }
	  // 结案前检查：应失败
	   err = svc.CheckCloseAllowed(caseID, "reviewer-bob")
	   if err == nil {
		  t.Fatal("hold 未 released 时结案检查应失败，但返回 nil")
	  }
	  if !strings.Contains(err.Error(), "hold") {
		  t.Errorf("错误信息应提及 hold: %v", err)
	   }
  }

// ─── Release↔Close 互斥：hold released 后结案成功 ────────────────────────────

   func TestCloseAllowedAfterRelease(t *testing.T) {
	  t.Parallel()
	  svc := amlhold.New()
	   caseID := "case-mutex-002"
	   h, err := svc.OpenHold(caseID, "user-y", "BTC", "0.1", "ops-alice", 86400)
	   if err != nil {
		  t.Fatal(err)
	  }
	  // 申请解冻（不同 actor）
	   ticket, err := svc.RequestRelease(h.ID, caseID, "ops-charlie", "审核完毕", "")
	  if err != nil {
		   t.Fatal("申请解冻失败:", err)
	   }
	   // 批准解冻（四眼：approvedBy ≠ requestedBy）
	   _, _, err = svc.ApproveRelease(ticket.ID, "reviewer-diana", ticket.FenceEpoch, ticket.FenceToken)
	   if err != nil {
		  t.Fatal("批准解冻失败:", err)
	  }
	  // 结案前检查：应成功
	  err = svc.CheckCloseAllowed(caseID, "reviewer-eve")
	   if err != nil {
		   t.Fatalf("hold released 后结案检查应成功，得到: %v", err)
	  }
   }

// ─── 同一 actor 四眼检查：申请解冻和审批解冻不得相同 ────────────────────────

   func TestFourEyeReleaseApproval(t *testing.T) {
	   t.Parallel()
	  svc := amlhold.New()
	   h, err := svc.OpenHold("case-foureye", "user-z", "USDT", "500", "ops-alice", 86400)
	   if err != nil {
		   t.Fatal(err)
	  }
	   ticket, err := svc.RequestRelease(h.ID, "case-foureye", "ops-bob", "申请解冻", "")
	  if err != nil {
		  t.Fatal(err)
	   }
	  // 同一 actor 自批：应失败
	   _, _, err = svc.ApproveRelease(ticket.ID, "ops-bob", ticket.FenceEpoch, ticket.FenceToken)
	  if err == nil {
		  t.Fatal("同一 actor 不得既申请又审批解冻，但返回 nil 错误")
	  }
	  if !strings.Contains(err.Error(), "four-eye") && !strings.Contains(err.Error(), "actor") {
		   t.Errorf("错误信息应提及四眼: %v", err)
	  }
  }

// ─── 同一 actor 先申请解冻、再发起结案：互斥检查 ──────────────────────────────

   func TestSameActorReleaseThenClose(t *testing.T) {
	   t.Parallel()
	   svc := amlhold.New()
	  caseID := "case-mutex-003"
	  h, err := svc.OpenHold(caseID, "user-m", "ETH", "3.0", "ops-alice", 86400)
	   if err != nil {
		   t.Fatal(err)
	   }
	  // ops-bob 申请解冻
	   _, err = svc.RequestRelease(h.ID, caseID, "ops-bob", "申请", "")
	   if err != nil {
		   t.Fatal(err)
	   }
	   // ops-bob 试图发起结案：互斥检查应失败
	   err = svc.CheckSameActorReleaseThenClose(caseID, "ops-bob")
	  if err == nil {
		   t.Fatal("同一 actor 申请解冻后不得发起结案，但互斥检查返回 nil")
	   }
  }

// ─── 不同 actor 解冻+结案流程 ────────────────────────────────────────────────

  func TestDifferentActorReleaseAndClose(t *testing.T) {
	   t.Parallel()
	   svc := amlhold.New()
	   caseID := "case-mutex-004"
	  h, err := svc.OpenHold(caseID, "user-n", "XRP", "1000", "ops-alice", 86400)
	   if err != nil {
		  t.Fatal(err)
	   }
	  // ops-charlie 申请解冻，reviewer-diana 审批
	   ticket, err := svc.RequestRelease(h.ID, caseID, "ops-charlie", "解冻申请", "reviewer-eve")
	   if err != nil {
		   t.Fatal("申请解冻失败:", err)
	  }
	  _, _, err = svc.ApproveRelease(ticket.ID, "reviewer-diana", ticket.FenceEpoch, ticket.FenceToken)
	  if err != nil {
		  t.Fatal("批准解冻失败:", err)
	   }
	  // reviewer-eve 发起结案互斥检查（与 ops-charlie 不同 actor）
	  err = svc.CheckSameActorReleaseThenClose(caseID, "reviewer-eve")
	   if err != nil {
		   t.Fatalf("不同 actor 应允许结案，得到: %v", err)
	   }
	  // 结案前 hold 检查
	  err = svc.CheckCloseAllowed(caseID, "reviewer-eve")
	  if err != nil {
		   t.Fatalf("hold 已 released，结案应允许，得到: %v", err)
	  }
   }

// ─── 账本漂移缺口单 ───────────────────────────────────────────────────────

  func TestDriftGapOnOpen(t *testing.T) {
	   t.Parallel()
	  ledger := newMockLedger()
	   ledger.SetFrozen("user-drift", "USDT", false) // 账本未冻结
	   svc := amlhold.NewWithLedger(ledger)
	  h, err := svc.OpenHold("case-drift", "user-drift", "USDT", "1000", "ops-agent", 86400)
	   if err != nil {
		   t.Fatal(err)
	   }
	  // 账本未冻结时应开启缺口单
	  if h.GapTicketID == "" {
		  t.Error("账本未冻结时开启 hold 应自动创建缺口单")
	   }
	  if h.LedgerFrozenBit {
		   t.Error("账本未冻结时 LedgerFrozenBit 应为 false")
	   }
	  gaps := svc.GapTickets(h.ID)
	   if len(gaps) == 0 {
		   t.Error("应有至少一条缺口单")
	   }
	   if gaps[0].Resolved {
		   t.Error("新开缺口单不应已为 resolved")
	   }
   }

   func TestDriftPatrolGap(t *testing.T) {
	  t.Parallel()
	  ledger := newMockLedger()
	   ledger.SetFrozen("user-patrol", "BTC", true) // 开启时账本正常
	  svc := amlhold.NewWithLedger(ledger)
	   h, err := svc.OpenHold("case-patrol", "user-patrol", "BTC", "0.1", "ops-agent", 86400)
	  if err != nil {
		   t.Fatal(err)
	   }
	   if h.GapTicketID != "" {
		   t.Error("账本正常时不应开缺口单")
	   }
	  // 账本已解冻（漂移）
	   ledger.SetFrozen("user-patrol", "BTC", false)
	   gaps := svc.CheckDrift()
	   if len(gaps) == 0 {
		   t.Error("漂移巡检应发现缺口")
	  }
	   // 验证漂移证明已记录
	  proofs := svc.Proofs(h.ID)
	   hasDriftProof := false
	   for _, p := range proofs {
		  if p.ProofKind == amlhold.ProofKindDrift {
			   hasDriftProof = true
			   break
		   }
	  }
	  if !hasDriftProof {
		   t.Error("漂移巡检应追加 drift 类型证明")
	   }
  }

// ─── HoldProof 证明 ───────────────────────────────────────────────────────

   func TestHoldProofAppended(t *testing.T) {
	   t.Parallel()
	   svc := amlhold.New()
	   h, err := svc.OpenHold("case-proof", "user-p", "ETH", "5.0", "ops-alice", 86400)
	  if err != nil {
		   t.Fatal(err)
	  }
	   // 开启后应有 occupy 证明
	  proofs := svc.Proofs(h.ID)
	  if len(proofs) == 0 {
		  t.Fatal("开启 hold 后应有证明记录")
	   }
	   if proofs[0].ProofKind != amlhold.ProofKindOccupy {
		   t.Errorf("首条证明应为 occupy，得到: %s", proofs[0].ProofKind)
	   }
	  if proofs[0].Checksum == "" {
		   t.Error("证明 checksum 不得为空")
	  }
	  // 老化升级后应追加老化证明
	  h, _, err = svc.AdvanceAging(h.ID, "system", "test", h.FenceEpoch, h.FenceToken)
	   if err != nil {
		  t.Fatal(err)
	   }
	   proofs = svc.Proofs(h.ID)
	  if len(proofs) < 2 {
		   t.Fatalf("老化升级后应有至少 2 条证明，得到 %d 条", len(proofs))
	  }
   }

// ─── HoldProof 按 Case 导出 ───────────────────────────────────────────────────

  func TestExportProofsByCase(t *testing.T) {
	  t.Parallel()
	  svc := amlhold.New()
	  caseID := "case-export"
	  _, err := svc.OpenHold(caseID, "user-e1", "USDT", "1000", "ops-alice", 86400)
	   if err != nil {
		  t.Fatal(err)
	   }
	  _, err = svc.OpenHold(caseID, "user-e1", "BTC", "0.5", "ops-alice", 86400)
	   if err != nil {
		   t.Fatal(err)
	   }
	  exported,
		   err := svc.ExportProofsForCase(caseID)
	  if err != nil {
		   t.Fatal(err)
	  }
	   if len(exported) < 2 {
		   t.Errorf("应有至少 2 条导出证明（每个 hold 一条 occupy），得到 %d", len(exported))
	  }
	  for _, ep := range exported {
		  if ep.Proof.CaseID != caseID {
			  t.Errorf("导出证明 case_id 不匹配: %s", ep.Proof.CaseID)
		   }
		   if ep.Proof.Checksum == "" {
			  t.Error("导出证明 checksum 不得为空")
		  }
	   }
   }

// ─── 解冻申请幂等性 ───────────────────────────────────────────────────────────

   func TestRequestReleaseIdempotent(t *testing.T) {
	  t.Parallel()
	  svc := amlhold.New()
	  h, err := svc.OpenHold("case-idem-release", "user-ir", "USDT", "100", "ops-alice", 86400)
	  if err != nil {
		  t.Fatal(err)
	   }
	  t1, err := svc.RequestRelease(h.ID, "case-idem-release", "ops-bob", "申请1", "")
	   if err != nil {
		   t.Fatal(err)
	   }
	   t2, err := svc.RequestRelease(h.ID, "case-idem-release", "ops-bob", "申请2", "")
	  if err != nil {
		   t.Fatal(err)
	  }
	   if t1.ID != t2.ID {
		  t.Fatal("重复申请解冻应返回同一 ticket（幂等）")
	  }
   }

// ─── 终态 hold 不可再操作 ──────────────────────────────────────────────────────

   func TestTerminalHoldRejectOps(t *testing.T) {
	   t.Parallel()
	   svc := amlhold.New()
	  caseID := "case-terminal"
	   h, err := svc.OpenHold(caseID, "user-t", "USDT", "200", "ops-alice", 86400)
	  if err != nil {
		   t.Fatal(err)
	  }
	  // 完成解冻
	   ticket, err := svc.RequestRelease(h.ID, caseID, "ops-bob", "申请", "")
	  if err != nil {
		  t.Fatal(err)
	   }
	   _, _, err = svc.ApproveRelease(ticket.ID, "reviewer-carol", ticket.FenceEpoch, ticket.FenceToken)
	   if err != nil {
		   t.Fatal(err)
	  }
	   // 验证 hold 已进入终态
	   cur, err := svc.GetHold(h.ID)
	   if err != nil {
		   t.Fatal(err)
	   }
	   if cur.Status != amlhold.HoldStatusReleased {
		   t.Fatalf("期望 released，得到 %s", cur.Status)
	  }
	  // 试图再次升级老化：应失败
	  _, _, err = svc.AdvanceAging(h.ID, "system", "test", cur.FenceEpoch, cur.FenceToken)
	   if err == nil {
		   t.Error("终态 hold 不得继续老化升级")
	   }
   }

// ─── 围栏（stale fence） ──────────────────────────────────────────────────

   func TestStaleFenceRejected(t *testing.T) {
	  t.Parallel()
	  svc := amlhold.New()
	   h, err := svc.OpenHold("case-fence", "user-fence", "USDT", "100", "ops-alice", 86400)
	   if err != nil {
		  t.Fatal(err)
	  }
	   // 使用错误 token 升级老化：应失败
	  _, _, err = svc.AdvanceAging(h.ID, "system", "test", h.FenceEpoch, "wrong-token")
	   if err == nil {
		  t.Fatal("错误 fence token 不应允许操作")
	   }
   }

// ─── 统计数据 ──────────────────────────────────────────────────────────────

   func TestStats(t *testing.T) {
	  t.Parallel()
	  svc := amlhold.New()
	   // 开启 3 个 hold
	   for i := 0; i < 3; i++ {
		   caseID := "case-stats-" + string(rune('A'+i))
		  userID := "user-stats-" + string(rune('A'+i))
		  _, err := svc.OpenHold(caseID, userID, "USDT", "100", "ops-agent", 86400)
		   if err != nil {
			  t.Fatal(err)
		  }
	  }
	   stats := svc.Stats()
	   if stats.Total < 3 {
		   t.Errorf("期望至少 3 个 hold，得到 %d", stats.Total)
	   }
	   if stats.Open < 3 {
		   t.Errorf("期望至少 3 个 open hold，得到 %d", stats.Open)
	   }
   }

// ─── AbortHold ───────────────────────────────────────────────────────────

  func TestAbortHold(t *testing.T) {
	   t.Parallel()
	  svc := amlhold.New()
	   h, err := svc.OpenHold("case-abort", "user-abort", "USDT", "100", "ops-alice", 86400)
	  if err != nil {
		  t.Fatal(err)
	  }
	   aborted, err := svc.AbortHold(h.ID, "admin-override", "紧急终止", h.FenceEpoch, h.FenceToken)
	   if err != nil {
		   t.Fatal(err)
	   }
	   if aborted.Status != amlhold.HoldStatusAborted {
		   t.Errorf("期望 aborted，得到 %s", aborted.Status)
	  }
	  if aborted.AbortedBy != "admin-override" {
		  t.Errorf("AbortedBy 应为 admin-override，得到 %s", aborted.AbortedBy)
	  }
	  // 验证 abort 证明
	   proofs := svc.Proofs(h.ID)
	  hasAbortProof := false
	   for _, p := range proofs {
		   if p.ProofKind == amlhold.ProofKindAbort {
			  hasAbortProof = true
		  }
	  }
	  if !hasAbortProof {
		  t.Error("abort 操作应追加 abort 类型证明")
	   }
  }

// ─── SLA 报告 ────────────────────────────────────────────────────────────

  func TestSLAScan(t *testing.T) {
	   t.Parallel()
	   base := time.Date(2024, 6, 17, 0, 0, 0, 0, time.UTC)
	  svc := amlhold.New()
	   svc.WithClock(fixedClock(base))
	   _, err := svc.OpenHold("case-sla-scan", "user-sla", "ETH", "1.0", "ops-agent", 86400)
	   if err != nil {
		  t.Fatal(err)
	  }
	   // SLA 未超时
	   now := base.Add(12 * time.Hour)
	   reports := svc.SLAScan(now)
	   if len(reports) == 0 {
		   t.Fatal("应有 SLA 报告")
	  }
	   if reports[0].Overdue {
		   t.Error("12h 内不应标记为 overdue")
	   }
	  // SLA 超时
	   now = base.Add(25 * time.Hour)
	   reports = svc.SLAScan(now)
	  if !reports[0].Overdue {
		  t.Error("25h 后应标记为 overdue")
	  }
	  if reports[0].NextTier == "" {
		  t.Error("overdue 后应有 NextTier 提示")
	  }
   }
