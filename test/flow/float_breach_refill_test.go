package flow

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/floatdesk"
)

// ── 穿仓评级 ──────────────────────────────────────────────────────────────

  func TestBreachGraderTable(t *testing.T) {
	  now := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	   cases := []struct {
		   name      string
		   tank      floatdesk.Tank
		  wantLevel floatdesk.BreachLevel
		  wantBlocked bool
		   wantDeficit string
	   }{
		  {
			   name: "ok_full",
			   tank: floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "10", Min: "2", Target: "8"},
			   wantLevel:   floatdesk.BreachOK,
			  wantBlocked: false,
			  wantDeficit: "",
		   },
		   {
			   name: "warn_below_target",
			   tank: floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "3", Min: "2", Target: "8"},
			   wantLevel:   floatdesk.BreachWarn,
			  wantBlocked: false,
			  wantDeficit: "5",
		  },
		  {
			  name: "crit_below_min",
			  tank: floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "0.5", Min: "2", Target: "8"},
			   wantLevel:   floatdesk.BreachCrit,
			   wantBlocked: true,
			  wantDeficit: "7.5",
		   },
		   {
			   name: "halt_zero_avail",
			   tank: floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "0", Min: "2", Target: "8"},
			  wantLevel: floatdesk.BreachHalt,
			   wantBlocked: true,
			  wantDeficit: "8",
		   },
		  {
			  name:      "tron_crit",
			  tank:      floatdesk.Tank{Chain: "TRON", Native: "TRX", Avail: "50", Min: "200", Target: "800"},
			  wantLevel: floatdesk.BreachCrit,
			   wantBlocked: true,
			   wantDeficit: "750",
		   },
		   {
			  name: "btc_ok",
			   tank: floatdesk.Tank{Chain: "BTC", Native: "BTC", Avail: "0.5", Min: "0.05", Target: "0.4"},
			  wantLevel:   floatdesk.BreachOK,
			  wantBlocked: false,
			  wantDeficit: "",
		  },
		  {
			  name: "btc_warn_exactly_min",
			   tank: floatdesk.Tank{Chain: "BTC", Native: "BTC", Avail: "0.05", Min: "0.05", Target: "0.4"},
			   wantLevel:   floatdesk.BreachWarn,
			   wantBlocked: false,
			  wantDeficit: "0.35",
		  },
		  {
			  name: "sol_warn",
			   tank: floatdesk.Tank{Chain: "SOL", Native: "SOL", Avail: "6", Min: "5", Target: "20"},
			   wantLevel: floatdesk.BreachWarn,
			  wantBlocked: false,
			   wantDeficit: "14",
		   },
	  }
	  for _, tc := range cases {
		   t.Run(tc.name, func(t *testing.T) {
			   grade, err := floatdesk.GradeTank(tc.tank, now)
			   if err != nil {
				  t.Fatalf("GradeTank error: %v", err)
			   }
			  if grade.Level != tc.wantLevel {
				  t.Errorf("level: got %s want %s", grade.Level, tc.wantLevel)
			   }
			  if tc.wantDeficit != "" && grade.Deficit != tc.wantDeficit {
				  t.Errorf("deficit: got %q want %q", grade.Deficit, tc.wantDeficit)
			   }
			  if tc.wantDeficit == "" && grade.Deficit != "" {
				  t.Errorf("deficit: want empty, got %q", grade.Deficit)
			   }
			   // 验证出金门控事件 Blocked 字段与预期一致。
			  evt := floatdesk.CheckEgress([]floatdesk.BreachGrade{grade}, tc.tank.Chain, tc.tank.Native, now)
			  if evt.Blocked != tc.wantBlocked {
				   t.Errorf("egress blocked: got %v want %v", evt.Blocked, tc.wantBlocked)
			  }
		  })
	   }
   }

