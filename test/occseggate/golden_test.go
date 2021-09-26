package occseggate_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"account-assets.kucoin.net/funding/internal/occseggate"
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
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	GrantIdem string `json:"grant_idem"`
}

type goldenStep struct {
	Step   int    `json:"step"`
	Action string `json:"action"`
	HasGen    bool   `json:"has_gen"`
	OccSegGen uint64 `json:"occ_seg_gen"`
	Expect map[string]interface{} `json:"expect"`
}

  func testdataDir(t *testing.T) string {
	   t.Helper()
	  _, file, _, ok := runtime.Caller(0)
	  if !ok {
		   t.Fatal("caller")
	   }
	  return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "occseggate")
   }

   func loadGolden(t *testing.T, name string) goldenFixture {
	  t.Helper()
	   b,
		   err := os.ReadFile(filepath.Join(testdataDir(t), name))
	  if err != nil {
		  t.Fatal(err)
	  }
	   var f goldenFixture
	  if err := json.Unmarshal(b, &f); err != nil {
		  t.Fatal(err)
	  }
	  return f
  }

   func expectBool(m map[string]interface{}, k string) (bool, bool) {
	  v, ok := m[k]
	  if !ok {
		   return false, false
	   }
	   b, ok := v.(bool)
	   return b, ok
  }

		   "golden_stale_gen.json",
		  "golden_refuse_live_unfreeze.json",
		   "golden_abort_then_commit.json",
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
	  eng, _ := newEng(t)
	   ctx := context.Background()
	  var ticketID string
	   var lastGen uint64
	  var lastOcc *occseggate.OccOccupyResult
	  for _, st := range f.Steps {
		   switch st.Action {
		  case "open_admit":
			   tk, err := eng.OpenAndAdmit(ctx, f.Setup.UserID, f.Setup.Asset, f.Setup.Amount)
			   if err != nil {
				   t.Fatalf("step %d open: %v", st.Step, err)
			  }
			  ticketID = tk.ID
			  if want, ok := expectStr(st.Expect, "ticket_state"); ok && string(tk.State) != want {
				   t.Fatalf("step %d state %s want %s", st.Step, tk.State, want)
			  }
		   case "before_freeze":
			   res, err := eng.BeforeFreezeWithGen(ctx, occseggate.OccOccupyRequest{
				  TicketID:  ticketID,
				  GrantIdem: f.Setup.GrantIdem, Amount: f.Setup.Amount, Asset: f.Setup.Asset,
			   })
			  if err != nil {
				  t.Fatalf("step %d occupy: %v", st.Step, err)
			  }
			  lastOcc = res
			   lastGen = res.Seg.Gen
			  if want, ok := expectBool(st.Expect, "has_gen"); ok && want && res.Seg.Gen == 0 {
				   t.Fatalf("step %d missing gen", st.Step)
			  }
			  if want, ok := expectStr(st.Expect, "seg_state"); ok && string(res.Seg.State) != want {
				  t.Fatalf("step %d seg %s", st.Step, res.Seg.State)
			  }
		  case "commit", "abort", "commit_old", "abort_old", "commit_stale":
			  req := occseggate.GenRequest{GrantIdem: f.Setup.GrantIdem}
			  if st.Action == "commit_stale" {
				   g := st.OccSegGen
				   req.OccSegGen = &g
			   } else if st.HasGen {
				  g := lastGen
				  req.OccSegGen = &g
			  }
			   var res *occseggate.GateResult
			  var err error
			   if st.Action == "abort" || st.Action == "abort_old" {
				   res, err = eng.AbortAssertGen(ctx, req)
			  } else {
				  res,
					  err = eng.CommitAssertGen(ctx, req)
			  }
			  checkGate(t, st, res, err)
		  case "expire_revoke":
			  rev, err := eng.ExpireRevokeGens(ctx, ticketID, "golden")
			  if err != nil {
				   t.Fatalf("step %d expire: %v", st.Step, err)
			   }
			  if want, ok := expectBool(st.Expect, "revoked"); ok && want && rev.RevokedN < 1 {
				  t.Fatalf("step %d revoke n=%d", st.Step, rev.RevokedN)
			  }
		   case "reopen_admit":
			   tk, err := eng.ReopenAdmit(ctx, ticketID)
			  if err != nil {
				  t.Fatalf("step %d reopen: %v", st.Step, err)
			  }
			   if want, ok := expectStr(st.Expect, "ticket_state"); ok && string(tk.State) != want {
				   t.Fatalf("step %d %s", st.Step, tk.State)
			  }
		   case "refuse_live":
			   p,
				  err := eng.RefuseLiveUnfreeze(ctx, f.Setup.GrantIdem)
			   if err == nil {
				   t.Fatalf("step %d live must fail", st.Step)
			   }
			  if want, ok := expectBool(st.Expect, "has_proof"); ok && want && p == nil {
				  t.Fatalf("step %d proof", st.Step)
			  }
		   default:
			  t.Fatalf("unknown action %s", st.Action)
		  }
	   }
	   _ = lastOcc
   }

  func checkGate(t *testing.T, st goldenStep, res *occseggate.GateResult, err error) {
	  t.Helper()
	   if want, ok := expectBool(st.Expect, "allowed"); ok {
		  got := err == nil && res != nil && res.Allowed
		   if got != want {
			  t.Fatalf("step %d allowed got=%v want=%v err=%v", st.Step, got, want, err)
		  }
	  }
	  if want, ok := expectStr(st.Expect, "reason"); ok {
		   if res == nil || res.RejectReason != want {
			  t.Fatalf("step %d reason %v want %s", st.Step, res, want)
		  }
	   }
	   if want, ok := expectStr(st.Expect, "seg_state"); ok {
		  if res == nil || res.Seg == nil || string(res.Seg.State) != want {
			   t.Fatalf("step %d seg %+v", st.Step, res)
		   }
	  }
	   if want, ok := expectBool(st.Expect, "has_proof"); ok && want {
		   if res == nil || res.Proof == nil {
			  t.Fatalf("step %d proof missing", st.Step)
		   }
	  }
   }
