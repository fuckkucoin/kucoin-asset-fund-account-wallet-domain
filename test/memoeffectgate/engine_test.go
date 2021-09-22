package memoeffectgate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/memoeffectgate"
)

func TestApproveHappyPath(t *testing.T) {
	eng, book, cred, _ := testEng(t)
	tk := pendingTicket(book, "XRP", "0xabc", "memo-1")
	resp := mustApprove(t, eng, tk.ID, "XRP")
	if resp.Ticket.Status != "approved" {
		t.Fatalf("status %s", resp.Ticket.Status)
	}
	if resp.Effect.State != memoeffectgate.EffectCredited {
		t.Fatalf("effect %s", resp.Effect.State)
	}
	if resp.Effect.ID == "" || resp.Effect.OutputKey == "" {
		t.Fatalf("missing effect bind %#v", resp.Effect)
	}
	if resp.Fence.Phase != memoeffectgate.FenceSealed || resp.Fence.CreditCount != 1 {
		t.Fatalf("fence %#v", resp.Fence)
	}
	if cred.ApplyCount() != 1 || cred.ObserveCount() != 1 {
		t.Fatalf("obs=%d apply=%d", cred.ObserveCount(), cred.ApplyCount())
	}
}

func TestAutoClaimOccupiedRejectsApprove(t *testing.T) {
	eng, book, _, mux := testEng(t)
	tk := pendingTicket(book, "BTC", "deadbeef", "m")
	if _, err := mux.SeedAutoClaim("BTC", "deadbeef", 0, "auto-1"); err != nil {
		t.Fatal(err)
	}
	resp, err := eng.ApproveMemoWithEffect(context.Background(), memoeffectgate.ApproveMemoWithEffectReq{
		TicketID:	tk.ID, Asset: "BTC", Actor: "ops",
	})
	if !errors.Is(err, memoeffectgate.ErrAutoClaimOccupied) {
		t.Fatalf("got %v", err)
	}
	if resp.Ticket.Status == "approved" {
		t.Fatalf("ticket must stay pending, got %s", resp.Ticket.Status)
	}
	fresh, _ := book.GetMemoTicket(context.Background(), tk.ID)
	if fresh.Status != "pending" {
		t.Fatalf("fresh %s", fresh.Status)
	}
}

func TestObserveOKApplyFailRetryNoSecondFullCredit(t *testing.T) {
	eng, book, cred, _ := testEng(t)
	cred.FailApplyTimes(1, errApplyBoom)
	tk := pendingTicket(book, "ETH", "0xee", "m2")
	first, err := eng.ApproveMemoWithEffect(context.Background(), memoeffectgate.ApproveMemoWithEffectReq{
		TicketID:	tk.ID, Asset: "ETH", Actor: "ops", Idem: "same",
	})
	if !errors.Is(err, memoeffectgate.ErrApplyFailed) {
		t.Fatalf("first %v", err)
	}
	if first.Ticket.Status == "approved" {
		t.Fatal("must not approve on half-cut")
	}
	if !first.Fence.HalfCut || !first.Fence.ObserveOK || first.HalfProof == nil {
		t.Fatalf("half %#v proof %#v", first.Fence, first.HalfProof)
	}
	if first.Fence.CreditCount != 1 {
		t.Fatalf("credit_count %d", first.Fence.CreditCount)
	}
	obs1 := cred.ObserveCount()
	second, err := eng.ApproveMemoWithEffect(context.Background(), memoeffectgate.ApproveMemoWithEffectReq{
		TicketID:	tk.ID, Asset: "ETH", Actor: "ops", Idem: "same",
	})
	if err != nil {
		t.Fatalf("resume %v", err)
	}
	if !second.OK || second.Ticket.Status != "approved" {
		t.Fatalf("resume %#v", second)
	}
	if second.Fence.CreditCount != 1 {
		t.Fatalf("second full credit count %d", second.Fence.CreditCount)
	}
	if cred.ObserveCount() != obs1 {
		t.Fatalf("observe replayed %d -> %d", obs1, cred.ObserveCount())
	}
}

