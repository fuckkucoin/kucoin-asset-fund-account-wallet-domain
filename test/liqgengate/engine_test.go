package liqgengate_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/liqgengate"
	"account-assets.kucoin.net/funding/internal/marginhook"
)

   func seedView(user, borrow, col, asset, bamt, camt string, ltv, cap int) *liqgengate.SnapshotView {
	   v := liqgengate.NewSnapshotView()
	  v.SeedBorrow(liqgengate.LiveBorrow{
		   ID: borrow, UserID: user, Asset: asset, Amount: bamt,
		   Collateral: col, LTVBPS: ltv, Status: "open",
	  }, liqgengate.LiveHold{
		  ID: col, UserID: user, Kind: "collateral", Asset: asset,
		   Amount: camt, CapBPS: cap, Status: "open", BorrowID: borrow,
	   })
	   return v
  }

   func newEng(t *testing.T, v *liqgengate.SnapshotView, hook liqgengate.FreezeHook, ttl time.Duration) *liqgengate.Engine {
	  t.Helper()
	  return liqgengate.NewEngine(liqgengate.NewMemStore(), v, hook, liqgengate.Config{LeaseTTL: ttl})
  }

  func TestFreezeAdvancesGenAndFenceOldRelease(t *testing.T) {
	  ctx := context.Background()
	   v := seedView("u1", "b1", "c1", "USDT", "8000", "10000", 8000, 6500)
	   eng := newEng(t,
		   v, nil, time.Minute)
	  fr, err := eng.FreezeWithGen(ctx, liqgengate.FreezeRequest{UserID: "u1", BorrowID: "b1"})
	   if err != nil || !fr.Allowed || fr.Generation.Gen != 1 {
		   t.Fatalf("freeze: %+v %v", fr, err)
	  }
	   rel, err := eng.ReleaseWithGen(ctx, liqgengate.ReleaseRequest{UserID: "u1", BorrowID: "b1", LiqGen: 0, HasGen: true})
	   if err == nil || rel.Allowed || rel.RaceProof == nil {
		  t.Fatalf("stale release should fail with proof: %+v %v", rel, err)
	   }
	  if rel.RejectReason != string(liqgengate.RaceStaleGen) {
		   t.Fatalf("reason %s", rel.RejectReason)
	   }
	   cl, err := eng.CloseWithGen(ctx, liqgengate.CloseRequest{UserID: "u1", BorrowID: "b1", LiqGen: 0, HasGen: true})
	   if err == nil || cl.Allowed {
		   t.Fatalf("stale close should fail")
	  }
   }

   func TestMissingGenFailClosed(t *testing.T) {
	   ctx := context.Background()
	   v := seedView("u1", "b1", "c1", "USDT", "8000", "10000", 8000, 6500)
	  eng := newEng(t, v, nil, time.Minute)
	   if _, err := eng.FreezeWithGen(ctx, liqgengate.FreezeRequest{UserID: "u1", BorrowID: "b1"}); err != nil {
		  t.Fatal(err)
	   }
	   rel, err := eng.ReleaseWithGen(ctx, liqgengate.ReleaseRequest{UserID: "u1", BorrowID: "b1", HasGen: false})
	  if err == nil || rel.Allowed || rel.RejectReason != string(liqgengate.RaceMissingGen) {
		  t.Fatalf("missing gen: %+v %v", rel, err)
	   }
   }

  func TestHookFailHalfBlocksRelease(t *testing.T) {
	  ctx := context.Background()
	  v := seedView("u1", "b1", "c1", "ETH", "70", "10", 9000, 6500)
	  hook := func(string, account.Book, bool) error { return errors.New("book freeze down") }
	  eng := newEng(t,
		   v, hook, time.Minute)
	   fr, err := eng.FreezeWithGen(ctx, liqgengate.FreezeRequest{UserID: "u1", BorrowID: "b1"})
	   if !errors.Is(err, liqgengate.ErrHookFailed) {
		  t.Fatalf("want hook failed, got %v", err)
	  }
	  if fr.HalfProof == nil || fr.Generation.Phase != liqgengate.PhaseHalfOpen {
		  t.Fatalf("half: %+v", fr)
	   }
	  rel, err := eng.ReleaseWithGen(ctx, liqgengate.ReleaseRequest{UserID: "u1", BorrowID: "b1", LiqGen: 1, HasGen: true})
	  if err == nil || rel.Allowed || rel.RejectReason != string(liqgengate.RaceHalfBlocked) {
		  t.Fatalf("half must block release: %+v %v", rel, err)
	  }
	   b, _ := v.LookupBorrow("b1")
	   if b.Status == "closed" || b.LiqPhase == string(liqgengate.PhaseClosed) {
		   t.Fatalf("must not treat as fully liquidated: %+v", b)
	  }
   }

  func TestConcurrentFreezeAndReleaseMutex(t *testing.T) {
	  ctx := context.Background()
	  v := seedView("u1", "b1", "c1", "KCS", "5000", "6000", 8300, 6500)
	   store := liqgengate.NewMemStore()
	   // 先占 freeze 互斥， Freeze 进行中。
	  if err := store.InsertGeneration(ctx, &liqgengate.LiqGeneration{
		   ID: "g", BorrowID: "b1", UserID: "u1", CollateralID: "c1", Phase: liqgengate.PhaseOpen, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	  }); err != nil {
		  t.Fatal(err)
	   }
	   if err := store.TryAcquireMutex(ctx, &liqgengate.MutexRecord{BorrowID: "b1", Owner: "freeze:test", Kind: "freeze", Gen: 1, AcquiredAt: time.Now()}); err != nil {
		  t.Fatal(err)
	  }
	   eng := liqgengate.NewEngine(store, v, nil, liqgengate.Config{LeaseTTL: time.Minute})
	   rel, err := eng.ReleaseWithGen(ctx, liqgengate.ReleaseRequest{UserID: "u1", BorrowID: "b1", LiqGen: 0, HasGen: true})
	   if err == nil || rel.Allowed {
		  t.Fatalf("release during freeze mutex must fail: %+v %v", rel, err)
	   }
	   if rel.RejectReason != string(liqgengate.RaceMutexHeld) && rel.RejectReason != string(liqgengate.RaceUnexpectedPhase) {
		   t.Fatalf("reason %s", rel.RejectReason)
	   }
   }

   func TestConcurrentFreezeReleaseOneWins(t *testing.T) {
	  ctx := context.Background()
	  v := seedView("u1", "b1", "c1", "USDT", "9000", "10000", 9000, 6500)
	  started := make(chan struct{})
	   hook := func(string, account.Book, bool) error {
		  close(started)
		  time.Sleep(40 * time.Millisecond)
		  return nil
	  }
	  eng := newEng(t,
		  v, hook, time.Minute)
	   var freezeOK, releaseOK int32
	  var wg sync.WaitGroup
	  wg.Add(2)
	  go func() {
		   defer wg.Done()
		   res, err := eng.FreezeWithGen(ctx, liqgengate.FreezeRequest{UserID: "u1", BorrowID: "b1"})
		  if err == nil && res != nil && res.Allowed {
			   atomic.AddInt32(&freezeOK, 1)
		  }
	  }()
	   go func() {
		  defer wg.Done()
		   <-started
		  res, err := eng.ReleaseWithGen(ctx, liqgengate.ReleaseRequest{UserID: "u1", BorrowID: "b1", LiqGen: 1, HasGen: true})
		   if err == nil && res != nil && res.Allowed {
			   atomic.AddInt32(&releaseOK, 1)
		  }
	   }()
	  wg.Wait()
	  if freezeOK+releaseOK > 1 {
		  t.Fatalf("double success freeze=%d release=%d", freezeOK, releaseOK)
	   }
	  if freezeOK == 0 {
		   t.Fatalf("freeze should complete")
	   }
   }

  func TestMatchingGenReleaseClose(t *testing.T) {
	  ctx := context.Background()
	  v := seedView("u1", "b1", "c1", "USDT", "9000", "10000", 9000, 6500)
	  eng := newEng(t, v, nil, time.Minute)
	   fr, err := eng.FreezeWithGen(ctx, liqgengate.FreezeRequest{UserID: "u1", BorrowID: "b1"})
	  if err != nil {
		  t.Fatal(err)
	  }
	  rel, err := eng.ReleaseWithGen(ctx, liqgengate.ReleaseRequest{
		  UserID: "u1", BorrowID: "b1", HoldID: fr.HoldID, LiqGen: 1, HasGen: true,
	   })
	  if err != nil || !rel.Allowed {
		   t.Fatalf("release: %+v %v", rel, err)
	   }
	  cl, err := eng.CloseWithGen(ctx, liqgengate.CloseRequest{UserID: "u1", BorrowID: "b1", LiqGen: 1, HasGen: true})
	  if err != nil || !cl.Allowed || cl.Generation.Phase != liqgengate.PhaseClosed {
		   t.Fatalf("close: %+v %v", cl, err)
	  }
  }

  func TestExpiredLeaseRejectsRelease(t *testing.T) {
	  ctx := context.Background()
	  v := seedView("u1", "b1", "c1", "USDT", "8000", "10000", 8000, 6500)
	  now := time.Now().UTC()
	   eng := newEng(t, v, nil, time.Millisecond).WithClock(func() time.Time { return now })
	   if _, err := eng.FreezeWithGen(ctx, liqgengate.FreezeRequest{UserID: "u1", BorrowID: "b1"}); err != nil {
		   t.Fatal(err)
	   }
	   eng.WithClock(func() time.Time { return now.Add(2 * time.Second) })
	   rel, err := eng.ReleaseWithGen(ctx, liqgengate.ReleaseRequest{UserID: "u1", BorrowID: "b1", LiqGen: 1, HasGen: true})
	  if err == nil || rel.Allowed || rel.RejectReason != string(liqgengate.RaceExpiredLease) {
		   t.Fatalf("expired: %+v %v", rel, err)
	  }
   }

   func TestMarginhookBindStaleGen(t *testing.T) {
	  svc := marginhook.New()
	  store := liqgengate.NewMemStore()
	  eng := liqgengate.NewEngine(store, nil, nil, liqgengate.Config{LeaseTTL: time.Minute})
	   svc.WithLiqEngine(eng)
	   h, err := svc.OpenCollateral(marginhook.OpenCollateralReq{UserID: "user-liq-aa", Asset: "USDT", Amount: "10000", CapBPS: 5000})
	  if err != nil {
		  t.Fatal(err)
	   }
	   // 超 cap 才能 FreezeForLTV
	  br, err := svc.OpenBorrow(marginhook.BorrowReq{UserID: "user-liq-aa", CollateralID: h.ID, Asset: "USDT", Amount: "9000", IdempotencyKey: "idem-liq-1"})
	  if err == nil {
		  t.Fatalf("expected ltv fail for open, got %+v", br)
	  }
	   // 先开在 cap 内，再手工抬借款金额以触发扫描面——不改 CheckLTV 公式：
	   // 用低借开仓，再 PutBorrow 抬金额（夹具，不走公式写路径）。
	   br, err = svc.OpenBorrow(marginhook.BorrowReq{UserID: "user-liq-aa", CollateralID: h.ID, Asset: "USDT", Amount: "1000", IdempotencyKey: "idem-liq-2"})
	   if err != nil {
		   t.Fatal(err)
	   }
	   br.Amount = "9000"
	  br.LTVBPS = 9000
	  svc.Book().PutBorrow(br)
	  fr, err := svc.FreezeForLTV("user-liq-aa", br.ID)
	  if err != nil {
		   t.Fatal(err)
	  }
	   if err := svc.ReleaseWithGen(fr.ID, 0, true); err == nil {
		   t.Fatal("stale gen release must fail")
	  }
  }

   func TestPhaseTransitionsTable(t *testing.T) {
	  cases := []struct {
		  from, to liqgengate.LiqPhase
		   ok       bool
	   }{
		  {liqgengate.PhaseOpen, liqgengate.PhaseFreezing, true},
		   {liqgengate.PhaseFreezing,
			   liqgengate.PhaseFrozen, true},
		   {liqgengate.PhaseFreezing, liqgengate.PhaseHalfOpen, true},
		  {liqgengate.PhaseFrozen, liqgengate.PhaseReleasing, true},
		  {liqgengate.PhaseReleasing, liqgengate.PhaseClosed, true},
		  {liqgengate.PhaseHalfOpen, liqgengate.PhaseFrozen, true},
		   {liqgengate.PhaseClosed, liqgengate.PhaseOpen, false},
		  {liqgengate.PhaseFrozen, liqgengate.PhaseOpen, false},
		   {liqgengate.PhaseOpen, liqgengate.PhaseClosed, false},
	  }
	   for _, tc := range cases {
		   if got := liqgengate.CanTransitPhase(tc.from, tc.to); got != tc.ok {
			   t.Fatalf("%s→%s got %v want %v", tc.from, tc.to, got, tc.ok)
		  }
	  }
   }

  func TestAssertReleaseFence(t *testing.T) {
	  if err := liqgengate.AssertReleaseFence(nil, 1, true); !errors.Is(err, liqgengate.ErrMissingGen) {
		  t.Fatal(err)
	   }
	   g := &liqgengate.LiqGeneration{Gen: 2, Phase: liqgengate.PhaseFrozen}
	  if err := liqgengate.AssertReleaseFence(g, 1, true); !errors.Is(err, liqgengate.ErrStaleGen) {
		   t.Fatal(err)
	  }
	  if err := liqgengate.AssertReleaseFence(g, 2, false); !errors.Is(err, liqgengate.ErrMissingGen) {
		   t.Fatal(err)
	   }
	   g.Phase = liqgengate.PhaseHalfOpen
	  if err := liqgengate.AssertReleaseFence(g, 2, true); !errors.Is(err, liqgengate.ErrHalfOpen) {
		   t.Fatal(err)
	  }
   }

  func TestForceCloseRequiresNote(t *testing.T) {
	   ctx := context.Background()
	  v := seedView("u1", "b1", "c1", "USDT", "8000", "10000", 8000, 6500)
	  eng := newEng(t, v, func(string, account.Book, bool) error { return errors.New("x") }, time.Minute)
	  if _, err := eng.FreezeWithGen(ctx, liqgengate.FreezeRequest{UserID: "u1", BorrowID: "b1"}); err == nil {
		   t.Fatal("want hook fail")
	   }
	   if _, err := eng.ForceClose(ctx, "b1", ""); !errors.Is(err, liqgengate.ErrForceDenied) {
		   t.Fatal(err)
	   }
	   g, err := eng.ForceClose(ctx, "b1", "ops half close")
	  if err != nil || g.Phase != liqgengate.PhaseClosed {
		   t.Fatalf("%+v %v", g, err)
	   }
   }

  func TestIdempotentFreeze(t *testing.T) {
	  ctx := context.Background()
	  v := seedView("u1", "b1", "c1", "USDT", "8000", "10000", 8000, 6500)
	   eng := newEng(t, v, nil, time.Minute)
	  a, err := eng.FreezeWithGen(ctx, liqgengate.FreezeRequest{UserID: "u1", BorrowID: "b1", IdemKey: "k1"})
	   if err != nil {
		   t.Fatal(err)
	   }
	  b, err := eng.FreezeWithGen(ctx, liqgengate.FreezeRequest{UserID: "u1", BorrowID: "b1", IdemKey: "k1"})
	   if err != nil {
		  t.Fatal(err)
	   }
	   if a.Lease.ID != b.Lease.ID {
		  t.Fatalf("idem lease mismatch")
	  }
  }
