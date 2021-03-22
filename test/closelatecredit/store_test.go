package closelatecredit_test

import (
	"context"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/closelatecredit"
)

   func TestMemStoreEpochRoundtrip(t *testing.T) {
	  ctx := context.Background()
	   s := closelatecredit.NewMemStore()
	   now := time.Date(2021, 8, 24, 1, 0, 0, 0, time.UTC)
	  ep := &closelatecredit.CloseEpoch{
		  ID: "e1", UserID: "u", Book: "trading", N: 1, State: closelatecredit.EpochOpen,
		   CreatedAt: now, UpdatedAt: now,
	   }
	   if err := s.InsertEpoch(ctx, ep); err != nil {
		  t.Fatal(err)
	   }
	  got, err := s.GetEpochByUserBook(ctx, "u", "trading")
	  if err != nil || got.N != 1 {
		  t.Fatalf("%+v %v", got, err)
	   }
	  got.State = closelatecredit.EpochClosing
	  if err := s.UpdateEpoch(ctx, got); err != nil {
		   t.Fatal(err)
	   }
	  list, err := s.ListEpochsByState(ctx, closelatecredit.EpochClosing, 10)
	  if err != nil || len(list) != 1 {
		  t.Fatalf("%v %v", list, err)
	  }
  }

  func TestMemStoreProofHoldPending(t *testing.T) {
	  ctx := context.Background()
	  s := closelatecredit.NewMemStore()
	   now := time.Now().UTC()
	  p := &closelatecredit.CloseLateCreditProof{
		   ID: "p1", UserID: "u", Source: closelatecredit.SourceObserve, Asset: "USDT", Amount: "1",
		   Reason: closelatecredit.ReasonClosedDispute, State: closelatecredit.LateDisputed,
		  IdemKey: "idem-p", Digest: "abc", CreatedAt: now, UpdatedAt: now,
	   }
	  if err := s.InsertProof(ctx, p); err != nil {
		  t.Fatal(err)
	  }
	   if _, err := s.GetProofByIdem(ctx, "idem-p"); err != nil {
		   t.Fatal(err)
	  }
	   h := &closelatecredit.DisputeHoldCase{
		  ID: "h1", ProofID: "p1", UserID: "u", Asset: "USDT", Amount: "1",
		  Source: closelatecredit.SourceObserve, State: closelatecredit.HoldParked,
		   IdemKey: "idem-h", CreatedAt: now, UpdatedAt: now,
	  }
	   if err := s.InsertHold(ctx, h); err != nil {
		   t.Fatal(err)
	  }
	   if _, err := s.GetHoldByProof(ctx, "p1"); err != nil {
		   t.Fatal(err)
	  }
	  pend := &closelatecredit.PendingCredit{ID: "n1", UserID: "u", Source: closelatecredit.SourceOutbox, Asset: "USDT", Amount: "1", SeenAt: now}
	   if err := s.InsertPending(ctx, pend); err != nil {
		  t.Fatal(err)
	   }
	   rows, err := s.ListUnscannedPending(ctx, 5)
	   if err != nil || len(rows) != 1 {
		  t.Fatalf("%v %v", rows, err)
	  }
  }

   func TestMemStoreConcurrentInsert(t *testing.T) {
	   s := closelatecredit.NewMemStore()
	  ctx := context.Background()
	   var wg sync.WaitGroup
	   for i := 0; i < 16; i++ {
		  wg.Add(1)
		  go func(i int) {
			  defer wg.Done()
			   id := string(rune('a' + i))
			  _ = s.InsertEpoch(ctx, &closelatecredit.CloseEpoch{
				  ID: id, UserID: "u-" + id, Book: "trading", N: 1, State: closelatecredit.EpochOpen,
				  CreatedAt: time.Now(), UpdatedAt: time.Now(),
			   })
		   }(i)
	  }
	   wg.Wait()
	   rows, err := s.ListEpochsByState(ctx, closelatecredit.EpochOpen, 100)
	   if err != nil || len(rows) != 16 {
		  t.Fatalf("n=%d err=%v", len(rows), err)
	   }
   }

  func TestSQLStoreUnavailable(t *testing.T) {
	   s := closelatecredit.NewSQLStore(nil)
	   if _, err := s.GetEpoch(context.Background(), "x"); err != closelatecredit.ErrUnavailable {
		  t.Fatalf("%v", err)
	   }
   }
