package occseggate_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"account-assets.kucoin.net/funding/internal/occseggate"
	"account-assets.kucoin.net/funding/internal/wdrelease"
)

   func newEng(t *testing.T) (*occseggate.Engine, *wdrelease.OccDesk) {
	   t.Helper()
	   desk := wdrelease.NewOccDesk(wdrelease.NewOccMemStore())
	  eng := occseggate.NewEngine(occseggate.NewOccMemStore(), occseggate.DeskBridge{Desk: desk}, occseggate.Config{})
	  return eng, desk
  }

  func occupy(t *testing.T, eng *occseggate.Engine, user, asset, amt, idem string) *occseggate.OccOccupyResult {
	   t.Helper()
	  ctx := context.Background()
	   tk, err := eng.OpenAndAdmit(ctx, user, asset, amt)
	   if err != nil {
		  t.Fatal(err)
	   }
	  res, err := eng.BeforeFreezeWithGen(ctx, occseggate.OccOccupyRequest{
		   TicketID: tk.ID, GrantIdem: idem, Amount: amt, Asset: asset,
	  })
	  if err != nil {
		  t.Fatal(err)
	  }
	  if res.Seg == nil || res.Seg.Gen == 0 {
		   t.Fatal("occ_seg_gen missing")
	   }
	  return res
  }

   func TestBeforeFreezeIssuesGen(t *testing.T) {
	  eng, _ := newEng(t)
	   res := occupy(t, eng, "u1", "USDT", "10", "idem-1")
	  if res.Seg.State != occseggate.SegReserved {
		  t.Fatalf("state %s", res.Seg.State)
	  }
	  if res.Seg.Asset != "USDT" || res.Seg.Amount != "10" {
		  t.Fatalf("snapshot %+v", res.Seg)
	  }
   }

  func TestCommitThenAbortGhost(t *testing.T) {
	  eng, _ := newEng(t)
	  res := occupy(t, eng, "u2", "USDT", "3", "idem-2")
	   gen := res.Seg.Gen
	  ok, err := eng.CommitAssertGen(context.Background(), occseggate.GenRequest{GrantIdem: "idem-2", OccSegGen: &gen})
	   if err != nil || !ok.Allowed {
		   t.Fatalf("commit %v %+v", err, ok)
	   }
	  bad, err := eng.AbortAssertGen(context.Background(), occseggate.GenRequest{GrantIdem: "idem-2", OccSegGen: &gen})
	   if err == nil || bad.Allowed {
		   t.Fatal("abort after commit must fail")
	   }
	  if !errors.Is(err, occseggate.ErrAlreadyFrozen) {
		   t.Fatalf("want frozen, got %v", err)
	   }
	   if bad.Proof == nil || !bad.Proof.GhostPair() {
		  t.Fatalf("ghost proof required %+v", bad.Proof)
	   }
   }

  func TestAbortThenCommit(t *testing.T) {
	  eng, _ := newEng(t)
	  res := occupy(t, eng, "u3", "ETH", "1", "idem-3")
	   gen := res.Seg.Gen
	   ok, err := eng.AbortAssertGen(context.Background(), occseggate.GenRequest{GrantIdem: "idem-3", OccSegGen: &gen})
	  if err != nil || !ok.Allowed {
		  t.Fatalf("abort %v", err)
	   }
	   bad, err := eng.CommitAssertGen(context.Background(), occseggate.GenRequest{GrantIdem: "idem-3", OccSegGen: &gen})
	   if err == nil || bad.Allowed {
		  t.Fatal("commit after abort must fail")
	  }
   }

  func TestMissingGenFailClosed(t *testing.T) {
	   eng, _ := newEng(t)
	   _ = occupy(t, eng, "u4", "BTC", "0.1", "idem-4")
	  res, err := eng.CommitAssertGen(context.Background(), occseggate.GenRequest{GrantIdem: "idem-4"})
	  if err == nil || res.Allowed {
		   t.Fatal("missing gen")
	  }
	   if !errors.Is(err, occseggate.ErrMissingGen) {
		   t.Fatalf("got %v", err)
	   }
   }

   func TestStaleGenFailClosed(t *testing.T) {
	   eng, _ := newEng(t)
	   _ = occupy(t, eng, "u5", "KCS", "2", "idem-5")
	  stale := uint64(99)
	  res, err := eng.CommitAssertGen(context.Background(), occseggate.GenRequest{GrantIdem: "idem-5", OccSegGen: &stale})
	  if err == nil || res.Allowed {
		   t.Fatal("stale")
	  }
	  if !errors.Is(err, occseggate.ErrStaleGen) {
		  t.Fatalf("got %v", err)
	   }
  }

  func TestExpireReopenOldGrantFails(t *testing.T) {
	  eng, _ := newEng(t)
	  res := occupy(t, eng, "u6", "USDT", "9", "idem-6")
	   oldGen := res.Seg.Gen
	   if _, err := eng.ExpireRevokeGens(context.Background(), res.Ticket.ID, "ops"); err != nil {
		  t.Fatal(err)
	  }
	   if _, err := eng.ReopenAdmit(context.Background(), res.Ticket.ID); err != nil {
		  t.Fatal(err)
	   }
	  c, err := eng.CommitAssertGen(context.Background(), occseggate.GenRequest{GrantIdem: "idem-6", OccSegGen: &oldGen})
	   if err == nil || c.Allowed {
		  t.Fatal("old commit")
	  }
	   a, err := eng.AbortAssertGen(context.Background(), occseggate.GenRequest{GrantIdem: "idem-6", OccSegGen: &oldGen})
	   if err == nil || a.Allowed {
		   t.Fatal("old abort")
	  }
  }

   func TestRefuseLiveUnfreeze(t *testing.T) {
	   eng, desk := newEng(t)
	  _ = occupy(t, eng, "u7", "USDT", "1", "idem-7")
	  p, err := eng.RefuseLiveUnfreeze(context.Background(), "idem-7")
	   if err == nil {
		  t.Fatal("must refuse")
	   }
	   if p == nil || p.Reason != occseggate.ReasonLiveUnfreeze {
		  t.Fatalf("proof %+v", p)
	   }
	   if wdrelease.LiveUnfreezeAllowed() {
		   t.Fatal("live unfreeze allowed")
	   }
	  if err := desk.TryLiveUnfreeze(context.Background(), "idem-7"); !errors.Is(err, wdrelease.ErrRefuseLiveUnfreeze) {
		   t.Fatalf("desk %v", err)
	   }
	  if wdrelease.PathAllowsUnfreeze(wdrelease.PathLiveUnfreeze) {
		  t.Fatal("path leak")
	  }
   }

  func TestConcurrentCommitAbort(t *testing.T) {
	   eng, desk := newEng(t)
	  res := occupy(t, eng, "u8", "USDT", "12", "idem-8")
	   gen := res.Seg.Gen
	   var wg sync.WaitGroup
	   var cRes, aRes *occseggate.GateResult
	   var cErr, aErr error
	   wg.Add(2)
	   go func() {
		   defer wg.Done()
		  cRes, cErr = eng.CommitAssertGen(context.Background(), occseggate.GenRequest{GrantIdem: "idem-8", OccSegGen: &gen, Actor: "c"})
	   }()
	   go func() {
		   defer wg.Done()
		  aRes, aErr = eng.AbortAssertGen(context.Background(), occseggate.GenRequest{GrantIdem: "idem-8", OccSegGen: &gen, Actor: "a"})
	  }()
	  wg.Wait()
	  cOK := cErr == nil && cRes != nil && cRes.Allowed
	   aOK := aErr == nil && aRes != nil && aRes.Allowed
	  if cOK && aOK {
		   t.Fatal("both legs succeeded")
	   }
	   if !cOK && !aOK {
		  // 互斥双方皆败亦可，但必须有证明
		  if (cRes == nil || cRes.Proof == nil) && (aRes == nil || aRes.Proof == nil) {
			  t.Fatalf("need proof c=%v a=%v", cErr, aErr)
		  }
	  }
	   g, err := desk.GetGrantByIdem(context.Background(), "idem-8")
	  if err != nil {
		   t.Fatal(err)
	  }
	   seg, err := eng.GetSegByGrant(context.Background(), "idem-8")
	   if err != nil {
		  t.Fatal(err)
	   }
	  if g.FrozenOnLedger() && aOK {
		  t.Fatal("ghost pair: ledger frozen and abort succeeded")
	  }
	   if seg.State == occseggate.SegFrozen && aOK {
		  t.Fatal("seg frozen and abort succeeded")
	  }
  }

   func TestIdempotentOccupy(t *testing.T) {
	  eng, _ := newEng(t)
	  first := occupy(t, eng, "u9", "USDT", "4", "idem-9")
	  tk := first.Ticket
	  second, err := eng.BeforeFreezeWithGen(context.Background(), occseggate.OccOccupyRequest{
		  TicketID: tk.ID, GrantIdem: "idem-9", Amount: "4", Asset: "USDT",
	   })
	   if err != nil {
		   t.Fatal(err)
	   }
	  if second.Seg.ID != first.Seg.ID || second.Seg.Gen != first.Seg.Gen {
		  t.Fatal("idem occupy must reuse seg")
	   }
  }

  func TestAckProof(t *testing.T) {
	  eng, _ := newEng(t)
	  _ = occupy(t, eng, "u10", "USDT", "1", "idem-10")
	   res, err := eng.CommitAssertGen(context.Background(), occseggate.GenRequest{GrantIdem: "idem-10"})
	   if err == nil {
		   t.Fatal("expected missing")
	   }
	   p, err := eng.AckProof(context.Background(), res.Proof.ID, "ops")
	   if err != nil {
		  t.Fatal(err)
	  }
	  if p.State != occseggate.ProofAcked {
		   t.Fatalf("state %s", p.State)
	   }
  }

  func TestWorkersRunOnce(t *testing.T) {
	  eng, _ := newEng(t)
	  _ = occupy(t, eng, "u11", "USDT", "1", "idem-11")
	  cfg := occseggate.WorkerConfig{BatchSize: 10}
	   if _, err := occseggate.NewGhostInspectWorker(eng, cfg).RunOnce(context.Background()); err != nil {
		   t.Fatal(err)
	  }
	   if _, err := occseggate.NewExpireRevokeWorker(eng, cfg).RunOnce(context.Background()); err != nil {
		  t.Fatal(err)
	   }
	  if _, err := occseggate.NewProofExportWorker(eng, cfg).RunOnce(context.Background()); err != nil {
		  t.Fatal(err)
	  }
   }

   func TestSQLStoreUnavailable(t *testing.T) {
	  s := occseggate.NewOccSQLStore(nil)
	  if _, err := s.GetSegByID(context.Background(), "x"); !errors.Is(err, occseggate.ErrUnavailable) {
		  t.Fatalf("%v", err)
	   }
  }

   func TestIsolationInvariants(t *testing.T) {
	   // 本包不改写补偿 / holdrel / liq_gen；仅断言本域不变量。
	  if occseggate.AllowedWithoutGen("commit") || occseggate.AllowedWithoutGen("abort") {
		   t.Fatal("without-gen leak")
	   }
	   if !occseggate.ConflictingMutex(occseggate.MutexKindCommit, occseggate.MutexKindAbort) {
		   t.Fatal("mutex kinds")
	   }
	   if !occseggate.GhostIfAbortAfterFrozen(occseggate.SegFrozen, true) {
		   t.Fatal("ghost predicate")
	  }
   }
