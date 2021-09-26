package liqplan_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/liqplan"
)

// ─────────────────────────────────────────────
// trigger_plan_ok.json 对应
// ─────────────────────────────────────────────

   func TestGoldenTriggerPlanOK(t *testing.T) {
	   data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "liqplan", "trigger_plan_ok.json"))
	  if err != nil {
		  t.Fatalf("read fixture: %v", err)
	  }
	  var fixture struct {
		   Request struct {
			   BorrowID       string `json:"borrow_id"`
			   IdempotencyKey string `json:"idempotency_key"`
			  UserID      string `json:"user_id"`
			   BorrowAsset string `json:"borrow_asset"`
			  BorrowAmount string `json:"borrow_amount"`
			  CollateralID string `json:"collateral_id"`
			   CollateralAsset string `json:"collateral_asset"`
			   CollateralAmount string `json:"collateral_amount"`
			   TriggerLTVBPS int `json:"trigger_ltv_bps"`
			   CapBPS        int `json:"cap_bps"`
		   } `json:"request"`
		  MarkGate struct {
			  ID        string `json:"id"`
			  Asset     string `json:"asset"`
			   MarkPrice string `json:"mark_price"`
			  Generation    int64 `json:"generation"`
			  MaxAgeSeconds int   `json:"max_age_seconds"`
			  Poisoned bool `json:"poisoned"`
		   } `json:"mark_gate"`
		  Expect struct {
			  State string `json:"state"`
			   OccupyTokenNonEmpty bool `json:"occupy_token_non_empty"`
			  TriggerLTVBPS       int  `json:"trigger_ltv_bps"`
			   CapBPS int `json:"cap_bps"`
			  UserID string `json:"user_id"`
			  BorrowID        string `json:"borrow_id"`
			   CollateralAsset string `json:"collateral_asset"`
			   BorrowAsset     string `json:"borrow_asset"`
		  } `json:"expect"`
	   }
	   if err := json.Unmarshal(data, &fixture); err != nil {
		  t.Fatalf("parse fixture: %v", err)
	   }
	   svc := liqplan.New()
	  now := time.Now().UTC()
	  svc.RegisterMark(liqplan.MarkGate{
		  ID: fixture.MarkGate.ID,
		  Asset: fixture.MarkGate.Asset,
		  MarkPrice:  fixture.MarkGate.MarkPrice,
		   Generation: fixture.MarkGate.Generation,
		   ObservedAt: now,
		  MaxAgeSeconds: fixture.MarkGate.MaxAgeSeconds,
		   Poisoned: fixture.MarkGate.Poisoned,
	   })
	   plan, err := svc.TriggerPlan(liqplan.TriggerReq{
		  BorrowID: fixture.Request.BorrowID,
		  IdempotencyKey: fixture.Request.IdempotencyKey,
		  UserID:      fixture.Request.UserID,
		   BorrowAsset: fixture.Request.BorrowAsset,
		  BorrowAmount: fixture.Request.BorrowAmount,
		  CollateralID: fixture.Request.CollateralID,
		  CollateralAsset:  fixture.Request.CollateralAsset,
		  CollateralAmount: fixture.Request.CollateralAmount,
		  TriggerLTVBPS:    fixture.Request.TriggerLTVBPS,
		  CapBPS: fixture.Request.CapBPS,
	  })
	  if err != nil {
		  t.Fatalf("trigger plan: %v", err)
	   }
	   if string(plan.State) != fixture.Expect.State {
		  t.Errorf("state: got %q, want %q", plan.State, fixture.Expect.State)
	  }
	  if fixture.Expect.OccupyTokenNonEmpty && plan.OccupyToken == "" {
		   t.Error("occupy_token must not be empty")
	   }
	   if plan.TriggerLTVBPS != fixture.Expect.TriggerLTVBPS {
		  t.Errorf("trigger_ltv_bps: got %d, want %d", plan.TriggerLTVBPS, fixture.Expect.TriggerLTVBPS)
	  }
	   if plan.CapBPS != fixture.Expect.CapBPS {
		  t.Errorf("cap_bps: got %d, want %d", plan.CapBPS, fixture.Expect.CapBPS)
	  }
	   if plan.UserID != fixture.Expect.UserID {
		  t.Errorf("user_id: got %q, want %q", plan.UserID, fixture.Expect.UserID)
	  }
	   if plan.BorrowID != fixture.Expect.BorrowID {
		   t.Errorf("borrow_id: got %q, want %q", plan.BorrowID, fixture.Expect.BorrowID)
	  }
	   if plan.CollateralAsset != fixture.Expect.CollateralAsset {
		   t.Errorf("collateral_asset: got %q, want %q", plan.CollateralAsset, fixture.Expect.CollateralAsset)
	   }
	  if plan.BorrowAsset != fixture.Expect.BorrowAsset {
		   t.Errorf("borrow_asset: got %q, want %q", plan.BorrowAsset, fixture.Expect.BorrowAsset)
	  }
   }

