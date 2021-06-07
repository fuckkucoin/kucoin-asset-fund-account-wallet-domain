package flow

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/dustconv"
	"account-assets.kucoin.net/funding/internal/dustleak"
)

// ── 辅助 ─────────────────────────────────────────────────────────────────────

  func newDustConvWithBTC(floor, rate string) *dustconv.Service {
	  s := dustconv.New()
	  _ = s.PutPolicy(dustconv.Policy{Asset: "BTC", Floor: floor, MinConvert: "0.00000001", Enabled: true})
	  _ = s.PutRate("BTC", rate)
	  return s
   }

// ── ：世代未变时 Convert 正常通过守卫 ─────────────────────────────────────

   func TestFloorGuard_GenUnchanged_Passes(t *testing.T) {
	  dust := newDustConvWithBTC("0.001", "60000")
	  // 初始世代为 0（未调用 PutPolicyGen）
	  fg, err := dustleak.AcquireLock(dust, "BTC")
	   if err != nil {
		  t.Fatalf("AcquireLock: %v", err)
	  }
	   if fg.SnapGen() != 0 {
		   t.Errorf("want gen=0 got %d", fg.SnapGen())
	  }
	   floor, err := fg.ValidateLock("0.0005") // amount < floor=0.001
	   if err != nil {
		   t.Fatalf("ValidateLock: %v", err)
	  }
	   if floor != "0.001" {
		  t.Errorf("want floor=0.001 got %s", floor)
	   }
  }

// ── ：策略变更（floor 放松）→ 守卫拦截，整单失败 ─────────────────────────

  func TestFloorGuard_FloorRelaxed_Rejected(t *testing.T) {
	   dust := newDustConvWithBTC("0.001", "60000")
	  // 调用方在 Convert 请求时快照世代（Gen=1）
	  _, err := dust.PutPolicyGen(dustconv.Policy{Asset: "BTC", Floor: "0.001", MinConvert: "0.00000001", Enabled: true})
	   if err != nil {
		   t.Fatalf("PutPolicyGen: %v", err)
	  }
	  fg, err := dustleak.AcquireLock(dust, "BTC")
	  if err != nil {
		  t.Fatalf("AcquireLock: %v", err)
	   }
	  snapGen := fg.SnapGen()
	  if snapGen != 1 {
		  t.Errorf("want snapGen=1 got %d", snapGen)
	   }
	   // 并发操作：策略 floor 放松（0.001 → 0.0005，更松）
	  _, err = dust.PutPolicyGen(dustconv.Policy{Asset: "BTC", Floor: "0.0005", MinConvert: "0.00000001", Enabled: true})
	  if err != nil {
		   t.Fatalf("PutPolicyGen relax: %v", err)
	  }
	   // Convert 临界区内 ValidateLock：应拒绝（floor 放松，Gen 变化）
	  _, err = fg.ValidateLock("0.0003") // amount=0.0003 < new_floor=0.0005，但快照 floor=0.001
	  if err == nil {
		   t.Fatal("want ErrGenMismatch when floor relaxed, got nil")
	   }
	  t.Logf("correctly rejected: %v", err)
  }

// ── ：策略变更（floor 收紧）→ 守卫使用更严 floor，amount 仍为尘时接着 ────

  func TestFloorGuard_FloorTightened_UsesStricterFloor(t *testing.T) {
	   dust := newDustConvWithBTC("0.001", "60000")
	  _, err := dust.PutPolicyGen(dustconv.Policy{Asset: "BTC", Floor: "0.001", MinConvert: "0.00000001", Enabled: true})
	  if err != nil {
		   t.Fatalf("PutPolicyGen: %v", err)
	  }
	   fg, err := dustleak.AcquireLock(dust, "BTC")
	   if err != nil {
		   t.Fatalf("AcquireLock: %v", err)
	   }
	  // 策略收紧：floor 0.001 → 0.01（更严）
	  _, err = dust.PutPolicyGen(dustconv.Policy{Asset: "BTC", Floor: "0.01", MinConvert: "0.00000001", Enabled: true})
	  if err != nil {
		  t.Fatalf("PutPolicyGen tighten: %v", err)
	   }
	   // amount=0.005，快照 floor=0.001（amount > snap floor，本来是尘），新 floor=0.01（amount < new floor，仍是尘）
	  floor, err := fg.ValidateLock("0.005")
	   if err != nil {
		  t.Fatalf("ValidateLock: %v (want pass with stricter floor)", err)
	  }
	   if floor != "0.01" {
		  t.Errorf("want floor=0.01 got %s", floor)
	  }
   }