// ── GradeAll 批量评估 ─────────────────────────────────────────────────────

  func TestGradeAllDefaultTanks(t *testing.T) {
	  tanks := floatdesk.DefaultTanks()
	   now := time.Now().UTC()
	  grades, err := floatdesk.GradeAll(tanks, now)
	   if err != nil {
		   t.Fatalf("GradeAll: %v", err)
	  }
	  if len(grades) == 0 {
		   t.Fatal("GradeAll returned empty grades for default tanks")
	  }
	  for _, g := range grades {
		   if g.Chain == "" || g.Native == "" {
			  t.Errorf("grade missing chain/native: %+v", g)
		   }
		  if g.Label != g.Level.String() {
			  t.Errorf("label mismatch: %q vs %q", g.Label, g.Level.String())
		  }
	   }
  }

  func TestFilterBreach(t *testing.T) {
	   now := time.Now().UTC()
	   tanks := floatdesk.NewTanks()
	  _ = tanks.Put(floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "10", Min: "2", Target: "8"}) // OK
	  _ = tanks.Put(floatdesk.Tank{Chain: "BTC", Native: "BTC", Avail: "0.03", Min: "0.05", Target: "0.4"}) // Crit
	   _ = tanks.Put(floatdesk.Tank{Chain: "TRON", Native: "TRX", Avail: "0", Min: "200", Target: "800"})    // Halt
	   grades, err := floatdesk.GradeAll(tanks, now)
	   if err != nil {
		   t.Fatal(err)
	  }
	  crits := floatdesk.FilterBreach(grades, floatdesk.BreachCrit)
	   if len(crits) != 2 {
		  t.Errorf("want 2 crit-or-higher, got %d", len(crits))
	  }
	   halts := floatdesk.FilterBreach(grades,
		  floatdesk.BreachHalt)
	   if len(halts) != 1 {
		  t.Errorf("want 1 halt, got %d", len(halts))
	  }
	  max := floatdesk.MaxBreachLevel(grades)
	   if max != floatdesk.BreachHalt {
		   t.Errorf("max level: got %s want halt", max)
	  }
  }