// ─────────────────────────────────────────────
// stale_mark_blocks_trigger.json 对应
// ─────────────────────────────────────────────

   func TestGoldenStalMarkBlocksTrigger(t *testing.T) {
	   data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "liqplan", "stale_mark_blocks_trigger.json"))
	   if err != nil {
		   t.Fatalf("read fixture: %v", err)
	  }
	  var fixture struct {
		  MarkGate struct {
			  ID string `json:"id"`
			   Asset string `json:"asset"`
			   MarkPrice string `json:"mark_price"`
			   Generation           int64 `json:"generation"`
			  ObservedAtOffsetSecs int   `json:"observed_at_offset_seconds"`
			  MaxAgeSeconds int `json:"max_age_seconds"`
			   Poisoned bool `json:"poisoned"`
		   } `json:"mark_gate"`
		   Request struct {
			   BorrowID string `json:"borrow_id"`
			  IdempotencyKey string `json:"idempotency_key"`
			  UserID          string `json:"user_id"`
			   CollateralAsset string `json:"collateral_asset"`
			  TriggerLTVBPS   int    `json:"trigger_ltv_bps"`
			   CapBPS int `json:"cap_bps"`
		  } `json:"request"`
		   Expect struct {
			   PlanCreated bool `json:"plan_created"`
		   } `json:"expect"`
	  }
	   if err := json.Unmarshal(data, &fixture); err != nil {
		  t.Fatalf("parse fixture: %v", err)
	   }
	  svc := liqplan.New().WithMarkMaxAge(fixture.MarkGate.MaxAgeSeconds)
	  now := time.Now().UTC()
	  obsAt := now.Add(time.Duration(fixture.MarkGate.ObservedAtOffsetSecs) * time.Second)
	   svc.RegisterMark(liqplan.MarkGate{
		  ID: fixture.MarkGate.ID,
		  Asset:         fixture.MarkGate.Asset,
		   MarkPrice:     fixture.MarkGate.MarkPrice,
		   Generation:    fixture.MarkGate.Generation,
		   ObservedAt:    obsAt,
		   MaxAgeSeconds: fixture.MarkGate.MaxAgeSeconds,
		  Poisoned:      fixture.MarkGate.Poisoned,
	  })
	   _, err = svc.TriggerPlan(liqplan.TriggerReq{
		   BorrowID:       fixture.Request.BorrowID,
		   IdempotencyKey: fixture.Request.IdempotencyKey,
		   UserID:          fixture.Request.UserID,
		  CollateralAsset: fixture.Request.CollateralAsset,
		   TriggerLTVBPS:   fixture.Request.TriggerLTVBPS,
		   CapBPS: fixture.Request.CapBPS,
	  })
	  if !fixture.Expect.PlanCreated {
		   if err == nil {
			  t.Error("expected error for stale mark, but trigger succeeded")
		   } else {
			  t.Logf("correctly rejected with error: %v", err)
		   }
	  } else {
		  if err != nil {
			  t.Errorf("expected plan to be created, got error: %v", err)
		   }
	   }
  }

