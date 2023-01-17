package wdrelease_test

import (
	"context"
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/wdrelease"
)

func TestDeskOccupyCommitAbort(t *testing.T) {
	d := wdrelease.NewOccDesk(wdrelease.NewOccMemStore())
	ctx := context.Background()
	tk, err := d.OpenTicket(ctx, "u", "USDT", "5")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Admit(ctx, tk.ID); err != nil {
		t.Fatal(err)
	}
	occ, err := d.BeforeFreeze(ctx, wdrelease.OccOccupyRequest{TicketID: tk.ID, GrantIdem: "g1", Amount: "5", Asset: "USDT"})
	if err != nil {
		t.Fatal(err)
	}
	if occ.Grant.State != wdrelease.OccGrantReserved {
		t.Fatal(occ.Grant.State)
	}
	if _, err := d.CommitFreeze(ctx, "g1"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.AbortFreeze(ctx, "g1"); !errors.Is(err, wdrelease.ErrAlreadyFrozen) {
		t.Fatalf("abort frozen: %v", err)
	}
}

func TestDeskExpireReopen(t *testing.T) {
	d := wdrelease.NewOccDesk(wdrelease.NewOccMemStore())
	ctx := context.Background()
	tk, _ := d.OpenTicket(ctx, "u", "BTC", "1")
	_, _ = d.Admit(ctx, tk.ID)
	_, _ = d.BeforeFreeze(ctx, wdrelease.OccOccupyRequest{TicketID: tk.ID, GrantIdem: "g2", Amount: "1", Asset: "BTC"})
	if _, err := d.Expire(ctx, tk.ID, "ops"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.CommitFreeze(ctx, "g2"); err == nil {
		t.Fatal("expired grant commit")
	}
	tk, err := d.Reopen(ctx, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Epoch != 2 || tk.State != wdrelease.OccTicketPending {
		t.Fatalf("%+v", tk)
	}
}

func TestRefuseMap(t *testing.T) {
	for p, ok := range wdrelease.ExportRefuseMap() {
		if ok {
			t.Fatalf("path %s allows unfreeze", p)
		}
	}
	if wdrelease.LiveUnfreezeAllowed() || wdrelease.PathAllowsUnfreeze(wdrelease.PathAbortFreeze) {
		t.Fatal("unfreeze leak")
	}
}

func TestTicketFSM(t *testing.T) {
	if !wdrelease.CanTransitOccTicket(wdrelease.OccTicketReserved, wdrelease.OccTicketFrozen) {
		t.Fatal("reserved→frozen")
	}
	if wdrelease.CanTransitOccTicket(wdrelease.OccTicketFrozen, wdrelease.OccTicketAborted) {
		t.Fatal("frozen cannot abort on ticket")
	}
	if !wdrelease.CanTransitOccGrant(wdrelease.OccGrantReserved, wdrelease.OccGrantFrozen) || wdrelease.CanTransitOccGrant(wdrelease.OccGrantFrozen, wdrelease.OccGrantAborted) {
		t.Fatal("grant fsm")
	}
}
