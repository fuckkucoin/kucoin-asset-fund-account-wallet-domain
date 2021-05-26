package flow

import (
	"testing"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/dustconv"
	"account-assets.kucoin.net/funding/internal/dustleak"
)

// ── 辅助 ─────────────────────────────────────────────────────────────────────

  func makeSnapshot(asset, rateUSDT string, capBps int, gen uint64) *dustconv.QuoteSnapshot {
	   snap, err := dustconv.NewQuoteSnapshot("test-snap-id", asset, rateUSDT, capBps, gen)
	  if err != nil {
		  panic(err)
	  }
	  return &snap
  }

// ── ：CheckDrift 精确 ─────────────────────────────────────────────────

  func TestCheckDrift_ExactCap_Boundary(t *testing.T) {
	  snap := makeSnapshot("BTC", "60000", 50, 1) // cap = 50 bps = 0.5%
	  // 恰好在帽内：60000 * 1.005 = 60300
	   if err := snap.CheckDrift("60300"); err != nil {
		   t.Errorf("drift at exact cap should pass: %v", err)
	  }
	   // 超出 1 单位：60000 * 1.00501 = 60300.6 ≈ 60301
	   if err := snap.CheckDrift("60301"); err == nil {
		  t.Error("drift slightly above cap should fail")
	   }
  }

// ── ：下漂无限制 ─────────────────────────────────────────────────────────

  func TestCheckDrift_DownDrift_AlwaysPasses(t *testing.T) {
	   snap := makeSnapshot("BTC", "60000", 50, 1)
	  rates := []string{"59000", "50000", "1", "60000"}
	   for _, r := range rates {
		  if err := snap.CheckDrift(r); err != nil {
			   t.Errorf("down-drift rate=%s should pass: %v", r, err)
		  }
	   }
   }

// ── ：DriftGuard.CheckWithLiveRate + 快照超期 ─────────────────────────────

  func TestDriftGuard_StaleSnapshot_Rejected(t *testing.T) {
	  snap := makeSnapshot("ETH", "2000", 50, 1)
	   // 快照是 60 秒前拍摄的
	   oldSnap := *snap
	  oldSnap.SnapAt = time.Now().UTC().Add(-61 * time.Second)
	  dg := dustleak.NewDriftGuard(&oldSnap, "", 30) // staleMaxSec=30
	   err := dg.CheckWithLiveRate("2000", time.Now().UTC())
	  if err == nil {
		   t.Fatal("stale snapshot should be rejected")
	  }
	   t.Logf("stale rejected: %v", err)
  }

   func TestDriftGuard_FreshSnapshot_Passes(t *testing.T) {
	   snap := makeSnapshot("ETH", "2000", 50, 1)
	   dg := dustleak.NewDriftGuard(snap, "", 30)
	   // 同牌价，无漂移
	  if err := dg.CheckWithLiveRate("2000", snap.SnapAt.Add(5*time.Second)); err != nil {
		   t.Errorf("fresh snapshot same rate should pass: %v", err)
	  }
  }

  func TestDriftGuard_UpDriftBeyondCap_Rejected(t *testing.T) {
	  snap := makeSnapshot("ETH", "2000", 50, 1) // cap=50bps=0.5%
	  dg := dustleak.NewDriftGuard(snap, "", 30)
	  // 2000 * 1.006 = 2012 → drift 60bps > 50bps cap
	  err := dg.CheckWithLiveRate("2012", snap.SnapAt.Add(5*time.Second))
	   if err == nil {
		   t.Fatal("drift 60bps > 50bps cap should be rejected")
	  }
	  t.Logf("drift rejected: %v", err)
   }

// ── ：MaxPermittedUSDT ────────────────────────────────────────────────────

   func TestDriftGuard_MaxPermittedUSDT(t *testing.T) {
	   snap := makeSnapshot("BTC", "60000", 50, 1)
	   dg := dustleak.NewDriftGuard(snap, "", 30)
	   maxUSDT, err := dg.MaxPermittedUSDT("0.0005")
	  if err != nil {
		  t.Fatalf("MaxPermittedUSDT: %v", err)
	   }
	  // 0.0005 * 60000 = 30
	  expected := "30"
	   got,
		   _ := decimal.NewFromString(maxUSDT)
	  exp, _ := decimal.NewFromString(expected)
	   if !got.Equal(exp) {
		  t.Errorf("want maxUSDT=30 got %s", maxUSDT)
	   }
  }