// ── ：策略收紧后 amount >= 新 floor → ValidateAmountBelowFloor 拒绝 ─────

   func TestFloorGuard_FloorTightened_AmountNotDust_Rejected(t *testing.T) {
	  dust := newDustConvWithBTC("0.001", "60000")
	   _, _ = dust.PutPolicyGen(dustconv.Policy{Asset: "BTC", Floor: "0.001", MinConvert: "0.00000001", Enabled: true})
	  fg, err := dustleak.AcquireLock(dust, "BTC")
	   if err != nil {
		  t.Fatalf("AcquireLock: %v", err)
	  }
	   // 策略收紧：floor 0.001 → 0.01
	   _, _ = dust.PutPolicyGen(dustconv.Policy{Asset: "BTC", Floor: "0.01", MinConvert: "0.00000001", Enabled: true})
	   // amount=0.009，新 floor=0.01，amount < new floor，ValidateLock 通过（返回更严 floor）
	   floor, err := fg.ValidateLock("0.009")
	  if err != nil {
		  t.Fatalf("want pass for amount=0.009 < new_floor=0.01: %v", err)
	   }
	  if floor != "0.01" {
		  t.Errorf("want floor=0.01 got %s", floor)
	  }
	   // ValidateLock 只守 Gen ，不做 amount<floor 的校验（由 ValidateAmountBelowFloor 负责）。
	  // 当策略收紧后 amount >= 新 floor 时，ValidateLock 返回新 floor，
	  // 调用方须再调用 ValidateAmountBelowFloor 以拒绝非尘金额。
	   fg2, _ := dustleak.AcquireLock(dust, "BTC")
	   floorAfterTighten, err := fg2.ValidateLock("0.01") // gen 已是最新，gen 不变，通过守卫
	  if err != nil {
		   // 此时 gen 已在 2，fg2.snapGen==2，ValidateLock gen 未变，应返回 floor（不报错）
		   t.Logf("ValidateLock gen-change rejection: %v (not expected here)", err)
	   }
	   // 额外的 amount<floor 校验（调用方职责）
	   if floorAfterTighten != "" {
		  err2 := dustleak.ValidateAmountBelowFloor("0.01", floorAfterTighten)
		  if err2 == nil {
			   t.Error("want ErrFloor from ValidateAmountBelowFloor when amount == floor")
		  } else {
			   t.Logf("correctly rejected by ValidateAmountBelowFloor: %v", err2)
		   }
	  }
   }

// ── ：FloorGuardForConvert 一体化调用 ────────────────────────────────────

   func TestFloorGuardForConvert_Passes(t *testing.T) {
	   dust := newDustConvWithBTC("0.001", "60000")
	  _ = s_putPolicyGen(dust, "BTC", "0.001")
	   floor, gen, err := dustleak.FloorGuardForConvert(dust, "BTC", "0.0005")
	   if err != nil {
		   t.Fatalf("FloorGuardForConvert: %v", err)
	   }
	  if floor == "" {
		   t.Error("want non-empty floor")
	  }
	  if gen == 0 {
		  t.Error("want gen > 0")
	  }
	   t.Logf("floor=%s gen=%d", floor, gen)
  }

// ── ：PolicyGenAssertion 断言回归 ────────────────────────────────────────

  func TestPolicyGenAssertion_NoRegression(t *testing.T) {
	   // actualFloor >= snapFloor → ok
	   ok, err := dustleak.PolicyGenAssertion("0.001", "0.001")
	   if err != nil || !ok {
		  t.Errorf("want ok for equal floors: %v %v", ok, err)
	  }
	  ok, err = dustleak.PolicyGenAssertion("0.01", "0.001")
	   if err != nil || !ok {
		   t.Errorf("want ok for tighter actual floor: %v %v", ok, err)
	  }
	  // actualFloor < snapFloor → regression
	  _, err = dustleak.PolicyGenAssertion("0.0005", "0.001")
	   if err == nil {
		   t.Error("want error for floor regression, got nil")
	  }
	   t.Logf("regression detected: %v", err)
  }

// ── ：AssertFloorChain ─────────────────────────────────────────────────

   func TestAssertFloorChain(t *testing.T) {
	  cases := []struct {
		   amount, snap, cur string
		  wantErr           bool
	  }{
		   {"0.0005", "0.001", "0.001", false}, // amount < floor，无回归
		   {"0.0005", "0.001", "0.01", false}, // floor 收紧，amount 仍是尘
		  {"0.005", "0.001", "0.01", false},  // floor 收紧，amount 仍 < new floor
		   {"0.01", "0.001", "0.01", true}, // amount == new floor，不是尘
		   {"0.015", "0.001", "0.01", true},    // amount > new floor，不是尘
		   {"0.0005", "0.001", "0.0005", true}, // floor 放松 → 回归
	   }
	  for _, tc := range cases {
		  err := dustleak.AssertFloorChain(tc.amount, tc.snap, tc.cur)
		  if (err != nil) != tc.wantErr {
			  t.Errorf("AssertFloorChain(amount=%s snap=%s cur=%s) err=%v wantErr=%v",
				   tc.amount, tc.snap, tc.cur, err, tc.wantErr)
		  }
	  }
   }

// ── ：ValidateAmountBelowFloor ────────────────────────────────────────────

  func TestValidateAmountBelowFloor(t *testing.T) {
	  if err := dustleak.ValidateAmountBelowFloor("0.0005", "0.001"); err != nil {
		  t.Errorf("want pass: %v", err)
	   }
	   if err := dustleak.ValidateAmountBelowFloor("0.001", "0.001"); err == nil {
		   t.Error("want ErrFloor for amount == floor")
	   }
	   if err := dustleak.ValidateAmountBelowFloor("0.002", "0.001"); err == nil {
		   t.Error("want ErrFloor for amount > floor")
	   }
  }

// ── ：PolicyGenLabel ──────────────────────────────────────────────────────

  func TestPolicyGenLabel(t *testing.T) {
	   label := dustleak.PolicyGenLabel("btc", 42)
	  if label != "BTC#42" {
		  t.Errorf("got %q want BTC#42", label)
	  }
  }

// ── 辅助内部函数 ──────────────────────────────────────────────────────────────

   func s_putPolicyGen(dust *dustconv.Service, asset, floor string) uint64 {
	   pg, err := dust.PutPolicyGen(dustconv.Policy{Asset: asset, Floor: floor, MinConvert: "0.00000001", Enabled: true})
	   if err != nil {
		  panic(err)
	   }
	   return pg.Gen
  }