// ── 补仓互斥锁 ────────────────────────────────────────────────────────────

  func TestRefillMutexTable(t *testing.T) {
	  cases := []struct {
		   name string
		  run  func(t *testing.T)
	   }{
		  {
			  name: "acquire_release_ok",
			  run: func(t *testing.T) {
				  m := floatdesk.NewRefillMutex()
				   tok, err := m.Acquire("ETH")
				  if err != nil {
					   t.Fatalf("first acquire: %v", err)
				  }
				   if tok.Chain != "ETH" {
					   t.Errorf("chain: %q", tok.Chain)
				   }
				   if err := m.Release(tok); err != nil {
					  t.Fatalf("release: %v", err)
				  }
			   },
		  },
		  {
			  name: "parallel_conflict",
			  run: func(t *testing.T) {
				  m := floatdesk.NewRefillMutex()
				   tok, err := m.Acquire("ETH")
				  if err != nil {
					  t.Fatalf("first acquire: %v", err)
				   }
				   _, err2 := m.Acquire("ETH")
				   if !errors.Is(err2, floatdesk.ErrRefillConflict) {
					  t.Fatalf("second acquire: want ErrRefillConflict, got %v", err2)
				  }
				   // 不同链不冲突。
				   tok2, err3 := m.Acquire("BTC")
				  if err3 != nil {
					  t.Fatalf("btc acquire: %v", err3)
				   }
				   _ = m.Release(tok)
				   _ = m.Release(tok2)
			   },
		   },
		  {
			  name: "stale_token_double_release",
			   run: func(t *testing.T) {
				  m := floatdesk.NewRefillMutex()
				  tok, _ := m.Acquire("ETH")
				   _ = m.Release(tok)
				  // 二次释放同一令牌须返回 ErrRefillStale。
				   if err := m.Release(tok); !errors.Is(err, floatdesk.ErrRefillStale) {
					  t.Fatalf("double release: want ErrRefillStale, got %v", err)
				  }
			  },
		   },
		   {
			   name: "reacquire_after_release",
			   run: func(t *testing.T) {
				  m := floatdesk.NewRefillMutex()
				   tok1, _ := m.Acquire("ETH")
				   _ = m.Release(tok1)
				   tok2, err := m.Acquire("ETH")
				  if err != nil {
					  t.Fatalf("reacquire: %v", err)
				  }
				   if tok2.Generation <= tok1.Generation {
					  t.Errorf("generation should increase: %d <= %d", tok2.Generation, tok1.Generation)
				   }
				  _ = m.Release(tok2)
			   },
		   },
		  {
			   name: "plan_refill_locked_crit",
			   run: func(t *testing.T) {
				   m := floatdesk.NewRefillMutex()
				  tank := floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "0.5", Min: "2", Target: "8"}
				  refill, tok, err := m.PlanRefillLocked(tank)
				   if err != nil {
					   t.Fatalf("PlanRefillLocked: %v", err)
				  }
				   if refill.Amount != "7.5" {
					  t.Errorf("refill amount: got %q want 7.5", refill.Amount)
				  }
				  // 持有令牌时再次尝试应冲突。
				   _, _, err2 := m.PlanRefillLocked(tank)
				  if !errors.Is(err2, floatdesk.ErrRefillConflict) {
					   t.Fatalf("second plan: want ErrRefillConflict, got %v", err2)
				  }
				   if err := m.Release(tok); err != nil {
					  t.Fatalf("release: %v", err)
				  }
			   },
		  },
		   {
			  name: "plan_refill_locked_no_refill_needed",
			  run: func(t *testing.T) {
				   m := floatdesk.NewRefillMutex()
				   // Avail >= Min，不触发补仓，PlanRefillLocked 应返回 ErrPlan 且不持有锁。
				   tank := floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "5", Min: "2", Target: "8"}
				   _, _, err := m.PlanRefillLocked(tank)
				   if !errors.Is(err, floatdesk.ErrPlan) {
					  t.Fatalf("want ErrPlan, got %v", err)
				  }
				  // 锁已自动释放，可以再次 Acquire。
				   tok, err2 := m.Acquire("ETH")
				  if err2 != nil {
					  t.Fatalf("reacquire after failed plan: %v", err2)
				   }
				  _ = m.Release(tok)
			   },
		  },
	  }
	  for _, tc := range cases {
		  t.Run(tc.name, tc.run)
	  }
  }

