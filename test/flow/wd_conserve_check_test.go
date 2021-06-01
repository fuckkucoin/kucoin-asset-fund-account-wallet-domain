package flow

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/idemstore"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/wdguard"
	"account-assets.kucoin.net/funding/internal/withdraw"
	"github.com/shopspring/decimal"
)

   func TestWithdrawConserveIdemLiveAbuse(t *testing.T) {
	  ctx := context.Background()
	  s := withdraw.New(nil)
	  if _, err := s.TripAbuse("ab:u1:USDT", "x"); err == nil {
		   t.Fatal("non wd key")
	  }
	   if err := wdguard.RequireAbuseKey("wd:u1:USDT"); err != nil {
		   t.Fatal(err)
	  }
	  if _, err := s.TripAbuse("wd:u1:USDT", "spam"); err != nil {
		   t.Fatal(err)
	  }
	   if err := s.AllowNew("u1", "USDT"); !errors.Is(err, withdraw.ErrAbuseBlocked) {
		   t.Fatalf("abuse block %v", err)
	  }
	  if err := s.AllowNew("u1", "ETH"); err != nil {
		  t.Fatalf("other asset %v", err)
	  }
	   s2 := withdraw.New(nil)
	   s2.Runs().Put(withdraw.Run{
		   ID: "r1", TransferID: "t1", UserID: "u1", Asset: "USDT", Amount: "3",
		  Freeze: withdraw.FreezeBroadcast, Step: withdraw.StepSubmitted,
	   })
	   s2.Broadcasts().Track("t1", "ETH", "", "USDT", "3")
	   rep := s2.ScanConserve()
	   if len(rep.Blocks) != 1 {
		  t.Fatalf("blocks %+v", rep)
	  }
	  if err := s2.AllowNew("u1", "USDT"); !errors.Is(err, withdraw.ErrConserveBlocked) {
		   t.Fatalf("conserve %v", err)
	  }
	  if err := s2.AllowNew("u1", "BTC"); err != nil {
		   t.Fatalf("other asset blocked %v", err)
	  }
	  s3 := withdraw.New(nil)
	   if _, err := s3.Broadcasts().Track("t2", "ETH", "0xabc", "USDT", "1"); err != nil {
		  t.Fatal(err)
	   }
	   s3.Runs().Put(withdraw.Run{
		  ID: "r2", TransferID: "t2", UserID: "u1", Asset: "USDT", Amount: "1",
		   Freeze: withdraw.FreezeSigning, Step: withdraw.StepSigned,
	  })
	   if _, err := s3.RecoverStuck("t2", time.Now().UTC().Add(3*time.Hour)); !errors.Is(err, withdraw.ErrLiveHashUnfreeze) {
		  t.Fatalf("live %v", err)
	  }
	   if _, err := s3.ManualAdvance(ctx, "t2", withdraw.FreezeCancelled, "ops-a", "ops-a", "no"); !errors.Is(err, withdraw.ErrDualApproval) {
		   t.Fatalf("dual %v", err)
	   }
	  rec, err := s3.ManualAdvance(ctx, "t2", withdraw.FreezeCancelled, "ops-a", "ops-b", "stop")
	  if !errors.Is(err, withdraw.ErrLiveHashUnfreeze) || rec.Unfrozen {
		   t.Fatalf("manual unfreeze %+v %v", rec, err)
	  }
	  if n, err := s3.SweepStuck(time.Now().UTC(), time.Hour, 10); err != nil || n != 0 {
		   t.Fatalf("sweep %d %v", n, err)
	   }
	  s4 := withdraw.New(nil)
	   if _, err := s4.Broadcasts().Track("t3", "ETH", "", "USDT", "1"); err != nil {
		   t.Fatal(err)
	  }
	  row, _ := s4.Broadcasts().Get("t3")
	   if withdraw.LiveHash(row.TxHash) || withdraw.RefuseAutoUnfreeze(row) != nil {
		   t.Fatal("signed empty should allow")
	   }
	  act, err := idemstore.Decide(idemstore.ScopeWithdraw, &idemstore.Record{
		   Scope: idemstore.ScopeWithdraw, Key: "k", Fingerprint: "fp", Status: idemstore.StatusDone, RefID: "t",
	   }, "fp", true)
	  if err != nil || act != idemstore.DeskReplay {
		  t.Fatalf("replay %s %v", act, err)
	  }
	   act, err = idemstore.Decide(idemstore.ScopeWithdraw, &idemstore.Record{
		   Scope: idemstore.ScopeWithdraw, Key: "k", Fingerprint: "fp", Status: idemstore.StatusInflight,
	   }, "fp", true)
	  if err != nil || act != idemstore.DeskContinue {
		   t.Fatalf("continue %s %v", act, err)
	  }
	   act, err = idemstore.Decide(idemstore.ScopeWithdraw, &idemstore.Record{
		  Scope: idemstore.ScopeWithdraw, Key: "k", Fingerprint: "fp", Status: idemstore.StatusInflight,
	   }, "fp", false)
	  if err != nil || act != idemstore.DeskIncident {
		   t.Fatalf("incident %s %v", act, err)
	   }
	   act, err = idemstore.Decide(idemstore.ScopeWithdraw, &idemstore.Record{
		   Scope: idemstore.ScopeWithdraw, Key: "k", Fingerprint: "other", Status: idemstore.StatusDone, RefID: "t",
	  }, "fp", false)
	   if !errors.Is(err, idemstore.ErrConflict) || act != idemstore.DeskConflict {
		  t.Fatalf("conflict %s %v", act, err)
	   }
	  // 跳闸后在途筛查不得被恢复解冻。
	  s5 := withdraw.New(nil)
	   if _, err := s5.TripAbuse("wd:u9:USDT", "spam"); err != nil {
		  t.Fatal(err)
	  }
	   if _, err := s5.Broadcasts().Track("t9", "ETH", "", "USDT", "1"); err != nil {
		  t.Fatal(err)
	   }
	   s5.Runs().Put(withdraw.Run{
		   ID: "r9", TransferID: "t9", UserID: "u9", Asset: "USDT", Amount: "1",
		   Freeze: withdraw.FreezeScreening, Step: withdraw.StepReview,
	  })
	   // 把更新时间推旧，让分类进入 recover。
	  row9, _ := s5.Broadcasts().Get("t9")
	  _ = row9
	  if _, err := s5.RecoverStuck("t9", time.Now().UTC().Add(2*time.Hour)); !errors.Is(err, withdraw.ErrAbuseInFlight) {
		   t.Fatalf("abuse unfreeze %v", err)
	  }
	  rt := openMem(t)
	   uid := seedUser(rt)
	  to := rt.Wallet.AddressFor(ctx, uid, "ETH")
	  hash, err := rt.Wallet.InjectInbound("ETH", to, "USDT", decimal.RequireFromString("4"))
	   if err != nil {
		   t.Fatal(err)
	  }
	  dep := deposit.New(rt.Ledger, rt.Wallet)
	  dtr, err := dep.Observe(ctx, deposit.ObserveReq{
		   UserID: uid, Chain: "ETH", TxHash: hash, Asset: "USDT",
		  IdempotencyKey: "wd-cons-dep", WaitConfirm: true,
	   })
	  if err != nil || dtr == nil || dtr.State != ledger.StateCredited {
		   t.Fatalf("deposit %v %+v", err, dtr)
	   }
	  wd := withdraw.New(rt.Ledger)
	  wdest := extDest("ETH")
	   first, err := wd.Request(ctx, withdraw.Request{
		   UserID: uid, Asset: "USDT", Amount: decimal.RequireFromString("1"),
		  Chain: "ETH", ToAddress: wdest, IdempotencyKey: "wd-cons-1",
	  })
	   if err != nil || first == nil {
		   t.Fatalf("first withdraw %v", err)
	  }
	  second, err := wd.Request(ctx, withdraw.Request{
		   UserID: uid, Asset: "USDT", Amount: decimal.RequireFromString("1"),
		   Chain: "ETH", ToAddress: wdest, IdempotencyKey: "wd-cons-1",
	   })
	   if err != nil || second == nil || second.ID != first.ID {
		  t.Fatalf("replay %+v %v", second, err)
	   }
	  if _, err := wd.Request(ctx, withdraw.Request{
		   UserID: uid, Asset: "USDT", Amount: decimal.RequireFromString("2"),
		  Chain: "ETH", ToAddress: wdest, IdempotencyKey: "wd-cons-1",
	   }); !errors.Is(err, withdraw.ErrIdemConflict) {
		   t.Fatalf("conflict %v", err)
	  }
	  acc, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	  if err != nil {
		  t.Fatal(err)
	  }
	   if !acc.Frozen.Equal(decimal.RequireFromString("1")) {
		  t.Fatalf("frozen doubled %s", acc.Frozen)
	  }
	  kv := persist.NewMemKV()
	   src := withdraw.New(nil)
	  if err := src.BindGuardKV(kv); err != nil {
		   t.Fatal(err)
	  }
	   if _, err := src.TripAbuse("wd:u7:USDT", "spam"); err != nil {
		  t.Fatal(err)
	  }
	   src.Runs().Put(withdraw.Run{
		  ID: "rb", TransferID: "tb", UserID: "u8", Asset: "USDT", Amount: "2",
		   Freeze: withdraw.FreezeBroadcast, Step: withdraw.StepSubmitted,
	  })
	  if _, err := src.Broadcasts().Track("tb", "ETH", "", "USDT", "2"); err != nil {
		  t.Fatal(err)
	   }
	  if rep := src.ScanConserve(); len(rep.Blocks) != 1 || rep.PersistErr != "" {
		  t.Fatalf("scan %+v", rep)
	   }
	  if _, err := src.Broadcasts().Track("tl", "ETH", "0xabc", "USDT", "1"); err != nil {
		   t.Fatal(err)
	   }
	  src.Runs().Put(withdraw.Run{
		   ID: "rl", TransferID: "tl", UserID: "u8", Asset: "USDT", Amount: "1",
		  Freeze: withdraw.FreezeBroadcast, Step: withdraw.StepSubmitted,
	  })
	  if _, err := src.RecoverStuck("tl", time.Now().UTC().Add(3*time.Hour)); !errors.Is(err, withdraw.ErrLiveHashUnfreeze) {
		   t.Fatalf("persist live %v", err)
	   }
	   dst := withdraw.New(nil)
	  if err := dst.BindGuardKV(kv); err != nil {
		  t.Fatal(err)
	   }
	  if err := dst.AllowNew("u7", "USDT"); !errors.Is(err, withdraw.ErrAbuseBlocked) {
		  t.Fatalf("hydrated abuse %v", err)
	   }
	   if err := dst.AllowNew("u7", "BTC"); err != nil {
		  t.Fatalf("hydrated other asset %v", err)
	   }
	  if err := dst.AllowNew("u8", "USDT"); !errors.Is(err, withdraw.ErrConserveBlocked) {
		   t.Fatalf("hydrated block %v", err)
	   }
	  if len(dst.LiveDenies()) == 0 {
		   t.Fatal("hydrated live deny empty")
	   }
  }

  func TestColdScanRetainsHydratedBlock(t *testing.T) {
	  kv := persist.NewMemKV()
	   src := withdraw.New(nil)
	  if err := src.BindGuardKV(kv); err != nil {
		   t.Fatal(err)
	   }
	  src.Runs().Put(withdraw.Run{
		  ID: "rb", TransferID: "tb", UserID: "u8", Asset: "USDT", Amount: "2",
		   Freeze: withdraw.FreezeBroadcast, Step: withdraw.StepSubmitted,
	   })
	  if _, err := src.Broadcasts().Track("tb", "ETH", "", "USDT", "2"); err != nil {
		  t.Fatal(err)
	  }
	  if rep := src.ScanConserve(); len(rep.Blocks) != 1 || rep.PersistErr != "" {
		  t.Fatalf("scan %+v", rep)
	  }
	   cold := withdraw.New(nil)
	  if err := cold.BindGuardKV(kv); err != nil {
		  t.Fatal(err)
	   }
	  if rep := cold.ScanConserve(); len(rep.Blocks) != 1 {
		   t.Fatalf("cold scan dropped %+v", rep)
	  }
	   if err := cold.AllowNew("u8", "USDT"); !errors.Is(err, withdraw.ErrConserveBlocked) {
		  t.Fatalf("cold allow %v", err)
	  }
	  if err := cold.AllowNew("u8", "BTC"); err != nil {
		   t.Fatalf("other asset %v", err)
	   }
	  src.Runs().Put(withdraw.Run{
		  ID: "rb", TransferID: "tb", UserID: "u8", Asset: "USDT", Amount: "2",
		  Freeze: withdraw.FreezeSucceeded, Step: withdraw.StepConfirmed,
	   })
	   if _, err := src.Broadcasts().Track("tb", "ETH", "0xabc", "USDT", "2"); err != nil {
		   t.Fatal(err)
	   }
	   if rep := src.ScanConserve(); len(rep.Blocks) != 0 || rep.PersistErr != "" {
		  t.Fatalf("resolved scan %+v", rep)
	   }
	   ctx := context.Background()
	  rt := openMem(t)
	   uid := seedUser(rt)
	  to := rt.Wallet.AddressFor(ctx, uid, "ETH")
	   hash, err := rt.Wallet.InjectInbound("ETH", to, "USDT", decimal.RequireFromString("4"))
	   if err != nil {
		  t.Fatal(err)
	   }
	   dep := deposit.New(rt.Ledger, rt.Wallet)
	  if _, err := dep.Observe(ctx, deposit.ObserveReq{
		   UserID: uid, Chain: "ETH", TxHash: hash, Asset: "USDT",
		   IdempotencyKey: "wd-cold-dep", WaitConfirm: true,
	   }); err != nil {
		   t.Fatal(err)
	   }
	   kv2 := persist.NewMemKV()
	  wd := withdraw.New(rt.Ledger)
	   if err := wd.BindGuardKV(kv2); err != nil {
		   t.Fatal(err)
	   }
	   tr, err := wd.Request(ctx, withdraw.Request{
		   UserID: uid, Asset: "USDT", Amount: decimal.RequireFromString("1"),
		   Chain: "ETH", ToAddress: extDest("ETH"), IdempotencyKey: "wd-cold-1",
	   })
	   if err != nil || tr == nil {
		   t.Fatalf("withdraw %v", err)
	  }
	  runs := wd.Runs().List()
	  if len(runs) != 1 {
		   t.Fatalf("runs %d", len(runs))
	   }
	   run := runs[0]
	  run.Freeze = withdraw.FreezeBroadcast
	   wd.Runs().Put(run)
	   if rep := wd.ScanConserve(); len(rep.Blocks) != 1 {
		   t.Fatalf("ledger scan %+v", rep)
	   }
	  next := withdraw.New(rt.Ledger)
	   if err := next.BindGuardKV(kv2); err != nil {
		   t.Fatal(err)
	   }
	  if rep := next.ScanConserve(); len(rep.Blocks) != 1 || rep.PersistErr != "" {
		   t.Fatalf("restart scan %+v", rep)
	  }
	  if err := next.AllowNew(uid, "USDT"); !errors.Is(err, withdraw.ErrConserveBlocked) {
		   t.Fatalf("restart allow %v", err)
	   }
  }

   func TestPhaseRestartHoldsUnfreezeAndReplay(t *testing.T) {
	   ctx := context.Background()
	   rt := openMem(t)
	  uid := seedUser(rt)
	   to := rt.Wallet.AddressFor(ctx, uid, "ETH")
	   hash, err := rt.Wallet.InjectInbound("ETH", to, "USDT", decimal.RequireFromString("4"))
	  if err != nil {
		   t.Fatal(err)
	   }
	  dep := deposit.New(rt.Ledger, rt.Wallet)
	  if _, err := dep.Observe(ctx, deposit.ObserveReq{
		  UserID: uid, Chain: "ETH", TxHash: hash, Asset: "USDT",
		  IdempotencyKey: "wd-phase-dep", WaitConfirm: true,
	   }); err != nil {
		  t.Fatal(err)
	   }
	  kv := persist.NewMemKV()
	  wd := withdraw.New(rt.Ledger)
	   if err := wd.BindGuardKV(kv); err != nil {
		  t.Fatal(err)
	   }
	  wdest := extDest("ETH")
	  first, err := wd.Request(ctx, withdraw.Request{
		   UserID: uid, Asset: "USDT", Amount: decimal.RequireFromString("1"),
		   Chain: "ETH", ToAddress: wdest, IdempotencyKey: "wd-phase-1",
	  })
	  if err != nil || first == nil {
		  t.Fatalf("first %v", err)
	  }
	   wd2 := withdraw.New(rt.Ledger)
	  if err := wd2.BindGuardKV(kv); err != nil {
		  t.Fatal(err)
	   }
	   second, err := wd2.Request(ctx, withdraw.Request{
		  UserID: uid, Asset: "USDT", Amount: decimal.RequireFromString("1"),
		  Chain: "ETH", ToAddress: wdest, IdempotencyKey: "wd-phase-1",
	  })
	   if err != nil || second == nil || second.ID != first.ID {
		  t.Fatalf("replay after restart %+v %v", second, err)
	  }
	  brun, ok := wd2.Runs().ByTransfer(first.ID)
	  if !ok {
		   t.Fatal("broadcast run missing")
	   }
	  if brun.Freeze != withdraw.FreezeBroadcast {
		   if err := wd2.MarkFreezePath(ctx, &brun, []withdraw.FreezeState{
			  withdraw.FreezeScreening, withdraw.FreezeApproved, withdraw.FreezeSigning, withdraw.FreezeBroadcast,
		   }, "desk", "broadcast"); err != nil {
			   t.Fatal(err)
		  }
		   wd2.Runs().Put(brun)
	   }
	   wd3 := withdraw.New(rt.Ledger)
	   if err := wd3.BindGuardKV(kv); err != nil {
		  t.Fatal(err)
	   }
	   if st, ok := wd3.RunFreeze(first.ID); !ok || st != withdraw.FreezeBroadcast {
		  t.Fatalf("phase %s %v", st, ok)
	   }
	   if _, err := wd3.Broadcasts().Track(first.ID, "ETH", "", "USDT", "1"); err != nil {
		  t.Fatal(err)
	   }
	  if _, err := wd3.RecoverStuck(first.ID, time.Now().UTC().Add(3*time.Hour)); !errors.Is(err, withdraw.ErrFreezeUnlockDenied) {
		   t.Fatalf("broadcast phase %v", err)
	  }
	  if _, err := wd3.TripAbuse("wd:"+uid+":USDT", "spam"); err != nil {
		   t.Fatal(err)
	  }
	  if _, err := wd3.RecoverStuck(first.ID, time.Now().UTC().Add(3*time.Hour)); !errors.Is(err, withdraw.ErrAbuseInFlight) {
		   t.Fatalf("abuse in flight %v", err)
	  }
	  if _, err := wd3.Broadcasts().Track(first.ID, "ETH", "0xabc", "USDT", "1"); err != nil {
		   t.Fatal(err)
	   }
	   rec, err := wd3.ManualAdvance(ctx, first.ID, withdraw.FreezeCancelled, "ada", "bob", "live hash")
	  if !errors.Is(err, withdraw.ErrLiveHashUnfreeze) || rec.Unfrozen {
		  t.Fatalf("manual %+v %v", rec, err)
	   }
	   acc, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	   if err != nil {
		   t.Fatal(err)
	  }
	  if !acc.Frozen.Equal(decimal.RequireFromString("1")) {
		   t.Fatalf("frozen %s", acc.Frozen)
	   }
   }
