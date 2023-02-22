package haltclear_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
	"account-assets.kucoin.net/funding/internal/haltclear"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ---- 辅助 -----------------------------------------------------------

   func seedOpenCase(t *testing.T, caseRepo haltclear.CaseRepo, id, scope string) *haltclear.MisClearCase {
	  t.Helper()
	   now := time.Now().UTC()
	   mc := &haltclear.MisClearCase{
		   MisclearID: id,
		  Scope: scope,
		   State: haltclear.MisClearStateOpen,
		  Trigger: haltclear.TriggerOpenDiffScan,
		   DetectedAt:    now,
		   OpenDiffCount: 3,
		   CreatedAt:     now,
		   UpdatedAt:     now,
	   }
	  if err := caseRepo.Create(context.Background(), mc); err != nil {
		   t.Fatalf("seed case: %v", err)
	  }
	   return mc
  }

  func newRollbackHelper(gate *custodyhalt.Gate, caseRepo haltclear.CaseRepo, legRepo haltclear.LegRepo, diffCount int) *haltclear.ReEngageRollback {
	   diffQuery := &haltclear.StaticDiffQuerier{Count: diffCount}
	  return haltclear.NewReEngageRollback(gate, caseRepo, legRepo, diffQuery)
   }

  func newGateHelper() *custodyhalt.Gate {
	  kv := persist.NewMemKV()
	  return custodyhalt.New(kv)
  }

// ---- ScheduleReEngage ------------------------------------------

var scheduleTable = []struct {
	name      string
	caseState haltclear.MisClearCaseState
	wantErr   error
}{
	{"open case ok", haltclear.MisClearStateOpen, nil},
	{"reengaged case blocked", haltclear.MisClearStateReengaged, haltclear.ErrCaseNotOpen},
	{"closed case blocked", haltclear.MisClearStateClosed, haltclear.ErrCaseNotOpen},
}

  func TestScheduleReEngage(t *testing.T) {
	   for _, tc := range scheduleTable {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			  gate := newGateHelper()
			   ctx := context.Background()
			   caseRepo := haltclear.NewMemCaseRepo()
			  legRepo := haltclear.NewMemLegRepo()
			   rb := newRollbackHelper(gate, caseRepo, legRepo, 3)
			  now := time.Now().UTC()
			  mc := &haltclear.MisClearCase{
				  MisclearID: "mc-sched",
				   Scope:      "global",
				   State:      tc.caseState,
				   Trigger:       haltclear.TriggerOpenDiffScan,
				  DetectedAt:    now,
				  OpenDiffCount: 3,
				  CreatedAt:     now,
				  UpdatedAt:     now,
			   }
			   if err := caseRepo.Create(ctx, mc); err != nil {
				  t.Fatal(err)
			  }
			   leg, err := rb.ScheduleReEngage(ctx, "mc-sched", "ops-alice", "scheduled test", false)
			  if tc.wantErr != nil {
				  if !errors.Is(err, tc.wantErr) {
					   t.Errorf("got %v want %v", err, tc.wantErr)
				   }
				   return
			   }
			  if err != nil {
				   t.Fatalf("unexpected error: %v", err)
			  }
			  if leg.State != haltclear.ReEngageStatePending {
				  t.Errorf("leg state=%s want pending", leg.State)
			   }
			  if leg.MisclearID != "mc-sched" {
				  t.Errorf("misclear_id=%s want mc-sched", leg.MisclearID)
			   }
		  })
	   }
   }

// TestScheduleReEngage_Idempotent 重复调度返回已有活跃腿，不重复创建。
   func TestScheduleReEngage_Idempotent(t *testing.T) {
	   gate := newGateHelper()
	  ctx := context.Background()
	  caseRepo := haltclear.NewMemCaseRepo()
	   legRepo := haltclear.NewMemLegRepo()
	  rb := newRollbackHelper(gate, caseRepo, legRepo, 3)
	  seedOpenCase(t, caseRepo, "mc-idem", "global")
	  leg1, err := rb.ScheduleReEngage(ctx, "mc-idem", "alice", "first", false)
	  if err != nil {
		  t.Fatal(err)
	  }
	   leg2, err := rb.ScheduleReEngage(ctx, "mc-idem", "alice", "second", false)
	   if err != nil {
		   t.Fatal(err)
	  }
	   if leg1.LegID != leg2.LegID {
		  t.Errorf("expected idempotent: leg1=%s leg2=%s", leg1.LegID, leg2.LegID)
	   }
  }