// ── 出金预检门控 ──────────────────────────────────────────────────────────

   func TestEgressGateTable(t *testing.T) {
	  now := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	  cases := []struct {
		  name  string
		   tank  floatdesk.Tank
		   chain string
		  asset string
		   wantBlocked bool
		  wantAction  floatdesk.EgressAction
	   }{
		  {
			  name: "ok_allow",
			  tank: floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "10", Min: "2", Target: "8"},
			   chain: "ETH", asset: "ETH",
			   wantBlocked: false, wantAction: floatdesk.EgressAllow,
		  },
		  {
			   name: "warn_pass",
			   tank:  floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "3", Min: "2", Target: "8"},
			  chain: "ETH", asset: "ETH",
			   wantBlocked: false, wantAction: floatdesk.EgressWarn,
		   },
		  {
			   name:  "crit_defer",
			  tank:  floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "1", Min: "2", Target: "8"},
			  chain: "ETH", asset: "ETH",
			   wantBlocked: true, wantAction: floatdesk.EgressDefer,
		  },
		   {
			   name:  "halt_block",
			  tank:  floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "0", Min: "2", Target: "8"},
			   chain: "ETH", asset: "ETH",
			   wantBlocked: true, wantAction: floatdesk.EgressHaltBatch,
		   },
	   }
	  for _, tc := range cases {
		   t.Run(tc.name, func(t *testing.T) {
			   grade, err := floatdesk.GradeTank(tc.tank, now)
			   if err != nil {
				   t.Fatalf("GradeTank: %v", err)
			  }
			  evt := floatdesk.CheckEgress([]floatdesk.BreachGrade{grade}, tc.chain, tc.asset, now)
			  if evt.Blocked != tc.wantBlocked {
				   t.Errorf("blocked: got %v want %v", evt.Blocked, tc.wantBlocked)
			  }
			  if evt.Action != tc.wantAction {
				  t.Errorf("action: got %q want %q", evt.Action, tc.wantAction)
			  }
		   })
	  }
   }

  func TestEgressGateFromTanksHaltFlag(t *testing.T) {
	   now := time.Now().UTC()
	  tanks := floatdesk.DefaultTanks()
	   halt := floatdesk.NewFlags()
	   _ = halt.HaltChain("ETH")
	  // ETH 被手动停仓 → Halt。
	  evt, err := floatdesk.CheckEgressFromTanks(tanks, halt, "ETH", "ETH", now)
	  if err != nil {
		  t.Fatalf("CheckEgressFromTanks: %v", err)
	  }
	  if !evt.Blocked || evt.Action != floatdesk.EgressHaltBatch {
		  t.Errorf("expected halt_batch, got blocked=%v action=%q", evt.Blocked, evt.Action)
	   }
	   // BTC 未停仓 → 根据水位判断（DefaultTanks BTC Avail=0.2, Min=0.05, Target=0.4 → Warn）。
	   evt2, err := floatdesk.CheckEgressFromTanks(tanks, halt, "BTC", "BTC", now)
	  if err != nil {
		   t.Fatalf("CheckEgressFromTanks BTC: %v", err)
	   }
	  if evt2.Blocked {
		  t.Errorf("BTC should not be blocked (warn), got blocked=true action=%q", evt2.Action)
	   }
   }

   func TestEgressGateRefreshAndCheck(t *testing.T) {
	   now := time.Now().UTC()
	   gate := floatdesk.NewEgressGate()
	   tanks := floatdesk.NewTanks()
	   _ = tanks.Put(floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "0", Min: "2", Target: "8"})
	  grades, err := floatdesk.GradeAll(tanks, now)
	  if err != nil {
		  t.Fatal(err)
	  }
	  gate.Refresh(grades)
	   evt := gate.Check("ETH", "ETH")
	  if !evt.Blocked || evt.Action != floatdesk.EgressHaltBatch {
		   t.Errorf("want halt_batch, got blocked=%v action=%q", evt.Blocked, evt.Action)
	   }
	  // 刷新为正常水位。
	   tanks2 := floatdesk.NewTanks()
	   _ = tanks2.Put(floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "10", Min: "2", Target: "8"})
	   grades2, _ := floatdesk.GradeAll(tanks2, now)
	  gate.Refresh(grades2)
	   evt2 := gate.Check("ETH", "ETH")
	  if evt2.Blocked {
		   t.Errorf("after refresh to ok, should not be blocked")
	   }
   }

