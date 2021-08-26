package liqgengate_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/liqgengate"
)

type goldenFixture struct {
	Comment string `json:"_comment"`
	Scenario string      `json:"_scenario"`
	Setup    goldenSetup `json:"setup"`
	Steps []goldenStep `json:"steps"`
	Invariants []string `json:"invariants"`
}

type goldenSetup struct {
	UserID string `json:"user_id"`
	BorrowID string `json:"borrow_id"`
	CollateralID string `json:"collateral_id"`
	Asset        string `json:"asset"`
	BorrowAmt string `json:"borrow_amt"`
	CollateralAmt string `json:"collateral_amt"`
	LTVBPS int `json:"ltv_bps"`
	CapBPS     int  `json:"cap_bps"`
	HookFail   bool `json:"hook_fail"`
	LeaseTTLMs int  `json:"lease_ttl_ms"`
}

type goldenStep struct {
	Step   int    `json:"step"`
	Action string `json:"action"`
	LiqGen uint64 `json:"liq_gen"`
	HasGen bool   `json:"has_gen"`
	SleepMs int                    `json:"sleep_ms"`
	Expect  map[string]interface{} `json:"expect"`
}

  func testdataDir(t *testing.T) string {
	   t.Helper()
	   _, file, _, ok := runtime.Caller(0)
	  if !ok {
		   t.Fatal("caller")
	   }
	  return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "liqgengate")
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

   func expectBool(m map[string]interface{}, k string) (bool, bool) {
	   v, ok := m[k]
	  if !ok {
		  return false, false
	  }
	  b, ok := v.(bool)
	  return b, ok
   }

  func expectStr(m map[string]interface{}, k string) string {
	  v, ok := m[k]
	   if !ok {
		   return ""
	  }
	  s, _ := v.(string)
	   return s
  }

   func TestGoldenFixtures(t *testing.T) {
	  files := []string{
		   "golden_stale_gen_release.json",
		  "golden_missing_gen_fail_closed.json",
		   "golden_hook_fail_half.json",
		  "golden_mutex_freeze_release.json",
		   "golden_normal_release_close.json",
		   "golden_expired_lease.json",
	   }
	  for _, name := range files {
		  name := name
		  t.Run(name, func(t *testing.T) {
			  runGolden(t, loadGolden(t, name))
		   })
	  }
   }

   func runGolden(t *testing.T, f goldenFixture) {
	  t.Helper()
	  s := f.Setup
	   v := seedView(s.UserID, s.BorrowID, s.CollateralID, s.Asset, s.BorrowAmt, s.CollateralAmt, s.LTVBPS, s.CapBPS)
	   var hook liqgengate.FreezeHook
	   if s.HookFail {
		  hook = func(string, account.Book, bool) error { return errors.New("hook fail fixture") }
	   }
	  ttl := time.Minute
	  if s.LeaseTTLMs > 0 {
		  ttl = time.Duration(s.LeaseTTLMs) * time.Millisecond
	   }
	  store := liqgengate.NewMemStore()
	  eng := liqgengate.NewEngine(store,
		   v, hook, liqgengate.Config{LeaseTTL: ttl})
	   ctx := context.Background()
	   var lastGen uint64
	  for _, step := range f.Steps {
		  switch step.Action {
		   case "freeze":
			   res,
				  err := eng.FreezeWithGen(ctx, liqgengate.FreezeRequest{UserID: s.UserID, BorrowID: s.BorrowID})
			  checkFreeze(t, step, res, err)
			   if res != nil && res.Generation != nil {
				  lastGen = res.Generation.Gen
			  }
		  case "release":
			  res, err := eng.ReleaseWithGen(ctx, liqgengate.ReleaseRequest{
				  UserID: s.UserID,
				  BorrowID: s.BorrowID, LiqGen: step.LiqGen, HasGen: step.HasGen,
			   })
			   checkGate(t, step, res, err)
		   case "close":
			   res, err := eng.CloseWithGen(ctx, liqgengate.CloseRequest{
				   UserID: s.UserID, BorrowID: s.BorrowID, LiqGen: step.LiqGen, HasGen: step.HasGen,
			  })
			   checkGate(t, step, res, err)
		   case "hold_mutex_freeze":
			   _ = store.InsertGeneration(ctx, &liqgengate.LiqGeneration{
				  ID: "seed", BorrowID: s.BorrowID, UserID: s.UserID, CollateralID: s.CollateralID,
				  Phase: liqgengate.PhaseOpen, CreatedAt: time.Now(), UpdatedAt: time.Now(),
			   })
			   if err := store.TryAcquireMutex(ctx, &liqgengate.MutexRecord{
				   BorrowID: s.BorrowID, Owner: "freeze:golden", Kind: "freeze", AcquiredAt: time.Now(),
			   }); err != nil {
				   t.Fatal(err)
			   }
		  case "sleep_ms":
			   time.Sleep(time.Duration(step.SleepMs) * time.Millisecond)
		  default:
			   t.Fatalf("unknown action %s", step.Action)
		  }
	  }
	   for _, inv := range f.Invariants {
		   switch inv {
		   case "race_proof":
			   rows, err := eng.ListRaceProofs(ctx, 20)
			  if err != nil || len(rows) == 0 {
				  t.Fatalf("want race proof")
			  }
		  case "half_proof":
			  rows, err := eng.ListHalfProofs(ctx, 20)
			  if err != nil || len(rows) == 0 {
				   t.Fatalf("want half proof")
			   }
		  case "no_double_release":
			   g, err := eng.GetGeneration(ctx, s.BorrowID)
			  if err == nil && g.Phase == liqgengate.PhaseClosed && lastGen == 0 {
				  t.Fatalf("unexpected closed without matched path")
			  }
		   case "not_fully_liquidated":
			   g, err := eng.GetGeneration(ctx, s.BorrowID)
			  if err != nil || g.Phase == liqgengate.PhaseClosed || g.Phase == liqgengate.PhaseReleasing {
				  t.Fatalf("half must not be released: %+v", g)
			   }
		  }
	  }
  }

   func checkFreeze(t *testing.T, step goldenStep, res *liqgengate.FreezeResult, err error) {
	   t.Helper()
	  if want, ok := expectBool(step.Expect, "allowed"); ok {
		   got := err == nil && res != nil && res.Allowed
		   if got != want {
			  t.Fatalf("step %d freeze allowed=%v err=%v res=%+v", step.Step, got, err, res)
		  }
	   }
	  if p := expectStr(step.Expect, "phase"); p != "" && res != nil && res.Generation != nil {
		   if string(res.Generation.Phase) != p {
			  t.Fatalf("step %d phase %s want %s", step.Step, res.Generation.Phase, p)
		  }
	   }
	  if want, ok := expectBool(step.Expect, "has_half"); ok && want {
		   if res == nil || res.HalfProof == nil {
			   t.Fatalf("step %d want half proof", step.Step)
		   }
	  }
   }

  func checkGate(t *testing.T, step goldenStep, res *liqgengate.GateResult, err error) {
	   t.Helper()
	  if want, ok := expectBool(step.Expect, "allowed"); ok {
		  got := err == nil && res != nil && res.Allowed
		   if got != want {
			   t.Fatalf("step %d allowed=%v err=%v reason=%v", step.Step, got, err, func() string {
				   if res != nil {
					  return res.RejectReason
				   }
				  return ""
			   }())
		  }
	  }
	   if r := expectStr(step.Expect, "reason"); r != "" {
		   if res == nil || res.RejectReason != r {
			   t.Fatalf("step %d reason %v want %s", step.Step, res, r)
		   }
	   }
	  if want, ok := expectBool(step.Expect, "has_proof"); ok && want {
		   if res == nil || res.RaceProof == nil {
			  t.Fatalf("step %d want race proof", step.Step)
		   }
	   }
	  if p := expectStr(step.Expect, "phase"); p != "" && res != nil && res.Generation != nil {
		   if string(res.Generation.Phase) != p {
			  t.Fatalf("step %d phase %s", step.Step, res.Generation.Phase)
		   }
	   }
   }