// ---- ExecuteLeg ------------------------------------------------

  func TestExecuteLeg_OK(t *testing.T) {
	   gate := newGateHelper()
	   ctx := context.Background()
	   // 关闭熔断状态（误清）。
	  caseRepo := haltclear.NewMemCaseRepo()
	  legRepo := haltclear.NewMemLegRepo()
	  rb := newRollbackHelper(gate, caseRepo, legRepo, 3)
	   seedOpenCase(t, caseRepo, "mc-exec", "global")
	  leg, err := rb.ScheduleReEngage(ctx, "mc-exec", "system", "auto", true)
	  if err != nil {
		  t.Fatal(err)
	  }
	  proof, err := rb.ExecuteLeg(ctx, leg.LegID)
	  if err != nil {
		  t.Fatalf("execute: %v", err)
	   }
	   if proof == nil {
		   t.Fatal("expected proof")
	  }
	  if !haltclear.VerifyRollbackProof(proof) {
		  t.Error("proof checksum invalid")
	   }
	   // Gate 应已恢复熔断。
	   halted, _ := gate.Halted(ctx)
	  if !halted {
		  t.Error("expected gate to be halted after reengage")
	   }
	  // 案例状态应为 reengaged。
	  mc, _ := caseRepo.Get(ctx, "mc-exec")
	   if mc.State != haltclear.MisClearStateReengaged {
		  t.Errorf("case state=%s want reengaged", mc.State)
	  }
	  if mc.ReengageLegID != leg.LegID {
		  t.Errorf("reengage_leg_id=%s want %s", mc.ReengageLegID, leg.LegID)
	  }
  }

  func TestExecuteLeg_NotPending(t *testing.T) {
	   gate := newGateHelper()
	   ctx := context.Background()
	  caseRepo := haltclear.NewMemCaseRepo()
	   legRepo := haltclear.NewMemLegRepo()
	   rb := newRollbackHelper(gate, caseRepo, legRepo, 0)
	   seedOpenCase(t, caseRepo, "mc-np", "global")
	   leg, err := rb.ScheduleReEngage(ctx, "mc-np", "alice", "test", true)
	   if err != nil {
		   t.Fatal(err)
	  }
	  // 先执行一次成功。
	   if _, err := rb.ExecuteLeg(ctx, leg.LegID); err != nil {
		  t.Fatal(err)
	  }
	  // 再执行一次应返回 ErrLegNotPending。
	   _, err = rb.ExecuteLeg(ctx, leg.LegID)
	  if !errors.Is(err, haltclear.ErrLegNotPending) {
		  t.Errorf("got %v want ErrLegNotPending", err)
	  }
  }

// TestAutoReEngage 端到端：自动调度并执行。
   func TestAutoReEngage(t *testing.T) {
	  gate := newGateHelper()
	   ctx := context.Background()
	   caseRepo := haltclear.NewMemCaseRepo()
	  legRepo := haltclear.NewMemLegRepo()
	   rb := newRollbackHelper(gate, caseRepo, legRepo, 2)
	  seedOpenCase(t, caseRepo, "mc-auto", "global")
	  proof, err := rb.AutoReEngage(ctx, "mc-auto")
	   if err != nil {
		  t.Fatalf("auto reengage: %v", err)
	  }
	  if proof == nil {
		  t.Fatal("expected proof")
	   }
	  if !haltclear.VerifyRollbackProof(proof) {
		  t.Error("proof invalid")
	  }
	  halted, _ := gate.Halted(ctx)
	   if !halted {
		   t.Error("gate should be halted after auto reengage")
	   }
  }

// ---- CloseCase -----------------------------------------------

var closeCaseTable = []struct {
	name string
	state haltclear.MisClearCaseState
	wantErr error
}{
	{"reengaged can close", haltclear.MisClearStateReengaged, nil},
	{"open can close", haltclear.MisClearStateOpen, nil},
	{"closed terminal", haltclear.MisClearStateClosed, haltclear.ErrCaseTerminal},
}

   func TestCloseCase(t *testing.T) {
	   for _, tc := range closeCaseTable {
		   tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   gate := newGateHelper()
			  ctx := context.Background()
			  caseRepo := haltclear.NewMemCaseRepo()
			  legRepo := haltclear.NewMemLegRepo()
			   rb := newRollbackHelper(gate, caseRepo, legRepo, 0)
			  now := time.Now().UTC()
			  mc := &haltclear.MisClearCase{
				  MisclearID: "mc-close",
				   Scope:      "global",
				   State: tc.state,
				   CreatedAt: now,
				  UpdatedAt: now,
			   }
			   if err := caseRepo.Create(ctx, mc); err != nil {
				  t.Fatal(err)
			  }
			  err := rb.CloseCase(ctx, "mc-close", "ops-alice", "resolved by recon team")
			   if tc.wantErr != nil {
				  if !errors.Is(err, tc.wantErr) {
					   t.Errorf("got %v want %v", err, tc.wantErr)
				  }
				  return
			   }
			  if err != nil {
				  t.Fatalf("unexpected error: %v", err)
			   }
			   updated, _ := caseRepo.Get(ctx, "mc-close")
			  if updated.State != haltclear.MisClearStateClosed {
				   t.Errorf("state=%s want closed", updated.State)
			  }
			   if updated.ClosedBy != "ops-alice" {
				   t.Errorf("closed_by=%s want ops-alice", updated.ClosedBy)
			  }
		   })
	   }
   }

// TestRollbackProofVerify 证明 checksum 可独立校验（无数据库依赖）。

  func TestRollbackProofVerify(t *testing.T) {
	   leg := &haltclear.ReEngageLeg{
		  LegID:      "leg-vp",
		  MisclearID: "mc-vp",
		   Scope:   "global",
		  ActorID: "system",
		   Reason: "test",
	  }
	  proof := haltclear.NewRollbackProof(leg, 3)
	  if !haltclear.VerifyRollbackProof(proof) {
		  t.Error("new proof should be valid")
	   }
	   // 篡改字段后应无效。
	  tampered := *proof
	  tampered.OpenDiffCount = 5
	  if haltclear.VerifyRollbackProof(&tampered) {
		   t.Error("tampered proof should be invalid")
	  }
	   nilOk := haltclear.VerifyRollbackProof(nil)
	   if nilOk {
		  t.Error("nil proof should be invalid")
	  }
   }
