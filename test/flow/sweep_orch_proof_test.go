package flow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/airgap"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/hotbound"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/signpipe"
	"account-assets.kucoin.net/funding/internal/sweeppol"
	"account-assets.kucoin.net/funding/internal/sweeprun"
	"account-assets.kucoin.net/funding/internal/treasury"
	"github.com/gin-gonic/gin"
)

  func TestSweepPlanEdges(t *testing.T) {
	  for _, e := range sweeprun.DenyPlanInventory() {
		  if sweeprun.AllowPlan(e[0], e[1]) {
			   t.Fatalf("plan deny %s → %s", e[0], e[1])
		  }
	   }
	   for _, e := range sweeprun.DenyLegInventory() {
		  if sweeprun.AllowLeg(e[0], e[1]) {
			   t.Fatalf("leg deny %s → %s", e[0], e[1])
		  }
	   }
	  if sweeprun.AllowLeg(sweeprun.LegProofOpen, sweeprun.LegCeremonyWait) != true {
		  t.Fatal("cold wait edge")
	   }
	  if err := sweeprun.ColdMustWait(true, sweeprun.LegProofOpen, sweeprun.LegReady); err == nil {
		  t.Fatal("cold skipped ceremony")
	   }
   }

  func TestSweepOrchFixtures(t *testing.T) {
	   dir := filepath.Join("..", "..", "docs", "fixtures", "sweep-orch")
	  ents, err := os.ReadDir(dir)
	   if err != nil {
		  t.Fatal(err)
	   }
	   if len(ents) < 5 {
		   t.Fatalf("fixtures %d", len(ents))
	   }
	  for _, ent := range ents {
		  if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			   continue
		   }
		   t.Run(ent.Name(), func(t *testing.T) {
			  raw, err := os.ReadFile(filepath.Join(dir, ent.Name()))
			  if err != nil {
				   t.Fatal(err)
			   }
			  var fx orchFixture
			   if err := json.Unmarshal(raw, &fx); err != nil {
				   t.Fatal(err)
			   }
			  applyOrchFixture(t, fx)
		   })
	   }
  }

   func TestSweepColdRefAndCompensate(t *testing.T) {
	   ctx := context.Background()
	   cur := time.Date(2022, 6, 18, 10, 0, 0, 0, time.UTC)
	  o := sweeprun.NewOrch().WithClock(func() time.Time { return cur })
	  if err := o.Seed("USDT", "900", "500", "5000"); err != nil {
		   t.Fatal(err)
	   }
	   if err := o.PutRule(usdtRule(false)); err != nil {
		  t.Fatal(err)
	  }
	   cer, gap, pipe := newRefs(t)
	   sweeprun.BindPlatformRefs(o,
		  cer, gap, pipe)
	   got, err := o.Draft(ctx, sweeprun.DraftReq{Actor: "alice", Idem: "idem-timeout", Chain: "ETH", Asset: "USDT"})
	  if err != nil {
		  t.Fatal(err)
	  }
	  if got.State != sweeprun.PlanStateGated {
		  t.Fatalf("state %s", got.State)
	  }
	  cur = cur.Add(16 * time.Minute)
	  if _, err := o.Tick(ctx, 4); err != nil {
		  t.Fatal(err)
	   }
	  got, err = o.Get(got.ID)
	   if err != nil {
		  t.Fatal(err)
	   }
	  if got.State != sweeprun.PlanStateRejected || got.Code != "deny_cold_ref" {
		  t.Fatalf("timeout %+v", got.State)
	  }
	   bal, err := o.Balance("USDT")
	  if err != nil {
		   t.Fatal(err)
	  }
	  if bal.Hot != "900" || bal.Warm != "500" || bal.Cold != "5000" {
		  t.Fatalf("desk moved %+v", bal)
	   }
	  cur = time.Date(2022, 6, 18, 11, 0, 0, 0, time.UTC)
	  o2 := sweeprun.NewOrch().WithClock(func() time.Time { return cur })
	  if err := o2.Seed("USDT", "900", "500", "5000"); err != nil {
		  t.Fatal(err)
	   }
	  if err := o2.PutRule(usdtRule(false)); err != nil {
		   t.Fatal(err)
	   }
	  cer, gap, pipe = newRefs(t)
	  sweeprun.BindPlatformRefs(o2, cer, gap, pipe)
	   plan, err := o2.Draft(ctx, sweeprun.DraftReq{Actor: "alice", Idem: "idem-ok", Chain: "ETH", Asset: "USDT"})
	   if err != nil {
		  t.Fatal(err)
	   }
	   cold := coldLeg(t, plan)
	   if _, err := o2.AttachRefs(ctx, plan.ID, cold.ID, "alice", sweeprun.RefBind{}); err == nil {
		  t.Fatal("empty ref")
	   }
	   if _, err := o2.AttachRefs(ctx, plan.ID, hotLeg(t, plan).ID, "alice", fakeRef()); err == nil {
		   t.Fatal("ref on warm leg")
	  }
	   bindLeg(t, o2, cer, gap, pipe, plan, cold)
	  drive(t, o2, plan.ID, func(p sweeprun.SweepPlan) bool { return p.State == sweeprun.PlanStateSettled })
	  if err := o2.Verify(plan.ID); err != nil {
		  t.Fatal(err)
	  }
	  bal, err = o2.Balance("USDT")
	   if err != nil {
		  t.Fatal(err)
	   }
	  if bal.Hot != "800" || bal.Warm != "200" || bal.Cold != "5400" {
		   t.Fatalf("settled desk %+v", bal)
	  }
	   links, err := o2.Links(plan.ID)
	   if err != nil {
		  t.Fatal(err)
	  }
	   sawReady := false
	   for _, lk := range links {
		  if lk.State != string(sweeprun.LegReady) {
			   continue
		   }
		  if lk.From != "cold" && lk.To != "cold" {
			   continue
		   }
		  sawReady = true
		  if lk.CeremonyPhase != "activate" {
			   t.Fatalf("phase %s", lk.CeremonyPhase)
		  }
	  }
	  if !sawReady {
		   t.Fatal("no cold ready link")
	   }
	   blank := append([]sweeprun.ProofLink{}, links...)
	  for i := range blank {
		  if blank[i].State == string(sweeprun.LegReady) && (blank[i].From == "cold" || blank[i].To == "cold") {
			  blank[i].CeremonyPhase = ""
			  break
		  }
	   }
	   if err := sweeprun.VerifyLinks(blank); err == nil {
		  t.Fatal("stripped phase accepted")
	  }
	   links[1].Hash = strings.Repeat("ab", 32)
	   if err := sweeprun.VerifyLinks(links); err == nil {
		  t.Fatal("tamper accepted")
	  }
	  cur = time.Date(2022, 6, 18, 12, 0, 0, 0, time.UTC)
	  o3 := sweeprun.NewOrch().WithClock(func() time.Time { return cur })
	  if err := o3.Seed("USDT", "900", "500", "5000"); err != nil {
		   t.Fatal(err)
	   }
	  if err := o3.PutRule(usdtRule(false)); err != nil {
		  t.Fatal(err)
	  }
	   cer, gap, pipe = newRefs(t)
	  sweeprun.BindPlatformRefs(o3, cer, gap, pipe)
	   plan, err = o3.Draft(ctx, sweeprun.DraftReq{Actor: "alice", Idem: "idem-comp", Chain: "ETH", Asset: "USDT"})
	   if err != nil {
		   t.Fatal(err)
	   }
	   bindLeg(t, o3, cer, gap, pipe, plan, coldLeg(t, plan))
	  drive(t, o3, plan.ID, func(p sweeprun.SweepPlan) bool {
		   if p.State == sweeprun.PlanStatePartial {
			  o3.ArmFault("drill")
		   }
		  if p.State == sweeprun.PlanStateCompensating {
			  for _, lg := range p.Legs {
				   if lg.Compensate && lg.Cold && !lg.Refs.Bound() {
					  bindLeg(t, o3, cer, gap, pipe, p, lg)
				   }
			  }
		  }
		  return p.State == sweeprun.PlanStateCompensated
	   })
	   if err := o3.Verify(plan.ID); err != nil {
		   t.Fatal(err)
	   }
	   bal, err = o3.Balance("USDT")
	   if err != nil {
		  t.Fatal(err)
	  }
	  if bal.Hot != "900" || bal.Warm != "500" || bal.Cold != "5000" {
		   t.Fatalf("comp desk %+v", bal)
	  }
	   again, err := o3.Draft(ctx, sweeprun.DraftReq{Actor: "alice", Idem: "idem-comp", Chain: "ETH", Asset: "USDT"})
	   if err != nil || again.ID != plan.ID {
		   t.Fatalf("replay %+v %v", again.ID, err)
	   }
	  if _, err := o3.Draft(ctx, sweeprun.DraftReq{Actor: "alice", Idem: "idem-comp", Chain: "ETH", Asset: "BTC"}); !errors.Is(err, sweeprun.ErrIdem) {
		  t.Fatalf("conflict %v", err)
	  }
   }

   func TestSweepAgreeGate(t *testing.T) {
	  rule := usdtRule(false)
	   high := treasury.Bal{Asset: "USDT", Hot: "900", Warm: "100", Cold: "5000"}
	   if code, err := sweeprun.AgreeGate("USDT", high, rule, []string{"hot_warm"}); err != nil || code != "" {
		   t.Fatalf("sweep %s %v", code, err)
	   }
	   if _, err := sweeprun.AgreeGate("USDT", high, rule, []string{"warm_hot"}); err == nil {
		  t.Fatal("replenish while hot high")
	   }
	  in := treasury.Bal{Asset: "USDT", Hot: "200", Warm: "100", Cold: "5000"}
	   if _, err := sweeprun.AgreeGate("USDT", in, rule, []string{"hot_warm"}); err == nil {
		  t.Fatal("in band sweep")
	  }
  }

   func TestSweepGateBeforeMove(t *testing.T) {
	  ctx := context.Background()
	  cur := time.Date(2022, 6, 20, 10, 0, 0, 0, time.UTC)
	  o := sweeprun.NewOrch().WithClock(func() time.Time { return cur })
	  if err := o.Seed("USDT", "900", "100", "5000"); err != nil {
		  t.Fatal(err)
	  }
	   if err := o.PutRule(usdtRule(false)); err != nil {
		   t.Fatal(err)
	  }
	  plan, err := o.Draft(ctx, sweeprun.DraftReq{Actor: "alice", Idem: "idem-gate", Chain: "ETH", Asset: "USDT"})
	  if err != nil {
		  t.Fatal(err)
	  }
	   if plan.State != sweeprun.PlanStateArmed {
		  t.Fatalf("state %s", plan.State)
	   }
	   if len(plan.Legs) != 1 || plan.Legs[0].Kind != "hot_warm" || plan.Legs[0].Cold {
		   t.Fatalf("legs %+v", plan.Legs)
	  }
	   if err := o.Seed("USDT", "200", "100", "5000"); err != nil {
		  t.Fatal(err)
	  }
	   drive(t, o, plan.ID, func(p sweeprun.SweepPlan) bool { return p.State == sweeprun.PlanStateRejected })
	   got, err := o.Get(plan.ID)
	  if err != nil {
		  t.Fatal(err)
	  }
	  if got.Code != "leg_failed" {
		   t.Fatalf("code %s %s", got.Code, got.Reason)
	  }
	  bal, err := o.Balance("USDT")
	  if err != nil {
		  t.Fatal(err)
	  }
	   if bal.Hot != "200" || bal.Warm != "100" || bal.Cold != "5000" {
		  t.Fatalf("desk moved %+v", bal)
	  }
   }

  func TestSweepCapCooldownHalt(t *testing.T) {
	  ctx := context.Background()
	  cur := time.Date(2022, 6, 19, 9, 0, 0, 0, time.UTC)
	   o := sweeprun.NewOrch().WithClock(func() time.Time { return cur })
	  if err := o.Seed("USDT", "900", "200", "5000"); err != nil {
		   t.Fatal(err)
	   }
	   rule := usdtRule(true)
	   rule.CooldownSec = 3600
	   if err := o.PutRule(rule); err != nil {
		  t.Fatal(err)
	  }
	  if err := o.SetLaneCap("USDT", "10", time.Time{}); err != nil {
		  t.Fatal(err)
	   }
	  if _, err := o.Draft(ctx, sweeprun.DraftReq{Actor: "alice", Idem: "cap-1", Chain: "ETH", Asset: "USDT"}); !errors.Is(err, sweeprun.ErrLaneCap) {
		   t.Fatalf("cap %v", err)
	  }
	  if len(o.List("", 10)) != 0 {
		   t.Fatal("cap stored a plan")
	  }
	  if err := o.SetLaneCap("USDT", "100000", time.Time{}); err != nil {
		   t.Fatal(err)
	  }
	  first, err := o.Draft(ctx, sweeprun.DraftReq{Actor: "alice", Idem: "cap-1", Chain: "ETH", Asset: "USDT"})
	  if err != nil {
		  t.Fatal(err)
	   }
	   if _, err := o.Draft(ctx, sweeprun.DraftReq{Actor: "alice", Idem: "cap-2", Chain: "ETH", Asset: "USDT"}); !errors.Is(err, sweeprun.ErrCooldown) {
		   t.Fatalf("cool %v", err)
	   }
	  if _, err := o.Halt(ctx, first.ID, "bob"); err != nil {
		  t.Fatal(err)
	   }
	  rep, err := o.Tick(ctx, 4)
	   if err != nil {
		   t.Fatal(err)
	   }
	   if rep.Advanced != 0 {
		   t.Fatalf("halt advanced %d", rep.Advanced)
	   }
	   if _, err := o.Resume(ctx, first.ID, "bob"); err != nil {
		   t.Fatal(err)
	   }
	  got, err := o.Get(first.ID)
	   if err != nil {
		   t.Fatal(err)
	   }
	  if got.State != sweeprun.PlanStateGated {
		  t.Fatalf("resume %s", got.State)
	  }
  }

  func TestSweepRefFailClosed(t *testing.T) {
	   ctx := context.Background()
	  o := sweeprun.NewOrch()
	   if err := o.Seed("USDT", "900", "500", "5000"); err != nil {
		  t.Fatal(err)
	   }
	   if err := o.PutRule(usdtRule(false)); err != nil {
		   t.Fatal(err)
	  }
	  cer, gap, pipe := newRefs(t)
	   sweeprun.BindPlatformRefs(o, cer, gap, pipe)
	   plan, err := o.Draft(ctx, sweeprun.DraftReq{Actor: "alice", Idem: "ref-1", Chain: "ETH", Asset: "USDT"})
	  if err != nil {
		  t.Fatal(err)
	  }
	   lg := coldLeg(t, plan)
	   early, err := cer.Start("alice", "early", 2, 2)
	  if err != nil {
		   t.Fatal(err)
	   }
	  sess := sealAir(t, gap, "USDT", lg.Amount)
	   job := approveSign(t, pipe, "ETH", lg.Amount, "move-early")
	   if _, err := o.AttachRefs(ctx, plan.ID, lg.ID, "alice", sweeprun.RefBind{CeremonyID: early.ID, AirgapID: sess.ID, SignID: job.ID}); err == nil {
		   t.Fatal("inactive ceremony")
	  }
	   bad := sealAir(t, gap, "USDT", "1")
	   okCer := activate(t, cer)
	  if _, err := o.AttachRefs(ctx, plan.ID, lg.ID, "alice", sweeprun.RefBind{CeremonyID: okCer.ID, AirgapID: bad.ID, SignID: job.ID}); err == nil {
		  t.Fatal("amount mismatch")
	  }
	   future := sweeprun.NewOrch().WithClock(func() time.Time { return time.Now().UTC().Add(2 * time.Hour) })
	   if err := future.Seed("USDT", "900", "500", "5000"); err != nil {
		   t.Fatal(err)
	  }
	   if err := future.PutRule(usdtRule(false)); err != nil {
		   t.Fatal(err)
	   }
	   sweeprun.BindPlatformRefs(future, cer, gap, pipe)
	   plan2, err := future.Draft(ctx, sweeprun.DraftReq{Actor: "alice", Idem: "ref-exp", Chain: "ETH", Asset: "USDT"})
	   if err != nil {
		   t.Fatal(err)
	   }
	  lg2 := coldLeg(t, plan2)
	   sess2 := sealAir(t, gap, "USDT", lg2.Amount)
	   job2 := approveSign(t, pipe, "ETH", lg2.Amount, "move-exp")
	   if _, err := future.AttachRefs(ctx, plan2.ID, lg2.ID, "alice", sweeprun.RefBind{CeremonyID: okCer.ID, AirgapID: sess2.ID, SignID: job2.ID}); err == nil {
		  t.Fatal("expired sign")
	  }
  }

   func TestHotboundReadGate(t *testing.T) {
	  ctx := context.Background()
	  st := ledger.NewMemStore()
	   st.Seed("system", ledger.AccountBridge, "USDT", "900")
	   st.Seed("system", ledger.AccountWarm, "USDT", "100")
	   st.Seed("system", ledger.AccountCold, "USDT", "5000")
	  hb := hotbound.New().BindStore(st)
	   band := hotinv.GateBand{HotLow: "100", HotHigh: "500", WarmLow: "50", WarmHigh: "200"}
	   before := len(hb.List(20))
	  got, err := hb.ReadGate(ctx, "USDT", band)
	  if err != nil {
		  t.Fatal(err)
	   }
	   if got.Code != hotinv.GateAllowSweep || !got.Allow {
		  t.Fatalf("gate %+v", got)
	  }
	  if len(hb.List(20)) != before {
		  t.Fatal("read gate wrote a sweep")
	   }
	   low := hotinv.JudgeBalances("USDT", "10", "40", "1000", band)
	   if low.Code != hotinv.GateAllowReplenish {
		   t.Fatalf("replenish %+v", low)
	  }
	   mid := hotinv.JudgeBalances("USDT", "200", "100", "1000", band)
	   if mid.Code != hotinv.GateDenyInBand || mid.Allow {
		   t.Fatalf("band %+v", mid)
	  }
	  empty := hotbound.New()
	   got, err = empty.ReadGate(ctx, "USDT", band)
	   if !errors.Is(err, hotbound.ErrStore) || got.Code != hotinv.GateDenyUnreadable {
		   t.Fatalf("unreadable %+v %v", got, err)
	   }
	  if len(empty.List(20)) != 0 {
		   t.Fatal("nil store swept")
	   }
  }

  func TestSweepOrchHTTP(t *testing.T) {
	  gin.SetMode(gin.TestMode)
	  o := sweeprun.NewOrch()
	  r := gin.New()
	   sweeprun.Routes(r.Group("/v1"), o)
	   deny := postJSON(r, "/v1/ops/sweep-orch/draft", `{"actor":"alice","idempotency_key":"h1","chain":"ETH","asset":"USDT"}`, "alice", "user")
	  if deny.Code != 403 {
		   t.Fatalf("user %d", deny.Code)
	  }
	  seed := postJSON(r, "/v1/ops/sweep-orch/desk", `{"actor":"alice","asset":"USDT","hot":"200","warm":"100","cold":"1000"}`, "alice", "custody")
	  if seed.Code != 200 {
		  t.Fatalf("seed %d %s", seed.Code, seed.Body.String())
	  }
	   rule := postJSON(r, "/v1/ops/sweep-orch/rule", `{"actor":"alice","rule":{"asset":"USDT","hot_low":"100","hot_target":"200","hot_high":"500","warm_low":"50","warm_target":"100","warm_high":"200","cold_floor":"100","min_leg":"1","window_start":0,"window_end":0,"max_legs":8}}`, "alice", "custody")
	  if rule.Code != 200 {
		  t.Fatalf("rule %d %s", rule.Code, rule.Body.String())
	   }
	  draft := postJSON(r, "/v1/ops/sweep-orch/draft", `{"actor":"alice","idempotency_key":"h1","chain":"ETH","asset":"USDT"}`, "alice", "custody")
	  if draft.Code != 409 {
		  t.Fatalf("draft %d %s", draft.Code, draft.Body.String())
	  }
	   inv := getJSON(r, "/v1/ops/sweep-orch/inventory", "alice", "custody")
	   if inv.Code != 200 || !bytes.Contains(inv.Body.Bytes(), []byte("plan_deny")) {
		  t.Fatalf("inventory %d", inv.Code)
	  }
  }

