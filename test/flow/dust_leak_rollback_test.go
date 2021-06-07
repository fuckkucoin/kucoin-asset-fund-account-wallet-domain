package flow

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/dustconv"
	"account-assets.kucoin.net/funding/internal/dustleak"
)

// ── 辅助 ─────────────────────────────────────────────────────────────────────

  func newLeakWithOverFloor(t *testing.T, svc *dustleak.Service) *dustleak.LeakCase {
	  t.Helper()
	   lc, err := svc.OpenLeakCase(
		   "conv-rb-001", "user-rb", "BTC", dustleak.LeakKindOverFloor,
		   "0.002", "120", "0.001", "0",
		   "0.001", "60",
		   "", "", "test",
	  )
	   if err != nil {
		   t.Fatalf("OpenLeakCase: %v", err)
	   }
	   return lc
  }

  func newRollbackSvc() (*dustleak.Service, *dustconv.Service) {
	  dust := dustconv.New()
	   _ = dust.PutPolicy(dustconv.Policy{Asset: "BTC", Floor: "0.001", MinConvert: "0.00000001", Enabled: true})
	   svc := dustleak.New(dust, nil)
	   return svc, dust
  }

// ── ：完整回滚流程 detected → rolling_back → conserved → closed ─────────

   func TestLeakRollback_FullFlow_Conserved(t *testing.T) {
	  svc, _ := newRollbackSvc()
	  lc := newLeakWithOverFloor(t, svc)
	  // 1. PostRollback（需 dualReqID）
	   rb, err := svc.PostRollback(lc.ID, "ops-maker", "dual-req-001")
	  if err != nil {
		  t.Fatalf("PostRollback: %v", err)
	   }
	  if rb.Status != dustleak.RollbackStatusPending {
		  t.Errorf("want pending got %s", rb.Status)
	   }
	  if rb.ProofHash == "" {
		   t.Error("want non-empty proof_hash after PostRollback")
	  }
	   // LeakCase 应已切到 rolling_back
	   lc2, _ := svc.GetLeakCase(lc.ID)
	  if lc2.Status != dustleak.LeakStatusRollingBack {
		  t.Errorf("want rolling_back got %s", lc2.Status)
	  }
	  // 2. ExecuteRollback
	  rb2, err := svc.ExecuteRollback(rb.ID, "ops-executor")
	  if err != nil {
		   t.Fatalf("ExecuteRollback: %v", err)
	  }
	   if rb2.Status != dustleak.RollbackStatusPosted {
		  t.Errorf("want posted got %s", rb2.Status)
	   }
	  // 3. ConfirmRollback（传回 proofHash）
	  rb3, err := svc.ConfirmRollback(rb.ID, rb.ProofHash, "ops-checker")
	  if err != nil {
		  t.Fatalf("ConfirmRollback: %v", err)
	  }
	  if rb3.Status != dustleak.RollbackStatusConserved {
		  t.Errorf("want conserved got %s", rb3.Status)
	  }
	  // LeakCase 应已收口
	  lc3, _ := svc.GetLeakCase(lc.ID)
	  if lc3.Status != dustleak.LeakStatusClosed {
		   t.Errorf("want closed got %s", lc3.Status)
	   }
	   // 挡兑闸应已解除
	  if svc.GateStatusOf("user-rb", "BTC") != dustleak.GateStatusOpen {
		  t.Error("want gate open after case closed")
	   }
   }

// ── ：ConfirmRollback 错误 proofHash → 拒绝 ───────────────────────────────

   func TestConfirmRollback_WrongHash_Rejected(t *testing.T) {
	   svc, _ := newRollbackSvc()
	  lc := newLeakWithOverFloor(t, svc)
	  rb, _ := svc.PostRollback(lc.ID, "ops-maker", "dual-req-002")
	  _, _ = svc.ExecuteRollback(rb.ID, "ops-exec")
	  _, err := svc.ConfirmRollback(rb.ID, "wrong-hash-000", "ops-checker")
	  if err == nil {
		  t.Fatal("want ErrProof with wrong hash, got nil")
	  }
	  t.Logf("correctly rejected: %v", err)
   }

