package flow

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/dustconv"
	"account-assets.kucoin.net/funding/internal/dustleak"
)

// ── PolicyGen CAS（dustconv 层）─────────────────────────────────────────────

  func TestPutPolicyGen_MonotonicallyIncreasing(t *testing.T) {
	   dust := dustconv.New()
	   pol := dustconv.Policy{Asset: "BTC", Floor: "0.001", MinConvert: "0.00000001", Enabled: true}
	   pg1, err := dust.PutPolicyGen(pol)
	   if err != nil {
		  t.Fatalf("PutPolicyGen 1: %v", err)
	  }
	   pg2, err := dust.PutPolicyGen(pol)
	  if err != nil {
		  t.Fatalf("PutPolicyGen 2: %v", err)
	   }
	   if pg2.Gen <= pg1.Gen {
		  t.Errorf("gen must increase: gen1=%d gen2=%d", pg1.Gen, pg2.Gen)
	   }
   }

   func TestPolicyGenOf_ReturnsCurrentGen(t *testing.T) {
	   dust := dustconv.New()
	  _ = dust.PutPolicy(dustconv.Policy{Asset: "ETH", Floor: "0.001", MinConvert: "0.000001", Enabled: true})
	  pg, err := dust.PutPolicyGen(dustconv.Policy{Asset: "ETH", Floor: "0.001", MinConvert: "0.000001", Enabled: true})
	  if err != nil {
		  t.Fatalf("PutPolicyGen: %v", err)
	  }
	  got, err := dust.PolicyGenOf("ETH")
	   if err != nil {
		   t.Fatalf("PolicyGenOf: %v", err)
	   }
	  if got.Gen != pg.Gen {
		  t.Errorf("want gen=%d got %d", pg.Gen, got.Gen)
	   }
	  if got.Floor != "0.001" {
		   t.Errorf("want floor=0.001 got %s", got.Floor)
	   }
  }

   func TestValidateGen_UnchangedGen_ReturnsSnapshot(t *testing.T) {
	  dust := dustconv.New()
	   _ = dust.PutPolicy(dustconv.Policy{Asset: "BTC", Floor: "0.001", MinConvert: "0.00000001", Enabled: true})
	   pg, _ := dust.PutPolicyGen(dustconv.Policy{Asset: "BTC", Floor: "0.001", MinConvert: "0.00000001", Enabled: true})
	   snap := dustconv.PolicyGen{Asset: "BTC", Gen: pg.Gen, Floor: pg.Floor}
	  result, err := dust.ValidateGen(snap)
	  if err != nil {
		  t.Fatalf("ValidateGen unchanged: %v", err)
	  }
	   if result.Gen != snap.Gen {
		   t.Errorf("want same gen %d got %d", snap.Gen, result.Gen)
	  }
  }

   func TestValidateGen_ChangedGen_ReturnsErrGenMismatch(t *testing.T) {
	   dust := dustconv.New()
	   _ = dust.PutPolicy(dustconv.Policy{Asset: "BTC", Floor: "0.001", MinConvert: "0.00000001", Enabled: true})
	  pg1, _ := dust.PutPolicyGen(dustconv.Policy{Asset: "BTC", Floor: "0.001", MinConvert: "0.00000001", Enabled: true})
	  // 改变世代
	  _, _ = dust.PutPolicyGen(dustconv.Policy{Asset: "BTC", Floor: "0.002", MinConvert: "0.00000001", Enabled: true})
	   // 快照持有旧世代 pg1.Gen
	   snap := dustconv.PolicyGen{Asset: "BTC", Gen: pg1.Gen, Floor: pg1.Floor}
	   _, err := dust.ValidateGen(snap)
	   if err == nil {
		   t.Fatal("want ErrGenMismatch when gen has changed")
	  }
	   t.Logf("correctly detected gen change: %v", err)
   }