// ── 穿仓证明 ──────────────────────────────────────────────────────────────

  func TestBreachProofTable(t *testing.T) {
	   now := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	  window := time.Hour
	   cases := []struct {
		  name  string
		   grade floatdesk.BreachGrade
		  refill floatdesk.Refill
		  wantVerifyOK bool
		   tamperAmount string
	   }{
		  {
			  name: "crit_proof_valid",
			   grade: floatdesk.BreachGrade{
				   Chain: "ETH", Native: "ETH", Avail: "0.5", Level: floatdesk.BreachCrit,
				  Label: "crit", Deficit: "7.5",
			  },
			  refill: floatdesk.Refill{Chain: "ETH", Native: "ETH", Amount: "7.5"},
			   wantVerifyOK: true,
			   tamperAmount: "9999",
		  },
		   {
			  name: "halt_proof_valid",
			   grade: floatdesk.BreachGrade{
				  Chain: "ETH", Native: "ETH", Avail: "0", Level: floatdesk.BreachHalt,
				  Label: "halt", Deficit: "8",
			   },
			   refill:       floatdesk.Refill{Chain: "ETH", Native: "ETH", Amount: "8"},
			   wantVerifyOK: true,
			   tamperAmount: "1",
		   },
		   {
			  name: "different_chains_different_digest",
			   grade: floatdesk.BreachGrade{
				  Chain: "BTC", Native: "BTC", Avail: "0.01", Level: floatdesk.BreachCrit,
				   Label: "crit", Deficit: "0.39",
			  },
			   refill: floatdesk.Refill{Chain: "BTC", Native: "BTC", Amount: "0.39"},
			  wantVerifyOK: true,
			  tamperAmount: "0.38",
		  },
	   }
	   for _, tc := range cases {
		  t.Run(tc.name, func(t *testing.T) {
			   p, err := floatdesk.BuildBreachProof(tc.grade, tc.refill, window, now)
			   if err != nil {
				   t.Fatalf("BuildBreachProof: %v", err)
			  }
			  if err := p.Verify(); err != nil {
				  t.Fatalf("Verify original: %v", err)
			   }
			  // 篡改 RefillAmount 后 Verify 须失败。
			   tampered := p
			  tampered.RefillAmount = tc.tamperAmount
			   if err := tampered.Verify(); err == nil {
				  t.Fatal("Verify tampered proof: expected error, got nil")
			   }
			  // 篡改 WaterLevel 后 Verify 须失败。
			  tampered2 := p
			   tampered2.WaterLevel = "9999"
			   if err := tampered2.Verify(); err == nil {
				  t.Fatal("Verify tampered water level: expected error, got nil")
			   }
		  })
	  }
  }

   func TestBuildProofSet(t *testing.T) {
	  now := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	  window := time.Hour
	  tanks := floatdesk.NewTanks()
	  _ = tanks.Put(floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "0.5", Min: "2", Target: "8"})
	   _ = tanks.Put(floatdesk.Tank{Chain: "BTC", Native: "BTC", Avail: "0.02", Min: "0.05", Target: "0.4"})
	  _ = tanks.Put(floatdesk.Tank{Chain: "SOL", Native: "SOL", Avail: "25", Min: "5", Target: "20"}) // OK
	   grades, err := floatdesk.GradeAll(tanks, now)
	  if err != nil {
		   t.Fatal(err)
	  }
	   refills := floatdesk.NewRefillMutex()
	  _ = refills
	  // 直接用 ScanRefill 生成补仓计划。
	   rawRefills := tanks.ScanRefill()
	   set, err := floatdesk.BuildProofSet(grades, rawRefills, window, now)
	  if err != nil {
		  t.Fatalf("BuildProofSet: %v", err)
	   }
	   // ETH 和 BTC 触发补仓证明，SOL 是 OK 级不含证明。
	  if len(set.Proofs) != 2 {
		   t.Errorf("want 2 proofs, got %d", len(set.Proofs))
	   }
	   if err := set.VerifyAll(); err != nil {
		   t.Fatalf("VerifyAll: %v", err)
	   }
  }

// ── 黄金文件 ──────────────────────────────────────────────────────────

   func TestFloatBreachGolden(t *testing.T) {
	   dir := filepath.Join("..", "..", "testdata", "golden", "float-breach")
	   ents, err := os.ReadDir(dir)
	   if err != nil {
		  t.Fatal(err)
	  }
	  now := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	   for _, ent := range ents {
		  if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			   continue
		   }
		   t.Run(ent.Name(), func(t *testing.T) {
			   raw,
				  err := os.ReadFile(filepath.Join(dir, ent.Name()))
			   if err != nil {
				  t.Fatal(err)
			   }
			   applyFloatBreachFixture(t, raw, now)
		  })
	  }
   }