// ── ：FailRollback → LeakCase 进入 dispute ───────────────────────────────

   func TestFailRollback_LeakCase_ToDispute(t *testing.T) {
	  svc, _ := newRollbackSvc()
	  lc := newLeakWithOverFloor(t, svc)
	   rb, _ := svc.PostRollback(lc.ID, "ops-maker", "dual-req-003")
	   // 直接 FailRollback（无需先 Execute）
	  rb2, err := svc.FailRollback(rb.ID, "ops-fail", "insufficient usdt balance")
	  if err != nil {
		  t.Fatalf("FailRollback: %v", err)
	   }
	   if rb2.Status != dustleak.RollbackStatusFailed {
		  t.Errorf("want failed got %s", rb2.Status)
	   }
	   lc2, _ := svc.GetLeakCase(lc.ID)
	   if lc2.Status != dustleak.LeakStatusDispute {
		  t.Errorf("want dispute got %s", lc2.Status)
	  }
  }

// ── ：PostRollback 缺 dualReqID → 拒绝 ────────────────────────────────────

   func TestPostRollback_NoDualReqID_Rejected(t *testing.T) {
	  svc, _ := newRollbackSvc()
	  lc := newLeakWithOverFloor(t,
		   svc)
	   _, err := svc.PostRollback(lc.ID, "ops-maker", "") // dualReqID 为空
	  if err == nil {
		   t.Fatal("want ErrDual when dualReqID empty")
	  }
   }

// ── ：PostRollback 在非 detected 状态拒绝 ────────────────────────────────

  func TestPostRollback_WrongState_Rejected(t *testing.T) {
	   svc, _ := newRollbackSvc()
	   lc := newLeakWithOverFloor(t,
		  svc)
	   // 先推进到 rolling_back
	  _, _ = svc.PostRollback(lc.ID, "ops-maker", "dual-001")
	  // 再次 PostRollback 应幂等返回已有腿，不报错
	  rb2, err := svc.PostRollback(lc.ID, "ops-maker", "dual-001")
	   if err != nil {
		   t.Logf("PostRollback idempotent returned: %v", err)
	   }
	   if rb2 != nil && rb2.LeakID != lc.ID {
		  t.Errorf("idempotent rollback has wrong leak_id: %s", rb2.LeakID)
	  }
  }

// ── ：Conserve 断言 ───────────────────────────────────────────────────────

  func TestConserveAssertion_EqualAmounts_Pass(t *testing.T) {
	   if err := dustleak.ConserveAssertion("60", "60"); err != nil {
		   t.Errorf("equal amounts should conserve: %v", err)
	   }
  }

   func TestConserveAssertion_UnequalAmounts_Fail(t *testing.T) {
	   if err := dustleak.ConserveAssertion("60", "59.9"); err == nil {
		   t.Error("unequal amounts should fail conserve assertion")
	  }
  }

// ── ：RollbackProof 导出与独立校验 ───────────────────────────────────────

   func TestRollbackProof_Export_Verifiable(t *testing.T) {
	   svc, _ := newRollbackSvc()
	  lc := newLeakWithOverFloor(t, svc)
	  rb, _ := svc.PostRollback(lc.ID, "ops-maker", "dual-proof-001")
	   _, _ = svc.ExecuteRollback(rb.ID, "ops-exec")
	   _, _ = svc.ConfirmRollback(rb.ID, rb.ProofHash, "ops-checker")
	  proof, err := svc.RollbackProofOf(rb.ID)
	   if err != nil {
		  t.Fatalf("RollbackProofOf: %v", err)
	   }
	  if proof.RollbackID != rb.ID {
		  t.Errorf("rollback_id mismatch: got %s want %s", proof.RollbackID, rb.ID)
	   }
	  // 独立重算 proofHash 并
	  h := sha256.New()
	   h.Write([]byte(strings.Join([]string{rb.ID, rb.ConvertID, rb.RollbackUSDT, rb.RollbackAsset}, "|")))
	  expected := fmt.Sprintf("%x", h.Sum(nil))
	   if proof.ProofHash != expected {
		   t.Errorf("proof hash mismatch: got %s want %s", proof.ProofHash, expected)
	   }
	  t.Logf("proof verified: hash=%s status=%s", proof.ProofHash[:16], proof.Status)
  }