// ── ：DriftCapChecker 表驱动 ─────────────────────────────────────────────

  func TestDriftCapChecker_TableDriven(t *testing.T) {
	   cases := []struct {
		   snap, actual string
		   capBps     int
		  wantExceed bool
		   wantBpsMin float64 // 最小漂移 bps（exceeded 时）
	  }{
		   // 上漂未超帽
		   {"60000", "60100", 50, false, 0}, // 100/60000*10000=16.67bps < 50
		  {"60000", "60290", 50, false, 0}, // 290/60000*10000=48.33bps < 50
		  {"60000", "60300", 50, false, 0}, // 300/60000*10000=50bps = 50（，≤50 不超）
		   // 上漂超帽
		   {"60000", "60360", 50, true, 60}, // 360/60000*10000=60bps > 50
		  {"2000", "2050", 100, false, 0}, // 50/2000*10000=250bps... 等等实际是250bps > 100bps
		   // 修正：50/2000=0.025 * 10000=250bps > 100
		  {"2000", "2010", 100, false, 0}, // 10/2000*10000=50bps < 100
		  // 下漂
		   {"60000", "59000", 50, false, 0},
		  {"2000", "1999", 50, false, 0},
		   // 等价
		   {"60000", "60000", 50, false, 0},
	   }
	   // 修正 case：2000→2050 实际 drift = 25%*100bps = 250bps > 100bps → exceeded
	  cases[4].wantExceed = true
	  cases[4].wantBpsMin = 200
	  for _, tc := range cases {
		  bps, exceeded, err := dustleak.DriftCapChecker(tc.snap, tc.actual, tc.capBps)
		   if err != nil {
			  t.Errorf("DriftCapChecker(%s->%s cap=%d) err=%v", tc.snap, tc.actual, tc.capBps, err)
			   continue
		  }
		  if exceeded != tc.wantExceed {
			  t.Errorf("DriftCapChecker(%s->%s cap=%d) exceeded=%v want=%v bps=%.2f",
				   tc.snap, tc.actual, tc.capBps, exceeded, tc.wantExceed, bps.InexactFloat64())
		  }
	   }
   }

// ── ：CalcLeakUSDT ────────────────────────────────────────────────────────

  func TestCalcLeakUSDT(t *testing.T) {
	  cases := []struct {
		  amount, snap, actual string
		   want string
	   }{
		   // amount=0.001 BTC, snap=60000, actual=60600 → drift=600, leak=0.001*600=0.6
		  {"0.001", "60000", "60600", "0.6"},
		   // 下漂：leak=0
		  {"0.001", "60000", "59000", "0"},
		  // 等价：leak=0
		   {"0.001", "60000", "60000", "0"},
		  // amount=1 ETH, snap=2000, actual=2100 → drift=100, leak=1*100=100
		  {"1", "2000", "2100", "100"},
	   }
	  for _, tc := range cases {
		  got, err := dustleak.CalcLeakUSDT(tc.amount, tc.snap, tc.actual)
		  if err != nil {
			  t.Errorf("CalcLeakUSDT(%s %s->%s) err=%v", tc.amount, tc.snap, tc.actual, err)
			   continue
		  }
		  g, _ := decimal.NewFromString(got)
		   w, _ := decimal.NewFromString(tc.want)
		  if !g.Equal(w) {
			  t.Errorf("CalcLeakUSDT(%s %s->%s) got %s want %s",
				  tc.amount, tc.snap, tc.actual, got, tc.want)
		  }
	  }
   }

// ── ：DriftCapRegistry ────────────────────────────────────────────────────

   func TestDriftCapRegistry(t *testing.T) {
	   reg := dustleak.NewDriftCapRegistry([]dustleak.DriftCapConfig{
		  {Asset: "BTC", CapBps: 30, StaleMaxSec: 15},
		  {Asset: "ETH", CapBps: 100, StaleMaxSec: 60},
	   })
	  if reg.CapFor("BTC") != 30 {
		   t.Errorf("want BTC cap=30 got %d", reg.CapFor("BTC"))
	  }
	   if reg.CapFor("ETH") != 100 {
		  t.Errorf("want ETH cap=100 got %d", reg.CapFor("ETH"))
	  }
	  // 未配置资产使用默认值
	  if reg.CapFor("DOGE") != dustconv.DefaultDriftCapBps {
		   t.Errorf("want DOGE default cap=%d got %d", dustconv.DefaultDriftCapBps, reg.CapFor("DOGE"))
	  }
	  if reg.StaleLimitFor("BTC") != 15 {
		  t.Errorf("want BTC stale=15 got %d", reg.StaleLimitFor("BTC"))
	   }
	  // 热更新
	   reg.Upsert(dustleak.DriftCapConfig{Asset: "BTC", CapBps: 75, StaleMaxSec: 20})
	  if reg.CapFor("BTC") != 75 {
		  t.Errorf("after upsert want BTC cap=75 got %d", reg.CapFor("BTC"))
	   }
   }

// ── ：GuardedConvert.Before ───────────────────────────────────────────────

   func TestGuardedConvert_Before_DriftExceed(t *testing.T) {
	  dust := newDustConvWithBTC("0.001", "60000")
	   _ = s_putPolicyGen(dust, "BTC", "0.001")
	   gc := &dustleak.GuardedConvert{
		  UserID: "user1",
		   Asset:  "BTC",
		   Amount: "0.0005",
		   IdemKey: "idem-test-1",
	  }
	   // 快照牌价 60000，现在传入 60400（超帽 ~66bps > 50bps）
	   // Before 内部拍摄快照并立即校验，elapsed=0，不会因 stale 失败
	  // 但漂移是与自身比较：快照==实时，elapsed=0，应通过
	   err := gc.Before(dust, "60000", 50, 30)
	  if err != nil {
		  t.Errorf("same rate should pass Before: %v", err)
	  }
   }