type floatBreachFixture struct {
	Comment string          `json:"_comment"`
	Tank    *floatdesk.Tank `json:"tank"`
	Chain   string          `json:"chain"`
	ExpectedLevel   string `json:"expected_level"`
	ExpectedBlocked bool   `json:"expected_blocked"`
	ExpectedAction  string `json:"expected_action"`
	ExpectedDeficit string `json:"expected_deficit"`
	FirstAcquireOK  bool   `json:"first_acquire_ok"`
	SecondAcquireConflict bool `json:"second_acquire_conflict"`
	AfterReleaseAcquireOK bool `json:"after_release_acquire_ok"`
	OriginalVerifyOK bool `json:"original_verify_ok"`
	TamperedVerifyOK bool `json:"tampered_verify_ok"`
	WaterLevel string `json:"water_level"`
	RefillAmount         string `json:"refill_amount"`
	TamperedRefillAmount string `json:"tampered_refill_amount"`
	WindowNS             int64  `json:"window_ns"`
}

  func applyFloatBreachFixture(t *testing.T, raw []byte, now time.Time) {
	   t.Helper()
	  var fx floatBreachFixture
	  if err := json.Unmarshal(raw, &fx); err != nil {
		   t.Fatalf("unmarshal fixture: %v", err)
	   }
	   switch {
	  case fx.Tank != nil && fx.ExpectedLevel != "":
		  // 穿仓评级 + 门控。
		   grade, err := floatdesk.GradeTank(*fx.Tank, now)
		   if err != nil {
			   t.Fatalf("GradeTank: %v", err)
		   }
		  if grade.Label != fx.ExpectedLevel {
			  t.Errorf("level: got %q want %q", grade.Label, fx.ExpectedLevel)
		   }
		   evt := floatdesk.CheckEgress([]floatdesk.BreachGrade{grade}, fx.Tank.Chain, fx.Tank.Native, now)
		   if evt.Blocked != fx.ExpectedBlocked {
			   t.Errorf("blocked: got %v want %v", evt.Blocked, fx.ExpectedBlocked)
		   }
		   if string(evt.Action) != fx.ExpectedAction {
			  t.Errorf("action: got %q want %q", evt.Action, fx.ExpectedAction)
		  }
		  if fx.ExpectedDeficit != "" && grade.Deficit != fx.ExpectedDeficit {
			  t.Errorf("deficit: got %q want %q", grade.Deficit, fx.ExpectedDeficit)
		   }
	   case fx.FirstAcquireOK || fx.SecondAcquireConflict:
		  // 补仓互斥锁。
		  m := floatdesk.NewRefillMutex()
		   tok, err := m.Acquire(fx.Chain)
		  if fx.FirstAcquireOK && err != nil {
			   t.Fatalf("first acquire: %v", err)
		  }
		   if fx.SecondAcquireConflict {
			   _, err2 := m.Acquire(fx.Chain)
			  if !errors.Is(err2, floatdesk.ErrRefillConflict) {
				  t.Fatalf("second acquire: want ErrRefillConflict, got %v", err2)
			  }
		   }
		  if fx.AfterReleaseAcquireOK {
			  _ = m.Release(tok)
			   _, err3 := m.Acquire(fx.Chain)
			   if err3 != nil {
				  t.Fatalf("reacquire after release: %v", err3)
			   }
		   }
	   case fx.WaterLevel != "" && fx.RefillAmount != "":
		   // 穿仓证明。
		   window := time.Duration(fx.WindowNS)
		  grade := floatdesk.BreachGrade{
			  Chain: "ETH", Native: "ETH", Avail: fx.WaterLevel,
			   Level: floatdesk.BreachCrit, Label: "crit",
		  }
		  refill := floatdesk.Refill{Chain: "ETH", Native: "ETH", Amount: fx.RefillAmount}
		  p, err := floatdesk.BuildBreachProof(grade, refill, window, now)
		   if err != nil {
			   t.Fatalf("BuildBreachProof: %v", err)
		   }
		  if fx.OriginalVerifyOK {
			  if err := p.Verify(); err != nil {
				  t.Fatalf("Verify original: %v", err)
			  }
		   }
		  if !fx.TamperedVerifyOK && fx.TamperedRefillAmount != "" {
			  tampered := p
			  tampered.RefillAmount = fx.TamperedRefillAmount
			   if err := tampered.Verify(); err == nil {
				   t.Fatal("tampered proof Verify should fail")
			   }
		   }
	  }
  }
