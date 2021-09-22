package liqgengate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/liqgengate"
)

   func TestMemStoreGenerationRoundtrip(t *testing.T) {
	  ctx := context.Background()
	   m := liqgengate.NewMemStore()
	  now := time.Now().UTC()
	  g := &liqgengate.LiqGeneration{ID: "g1", BorrowID: "b1", UserID: "u1", Gen: 1, Phase: liqgengate.PhaseOpen, CreatedAt: now, UpdatedAt: now}
	   if err := m.InsertGeneration(ctx, g); err != nil {
		   t.Fatal(err)
	  }
	   if err := m.InsertGeneration(ctx, g); !errors.Is(err, liqgengate.ErrDuplicate) {
		   t.Fatal(err)
	  }
	  got, err := m.GetGeneration(ctx, "b1")
	   if err != nil || got.Gen != 1 {
		  t.Fatalf("%+v %v", got, err)
	   }
	   got.Gen = 2
	  got.Phase = liqgengate.PhaseFrozen
	  if err := m.UpdateGeneration(ctx, got); err != nil {
		  t.Fatal(err)
	  }
	   again, _ := m.GetGeneration(ctx, "b1")
	   if again.Gen != 2 {
		  t.Fatal(again.Gen)
	  }
   }

   func TestMemStoreLeaseActiveIndex(t *testing.T) {
	   ctx := context.Background()
	  m := liqgengate.NewMemStore()
	  now := time.Now().UTC()
	  l := &liqgengate.LiqFreezeLease{
		   ID: "l1", BorrowID: "b1", UserID: "u1", State: liqgengate.LeaseFrozen,
		   IdemKey: "idem-l", IssuedAt: now, ExpiresAt: now.Add(time.Minute), CreatedAt: now, UpdatedAt: now,
	  }
	  if err := m.InsertLease(ctx, l); err != nil {
		   t.Fatal(err)
	  }
	   act, err := m.GetActiveLease(ctx, "b1")
	  if err != nil || act.ID != "l1" {
		  t.Fatal(err)
	  }
	  l.State = liqgengate.LeaseConsumed
	   if err := m.UpdateLease(ctx, l); err != nil {
		  t.Fatal(err)
	  }
	   if _, err := m.GetActiveLease(ctx, "b1"); !errors.Is(err, liqgengate.ErrNotFound) {
		  t.Fatal(err)
	  }
  }

  func TestMemStoreMutexOwner(t *testing.T) {
	  ctx := context.Background()
	  m := liqgengate.NewMemStore()
	   rec := &liqgengate.MutexRecord{BorrowID: "b1", Owner: "a", Kind: "freeze", AcquiredAt: time.Now()}
	  if err := m.TryAcquireMutex(ctx, rec); err != nil {
		  t.Fatal(err)
	  }
	  if err := m.TryAcquireMutex(ctx, &liqgengate.MutexRecord{BorrowID: "b1", Owner: "b", Kind: "release"}); !errors.Is(err, liqgengate.ErrMutexHeld) {
		   t.Fatal(err)
	   }
	  if err := m.ReleaseMutex(ctx, "b1", "b"); !errors.Is(err, liqgengate.ErrMutexHeld) {
		   t.Fatal(err)
	  }
	   if err := m.ReleaseMutex(ctx, "b1", "a"); err != nil {
		  t.Fatal(err)
	   }
	   if err := m.TryAcquireMutex(ctx, &liqgengate.MutexRecord{BorrowID: "b1", Owner: "b", Kind: "release"}); err != nil {
		   t.Fatal(err)
	  }
   }

  func TestMemStoreProofIdem(t *testing.T) {
	  ctx := context.Background()
	   m := liqgengate.NewMemStore()
	  p := &liqgengate.LiqProof{ID: "p1", Kind: liqgengate.ProofKindRace, BorrowID: "b1", IdemKey: "idem-p", State: liqgengate.ProofOpen, CreatedAt: time.Now()}
	  if err := m.InsertProof(ctx, p); err != nil {
		   t.Fatal(err)
	   }
	   if err := m.InsertProof(ctx, &liqgengate.LiqProof{ID: "p2", IdemKey: "idem-p"}); !errors.Is(err, liqgengate.ErrDuplicate) {
		   t.Fatal(err)
	   }
	  got, err := m.GetProofByIdem(ctx, "idem-p")
	   if err != nil || got.ID != "p1" {
		   t.Fatal(err)
	  }
  }

  func TestSQLStoreUnavailable(t *testing.T) {
	  s := liqgengate.NewSQLStore(nil)
	  if _, err := s.GetGeneration(context.Background(), "x"); !errors.Is(err, liqgengate.ErrUnavailable) {
		   t.Fatal(err)
	   }
  }
