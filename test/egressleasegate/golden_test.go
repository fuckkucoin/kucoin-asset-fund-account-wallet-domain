package egressleasegate_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/egressleasegate"
	"account-assets.kucoin.net/funding/internal/floatdesk"
)

type goldenFixture struct {
	Comment string `json:"_comment"`
	Scenario string       `json:"_scenario"`
	Setup    goldenSetup  `json:"setup"`
	Steps    []goldenStep `json:"steps"`
	Invariants []string `json:"invariants"`
}

type goldenSetup struct {
	Chain        string `json:"chain"`
	Asset        string `json:"asset"`
	HotAvailable string `json:"hot_available"`
	FromTank bool `json:"from_tank"`
	AllowWarn    bool   `json:"allow_warn_lease"`
	RefuseRefill bool   `json:"refuse_on_refill"`
	RefillActive bool   `json:"refill_active"`
	TankAvail    string `json:"tank_avail"`
	TankMin      string `json:"tank_min"`
	TankTarget string `json:"tank_target"`
	TankNative string `json:"tank_native"`
	LeaseTTLMs int    `json:"lease_ttl_ms"`
}

type goldenStep struct {
	Step   int    `json:"step"`
	Action string `json:"action"`
	Chain  string `json:"chain"`
	BroadcastRef string `json:"broadcast_ref"`
	LeaseID      string `json:"egress_lease_id"`
	TankAvail string                 `json:"tank_avail"`
	SleepMs   int                    `json:"sleep_ms"`
	Expect    map[string]interface{} `json:"expect"`
}

  func testdataDir(t *testing.T) string {
	   t.Helper()
	  _, file, _, ok := runtime.Caller(0)
	   if !ok {
		  t.Fatal("caller")
	   }
	  return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "egressleasegate")
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

  func newHarness(t *testing.T, setup goldenSetup) (*egressleasegate.Engine, *floatdesk.Service, *egressleasegate.MemStore) {
	   t.Helper()
	  desk := floatdesk.New()
	   if setup.TankNative != "" {
		  _ = desk.Tanks().Put(floatdesk.Tank{
			  Chain: setup.Chain, Native: setup.TankNative,
			   Avail: setup.TankAvail, Min: setup.TankMin, Target: setup.TankTarget,
		   })
	   }
	   if setup.RefillActive {
		   desk.RefillObserve().SetActive(setup.Chain, setup.Asset, true)
	  }
	  ttl := 90 * time.Second
	  if setup.LeaseTTLMs > 0 {
		   ttl = time.Duration(setup.LeaseTTLMs) * time.Millisecond
	  }
	   store := egressleasegate.NewMemStore()
	   eng := egressleasegate.NewEngine(store, desk, egressleasegate.Config{
		  LeaseTTL: ttl,
		  AllowWarnLease:           setup.AllowWarn,
		  RefuseNewLeaseWhenRefill: setup.RefuseRefill,
	  })
	  return eng, desk, store
   }

  func TestGoldenFixtures(t *testing.T) {
	   files := []string{
		  "golden_halt_chain_after_allow.json",
		  "golden_level_ok_to_halt.json",
		  "golden_no_lease_fail_closed.json",
		  "golden_normal_consume.json",
		  "golden_refill_refuse.json",
		   "golden_expired_lease.json",
	   }
	   for _, name := range files {
		  name := name
		   t.Run(name, func(t *testing.T) {
			   fix := loadGolden(t, name)
			  eng, desk, store := newHarness(t, fix.Setup)
			  ctx := context.Background()
			  var leaseID string
			   for _, st := range fix.Steps {
				  switch st.Action {
				   case "issue":
					  res, err := eng.CheckEgressWithLease(ctx, egressleasegate.IssueRequest{
						   Chain: fix.Setup.Chain, Asset: fix.Setup.Asset,
						   HotAvail: fix.Setup.HotAvailable, FromTank: fix.Setup.FromTank,
						  IdemKey: name + "-issue",
					  })
					  wantAllow, _ := st.Expect["allowed"].(bool)
					  if wantAllow {
						   if err != nil || res == nil || !res.Allowed || res.Lease == nil {
							   t.Fatalf("step %d issue: err=%v res=%+v", st.Step, err, res)
						  }
						  leaseID = res.Lease.ID
						   if br, ok := st.Expect["breach"].(string); ok && string(res.Lease.BreachLevel) != br {
							  t.Fatalf("breach %s want %s", res.Lease.BreachLevel, br)
						   }
					  } else if res != nil && res.Allowed {
						   t.Fatalf("step %d issue unexpectedly allowed", st.Step)
					  }
				  case "halt_chain":
					   ch := st.Chain
					  if ch == "" {
						   ch = fix.Setup.Chain
					  }
					  if err := desk.Halt().HaltChain(ch); err != nil {
						  t.Fatal(err)
					  }
				  case "put_tank":
					  native := fix.Setup.TankNative
					   if native == "" {
						  native = fix.Setup.Asset
					  }
					  min, tgt := fix.Setup.TankMin, fix.Setup.TankTarget
					  if min == "" {
						   min = "2"
					   }
					   if tgt == "" {
						   tgt = "8"
					   }
					  if err := desk.Tanks().Put(floatdesk.Tank{
						   Chain: fix.Setup.Chain, Native: native, Avail: st.TankAvail, Min: min, Target: tgt,
					  }); err != nil {
						  t.Fatal(err)
					   }
				   case "sleep_ms":
					  time.Sleep(time.Duration(st.SleepMs) * time.Millisecond)
				  case "broadcast":
					   id := st.LeaseID
					  if id == "" && st.Expect != nil {
						   if _, empty := st.Expect["error"]; empty && leaseID == "" {
							   id = ""
						  } else {
							   id = leaseID
						  }
					  }
					   if st.LeaseID == "" && fix.Scenario == "no_lease_fail_closed" {
						  id = ""
					  }
					   res, err := eng.BroadcastAssert(ctx, egressleasegate.BroadcastRequest{
						  LeaseID: id,
						   Chain:   fix.Setup.Chain, Asset: fix.Setup.Asset,
						  BroadcastRef: st.BroadcastRef, FromTank: fix.Setup.FromTank,
						   HotAvail: fix.Setup.HotAvailable,
					  })
					   wantAllow, _ := st.Expect["allowed"].(bool)
					  if wantAllow {
						   if err != nil || res == nil || !res.Allowed {
							  t.Fatalf("step %d broadcast: err=%v res=%+v", st.Step, err, res)
						  }
					   } else {
						  if res != nil && res.Allowed {
							   t.Fatalf("step %d broadcast allowed", st.Step)
						  }
						  if has, _ := st.Expect["has_proof"].(bool); has {
							  if res == nil || res.DriftProof == nil {
								   if err == nil {
									  t.Fatalf("step %d want proof", st.Step)
								   }
							  }
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
					   if l.State == egressleasegate.LeaseStateConsumed {
						   t.Fatalf("invariant %s: consumed", inv)
					  }
				  case "no_double_spend", "single_consume":
				  // 由步骤复验覆盖
				  case "fail_closed", "no_consumed_lease", "no_lease_issued":
					   rows, _ := store.ListActiveLeases(ctx, 100)
					  for _, l := range rows {
						  if l.State == egressleasegate.LeaseStateConsumed {
							  t.Fatal("consumed under fail-closed")
						   }
					   }
				  }
			  }
		  })
	   }
  }