func TestDecideRejectedAfterCreditRequiresProof(t *testing.T) {
	eng, book, _, _ := testEng(t)
	tk := pendingTicket(book, "TRX", "0xtrx", "tag")
	mustApprove(t, eng, tk.ID, "USDT")
	resp, err := eng.DecideMemoGuarded(context.Background(), memoeffectgate.DecideMemoGuardedReq{
		TicketID:	tk.ID, Status: "rejected", Actor: "ops",
	})
	if !errors.Is(err, memoeffectgate.ErrRejectAfterCredit) {
		t.Fatalf("got %v", err)
	}
	if resp.Proof == nil || resp.Proof.Kind != memoeffectgate.ProofRejectAfterCredit {
		t.Fatalf("proof %#v", resp.Proof)
	}
	if resp.Proof.SilentKeep {
		t.Fatal("silent keep forbidden")
	}
	if resp.Ticket.Status == "rejected" && !resp.Blocked {
		t.Fatal("must not silently reject-keep")
	}
}

func TestDecideRejectedAfterCreditForceReverse(t *testing.T) {
	eng, book, cred, _ := testEng(t)
	tk := pendingTicket(book, "SOL", "0xsol", "m")
	mustApprove(t, eng, tk.ID, "SOL")
	resp, err := eng.DecideMemoGuarded(context.Background(), memoeffectgate.DecideMemoGuardedReq{
		TicketID:	tk.ID, Status: "rejected", Actor: "ops", ForceReverse: true, Reason: "ops-reject",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Ticket.Status != "rejected" {
		t.Fatalf("%#v", resp)
	}
	if resp.Proof == nil || resp.Proof.State != memoeffectgate.HalfReversed {
		t.Fatalf("proof %#v", resp.Proof)
	}
	if cred.ReverseCount() < 1 {
		t.Fatal("expected reverse")
	}
}

func TestFailClosedNoPending(t *testing.T) {
	eng, book, _, _ := testEng(t)
	tk := book.Put(memoeffectgate.MemoTicketView{UserID: "u", Chain: "ETH", TxHash: "h", Memo: "m", Status: "rejected"})
	_, err := eng.ApproveMemoWithEffect(context.Background(), memoeffectgate.ApproveMemoWithEffectReq{
		TicketID:	tk.ID, Asset: "ETH", Actor: "ops",
	})
	if !errors.Is(err, memoeffectgate.ErrTicketNotPending) {
		t.Fatalf("got %v", err)
	}
}

func TestMissingAsset(t *testing.T) {
	eng, book, _, _ := testEng(t)
	tk := pendingTicket(book, "ETH", "h", "m")
	_, err := eng.ApproveMemoWithEffect(context.Background(), memoeffectgate.ApproveMemoWithEffectReq{TicketID: tk.ID})
	if !errors.Is(err, memoeffectgate.ErrAssetRequired) {
		t.Fatalf("got %v", err)
	}
}

func TestIdemReplayCredited(t *testing.T) {
	eng, book, cred, _ := testEng(t)
	tk := pendingTicket(book, "BNB", "0xbnb", "m")
	a := mustApprove(t, eng, tk.ID, "BNB")
	b, err := eng.ApproveMemoWithEffect(context.Background(), memoeffectgate.ApproveMemoWithEffectReq{
		TicketID:	tk.ID, Asset: "BNB", Actor: "ops", Idem: "idem-" + tk.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.Effect.ID != b.Effect.ID {
		t.Fatalf("id mismatch")
	}
	if cred.ApplyCount() != 1 {
		t.Fatalf("apply %d", cred.ApplyCount())
	}
}

func TestExpireBoundPatrol(t *testing.T) {
	store := memoeffectgate.NewMemStore()
	now := time.Date(2021,
		11, 8, 12, 0, 0, 0, time.UTC)
	eff := memoeffectgate.MemoEffect{
		ID:	"e1", TicketID: "t1", UserID: "u", Chain: "ETH", TxHash: "h",
		OutputKey:	"ETH|h|0", Asset: "ETH", State: memoeffectgate.EffectBound, Holder: memoeffectgate.HolderMemoApprove,
		TTL:		memoeffectgate.Duration(time.Minute),
		BoundAt:	now.Add(-2 * time.Minute), ExpiresAt: now.Add(-time.Minute),
		CreatedAt:	now.Add(-2 * time.Minute), UpdatedAt: now.Add(-2 * time.Minute),
	}
	if _, err := store.SaveEffect(context.Background(), eff); err != nil {
		t.Fatal(err)
	}
	eng := memoeffectgate.New(memoeffectgate.Config{Store: store,
		Clock:	func() time.Time { return now }})
	n, err := eng.ExpireBound(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expired %d", n)
	}
	got, _ := store.GetEffect(context.Background(), "e1")
	if got.State != memoeffectgate.EffectExpired {
		t.Fatalf("state %s", got.State)
	}
}
