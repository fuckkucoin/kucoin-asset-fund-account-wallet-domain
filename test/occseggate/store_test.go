package occseggate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/occseggate"
)

func TestMemStoreSegCRUD(t *testing.T) {
	s := occseggate.NewOccMemStore()
	ctx := context.Background()
	now := time.Now().UTC()
	seg := &occseggate.OccSegGen{
		ID:	"s1", TicketID: "t1", GrantIdem: "g1", Asset: "USDT", Amount: "1",
		Gen:	1, Epoch: 1, State: occseggate.SegReserved, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.InsertSeg(ctx, seg); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertSeg(ctx, seg); !errors.Is(err, occseggate.ErrDuplicate) {
		t.Fatal(err)
	}
	got, err := s.GetSegByGrantIdem(ctx, "g1")
	if err != nil || got.ID != "s1" {
		t.Fatal(err)
	}
	got.State = occseggate.SegFrozen
	if err := s.UpdateSeg(ctx, got); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListSegsByTicket(ctx, "t1", 10)
	if len(list) != 1 {
		t.Fatal(len(list))
	}
	act, _ := s.ListActiveSegs(ctx, 10)
	if len(act) != 0 {
		t.Fatal("frozen not active")
	}
}

func TestMemStoreMutex(t *testing.T) {
	s := occseggate.NewOccMemStore()
	ctx := context.Background()
	rec := &occseggate.MutexRecord{GrantIdem: "g", Owner: "a", Kind: occseggate.MutexKindCommit, AcquiredAt: time.Now()}
	if err := s.TryAcquireMutex(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if err := s.TryAcquireMutex(ctx, &occseggate.MutexRecord{GrantIdem: "g", Owner: "b", Kind: occseggate.MutexKindAbort}); !errors.Is(err, occseggate.ErrMutexHeld) {
		t.Fatal(err)
	}
	if err := s.ReleaseMutex(ctx, "g", "a"); err != nil {
		t.Fatal(err)
	}
}

func TestMemStoreProofAndRevoke(t *testing.T) {
	s := occseggate.NewOccMemStore()
	ctx := context.Background()
	now := time.Now().UTC()
	p := &occseggate.GhostOccupancyProof{ID: "p1", IdemKey: "i1", State: occseggate.ProofOpen, CreatedAt: now, UpdatedAt: now, Kind: occseggate.ProofKindGhost}
	if err := s.InsertProof(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetProofByIdem(ctx, "i1"); err != nil {
		t.Fatal(err)
	}
	r := &occseggate.ExpireRevocation{ID: "r1", TicketID: "t", IdemKey: "ri", State: "applied", CreatedAt: now, UpdatedAt: now}
	if err := s.InsertRevocation(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertAudit(ctx, &occseggate.OccAudit{ID: "a", TicketID: "t", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertCursor(ctx, &occseggate.TicketCursor{TicketID: "t", NextGen: 2, Epoch: 1, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if c, err := s.GetCursor(ctx, "t"); err != nil || c.NextGen != 2 {
		t.Fatal(err)
	}
}

func TestModelSnapshots(t *testing.T) {
	s := &occseggate.OccSegGen{TicketID: "t", GrantIdem: "g", Asset: "USDT", Amount: "1", Gen: 1, Epoch: 1, State: occseggate.SegReserved}
	if s.ScopeKey() == "" || s.Snapshot() == "" || !s.Matches("g", 1) {
		t.Fatal("snap")
	}
	if !(&occseggate.ExpireRevocation{State: "applied"}).Closed() {
		t.Fatal("rev")
	}
}