// ── QuoteSnapshot（dustleak 层）──────────────────────────────────────────────

   func TestTakeSnapshot_Idempotent(t *testing.T) {
	  dust := dustconv.New()
	  _ = dust.PutPolicy(dustconv.Policy{Asset: "BTC", Floor: "0.001", MinConvert: "0.00000001", Enabled: true})
	  _ = dust.PutRate("BTC", "60000")
	  svc := dustleak.New(dust, nil)
	  snap1, err := svc.TakeSnapshot("u1", "BTC", "idem-snap-1", "60000", 50, 1)
	   if err != nil {
		   t.Fatalf("TakeSnapshot 1: %v", err)
	  }
	  snap2, err := svc.TakeSnapshot("u1", "BTC", "idem-snap-1", "60100", 50, 1)
	   if err != nil {
		   t.Fatalf("TakeSnapshot 2: %v", err)
	  }
	  // 幂等：第二次返回同一快照，牌价不变
	  if snap1.ID != snap2.ID {
		  t.Errorf("want same snapshot ID, got %s vs %s", snap1.ID, snap2.ID)
	  }
	   if snap1.RateUSDT != snap2.RateUSDT {
		  t.Errorf("want same rate, got %s vs %s", snap1.RateUSDT, snap2.RateUSDT)
	   }
  }

  func TestTakeSnapshot_DifferentSnapKey_Different(t *testing.T) {
	  dust := dustconv.New()
	   svc := dustleak.New(dust, nil)
	   snap1, _ := svc.TakeSnapshot("u1", "BTC", "idem-a", "60000", 50, 1)
	   snap2, _ := svc.TakeSnapshot("u1", "BTC", "idem-b", "60000", 50, 1)
	   if snap1.ID == snap2.ID {
		  t.Error("different snapKeys should produce different snapshots")
	   }
   }

  func TestBindConvert_UpdatesConvertID(t *testing.T) {
	  dust := dustconv.New()
	   svc := dustleak.New(dust, nil)
	   snap, _ := svc.TakeSnapshot("u1", "BTC", "idem-bind", "60000", 50, 1)
	  snapKey := dustleak.SnapKey("u1", "BTC", "idem-bind")
	   if err := svc.BindConvert(snapKey, "conv-bound-001"); err != nil {
		   t.Fatalf("BindConvert: %v", err)
	   }
	   got,
		  err := svc.SnapshotByKey(snapKey)
	   if err != nil {
		   t.Fatalf("SnapshotByKey: %v", err)
	   }
	  if got.ConvertID != "conv-bound-001" {
		   t.Errorf("want convertID=conv-bound-001 got %s", got.ConvertID)
	  }
	   _ = snap
   }

// ── RecordPolicyGen 持久层幂等 ───────────────────────────────────────────

   func TestRecordPolicyGen_Idempotent(t *testing.T) {
	   dust := dustconv.New()
	  svc := dustleak.New(dust, nil)
	  pg1, err := svc.RecordPolicyGen("BTC", 1, "0.001", "0.00000001", true, "ops-test")
	  if err != nil {
		  t.Fatalf("RecordPolicyGen 1: %v", err)
	  }
	   pg2, err := svc.RecordPolicyGen("BTC", 1, "0.001", "0.00000001", true, "ops-test")
	  if err != nil {
		   t.Fatalf("RecordPolicyGen 2: %v", err)
	  }
	  // 相同 asset+gen 应返回同一记录
	  if pg1.ID != pg2.ID {
		   t.Errorf("want same ID for idempotent gen record, got %s vs %s", pg1.ID, pg2.ID)
	   }
  }

  func TestRecordPolicyGen_DifferentGen_DifferentRecord(t *testing.T) {
	  dust := dustconv.New()
	   svc := dustleak.New(dust, nil)
	  pg1, _ := svc.RecordPolicyGen("ETH", 1, "0.001", "0.000001", true, "ops-test")
	   pg2, _ := svc.RecordPolicyGen("ETH", 2, "0.002", "0.000001", true, "ops-test")
	   if pg1.ID == pg2.ID {
		   t.Error("different gen should produce different records")
	   }
	  if pg2.Floor != "0.002" {
		   t.Errorf("gen 2 floor should be 0.002 got %s", pg2.Floor)
	  }
   }

// ── LatestPolicyGen 返回最高世代 ─────────────────────────────────────────────

  func TestLatestPolicyGen_ReturnsBestGen(t *testing.T) {
	  dust := dustconv.New()
	   svc := dustleak.New(dust, nil)
	  _, _ = svc.RecordPolicyGen("BTC", 1, "0.001", "0.00000001", true, "ops")
	  _, _ = svc.RecordPolicyGen("BTC", 5, "0.005", "0.00000001", true, "ops")
	  _, _ = svc.RecordPolicyGen("BTC", 3, "0.003", "0.00000001", true, "ops")
	  latest, err := svc.LatestPolicyGen("BTC")
	   if err != nil {
		  t.Fatalf("LatestPolicyGen: %v", err)
	  }
	  if latest.Gen != 5 {
		  t.Errorf("want gen=5 got %d", latest.Gen)
	   }
  }

// ── NewQuoteSnapshot 参数校验 ─────────────────────────────────────────────────

  func TestNewQuoteSnapshot_InvalidRate_Error(t *testing.T) {
	   _, err := dustconv.NewQuoteSnapshot("id1", "BTC", "not-a-number", 50, 1)
	   if err == nil {
		  t.Error("want error for invalid rate")
	   }
	   _, err = dustconv.NewQuoteSnapshot("id2", "BTC", "-100", 50, 1)
	   if err == nil {
		  t.Error("want error for negative rate")
	   }
   }

   func TestNewQuoteSnapshot_CapClamped(t *testing.T) {
	  // capBps 超过最大值时应自动截断
	  snap, err := dustconv.NewQuoteSnapshot("id3", "BTC", "60000", 9999, 1)
	  if err != nil {
		   t.Fatalf("NewQuoteSnapshot: %v", err)
	  }
	   if snap.DriftCapBps > dustconv.MaxDriftCapBps {
		   t.Errorf("capBps not clamped: got %d want <= %d", snap.DriftCapBps, dustconv.MaxDriftCapBps)
	   }
  }
