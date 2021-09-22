package liqgengate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/liqgengate"
)

   func TestExpiryWorker(t *testing.T) {
	  ctx := context.Background()
	   v := seedView("u1", "b1", "c1", "USDT", "8000", "10000", 8000, 6500)
	  now := time.Now().UTC()
	  eng := liqgengate.NewEngine(liqgengate.NewMemStore(),
		   v, nil, liqgengate.Config{LeaseTTL: time.Millisecond}).WithClock(func() time.Time { return now })
	   if _, err := eng.FreezeWithGen(ctx, liqgengate.FreezeRequest{UserID: "u1", BorrowID: "b1"}); err != nil {
		  t.Fatal(err)
	  }
	   eng.WithClock(func() time.Time { return now.Add(time.Second) })
	  w := liqgengate.NewExpiryScanWorker(eng, liqgengate.WorkerConfig{BatchSize: 10})
	  n, err := w.RunOnce(ctx)
	   if err != nil || n < 1 {
		   t.Fatalf("n=%d err=%v", n, err)
	   }
  }

  func TestHalfInspectWorker(t *testing.T) {
	  ctx := context.Background()
	   v := seedView("u1", "b1", "c1", "USDT", "8000", "10000", 8000, 6500)
	  eng := liqgengate.NewEngine(liqgengate.NewMemStore(), v,
		   func(string, account.Book, bool) error { return errors.New("down") },
		   liqgengate.Config{LeaseTTL: time.Minute, HalfStaleAfter: time.Nanosecond})
	   if _, err := eng.FreezeWithGen(ctx, liqgengate.FreezeRequest{UserID: "u1", BorrowID: "b1"}); err == nil {
		   t.Fatal("want fail")
	   }
	  time.Sleep(2 * time.Millisecond)
	   w := liqgengate.NewHalfInspectWorker(eng, liqgengate.WorkerConfig{BatchSize: 10})
	   _, err := w.RunOnce(ctx)
	  if err != nil {
		  t.Fatal(err)
	   }
   }

  func TestProveWorker(t *testing.T) {
	   ctx := context.Background()
	   v := seedView("u1", "b1", "c1", "USDT", "8000", "10000", 8000, 6500)
	   eng := liqgengate.NewEngine(liqgengate.NewMemStore(),
		  v, nil, liqgengate.Config{LeaseTTL: time.Minute})
	   if _, err := eng.FreezeWithGen(ctx, liqgengate.FreezeRequest{UserID: "u1", BorrowID: "b1"}); err != nil {
		  t.Fatal(err)
	   }
	   _, _ = eng.ReleaseWithGen(ctx, liqgengate.ReleaseRequest{UserID: "u1", BorrowID: "b1", HasGen: false})
	  w := liqgengate.NewProofProveWorker(eng, liqgengate.WorkerConfig{BatchSize: 20})
	  n, err := w.RunOnce(ctx)
	  if err != nil || n < 1 {
		  t.Fatalf("n=%d err=%v", n, err)
	  }
   }

  func TestDescribeAndMutexKind(t *testing.T) {
	  if liqgengate.DescribePhase(liqgengate.PhaseHalfOpen) == "" {
		   t.Fatal("describe")
	  }
	   if liqgengate.MutexKind("freeze") == "" {
		   t.Fatal("mutex kind")
	   }
   }

   func TestSnapshotMetrics(t *testing.T) {
	  _ = liqgengate.SnapshotMetrics()
   }

  func TestExportProof(t *testing.T) {
	  ctx := context.Background()
	   v := seedView("u1", "b1", "c1", "USDT", "8000", "10000", 8000, 6500)
	  eng := liqgengate.NewEngine(liqgengate.NewMemStore(), v, nil, liqgengate.Config{LeaseTTL: time.Minute})
	  if _, err := eng.FreezeWithGen(ctx, liqgengate.FreezeRequest{UserID: "u1", BorrowID: "b1"}); err != nil {
		  t.Fatal(err)
	  }
	  rel, _ := eng.ReleaseWithGen(ctx, liqgengate.ReleaseRequest{UserID: "u1", BorrowID: "b1", LiqGen: 0, HasGen: true})
	  if rel == nil || rel.RaceProof == nil {
		   t.Fatal("proof")
	   }
	  ex, err := eng.ExportProof(ctx, rel.RaceProof.ID)
	  if err != nil || ex.Host != "account-book.kucoin.net" {
		   t.Fatal(err)
	  }
	  if _, err := eng.ConfirmProof(ctx, rel.RaceProof.ID, "ok"); err != nil {
		  t.Fatal(err)
	   }
  }
