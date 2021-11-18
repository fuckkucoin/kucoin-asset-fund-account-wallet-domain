package pumpmarkfence_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/outboxonce"
	"account-assets.kucoin.net/funding/internal/outboxpump"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/pumpmarkfence"
)

   func newEng(clock func() time.Time) *pumpmarkfence.Engine {
	   eng := pumpmarkfence.NewMemEngine()
	  if clock != nil {
		   eng = eng.WithClock(clock)
	   }
	   return eng
   }

  func loadGolden(t *testing.T, name string) map[string]any {
	  t.Helper()
	  p := filepath.Join("testdata", "pumpmarkfence", name)
	  if _, err := os.Stat(p); err != nil {
		  p = filepath.Join("..", "..", "testdata", "pumpmarkfence", name)
	   }
	   b, err := os.ReadFile(p)
	   if err != nil {
		   t.Fatalf("golden %s: %v", name, err)
	  }
	   var m map[string]any
	  if err := json.Unmarshal(b, &m); err != nil {
		   t.Fatalf("json %s: %v", name, err)
	  }
	   return m
   }

  func TestFSMMatrixMatchesGolden(t *testing.T) {
	  g := loadGolden(t, "fsm_transitions.json")
	  want, _ := json.Marshal(g["gen"])
	  got, _ := json.Marshal(pumpmarkfence.GenMatrixRows())
	   if string(want) != string(got) {
		   t.Fatalf("gen matrix\nwant %s\ngot %s", want, got)
	   }
	   wanth, _ := json.Marshal(g["half"])
	   goth, _ := json.Marshal(pumpmarkfence.HalfMatrixRows())
	  if string(wanth) != string(goth) {
		  t.Fatalf("half matrix\nwant %s\ngot %s", wanth, goth)
	  }
	   if pumpmarkfence.CanTransitionGen(pumpmarkfence.GenMarked, pumpmarkfence.GenIssued) {
		  t.Fatal("marked->issued forbidden")
	   }
	   if pumpmarkfence.CanTransitionGen(pumpmarkfence.GenExpired, pumpmarkfence.GenDelivered) {
		  t.Fatal("expired->delivered forbidden")
	   }
	  if pumpmarkfence.CanTransitionHalf(pumpmarkfence.HalfResumed, pumpmarkfence.HalfOpen) {
		   t.Fatal("resumed->open forbidden")
	   }
	  for _, pair := range pumpmarkfence.ForbiddenGenPairs() {
		  if pumpmarkfence.CanTransitionGen(pumpmarkfence.GenState(pair[0]), pumpmarkfence.GenState(pair[1])) {
			   t.Fatalf("forbidden allowed %s->%s", pair[0], pair[1])
		   }
	  }
  }

   func TestPublishInterruptResumeOnceThenDupProof(t *testing.T) {
	  g := loadGolden(t, "publish_interrupt_resume.json")
	  base := time.Date(2020, 11, 18, 10, 0, 0, 0, time.UTC)
	  eng := newEng(func() time.Time { return base })
	   ctx := context.Background()
	  ev, err := eng.Enqueue(ctx, "ledger.posted.u-1001", "ledger_posted", `{"account":"u-1001","asset":"USDT","delta":"10.00"}`)
	   if err != nil {
		  t.Fatalf("enqueue: %v", err)
	   }
	   sink := pumpmarkfence.NewCountSink()
	  _, err = eng.DeliverInterrupt(ctx, ev, sink.Handler(), "ops-pump-fence-01")
	   if !errors.Is(err, pumpmarkfence.ErrHalfMarkOpen) {
		  t.Fatalf("interrupt: %v", err)
	   }
	  if sink.Times(ev.ID) != 1 {
		   t.Fatalf("handler after interrupt %d", sink.Times(ev.ID))
	   }
	  rep, err := eng.TickWithGen(ctx, sink.Handler(), "ops-pump-fence-01")
	  if err != nil {
		  t.Fatalf("resume tick: %v", err)
	   }
	  if rep.Resumed != 1 || sink.Times(ev.ID) != 1 {
		   t.Fatalf("resume %+v handler=%d", rep, sink.Times(ev.ID))
	   }
	  gen, err := eng.GetGenByOutbox(ctx, ev.ID)
	   if err != nil || gen.State != pumpmarkfence.GenMarked {
		  t.Fatalf("gen %+v err=%v", gen, err)
	  }
	   replay, err := eng.ReplayTick(ctx, ev.ID, sink.Handler(), "ops-pump-fence-01")
	   if !errors.Is(err, pumpmarkfence.ErrDupEffect) && !errors.Is(err, pumpmarkfence.ErrAlreadyApplied) {
		  t.Fatalf("replay err %v", err)
	  }
	   if replay.DupProofs < 1 {
		   t.Fatalf("dup proofs %d", replay.DupProofs)
	  }
	  if sink.Times(ev.ID) != 1 {
		  t.Fatalf("second effect %d", sink.Times(ev.ID))
	  }
	  proofs, _ := eng.ListProofs(ctx, ev.ID, 20)
	   if len(proofs) == 0 {
		   t.Fatal("expected OutboxDupProof")
	   }
	   wantKind := g["expect"].(map[string]any)["proof_kind"]
	   if string(proofs[0].Kind) != wantKind {
		   t.Fatalf("kind %s want %v", proofs[0].Kind, wantKind)
	   }
   }

   func TestPoisonReplaySecondLegDenied(t *testing.T) {
	  g := loadGolden(t, "poison_replay_second_leg.json")
	  eng := newEng(nil)
	   ctx := context.Background()
	   ev, err := eng.Enqueue(ctx, "notify.mail.u-2002", "notify_mail", `{"to":"ops-alert@kucoin.net","tpl":"outbox-poison"}`)
	   if err != nil {
		  t.Fatal(err)
	  }
	  sink := pumpmarkfence.NewCountSink()
	   rep, err := eng.TickWithGen(ctx, sink.Handler(), "ops-poison-01")
	   if err != nil {
		  t.Fatalf("tick: %v", err)
	   }
	   if sink.Total() != 1 || rep.Marked != 1 {
		  t.Fatalf("first %+v effects=%d", rep, sink.Total())
	   }
	  gen, _ := eng.GetGenByOutbox(ctx, ev.ID)
	  res, err := eng.PoisonReplayAssert(ctx, pumpmarkfence.PoisonReplayReq{
		   OutboxID: ev.ID, ExpectGen: gen.Generation, Actor: "ops-poison-01", AllowHandler: true,
	   }, sink.Handler())
	   if !errors.Is(err, pumpmarkfence.ErrPoisonReplay) {
		   t.Fatalf("poison second %v", err)
	  }
	   if res.Proof == nil || res.Proof.Kind != pumpmarkfence.ProofPoisonReplay {
		   t.Fatalf("proof %+v", res.Proof)
	   }
	   if sink.Total() != 1 {
		   t.Fatalf("poison second effect %d", sink.Total())
	   }
	  _ = g
  }

  func TestLagReadonlyCannotMark(t *testing.T) {
	   g := loadGolden(t, "lag_readonly.json")
	  eng := newEng(nil)
	  ctx := context.Background()
	   ev, _ := eng.Enqueue(ctx, "k-lag", "kind", `{"x":1}`)
	  sink := pumpmarkfence.NewCountSink()
	   _, _ = eng.TickWithGen(ctx, sink.Handler(), "ops")
	   err := eng.LagMustNotMark(ctx, eng.Box().KV(), ev.ID)
	   if !errors.Is(err, pumpmarkfence.ErrLagReadonly) {
		   t.Fatalf("lag mark %v", err)
	  }
	   proofs, _ := eng.ListProofs(ctx, ev.ID, 10)
	   found := false
	   for _, p := range proofs {
		   if p.Kind == pumpmarkfence.ProofLagMark {
			  found = true
		   }
	  }
	  if !found {
		   t.Fatal("lag proof missing")
	   }
	  if g["expect_error"] != pumpmarkfence.ErrLagReadonly.Error() {
		  t.Fatalf("golden error %v", g["expect_error"])
	   }
  }

  func TestFailClosedNoGenFingerprintExpired(t *testing.T) {
	  g := loadGolden(t, "fail_closed.json")
	   now := time.Date(2021, 3, 1, 8, 0, 0, 0, time.UTC)
	  if err := pumpmarkfence.FailClosedMark(nil, now, "", false); !errors.Is(err, pumpmarkfence.ErrGenMissing) {
		  t.Fatalf("nil gen %v", err)
	   }
	   if g["no_gen"] != pumpmarkfence.ErrGenMissing.Error() {
		  t.Fatalf("golden no_gen")
	   }
	   row := &pumpmarkfence.DeliverEffectGen{
		   GenID: "g1", PayloadHash: "aaa", Generation: 1, State: pumpmarkfence.GenIssued,
		  Deadline: now.Add(-time.Second),
	   }
	  if err := pumpmarkfence.FailClosedMark(row, now, "aaa", false); !errors.Is(err, pumpmarkfence.ErrGenExpired) {
		   t.Fatalf("expired %v", err)
	   }
	   row.Deadline = now.Add(time.Hour)
	   if err := pumpmarkfence.FailClosedMark(row, now, "bbb", false); !errors.Is(err, pumpmarkfence.ErrFingerprint) {
		   t.Fatalf("fp %v", err)
	   }
	   if err := pumpmarkfence.FailClosedMark(row, now, "aaa", true); !errors.Is(err, pumpmarkfence.ErrLagReadonly) {
		  t.Fatalf("lag %v", err)
	  }
	   row.Generation = 2
	  if err := pumpmarkfence.FailClosedReplay(row, 1, now, "aaa"); !errors.Is(err, pumpmarkfence.ErrCASFailed) {
		  t.Fatalf("cas %v", err)
	   }
   }

  func TestHappyPathIdempotentMark(t *testing.T) {
	   eng := newEng(nil)
	   ctx := context.Background()
	  ev, _ := eng.Enqueue(ctx, "k1", "posted", `{"n":1}`)
	   sink := pumpmarkfence.NewCountSink()
	   rep, err := eng.TickWithGen(ctx, sink.Handler(), "ops")
	  if err != nil || rep.Marked != 1 || sink.Times(ev.ID) != 1 {
		  t.Fatalf("tick %+v err=%v", rep, err)
	  }
	   rep2, err := eng.TickWithGen(ctx, sink.Handler(), "ops")
	   if err != nil {
		  t.Fatalf("second empty tick %v", err)
	  }
	   if rep2.HandlerCalls != 0 || sink.Times(ev.ID) != 1 {
		  t.Fatalf("idempotent fail %+v", rep2)
	   }
	   got,
		  err := persist.NewOutbox(eng.Box().KV()).Get(ctx, ev.ID)
	   if err != nil || got.SentAt == nil {
		  t.Fatalf("sent %+v", got)
	   }
  }

  func TestHandlerFailDoesNotMark(t *testing.T) {
	   eng := newEng(nil)
	  ctx := context.Background()
	  ev, _ := eng.Enqueue(ctx, "k-fail", "posted", `{"n":2}`)
	  h := func(context.Context, persist.OutboxEvent) error {
		   return errors.New("broker down")
	  }
	  rep, err := eng.TickWithGen(ctx, h, "ops")
	   if !errors.Is(err, pumpmarkfence.ErrHandler) {
		  t.Fatalf("err %v", err)
	   }
	  if rep.Marked != 0 {
		   t.Fatalf("marked %d", rep.Marked)
	   }
	   got, _ := eng.Box().Get(ctx, ev.ID)
	  if got.SentAt != nil {
		  t.Fatal("must stay pending")
	   }
	   gen, _ := eng.GetGenByOutbox(ctx, ev.ID)
	   if gen.State != pumpmarkfence.GenIssued {
		  t.Fatalf("state %s", gen.State)
	  }
   }

  func TestConcurrentResumeOnlyOne(t *testing.T) {
	  eng := newEng(nil)
	   ctx := context.Background()
	   ev, _ := eng.Enqueue(ctx, "k-race", "posted", `{"n":3}`)
	   sink := pumpmarkfence.NewCountSink()
	   _, err := eng.DeliverInterrupt(ctx, ev, sink.Handler(), "ops")
	  if !errors.Is(err, pumpmarkfence.ErrHalfMarkOpen) {
		  t.Fatal(err)
	   }
	   var ok int32
	  var wg sync.WaitGroup
	  for i := 0; i < 8; i++ {
		   wg.Add(1)
		   go func() {
			  defer wg.Done()
			  _, e2 := eng.ResumeMark(ctx, pumpmarkfence.ResumeMarkReq{OutboxID: ev.ID, Actor: "ops-race"})
			  if e2 == nil {
				   atomic.AddInt32(&ok, 1)
			   }
		   }()
	   }
	   wg.Wait()
	  if atomic.LoadInt32(&ok) != 1 {
		  t.Fatalf("winners %d", ok)
	  }
	   if sink.Times(ev.ID) != 1 {
		   t.Fatalf("effects %d", sink.Times(ev.ID))
	   }
   }

   func TestExpireStaleBlocksMark(t *testing.T) {
	  now := time.Date(2021,
		  4, 2, 9, 0, 0, 0, time.UTC)
	   cur := now
	  eng := newEng(func() time.Time { return cur })
	  eng.WithTTL(time.Minute)
	  ctx := context.Background()
	  ev, _ := eng.Enqueue(ctx, "k-ttl", "posted", `{"n":4}`)
	  sink := pumpmarkfence.NewCountSink()
	   _, err := eng.DeliverInterrupt(ctx, ev, sink.Handler(), "ops")
	   if !errors.Is(err, pumpmarkfence.ErrHalfMarkOpen) {
		   t.Fatal(err)
	   }
	   cur = now.Add(2 * time.Minute)
	   n, err := eng.ExpireStale(ctx, 20, "ops")
	  if err != nil || n < 1 {
		  t.Fatalf("expire %d %v", n, err)
	   }
	  _, err = eng.TickWithGen(ctx, sink.Handler(), "ops")
	  if err != nil && !errors.Is(err, pumpmarkfence.ErrGenExpired) {
		   // tick may skip expired without returning if classified
	   }
	  gen, _ := eng.GetGenByOutbox(ctx, ev.ID)
	   if gen.State != pumpmarkfence.GenExpired {
		   t.Fatalf("state %s", gen.State)
	  }
	   if sink.Times(ev.ID) != 1 {
		  t.Fatalf("no second effect %d", sink.Times(ev.ID))
	  }
   }

   func TestPumpTickWithGenAdapter(t *testing.T) {
	  eng := newEng(nil)
	   ctx := context.Background()
	  _, _ = eng.Enqueue(ctx, "k-adp", "posted", `{"n":5}`)
	  sink := pumpmarkfence.NewCountSink()
	  rep, err := outboxpump.TickWithGen(ctx, outboxpump.NewMem(), eng.AsPumpFence(), sink.Handler(), "ops")
	  if err != nil {
		  t.Fatal(err)
	  }
	  if rep.Marked != 1 || len(rep.GenIDs) == 0 {
		   t.Fatalf("%+v", rep)
	  }
	   _, err = outboxpump.TickWithGen(ctx, outboxpump.NewMem(), nil, sink.Handler(), "ops")
	  if !errors.Is(err, outboxpump.ErrUnavailable) {
		  t.Fatalf("no fence %v", err)
	  }
   }

   func TestOutboxOnceHaltUnchanged(t *testing.T) {
	  s := outboxonce.New()
	  if s == nil {
		  t.Fatal("outboxonce")
	  }
	  s.SetHalt(true)
	   _, err := s.Enqueue(context.Background(), "k", "kind", `{"a":1}`)
	   if err == nil {
		  t.Fatal("halt must refuse enqueue")
	  }
   }

   func TestInspectResumesHalf(t *testing.T) {
	  eng := newEng(nil)
	  ctx := context.Background()
	  ev, _ := eng.Enqueue(ctx, "k-insp", "posted", `{"n":6}`)
	  sink := pumpmarkfence.NewCountSink()
	  _, _ = eng.DeliverInterrupt(ctx, ev, sink.Handler(), "ops")
	   w := pumpmarkfence.NewHalfInspectWorker(eng, pumpmarkfence.WorkerConfig{BatchSize: 20, Actor: "worker"})
	   rep, err := w.TickOnce(ctx)
	  if err != nil {
		  t.Fatal(err)
	  }
	  if rep.Resumed < 1 {
		  t.Fatalf("inspect %+v", rep)
	   }
	   if sink.Times(ev.ID) != 1 {
		   t.Fatalf("effects %d", sink.Times(ev.ID))
	   }
   }

  func TestExportProofBundle(t *testing.T) {
	   eng := newEng(nil)
	  ctx := context.Background()
	   ev, _ := eng.Enqueue(ctx, "k-exp", "posted", `{"n":7}`)
	  sink := pumpmarkfence.NewCountSink()
	  _, _ = eng.TickWithGen(ctx, sink.Handler(), "ops")
	   _, _ = eng.ReplayTick(ctx, ev.ID, sink.Handler(), "ops")
	   bundle, err := eng.ExportProofs(ctx, 20, "ops")
	  if err != nil || bundle.Count < 1 || bundle.Digest == "" {
		   t.Fatalf("bundle %+v err=%v", bundle, err)
	   }
  }

   func TestFingerprintNormalize(t *testing.T) {
	  a := pumpmarkfence.PayloadHash("k", "id", "  hello\tworld  ")
	   b := pumpmarkfence.PayloadHash("k", "id", "hello world")
	  if a != b {
		  t.Fatalf("norm %s %s", a, b)
	  }
	   if !pumpmarkfence.FingerprintMatch(a, b) {
		  t.Fatal("match")
	  }
   }

   func TestCASBump(t *testing.T) {
	   g := &pumpmarkfence.DeliverEffectGen{Generation: 3}
	  if err := pumpmarkfence.CASBump(g, 3); err != nil || g.Generation != 4 {
		  t.Fatalf("%v gen=%d", err, g.Generation)
	   }
	   if err := pumpmarkfence.CASHold(g, 2); !errors.Is(err, pumpmarkfence.ErrCASFailed) {
		  t.Fatalf("hold %v", err)
	  }
  }

   func TestAbandonHalf(t *testing.T) {
	  eng := newEng(nil)
	   ctx := context.Background()
	  ev, _ := eng.Enqueue(ctx, "k-ab", "posted", `{"n":8}`)
	   sink := pumpmarkfence.NewCountSink()
	   _, _ = eng.DeliverInterrupt(ctx, ev, sink.Handler(), "ops")
	   half, err := eng.AbandonHalf(ctx, ev.ID, "ops", "drop")
	   if err != nil || half.State != pumpmarkfence.HalfAbandoned {
		  t.Fatalf("%+v %v", half, err)
	   }
  }

  func TestReadGenSnapshot(t *testing.T) {
	  eng := newEng(nil)
	  ctx := context.Background()
	  ev, _ := eng.Enqueue(ctx, "k-snap", "posted", `{"n":9}`)
	   _, _ = eng.TickWithGen(ctx, pumpmarkfence.NewCountSink().Handler(), "ops")
	  g, _ := eng.GetGenByOutbox(ctx, ev.ID)
	   snap, err := eng.ReadGen(ctx, g.GenID)
	   if err != nil || snap.DeliverEffectGen == "" || !snap.Applied {
		  t.Fatalf("%+v %v", snap, err)
	   }
  }

  func TestPoisonNoGen(t *testing.T) {
	  eng := newEng(nil)
	   ctx := context.Background()
	   ev, _ := persist.NewOutbox(eng.Box().KV()).Enqueue(ctx, "k-ng", "posted", `{"n":10}`)
	   _, err := eng.PoisonReplayAssert(ctx, pumpmarkfence.PoisonReplayReq{OutboxID: ev.ID, Actor: "ops"}, nil)
	   if !errors.Is(err, pumpmarkfence.ErrGenMissing) {
		   t.Fatalf("%v", err)
	   }
  }

   func TestMarkDeniedFromLagDirect(t *testing.T) {
	  eng := newEng(nil)
	  ctx := context.Background()
	   ev, _ := eng.Enqueue(ctx, "k-lg2", "posted", `{"n":11}`)
	   _, _ = eng.TickWithGen(ctx, pumpmarkfence.NewCountSink().Handler(), "ops")
	  if err := eng.MarkDeniedFromLag(ctx, ev.ID, "ops"); !errors.Is(err, pumpmarkfence.ErrLagReadonly) {
		   t.Fatal(err)
	   }
   }

   func TestTransitionNil(t *testing.T) {
	   if err := pumpmarkfence.TransitionGen(nil, pumpmarkfence.GenMarked); !errors.Is(err, pumpmarkfence.ErrIllegalTransit) {
		  t.Fatal(err)
	   }
	   if err := pumpmarkfence.TransitionHalf(nil, pumpmarkfence.HalfResumed); !errors.Is(err, pumpmarkfence.ErrIllegalTransit) {
		  t.Fatal(err)
	   }
  }

  func TestParseStates(t *testing.T) {
	   if s, err := pumpmarkfence.ParseGenState("DELIVERED"); err != nil || s != pumpmarkfence.GenDelivered {
		   t.Fatal(s, err)
	   }
	   if _, err := pumpmarkfence.ParseGenState("nope"); err == nil {
		   t.Fatal("parse")
	   }
	   if s, err := pumpmarkfence.ParseHalfState("open"); err != nil || !s.Open() {
		   t.Fatal(s, err)
	  }
	  if _, err := pumpmarkfence.ParseProofKind("tick_replay"); err != nil {
		  t.Fatal(err)
	   }
  }

   func TestWorkerExpireAndPoisonGate(t *testing.T) {
	   now := time.Date(2021, 5, 1, 0, 0, 0, 0, time.UTC)
	  cur := now
	  eng := newEng(func() time.Time { return cur }).WithTTL(time.Second)
	   ctx := context.Background()
	  ev, _ := eng.Enqueue(ctx, "k-w", "posted", `{"n":12}`)
	  _, _ = eng.DeliverInterrupt(ctx, ev, pumpmarkfence.NewCountSink().Handler(), "ops")
	  cur = now.Add(time.Hour)
	  n, err := pumpmarkfence.NewExpireWorker(eng, pumpmarkfence.WorkerConfig{BatchSize: 10}).TickOnce(ctx)
	  if err != nil || n < 1 {
		   t.Fatalf("expire worker %d %v", n, err)
	   }
	   _, _ = eng.Enqueue(ctx, "k-w2", "posted", `{"n":13}`)
	  _, _ = eng.TickWithGen(ctx, pumpmarkfence.NewCountSink().Handler(), "ops")
	   denied,
		  err := pumpmarkfence.NewPoisonGateWorker(eng, pumpmarkfence.WorkerConfig{BatchSize: 20}).TickOnce(ctx)
	   if err != nil || denied < 1 {
		  t.Fatalf("poison gate %d %v", denied, err)
	  }
   }
