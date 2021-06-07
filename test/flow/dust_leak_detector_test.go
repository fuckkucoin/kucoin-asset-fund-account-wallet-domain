package flow

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/dustconv"
	"account-assets.kucoin.net/funding/internal/dustleak"
)

// ── 辅助 ─────────────────────────────────────────────────────────────────────

   func newDetectorSuite(t *testing.T) (*dustleak.Service, *dustleak.Detector) {
	  t.Helper()
	   dust := dustconv.New()
	  _ = dust.PutPolicy(dustconv.Policy{Asset: "BTC", Floor: "0.001", MinConvert: "0.00000001", Enabled: true})
	   _ = dust.PutRate("BTC", "60000")
	  svc := dustleak.New(dust, nil)
	   driftReg := dustleak.NewDriftCapRegistry(dustleak.DefaultDriftConfig)
	  detector := dustleak.NewDetector(svc,
		  dust, driftReg)
	  return svc, detector
  }

  func makeConvertRecord(id, userID, asset, amount, usdt, rate, idemKey string, at time.Time) dustleak.ConvertRecord {
	   return dustleak.ConvertRecord{
		   ID: id,
		   UserID: userID,
		  Asset: asset,
		   Amount: amount,
		   USDT:   usdt,
		   Rate:   rate,
		   At:     at,
		   IdemKey: idemKey,
	  }
   }

// ── ：OverFloor 泄漏检出 ─────────────────────────────────────────────────

  func TestDetector_OverFloor_Opens_LeakCase(t *testing.T) {
	   svc,
		  detector := newDetectorSuite(t)
	  // 注入 amount=0.002 >= floor=0.001 的违规 Convert
	   rec := makeConvertRecord("conv-001", "user1", "BTC", "0.002", "120", "60000", "idem-001", time.Now().UTC())
	  // 为 rec 设置快照（amount > floor 不需要快照就能检出）
	  result := detector.ScanConverts([]dustleak.ConvertRecord{rec}, dustleak.DetectorSourceAPI)
	   if result.Detected == 0 {
		   t.Fatal("want at least 1 leak detected for over-floor convert")
	  }
	   if len(result.Cases) == 0 {
		   t.Fatal("want at least 1 LeakCase")
	  }
	  lc := result.Cases[0]
	   if lc.Kind != dustleak.LeakKindOverFloor {
		   t.Errorf("want kind=over_floor got %s", lc.Kind)
	   }
	   if lc.Status != dustleak.LeakStatusDetected {
		  t.Errorf("want status=detected got %s", lc.Status)
	  }
	   if lc.UserID != "user1" || lc.Asset != "BTC" {
		   t.Errorf("wrong user/asset: %s/%s", lc.UserID, lc.Asset)
	   }
	   // 挡兑闸验证：同用户同资产现在应 blocked
	  gateStatus := svc.GateStatusOf("user1", "BTC")
	  if gateStatus != dustleak.GateStatusBlocked {
		  t.Errorf("want gate blocked, got %s", gateStatus)
	  }
   }

// ── ：DriftOverpay 泄漏检出 ──────────────────────────────────────────────

  func TestDetector_DriftOverpay_Opens_LeakCase(t *testing.T) {
	   svc, detector := newDetectorSuite(t)
	  // 先为 Convert 拍摄牌价快照（snapRate=60000）
	  _, err := svc.TakeSnapshot("user2", "BTC", "idem-002", "60000", 50, 1)
	   if err != nil {
		   t.Fatalf("TakeSnapshot: %v", err)
	   }
	   // 注入 amount=0.0005 BTC，使用 rate=60600（drift=600/60000*10000=100bps > 50bps cap）
	   rec := makeConvertRecord("conv-002", "user2", "BTC", "0.0005", "30.3", "60600", "idem-002", time.Now().UTC())
	   result := detector.ScanConverts([]dustleak.ConvertRecord{rec}, dustleak.DetectorSourceWorker)
	   if result.Detected == 0 {
		  t.Fatal("want drift overpay leak detected")
	  }
	   var found bool
	  for _, lc := range result.Cases {
		   if lc.Kind == dustleak.LeakKindDriftOverpay {
			  found = true
			  if lc.UserID != "user2" {
				   t.Errorf("want userID=user2 got %s", lc.UserID)
			   }
			  break
		  }
	  }
	  if !found {
		  t.Fatal("want LeakCase with kind=drift_overpay")
	   }
	   // 挡兑闸
	   if svc.GateStatusOf("user2", "BTC") != dustleak.GateStatusBlocked {
		  t.Error("want gate blocked after drift overpay leak")
	   }
  }

// ── ：DoubleDust 双花检出 ─────────────────────────────────────────────────

  func TestDetector_DoubleDust_Opens_LeakCase_For_Duplicates(t *testing.T) {
	   _, detector := newDetectorSuite(t)
	   now := time.Now().UTC()
	  records := []dustleak.ConvertRecord{
		   makeConvertRecord("conv-101", "user3", "BTC", "0.0005", "30", "60000", "idem-101", now),
		  makeConvertRecord("conv-102", "user3", "BTC", "0.0004", "24", "60000", "idem-102", now.Add(30*time.Second)),
		   makeConvertRecord("conv-103", "user3", "BTC", "0.0003", "18", "60000", "idem-103", now.Add(60*time.Second)),
	   }
	   result := detector.ScanConverts(records, dustleak.DetectorSourceWorker)
	   // 首笔 conv-101 合法，conv-102 和 conv-103 均应打 DoubleDust LeakCase
	  doubleDustCount := 0
	   for _, lc := range result.Cases {
		  if lc.Kind == dustleak.LeakKindDoubleDust {
			  doubleDustCount++
		   }
	  }
	   if doubleDustCount < 2 {
		  t.Errorf("want 2 double_dust cases, got %d total=%d", doubleDustCount, result.Detected)
	   }
  }

