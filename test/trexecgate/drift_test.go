package trexecgate_test

import (
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/treasury"
	"account-assets.kucoin.net/funding/internal/trexecgate"
)

  func TestCheckDrift_TableDriven(t *testing.T) {
	   band := treasury.Band{
		   Asset: "USDT",
		  Low:   "50000",
		   Target: "200000",
		  High:   "500000",
	  }
	   now := time.Date(2021, 6, 15, 12, 0, 0, 0, time.UTC)
	   makeSnap := func(asset, avail string) trexecgate.SnapshotDigest {
		  return trexecgate.SnapshotDigest{
			   Asset:    asset,
			   HotAvail: avail,
			  HotMax:  "1000000",
			  BandLow: band.Low,
			  BandTarget: band.Target,
			  BandHigh:   band.High,
			   ObservedAt: now,
		  }
	  }
	  cases := []struct {
		   name string
		  recordAvail   string
		  recheckAvail  string
		   wantExceeded  bool
		  wantReasonHas string
	   }{
		  {
			   name:        "below_target_reject",
			   recordAvail: "480000",
			  recheckAvail: "150000",
			  wantExceeded: true,
			  wantReasonHas: "below band target",
		  },
		  {
			   // record=205000, recheck=200000 → drift=5000/205000≈2.4% < 5%，且高于 target=200000。
			   name:         "just_at_target_small_drift_pass",
			  recordAvail:  "205000",
			  recheckAvail: "200000",
			  wantExceeded: false,
		  },
		  {
			  name:         "above_target_small_drift_pass",
			  recordAvail:  "480000",
			  recheckAvail: "460000",
			   wantExceeded: false,
		  },
		   {
			  // record=400000, recheck=380000 → drift=20000/400000=5%，容忍率为严格大于 5%，
			   // 恰好 5% 属于通过（tolerance 为 > 0.05）。
			   name: "relative_drift_exactly_5pct_pass",
			   recordAvail:  "400000",
			  recheckAvail: "380000",
			  wantExceeded: false,
		   },
		  {
			   name:         "relative_drift_above_5pct_reject",
			  recordAvail:  "400000",
			   recheckAvail: "369999",
			  wantExceeded:  true,
			  wantReasonHas: "relative drift",
		   },
		   {
			  name: "zero_record_hot_no_panic",
			   recordAvail: "0",
			  recheckAvail: "0",
			   wantExceeded: true,
			   wantReasonHas: "below band target",
		   },
		   {
			   name:         "hot_increased_pass",
			   recordAvail:  "300000",
			  recheckAvail: "350000",
			  wantExceeded: false,
		  },
		   {
			   name: "hot_unchanged_pass",
			  recordAvail:  "480000",
			   recheckAvail: "480000",
			   wantExceeded: false,
		   },
		   {
			  name:          "large_drop_below_target_reject",
			  recordAvail:   "500000",
			   recheckAvail:  "10000",
			   wantExceeded:  true,
			   wantReasonHas: "below band target",
		   },
		  {
			   name: "just_above_target_within_tolerance_pass",
			  recordAvail: "210000",
			  recheckAvail: "200500",
			  wantExceeded: false,
		   },
	   }
	  for _, tc := range cases {
		   tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			  rec := makeSnap("USDT", tc.recordAvail)
			  recheck := makeSnap("USDT", tc.recheckAvail)
			  dr, err := trexecgate.CheckDrift(rec, recheck, band)
			   if err != nil {
				   t.Fatalf("CheckDrift: %v", err)
			  }
			   if dr.Exceeded != tc.wantExceeded {
				  t.Errorf("Exceeded: got %v, want %v (reason: %s)", dr.Exceeded, tc.wantExceeded, dr.Reason)
			   }
			   if tc.wantReasonHas != "" && !strings.Contains(dr.Reason, tc.wantReasonHas) {
				  t.Errorf("Reason %q missing %q", dr.Reason, tc.wantReasonHas)
			  }
		  })
	  }
  }

  func TestBuildDriftProof_Fields(t *testing.T) {
	   now := time.Date(2021, 6, 15, 12, 0, 0, 0, time.UTC)
	   band := treasury.Band{
		  Asset: "BTC",
		  Low:    "5",
		  Target: "20",
		  High:   "80",
	  }
	   rec := trexecgate.SnapshotDigest{
		   Asset:    "BTC",
		  HotAvail: "75",
		  HotMax:   "80",
		   BandLow: band.Low, BandTarget: band.Target, BandHigh: band.High,
		   ObservedAt: now,
	   }
	  recheck := trexecgate.SnapshotDigest{
		   Asset: "BTC",
		  HotAvail: "10",
		   HotMax:  "80",
		  BandLow: band.Low, BandTarget: band.Target, BandHigh: band.High,
		   ObservedAt: now.Add(5 * time.Minute),
	   }
	   dr,
		  err := trexecgate.CheckDrift(rec, recheck, band)
	   if err != nil {
		   t.Fatalf("CheckDrift: %v", err)
	  }
	  if !dr.Exceeded {
		   t.Fatal("expected drift exceeded")
	   }
	   proof := trexecgate.BuildDriftProof("lease-001", "exec-001", "BTC", dr, now)
	   if proof.ProofID == "" {
		   t.Error("proof_id empty")
	   }
	   if proof.LeaseID != "lease-001" {
		   t.Errorf("lease_id: got %s", proof.LeaseID)
	  }
	   if proof.ExecID != "exec-001" {
		  t.Errorf("exec_id: got %s", proof.ExecID)
	  }
	  if proof.Asset != "BTC" {
		  t.Errorf("asset: got %s", proof.Asset)
	  }
	  if proof.Decision != "blocked" {
		   t.Errorf("decision: got %s, want blocked", proof.Decision)
	  }
	  if proof.DriftAbs == "" || proof.DriftAbs == "0" {
		   t.Errorf("drift_abs: got %s", proof.DriftAbs)
	   }
	   if proof.CreatedAt != now.UTC() {
		   t.Errorf("created_at mismatch")
	   }
   }

   func TestBuildDriftProof_Pass(t *testing.T) {
	  now := time.Date(2021, 6, 15, 12, 0, 0, 0, time.UTC)
	   band := treasury.Band{
		   Asset: "USDT",
		  Low:    "50000",
		  Target: "200000",
		  High:   "500000",
	   }
	  rec := trexecgate.SnapshotDigest{
		  Asset: "USDT", HotAvail: "480000", HotMax: "1000000",
		  BandLow: band.Low, BandTarget: band.Target, BandHigh: band.High,
		  ObservedAt: now,
	  }
	  recheck := trexecgate.SnapshotDigest{
		  Asset: "USDT", HotAvail: "460000", HotMax: "1000000",
		   BandLow:    band.Low,
		   BandTarget: band.Target, BandHigh: band.High,
		   ObservedAt: now.Add(3 * time.Minute),
	  }
	   dr, err := trexecgate.CheckDrift(rec, recheck, band)
	  if err != nil {
		   t.Fatalf("CheckDrift: %v", err)
	  }
	   if dr.Exceeded {
		  t.Fatalf("expected pass, got: %s", dr.Reason)
	  }
	  proof := trexecgate.BuildDriftProof("l-pass-001", "e-pass-001", "USDT", dr, now)
	  if proof.Decision != "pass" {
		  t.Errorf("pass proof decision: got %s, want pass", proof.Decision)
	   }
  }

   func TestSnapshotDigest_Deterministic(t *testing.T) {
	   now := time.Date(2021, 6, 15, 12, 0, 0, 0, time.UTC)
	   d1 := trexecgate.SnapshotDigest{
		  Asset: "USDT", HotAvail: "480000", HotMax: "1000000",
		   BandLow: "50000", BandTarget: "200000", BandHigh: "500000",
		   ObservedAt: now,
	   }
	  d2 := d1
	   if d1.Digest() != d2.Digest() {
		  t.Error("same snapshot should produce same digest")
	  }
	   d3 := d1
	  d3.HotAvail = "470000"
	  if d1.Digest() == d3.Digest() {
		   t.Error("different hot_avail should produce different digest")
	   }
   }

  func TestIdemKeyFor_Stable(t *testing.T) {
	  k1 := trexecgate.IdemKeyFor("USDT", "plan-001")
	  k2 := trexecgate.IdemKeyFor("USDT", "plan-001")
	   k3 := trexecgate.IdemKeyFor("usdt", "plan-001")
	  if k1 != k2 {
		  t.Error("same inputs must produce same idem_key")
	   }
	   if k1 != k3 {
		  t.Error("case-insensitive asset must produce same idem_key")
	  }
	   k4 := trexecgate.IdemKeyFor("BTC", "plan-001")
	   if k1 == k4 {
		   t.Error("different assets must produce different idem_key")
	  }
	   k5 := trexecgate.IdemKeyFor("USDT", "plan-002")
	  if k1 == k5 {
		   t.Error("different plan_id must produce different idem_key")
	  }
  }
