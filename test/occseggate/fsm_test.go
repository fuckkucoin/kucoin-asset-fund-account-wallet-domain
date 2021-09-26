package occseggate_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/occseggate"
)

func TestSegTransitions(t *testing.T) {
	cases := []struct {
		from, to	occseggate.SegState
		ok		bool
	}{
		{occseggate.SegReserved, occseggate.SegFrozen, true},
		{occseggate.SegReserved, occseggate.SegAborted, true},
		{occseggate.SegReserved, occseggate.SegExpiredRevoked, true},
		{occseggate.SegFrozen, occseggate.SegAborted, false},
		{occseggate.SegFrozen, occseggate.SegReserved, false},
		{occseggate.SegFrozen, occseggate.SegExpiredRevoked, true},
		{occseggate.SegAborted, occseggate.SegFrozen, false},
		{occseggate.SegAborted, occseggate.SegExpiredRevoked, true},
		{occseggate.SegExpiredRevoked, occseggate.SegReserved, false},
		{occseggate.SegExpiredRevoked, occseggate.SegFrozen, false},
	}
	for _, tc := range cases {
		got := occseggate.CanTransitSeg(tc.from,
			tc.to)
		if got != tc.ok {
			t.Fatalf("%s→%s got %v want %v", tc.from, tc.to, got, tc.ok)
		}
		err := occseggate.AssertSegTransit(tc.from, tc.to)
		if tc.ok && err != nil {
			t.Fatalf("assert %s→%s: %v", tc.from, tc.to, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("assert %s→%s want error", tc.from, tc.to)
		}
	}
}

func TestProofTransitions(t *testing.T) {
	if !occseggate.CanTransitProof(occseggate.ProofOpen, occseggate.ProofAcked) {
		t.Fatal("open→acked")
	}
	if occseggate.CanTransitProof(occseggate.ProofAcked, occseggate.ProofOpen) {
		t.Fatal("acked closed")
	}
}

func TestSegHelpers(t *testing.T) {
	if !occseggate.SegReserved.Abortable() || !occseggate.SegReserved.Committable() {
		t.Fatal("reserved")
	}
	if occseggate.SegFrozen.Abortable() || !occseggate.SegFrozen.Closed() {
		t.Fatal("frozen")
	}
	if st, ok := occseggate.SegAfterAction("commit"); !ok || st != occseggate.SegFrozen {
		t.Fatal("after commit")
	}
	if !occseggate.ActionRequiresGen("commit") || occseggate.AllowedWithoutGen("abort") {
		t.Fatal("gen required")
	}
}

func TestSnapshotEqual(t *testing.T) {
	a := occseggate.OccupancySnapshot{TicketID: "t", GrantIdem: "g", Asset: "USDT", Amount: "1", Epoch: 1}
	b := a
	if !a.Equal(b) {
		t.Fatal("equal")
	}
	b.Amount = "2"
	if a.Equal(b) {
		t.Fatal("amount")
	}
	cur := occseggate.TicketCursor{TicketID: "t", NextGen: 2, Epoch: 1}
	if a.Combined() == "" || cur.Combined() == "" {
		t.Fatal("combined")
	}
}

func TestGhostPair(t *testing.T) {
	p := &occseggate.GhostOccupancyProof{LedgerFrozen: true, AbortWouldOK: true}
	if !p.GhostPair() {
		t.Fatal("pair")
	}
	if occseggate.GhostIfAbortAfterFrozen(occseggate.SegReserved, true) {
		t.Fatal("only frozen")
	}
}