// ── ：幂等扫描 ────────────────────────────────────────────────────────────

  func TestDetector_Idempotent_SameConvert_OnlyOneLeakCase(t *testing.T) {
	   _, detector := newDetectorSuite(t)
	  rec := makeConvertRecord("conv-dup", "user4", "BTC", "0.002", "120", "60000", "idem-dup", time.Now().UTC())
	   result1 := detector.ScanConverts([]dustleak.ConvertRecord{rec}, dustleak.DetectorSourceWorker)
	  result2 := detector.ScanConverts([]dustleak.ConvertRecord{rec}, dustleak.DetectorSourceWorker)
	   // 第二次扫描同一笔 Convert 不产生新 LeakCase
	   if result2.Detected > 0 {
		   t.Errorf("second scan should not detect additional leaks, got %d", result2.Detected)
	  }
	  // 第一次应检出
	  if result1.Detected == 0 {
		  t.Error("first scan should detect over-floor leak")
	   }
   }

// ── ：无泄漏的核验 Convert 不产出 LeakCase ─────────────────────────────────

   func TestDetector_CompliantConvert_NoLeak(t *testing.T) {
	   _, detector := newDetectorSuite(t)
	   // amount=0.0005 < floor=0.001，rate=60000（与快照相同）
	  rec := makeConvertRecord("conv-ok", "user5", "BTC", "0.0005", "30", "60000", "idem-ok", time.Now().UTC())
	  result := detector.ScanConverts([]dustleak.ConvertRecord{rec}, dustleak.DetectorSourceWorker)
	  if result.Detected > 0 {
		   t.Errorf("compliant convert should not produce LeakCase, got %d", result.Detected)
	   }
  }

// ── ：LeakCase 状态转换 ───────────────────────────────────────────────────

   func TestLeakCase_Transition_DetectedToRollingBack(t *testing.T) {
	   svc, detector := newDetectorSuite(t)
	   rec := makeConvertRecord("conv-trans", "user6", "BTC", "0.005", "300", "60000", "idem-trans", time.Now().UTC())
	  result := detector.ScanConverts([]dustleak.ConvertRecord{rec}, dustleak.DetectorSourceAPI)
	   if result.Detected == 0 || len(result.Cases) == 0 {
		   t.Fatal("need a leak case to test transition")
	  }
	  leakID := result.Cases[0].ID
	  lc, err := svc.TransitionLeak(leakID, dustleak.LeakStatusDetected, dustleak.LeakStatusRollingBack, "ops-test")
	   if err != nil {
		   t.Fatalf("TransitionLeak: %v", err)
	  }
	  if lc.Status != dustleak.LeakStatusRollingBack {
		   t.Errorf("want rolling_back got %s", lc.Status)
	   }
   }

// ── ：挡兑闸强制操作 ──────────────────────────────────────────────────────

  func TestForceBlock_ForceRelease(t *testing.T) {
	  svc, _ := newDetectorSuite(t)
	   // 强制挡兑（需 dualReqID）
	   gate, err := svc.ForceBlock("user7", "BTC", "fake-leak-id", "ops-maker", "dual-req-001")
	  if err != nil {
		  t.Fatalf("ForceBlock: %v", err)
	  }
	  if gate.Status != dustleak.GateStatusBlocked {
		  t.Errorf("want blocked got %s", gate.Status)
	  }
	   // 验证挡兑
	  if svc.GateStatusOf("user7", "BTC") != dustleak.GateStatusBlocked {
		   t.Error("want blocked gate")
	  }
	   // 强制解挡（需 dualReqID）
	   if err := svc.ForceRelease("user7", "BTC", "ops-checker", "dual-req-002"); err != nil {
		  t.Fatalf("ForceRelease: %v", err)
	  }
	   if svc.GateStatusOf("user7", "BTC") != dustleak.GateStatusOpen {
		  t.Error("want open gate after release")
	  }
   }

// ── ：强制挡兑缺 dualReqID → 拒绝 ────────────────────────────────────────

  func TestForceBlock_NoDualReqID_Rejected(t *testing.T) {
	   svc, _ := newDetectorSuite(t)
	   _, err := svc.ForceBlock("user8", "BTC", "", "ops-maker", "") // dualReqID 为空
	  if err == nil {
		  t.Error("want ErrDual when dualReqID empty")
	  }
  }

// ── ：ListOpenLeaks ────────────────────────────────────────────────────────

   func TestListOpenLeaks(t *testing.T) {
	  svc, detector := newDetectorSuite(t)
	   rec1 := makeConvertRecord("conv-list-1", "user9", "BTC", "0.005", "300", "60000", "idem-list-1", time.Now().UTC())
	   rec2 := makeConvertRecord("conv-list-2", "user9", "ETH", "0.002", "4", "2000", "idem-list-2", time.Now().UTC())
	   _ = detector.ScanConverts([]dustleak.ConvertRecord{rec1, rec2}, dustleak.DetectorSourceAPI)
	   leaks, err := svc.ListOpenLeaks("user9", "")
	  if err != nil {
		  t.Fatalf("ListOpenLeaks: %v", err)
	  }
	  if len(leaks) == 0 {
		   t.Error("want some open leaks for user9")
	   }
	  // 只查 BTC
	   btcLeaks, _ := svc.ListOpenLeaks("user9", "BTC")
	   for _, lc := range btcLeaks {
		   if lc.Asset != "BTC" {
			   t.Errorf("filter by BTC: unexpected asset %s", lc.Asset)
		  }
	  }
  }
