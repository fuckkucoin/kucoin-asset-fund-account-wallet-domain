package memoeffectgate_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/memoeffectgate"
)

func TestInspectAndInvariants(t *testing.T) {
	eng, book, _, _ := testEng(t)
	tk := pendingTicket(book, "ETH", "0xview", "m")
	v, err := eng.InspectTicket(context.Background(), tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !v.FailClosed {
		t.Fatal("no effect must fail-closed")
	}
	resp := mustApprove(t, eng, tk.ID, "ETH")
	if !memoeffectgate.ApproveSuccessInvariant(resp) {
		t.Fatal("approve invariant")
	}
	v, err = eng.InspectTicket(context.Background(), tk.ID)
	if err != nil || v.Effect == nil || v.Fence == nil {
		t.Fatalf("%v %#v", err, v)
	}
	n, err := eng.CountCredited(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	d, _ := eng.DecideMemoGuarded(context.Background(), memoeffectgate.DecideMemoGuardedReq{
		TicketID:	tk.ID, Status: "rejected", Actor: "ops",
	})
	if !memoeffectgate.RejectGuardInvariant(d, true) {
		t.Fatalf("%#v", d)
	}
	if !memoeffectgate.SilentKeepInvariant(*d.Proof) {
		t.Fatal("silent")
	}
	open, err := eng.CountOpenHalf(context.Background())
	if err != nil || open < 1 {
		t.Fatalf("%d %v", open, err)
	}
}