type orchFixture struct {
	ID    string `json:"id"`
	Chain string `json:"chain"`
	Asset string `json:"asset"`
	At  string `json:"at"`
	Hot string `json:"hot"`
	Warm   string                 `json:"warm"`
	Cold   string                 `json:"cold"`
	Rule   sweeppol.RebalanceRule `json:"rule"`
	Expect string                 `json:"expect"`
	Store  *bool                  `json:"store"`
	Legs  []string `json:"legs"`
	Colds int      `json:"cold_legs"`
}

   func applyOrchFixture(t *testing.T, fx orchFixture) {
	  t.Helper()
	  ctx := context.Background()
	   at := time.Date(2022, 6, 18, 10, 0, 0, 0, time.UTC)
	   if fx.At != "" {
		  parsed, err := time.Parse(time.RFC3339, fx.At)
		   if err != nil {
			  t.Fatal(err)
		   }
		  at = parsed
	  }
	  o := sweeprun.NewOrch().WithClock(func() time.Time { return at })
	   if err := o.Seed(fx.Asset, fx.Hot, fx.Warm, fx.Cold); err != nil {
		   t.Fatal(err)
	  }
	   if err := o.PutRule(fx.Rule); err != nil {
		  t.Fatal(err)
	  }
	  got, err := o.Draft(ctx, sweeprun.DraftReq{Actor: "alice", Idem: "fx-" + fx.ID, Chain: fx.Chain, Asset: fx.Asset})
	  store := true
	  if fx.Store != nil {
		   store = *fx.Store
	   }
	   if !store {
		   if err == nil {
			  t.Fatal("expected refuse")
		  }
		  if len(o.List("", 10)) != 0 {
			  t.Fatal("stored a refused plan")
		  }
		  return
	   }
	  if fx.Expect == "deny_in_band" || fx.Expect == "deny_cold_floor" || fx.Expect == "deny_below_min_leg" {
		   if !errors.Is(err, sweeprun.ErrRejected) {
			  t.Fatalf("%s err %v", fx.ID, err)
		   }
		   if got.Code != fx.Expect || got.State != sweeprun.PlanStateRejected {
			   t.Fatalf("%s %+v", fx.ID, got)
		   }
		   return
	   }
	   if err != nil {
		  t.Fatalf("%s %v", fx.ID, err)
	  }
	  if string(got.State) != fx.Expect && !(fx.Expect == "gated" && got.State == sweeprun.PlanStateGated) {
		  t.Fatalf("%s state %s", fx.ID, got.State)
	  }
	   if len(fx.Legs) > 0 {
		  if len(got.Legs) != len(fx.Legs) {
			   t.Fatalf("%s legs %+v", fx.ID, got.Legs)
		   }
		   for i := range fx.Legs {
			   if got.Legs[i].Kind != fx.Legs[i] {
				   t.Fatalf("%s kind %s", fx.ID, got.Legs[i].Kind)
			  }
		   }
	  }
	   cold := 0
	  for _, lg := range got.Legs {
		  if lg.Cold {
			   cold++
		  }
	  }
	  if fx.Colds != 0 && cold != fx.Colds {
		   t.Fatalf("%s cold %d", fx.ID, cold)
	  }
	   if err := o.Verify(got.ID); err != nil {
		   t.Fatal(err)
	   }
   }

   func usdtRule(direct bool) sweeppol.RebalanceRule {
	   return sweeppol.RebalanceRule{
		  Asset: "USDT", HotLow: "100", HotTarget: "200", HotHigh: "500",
		   WarmLow: "50", WarmTarget: "100", WarmHigh: "200",
		   ColdFloor: "1000", MinLeg: "1", AllowDirect: direct, MaxLegs: 8,
	  }
   }

  func newRefs(t *testing.T) (*custody.CeremonyBook, *airgap.Service, *signpipe.Pipe) {
	   t.Helper()
	   return custody.NewCeremonyBook(), airgap.New(), signpipe.New(10 * time.Minute)
  }

  func activate(t *testing.T, b *custody.CeremonyBook) *custody.Ceremony {
	   t.Helper()
	  c, err := b.Start("alice", "sweep", 2, 2)
	  if err != nil {
		   t.Fatal(err)
	  }
	  for _, ph := range []custody.CeremonyPhase{custody.PhaseGenerate, custody.PhaseSplit, custody.PhaseCombine} {
		   if _, err := b.Advance(c.ID, "alice", ph); err != nil {
			   t.Fatal(err)
		  }
	  }
	   got, err := b.Advance(c.ID, "bob", custody.PhaseActivate)
	  if err != nil {
		  t.Fatal(err)
	  }
	  return got
   }

   func sealAir(t *testing.T, s *airgap.Service, asset, amount string) airgap.Session {
	   t.Helper()
	   row, err := s.Open("alice", "sweep-"+amount, 2, 2, airgap.Payload{Kind: "sweep", Asset: asset, Amount: amount, Dest: "cold"})
	  if err != nil {
		  t.Fatal(err)
	  }
	  if _, err := s.Sign(row.ID, "bob", "ok"); err != nil {
		  t.Fatal(err)
	  }
	  if _, err := s.Sign(row.ID, "carol", "ok"); err != nil {
		   t.Fatal(err)
	   }
	  got, err := s.Seal(row.ID, "alice")
	  if err != nil {
		   t.Fatal(err)
	   }
	  return got
  }

  func approveSign(t *testing.T, p *signpipe.Pipe, chain, amount, move string) *signpipe.Job {
	  t.Helper()
	  job, err := p.Open("alice", move, chain, "cold", amount)
	  if err != nil {
		   t.Fatal(err)
	   }
	   got, err := p.Approve(job.ID, "bob", job.Fence.Epoch, job.Fence.Token, move, chain, "cold", amount)
	  if err != nil {
		  t.Fatal(err)
	  }
	  return got
   }

   func bindLeg(t *testing.T, o *sweeprun.Orch, cer *custody.CeremonyBook, gap *airgap.Service, pipe *signpipe.Pipe, plan sweeprun.SweepPlan, lg sweeprun.SweepLeg) {
	  t.Helper()
	   c := activate(t, cer)
	   sess := sealAir(t,
		   gap, plan.Asset, lg.Amount)
	   job := approveSign(t, pipe, plan.Chain, lg.Amount, "move-"+lg.ID)
	  if _, err := o.AttachRefs(context.Background(), plan.ID, lg.ID, "alice", sweeprun.RefBind{
		  CeremonyID: c.ID,
		  AirgapID: sess.ID, SignID: job.ID,
	  }); err != nil {
		  t.Fatal(err)
	  }
   }

  func coldLeg(t *testing.T, p sweeprun.SweepPlan) sweeprun.SweepLeg {
	   t.Helper()
	   for _, lg := range p.Legs {
		  if lg.Cold && !lg.Compensate {
			   return lg
		   }
	   }
	  t.Fatal("no cold leg")
	   return sweeprun.SweepLeg{}
  }

  func hotLeg(t *testing.T, p sweeprun.SweepPlan) sweeprun.SweepLeg {
	  t.Helper()
	   for _, lg := range p.Legs {
		   if !lg.Cold {
			  return lg
		   }
	   }
	  t.Fatal("no warm leg")
	  return sweeprun.SweepLeg{}
  }

  func fakeRef() sweeprun.RefBind {
	   return sweeprun.RefBind{CeremonyID: "c", AirgapID: "a", SignID: "s"}
   }

   func drive(t *testing.T, o *sweeprun.Orch, id string, done func(sweeprun.SweepPlan) bool) {
	   t.Helper()
	  for i := 0; i < 48; i++ {
		  p, err := o.Get(id)
		  if err != nil {
			  t.Fatal(err)
		   }
		   if done(p) {
			  return
		   }
		   if _, err := o.Tick(context.Background(), 8); err != nil {
			   t.Fatalf("tick %d %v", i, err)
		   }
	  }
	  p, _ := o.Get(id)
	  t.Fatalf("stuck %s %s", p.State, p.Reason)
  }

  func postJSON(r http.Handler, path, body, actor, role string) *httptest.ResponseRecorder {
	   req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	   req.Header.Set("Content-Type", "application/json")
	  req.Header.Set("X-Actor", actor)
	  req.Header.Set("X-Role", role)
	  w := httptest.NewRecorder()
	   r.ServeHTTP(w, req)
	   return w
   }

  func getJSON(r http.Handler, path, actor, role string) *httptest.ResponseRecorder {
	  req := httptest.NewRequest(http.MethodGet, path, nil)
	  req.Header.Set("X-Actor", actor)
	  req.Header.Set("X-Role", role)
	  w := httptest.NewRecorder()
	   r.ServeHTTP(w, req)
	  return w
  }
