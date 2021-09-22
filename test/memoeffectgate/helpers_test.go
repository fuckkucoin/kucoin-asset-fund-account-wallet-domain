package memoeffectgate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/memoeffectgate"
)

func testEng(t *testing.T) (*memoeffectgate.Engine, *memoeffectgate.MemTicketBook, *memoeffectgate.MemCreditPort, *memoeffectgate.MemClaimMutex,) {
	t.Helper()
	mux := memoeffectgate.NewMemClaimMutex()
	book := memoeffectgate.NewMemTicketBook()
	cred := memoeffectgate.NewMemCreditPort()
	eng := memoeffectgate.New(memoeffectgate.Config{
		Store:	memoeffectgate.NewMemStore(), Mutex: mux, Tickets: book, Credit: cred,
		Reverse:	memoeffectgate.NewMemReversePort(), Clock: func() time.Time { return time.Date(2021, 11, 8, 12, 0, 0, 0, time.UTC) },
		TTL:	30 * time.Minute,
	})
	return eng, book, cred, mux
}

func pendingTicket(book *memoeffectgate.MemTicketBook, chain, hash, memo string) memoeffectgate.MemoTicketView {
	return book.Put(memoeffectgate.MemoTicketView{
		UserID:	"u-1", Chain: chain, TxHash: hash, Memo: memo, Address: "addr1", Status: "pending",
	})
}

func mustApprove(t *testing.T, eng *memoeffectgate.Engine, id, asset string) memoeffectgate.ApproveMemoWithEffectResp {
	t.Helper()
	resp, err := eng.ApproveMemoWithEffect(context.Background(), memoeffectgate.ApproveMemoWithEffectReq{
		TicketID:	id, Asset: asset, Actor: "ops", Idem: "idem-" + id,
	})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok %#v", resp)
	}
	return resp
}

var errApplyBoom = errors.New("apply inbound exploded")