// ─────────────────────────────────────────────
// seize_leg_residue.json 对应场景快跑
// ─────────────────────────────────────────────

   func TestGoldenSeizeLegResidue(t *testing.T) {
	  svc := liqplan.New()
	   now := time.Now().UTC()
	  svc.RegisterMark(liqplan.MarkGate{
		   ID:    "mg-residue-golden",
		   Asset: "ETH",
		  MarkPrice: "3000",
		   Generation: 2,
		   ObservedAt: now,
		  MaxAgeSeconds: 3600,
	   })
	   // Step 1: trigger
	   plan, err := svc.TriggerPlan(liqplan.TriggerReq{
		  BorrowID:       "borrow-residue-001",
		  IdempotencyKey: "golden-residue-idem",
		  UserID:       "user-residue",
		  BorrowAsset:  "USDT",
		  BorrowAmount: "8500",
		  CollateralID: "hold-residue-001",
		   CollateralAsset: "ETH",
		  CollateralAmount: "10000",
		   TriggerLTVBPS: 8500,
		  CapBPS:        8000,
	  })
	   if err != nil {
		  t.Fatalf("step1 trigger: %v", err)
	  }
	   if plan.State != liqplan.PlanTriggered {
		  t.Errorf("step1 state: got %q, want triggered", plan.State)
	  }
	  // Step 2: add leg
	  leg, err := svc.AddSeizeLeg(liqplan.AddLegReq{
		   PlanID:       plan.ID,
		   Asset:        "ETH",
		   TargetAmount: "10000",
		  HoldID:       "hold-residue-001",
		   IdempotencyKey: "golden-leg-1",
	  })
	   if err != nil {
		  t.Fatalf("step2 add leg: %v", err)
	   }
	  p, _ := svc.GetPlan(plan.ID)
	   if p.State != liqplan.PlanSeizing {
		   t.Errorf("step2 plan state: got %q, want seizing", p.State)
	  }
	   // Step 3: start
	   _, err = svc.StartLeg(leg.ID)
	   if err != nil {
		  t.Fatalf("step3 start: %v", err)
	  }
	  // Step 4: fail with partial
	   failedLeg, fp, err := svc.FailLeg(leg.ID, "3000", "network timeout during seize")
	   if err != nil {
		  t.Fatalf("step4 fail: %v", err)
	   }
	   if !failedLeg.HasResidue() {
		  t.Error("step4: leg should have residue")
	   }
	  if fp.State != liqplan.PlanResidued {
		  t.Errorf("step4 plan state: got %q, want residued", fp.State)
	   }
	   // Step 5: resume
	  resumed, err := svc.ResumeLeg(leg.ID)
	   if err != nil {
		   t.Fatalf("step5 resume: %v", err)
	   }
	  if resumed.State != liqplan.LegExecuting {
		  t.Errorf("step5 leg state: got %q, want executing", resumed.State)
	   }
	   // Step 6: complete
	  _, _, err = svc.CompleteLeg(resumed.ID, "10000")
	   if err != nil {
		   t.Fatalf("step6 complete: %v", err)
	   }
	  // Step 7: settle
	  p, _ = svc.GetPlan(plan.ID)
	  if p.State == liqplan.PlanSeizing || p.State == liqplan.PlanSettling {
		  p, err = svc.SettleBorrow(plan.ID, "8500")
		  if err != nil {
			   t.Fatalf("step7 settle: %v (state=%s)", err, p.State)
		   }
	   }
	  // 计划应该到达 closed（无残留）或 residued
	   p, _ = svc.GetPlan(plan.ID)
	  t.Logf("plan state after settle: %s", p.State)
	   // Step 8: build proof（若已 closed）
	   if p.State == liqplan.PlanClosed {
		  proof, err := svc.BuildProof(plan.ID)
		   if err != nil {
			  t.Fatalf("step8 build proof: %v", err)
		  }
		   if proof.Digest == "" {
			   t.Error("step8 digest must not be empty")
		   }
		   if proof.OccupyToken != plan.OccupyToken {
			   t.Error("step8 occupy token mismatch")
		  }
		  // Step 9: validate proof
		   validated, err := svc.ValidateProof(proof.ID, "ops-alice", "ops-bob")
		  if err != nil {
			   t.Fatalf("step9 validate: %v", err)
		   }
		  if validated.Status != liqplan.ProofValidated {
			   t.Errorf("step9 status: got %q, want validated", validated.Status)
		  }
		  t.Logf("golden scenario complete: proof %s validated", proof.ID)
	  }
   }
