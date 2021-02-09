package agreeleasegate_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/agreeleasegate"
	"account-assets.kucoin.net/funding/internal/sweeprun"
)

type goldenFixture struct {
	Comment  string      `json:"_comment"`
	Scenario string      `json:"_scenario"`
	Setup    goldenSetup `json:"setup"`
	Steps []goldenStep `json:"steps"`
	Invariants []string `json:"invariants"`
}

type goldenSetup struct {
	Chain string `json:"chain"`
	Asset     string `json:"asset"`
	ShadowBal string `json:"shadow_bal"`
	Waterline string `json:"waterline"`
	MinSweep string `json:"min_sweep"`
	MinRetain   string   `json:"min_retain"`
	Kinds       []string `json:"kinds"`
	RuleVersion uint64   `json:"rule_version"`
	LeaseTTLMs  int      `json:"lease_ttl_ms"`
}

type goldenStep struct {
	Step      int    `json:"step"`
	Action    string `json:"action"`
	ShadowBal string `json:"shadow_bal"`
	Waterline string `json:"waterline"`
	RuleVersion uint64 `json:"rule_version"`
	PlanRef     string `json:"plan_ref"`
	ExecRef     string `json:"exec_ref"`
	LeaseID string `json:"agree_lease_id"`
	SleepMs int `json:"sleep_ms"`
	Consume bool                   `json:"consume"`
	Expect  map[string]interface{} `json:"expect"`
}

   func testdataDir(t *testing.T) string {
	  t.Helper()
	   _, file, _, ok := runtime.Caller(0)
	   if !ok {
		   t.Fatal("caller")
	  }
	  return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "agreeleasegate")
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

   func seedGate(t *testing.T,
	  setup goldenSetup) *sweeprun.AgreeBook {
	  t.Helper()
	   g := sweeprun.NewAgreeBook()
	   if err := g.PutShadowBal(sweeprun.ShadowBal{Chain: setup.Chain, Asset: setup.Asset, Amount: setup.ShadowBal}); err != nil {
		   t.Fatal(err)
	   }
	  kinds := setup.Kinds
	  if len(kinds) == 0 {
		  kinds = []string{"hot_cold"}
	  }
	  if err := g.PutRule(sweeprun.RebalanceRuleSnap{
		   Asset: setup.Asset, Waterline: setup.Waterline, MinSweep: setup.MinSweep,
		   MinRetain: setup.MinRetain, Kinds: kinds, Version: setup.RuleVersion,
	   }); err != nil {
		  t.Fatal(err)
	  }
	  return g
  }

  func newHarness(t *testing.T, setup goldenSetup) (*agreeleasegate.Engine, *sweeprun.AgreeBook, *agreeleasegate.MemStore) {
	   t.Helper()
	  g := seedGate(t, setup)
	   ttl := 90 * time.Second
	  if setup.LeaseTTLMs > 0 {
		   ttl = time.Duration(setup.LeaseTTLMs) * time.Millisecond
	  }
	  store := agreeleasegate.NewMemStore()
	  eng := agreeleasegate.NewEngine(store, g, agreeleasegate.Config{LeaseTTL: ttl})
	  return eng, g, store
   }

  func TestGoldenFixtures(t *testing.T) {
	   files := []string{
		   "golden_bal_below_after_agree.json",
		  "golden_rule_version_raise.json",
		  "golden_expired_lease.json",
		  "golden_no_lease_fail_closed.json",
		   "golden_normal_consume.json",
		   "golden_waterline_raise.json",
	  }
	  for _, name := range files {
		  name := name
		   t.Run(name, func(t *testing.T) {
			  fix := loadGolden(t, name)
			  eng,
				  gate, store := newHarness(t, fix.Setup)
			   ctx := context.Background()
			   var leaseID string
			  var lastProof bool
			  for _, st := range fix.Steps {
				  switch st.Action {
				  case "agree":
					  res, err := eng.AgreeWithLease(ctx, agreeleasegate.IssueRequest{
						  Chain: fix.Setup.Chain, Asset: fix.Setup.Asset, IdemKey: name + "-agree",
					   })
					   wantAllow, _ := st.Expect["allowed"].(bool)
					   if wantAllow {
						   if err != nil || res == nil || !res.Allowed || res.Lease == nil {
							   t.Fatalf("step %d agree: err=%v res=%+v", st.Step, err, res)
						  }
						  leaseID = res.Lease.ID
					  } else if res != nil && res.Allowed {
						  t.Fatalf("step %d agree unexpectedly allowed", st.Step)
					  }
				  case "put_bal":
					  if err := gate.PutShadowBal(sweeprun.ShadowBal{
						   Chain: fix.Setup.Chain, Asset: fix.Setup.Asset, Amount: st.ShadowBal,
					   }); err != nil {
						  t.Fatal(err)
					  }
				   case "bump_rule":
					   if err := gate.BumpRuleVersion(fix.Setup.Asset, st.Waterline); err != nil {
						  t.Fatal(err)
					  }
				   case "put_rule":
					   rule,
						  ok := gate.GetRule(fix.Setup.Asset)
					   if !ok {
						   t.Fatal("rule missing")
					  }
					  rule.Waterline = st.Waterline
					  if st.RuleVersion > 0 {
						   rule.Version = st.RuleVersion
					  }
					   if err := gate.PutRule(rule); err != nil {
						  t.Fatal(err)
					  }
				  case "sleep_ms":
					  time.Sleep(time.Duration(st.SleepMs) * time.Millisecond)
				  case "plan":
					   id := st.LeaseID
					   if fix.Scenario == "no_lease_fail_closed" {
						  id = ""
					  } else if id == "" {
						  id = leaseID
					   }
					   res, err := eng.PlanAssertLease(ctx, agreeleasegate.PlanAssertRequest{
						   LeaseID: id, Chain: fix.Setup.Chain, Asset: fix.Setup.Asset, PlanRef: st.PlanRef,
					   })
					   wantAllow, _ := st.Expect["allowed"].(bool)
					  if wantAllow {
						   if err != nil || res == nil || !res.Allowed {
							  t.Fatalf("step %d plan: err=%v res=%+v", st.Step, err, res)
						  }
					  } else {
						   if res != nil && res.Allowed {
							   t.Fatalf("step %d plan allowed", st.Step)
						  }
						   if has, _ := st.Expect["has_proof"].(bool); has {
							  if res == nil || res.DriftProof == nil {
								   t.Fatalf("step %d want proof err=%v", st.Step, err)
							  }
							   lastProof = true
						  }
					  }
				   case "exec", "exec_skip_plan":
					  id := st.LeaseID
					  if fix.Scenario == "no_lease_fail_closed" {
						   id = ""
					  } else if id == "" {
						  id = leaseID
					   }
					   res, err := eng.ExecAssertLease(ctx, agreeleasegate.ExecAssertRequest{
						  LeaseID: id, Chain: fix.Setup.Chain, Asset: fix.Setup.Asset,
						  ExecRef: st.ExecRef,
						  Consume: st.Consume,
					  })
					  wantAllow, _ := st.Expect["allowed"].(bool)
					   if wantAllow {
						  if err != nil || res == nil || !res.Allowed {
							   t.Fatalf("step %d exec: err=%v res=%+v", st.Step, err, res)
						   }
					  } else {
						  if res != nil && res.Allowed {
							   t.Fatalf("step %d exec allowed", st.Step)
						  }
						  if has, _ := st.Expect["has_proof"].(bool); has {
							  if res == nil || res.DriftProof == nil {
								   t.Fatalf("step %d want proof err=%v", st.Step, err)
							   }
							   lastProof = true
						   }
					  }
				  default:
					  t.Fatalf("unknown action %s", st.Action)
				   }
			   }
			  for _, inv := range fix.Invariants {
				   switch inv {
				  case "lease_not_consumed", "not_consumed":
					   if leaseID == "" {
						   continue
					  }
					  l, err := store.GetLeaseByID(ctx, leaseID)
					   if err != nil {
						   t.Fatal(err)
					  }
					   if l.State == agreeleasegate.LeaseStateConsumed {
						   t.Fatalf("invariant %s: consumed", inv)
					  }
				  case "has_drift_proof":
					   if !lastProof && leaseID != "" {
						  l, _ := store.GetLeaseByID(ctx, leaseID)
						  if l == nil || l.DriftProofID == "" {
							  t.Fatal("want drift proof")
						  }
					  }
				  case "single_consume":
					   if leaseID == "" {
						   t.Fatal("no lease")
					  }
					  l, err := store.GetLeaseByID(ctx, leaseID)
					  if err != nil {
						  t.Fatal(err)
					   }
					   if l.State != agreeleasegate.LeaseStateConsumed {
						   t.Fatalf("want consumed got %s", l.State)
					   }
				   case "fail_closed", "no_consumed_lease":
					  rows, _ := store.ListActiveLeases(ctx, 100)
					   for _, l := range rows {
						  if l.State == agreeleasegate.LeaseStateConsumed {
							   t.Fatal("consumed under fail-closed")
						  }
					   }
				  case "no_double_spend":
					   if leaseID == "" {
						   continue
					  }
					   l, _ := store.GetLeaseByID(ctx, leaseID)
					  if l != nil && l.State == agreeleasegate.LeaseStateConsumed {
						   t.Fatal("double spend consume")
					   }
				  }
			  }
		  })
	   }
  }