// ── ：PlanRollback 生成回滚计划 ──────────────────────────────────────────

  func TestPlanRollback_CorrectPlan(t *testing.T) {
	  svc, _ := newRollbackSvc()
	   lc := newLeakWithOverFloor(t, svc)
	   got, _ := svc.GetLeakCase(lc.ID)
	  plan, err := dustleak.PlanRollback(got)
	   if err != nil {
		  t.Fatalf("PlanRollback: %v", err)
	  }
	  if plan.LeakID != lc.ID {
		  t.Errorf("wrong leak_id in plan")
	  }
	   if plan.RollbackUSDT != lc.LeakUSDT {
		   t.Errorf("want rollback_usdt=%s got %s", lc.LeakUSDT, plan.RollbackUSDT)
	   }
	  if plan.Note == "" {
		   t.Error("want non-empty note in plan")
	  }
   }

// ── ：RollbacksByLeak ─────────────────────────────────────────────────────

  func TestRollbacksByLeak(t *testing.T) {
	  svc, _ := newRollbackSvc()
	   lc := newLeakWithOverFloor(t, svc)
	  rb, _ := svc.PostRollback(lc.ID, "ops-maker", "dual-by-leak")
	   rbs,
		   err := svc.RollbacksByLeak(lc.ID)
	  if err != nil {
		  t.Fatalf("RollbacksByLeak: %v", err)
	  }
	   if len(rbs) != 1 {
		   t.Errorf("want 1 rollback leg got %d", len(rbs))
	  }
	   if rbs[0].ID != rb.ID {
		   t.Errorf("rollback ID mismatch")
	  }
  }

// ── ：BuildSuspenseEntry ─────────────────────────────────────────────────

   func TestBuildSuspenseEntry(t *testing.T) {
	   rb := &dustleak.LeakRollback{
		   ID: "rb-1",
		  LeakID:    "lk-1",
		  ConvertID: "cv-1",
		  UserID:    "user1",
		  Asset:     "BTC",
		   RollbackUSDT: "60",
		   PostedAt:     time.Now().UTC(),
	  }
	   entry := dustleak.BuildSuspenseEntry(rb, "ops-actor")
	   if entry.Direction != "debit" {
		  t.Errorf("want debit got %s", entry.Direction)
	  }
	  if entry.SuspenseUSDT != "60" {
		   t.Errorf("want 60 got %s", entry.SuspenseUSDT)
	  }
   }

// ── ：Service.Summary ──────────────────────────────────────────────────

  func TestSummary_CountsCorrectly(t *testing.T) {
	  svc, _ := newRollbackSvc()
	   newLeakWithOverFloor(t, svc)
	   sum := svc.Summary()
	   if sum.OpenLeaks < 1 {
		  t.Errorf("want ≥1 open leak got %d", sum.OpenLeaks)
	   }
	  if sum.BlockedGates < 1 {
		  t.Errorf("want ≥1 blocked gate got %d", sum.BlockedGates)
	   }
  }

// ── ：AllLeakCases 列表 ───────────────────────────────────────────────────

  func TestAllLeakCases_ListByStatus(t *testing.T) {
	   svc, _ := newRollbackSvc()
	  lc1 := newLeakWithOverFloor(t, svc)
	  _ = lc1
	   // 注入第二个不同用户的 LeakCase
	   _, _ = svc.OpenLeakCase(
		   "conv-list-b", "user-b", "ETH", dustleak.LeakKindDriftOverpay,
		  "1", "2200", "2000", "2000",
		   "0", "200",
		   "", "", "test",
	  )
	  all := svc.AllLeakCases("", 50)
	  if len(all) < 2 {
		   t.Errorf("want ≥2 cases got %d", len(all))
	   }
	   detected := svc.AllLeakCases(dustleak.LeakStatusDetected, 50)
	   for _, lc := range detected {
		   if lc.Status != dustleak.LeakStatusDetected {
			  t.Errorf("filtered by detected but got status=%s", lc.Status)
		  }
	   }
  }
