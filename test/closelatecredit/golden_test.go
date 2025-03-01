package closelatecredit_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"account-assets.kucoin.net/funding/internal/closelatecredit"
)

type goldenFixture struct {
	Comment  string `json:"_comment"`
	Scenario string `json:"_scenario"`
	Setup goldenSetup `json:"setup"`
	Steps      []goldenStep `json:"steps"`
	Invariants []string     `json:"invariants"`
}

type goldenSetup struct {
	UserID string `json:"user_id"`
	Book  string `json:"book"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	TxRef string `json:"tx_ref"`
}

type goldenStep struct {
	Step int `json:"step"`
	Action     string `json:"action"`
	HasEpoch   bool   `json:"has_epoch"`
	CloseEpoch uint64 `json:"close_epoch"`
	TxRef string `json:"tx_ref"`
	Expect map[string]interface{} `json:"expect"`
}

  func testdataDir(t *testing.T) string {
	  t.Helper()
	   _, file, _, ok := runtime.Caller(0)
	  if !ok {
		  t.Fatal("caller")
	  }
	   return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "closelatecredit")
   }

   func loadGolden(t *testing.T, name string) goldenFixture {
	   t.Helper()
	   b, err := os.ReadFile(filepath.Join(testdataDir(t), name))
	  if err != nil {
		   t.Fatal(err)
	  }
	   var f goldenFixture
	   if err := json.Unmarshal(b, &f); err != nil {
		   t.Fatal(err)
	   }
	   return f
  }

   func TestGoldenFixtures(t *testing.T) {
	  cases := []string{
		  "golden_closed_observe_dispute.json",
		   "golden_reopen_stale_refuse.json",
		  "golden_outbox_fiat_same_epoch.json",
		  "golden_missing_epoch_fail_closed.json",
		  "golden_live_admit.json",
		   "golden_dispute_resolve_no_live.json",
	   }
	   for _, name := range cases {
		   name := name
		   t.Run(name, func(t *testing.T) {
			  runGolden(t, loadGolden(t, name))
		  })
	   }
   }

   func runGolden(t *testing.T, f goldenFixture) {
	   t.Helper()
	   eng := newEng(t)
	  ctx := context.Background()
	  var last *closelatecredit.GateResult
	  for _, step := range f.Steps {
		   var epoch *uint64
		   if step.HasEpoch {
			   n := step.CloseEpoch
			   epoch = &n
		  }
		   tx := f.Setup.TxRef
		  if step.TxRef != "" {
			  tx = step.TxRef
		   }
		   switch step.Action {
		  case "ensure":
			  ep, err := eng.EnsureOpen(ctx, f.Setup.UserID, f.Setup.Book, "golden")
			   if err != nil {
				   t.Fatalf("step %d ensure: %v", step.Step, err)
			  }
			   assertEpochExpect(t, step, ep)
		   case "close":
			   ep, err := eng.CloseWithEpoch(ctx, closelatecredit.CloseRequest{UserID: f.Setup.UserID, Book: f.Setup.Book})
			  if err != nil {
				  t.Fatalf("step %d close: %v", step.Step, err)
			   }
			   assertEpochExpect(t, step, ep)
		   case "reopen":
			   ep, err := eng.ReopenWithEpoch(ctx, closelatecredit.ReopenRequest{UserID: f.Setup.UserID, Book: f.Setup.Book})
			  if err != nil {
				  t.Fatalf("step %d reopen: %v", step.Step, err)
			   }
			   assertEpochExpect(t, step, ep)
		  case "assert_observe":
			   res, _ := eng.AssertObserve(ctx, f.Setup.UserID, f.Setup.Asset, f.Setup.Amount, tx, epoch)
			   last = res
			   assertGateExpect(t, step, res)
		  case "assert_outbox":
			  res, _ := eng.AssertOutbox(ctx, f.Setup.UserID, f.Setup.Asset, f.Setup.Amount, tx, epoch)
			   last = res
			  assertGateExpect(t, step, res)
		   case "assert_fiat":
			   res, _ := eng.AssertFiat(ctx, f.Setup.UserID, f.Setup.Asset, f.Setup.Amount, tx, epoch)
			  last = res
			  assertGateExpect(t, step, res)
		   case "resolve":
			  if last == nil || last.Hold == nil {
				   t.Fatalf("step %d resolve without hold", step.Step)
			  }
			  res, err := eng.DisputeResolve(ctx, closelatecredit.DisputeResolveRequest{HoldID: last.Hold.ID, Actor: "ops"})
			   if err != nil {
				  t.Fatalf("step %d resolve: %v", step.Step, err)
			   }
			  assertGateExpect(t, step, res)
			  if st, ok := step.Expect["hold_state"].(string); ok && string(res.Hold.State) != st {
				   t.Fatalf("hold_state %s want %s", res.Hold.State, st)
			  }
		  default:
			   t.Fatalf("unknown action %s", step.Action)
		  }
	  }
	  for _, inv := range f.Invariants {
		  switch inv {
		   case "no_live_credit":
			   if last != nil && last.Allowed {
				  t.Fatal("invariant no_live_credit")
			  }
		  case "proof_required":
			   if last == nil || last.Proof == nil {
				   t.Fatal("invariant proof_required")
			   }
		   case "fail_closed":
			  if last != nil && last.Allowed {
				   t.Fatal("invariant fail_closed")
			   }
		  case "not_addrpool", "not_memo", "same_source_constraint", "epoch_bound":
		  }
	   }
   }

   func assertEpochExpect(t *testing.T, step goldenStep, ep *closelatecredit.CloseEpoch) {
	  t.Helper()
	  if st, ok := step.Expect["state"].(string); ok && string(ep.State) != st {
		   t.Fatalf("step %d state %s want %s", step.Step, ep.State, st)
	   }
	  if n, ok := step.Expect["close_epoch"].(float64); ok && ep.N != uint64(n) {
		   t.Fatalf("step %d epoch %d want %v", step.Step, ep.N, n)
	  }
   }

   func assertGateExpect(t *testing.T, step goldenStep, res *closelatecredit.GateResult) {
	  t.Helper()
	  if res == nil {
		  t.Fatalf("step %d nil gate", step.Step)
	  }
	  if v, ok := step.Expect["allowed"].(bool); ok && res.Allowed != v {
		   t.Fatalf("step %d allowed %v want %v", step.Step, res.Allowed, v)
	  }
	  if v, ok := step.Expect["decision"].(string); ok && string(res.Decision) != v {
		   t.Fatalf("step %d decision %s want %s", step.Step, res.Decision, v)
	  }
	  if v, ok := step.Expect["reason"].(string); ok && res.RejectReason != v && (res.Proof == nil || string(res.Proof.Reason) != v) {
		   t.Fatalf("step %d reason %s proof=%v want %s", step.Step, res.RejectReason, res.Proof, v)
	  }
	   if v, ok := step.Expect["has_proof"].(bool); ok {
		  if v && res.Proof == nil {
			   t.Fatalf("step %d missing proof", step.Step)
		  }
	  }
	   if v, ok := step.Expect["has_hold"].(bool); ok {
		  if v && res.Hold == nil {
			  t.Fatalf("step %d missing hold", step.Step)
		  }
	  }
  }
