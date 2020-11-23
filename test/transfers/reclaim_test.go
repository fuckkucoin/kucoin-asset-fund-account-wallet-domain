package transfers_test

import (
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/occuplock"
	"account-assets.kucoin.net/funding/internal/transfers"
)

func TestClassifyTable(t *testing.T) {
	cases := []struct {
		state	string
		want	string
	}{
		{transfers.StateCompleted, transfers.ActionSkip},
		{transfers.StateRolledBack, transfers.ActionSkip},
		{transfers.StateReclaimed, transfers.ActionSkip},
		{transfers.StateDebitDone, transfers.ActionComplete},
		{transfers.StateCreditPend, transfers.ActionComplete},
		{transfers.StateFailed, transfers.ActionRollback},
		{transfers.StateHalfFrozen, transfers.ActionRollback},
		{transfers.StatePending, transfers.ActionRollback},
		{transfers.StateProcessing, transfers.ActionRollback},
		{"unknown_open", transfers.ActionHold},
		{"", transfers.ActionHold},
	}
	for _, tc := range cases {
		got := transfers.Classify(transfers.Rec{ID: "x", State: tc.state})
		if got != tc.want {
			t.Fatalf("state=%s got=%s want=%s", tc.state, got, tc.want)
		}
	}
	if transfers.Classify(transfers.Rec{}) != transfers.ActionSkip {
		t.Fatal("empty id")
	}
}

func TestReclaimSuccessAndIdempotent(t *testing.T) {
	st := transfers.NewMem(transfers.Rec{ID: "t1", Kind: transfers.KindTransfer, State: transfers.StateDebitDone, UserID: "u1", Asset: "USDT"}, transfers.Rec{ID: "t2", Kind: transfers.KindTransfer, State: transfers.StateCompleted, UserID: "u1", Asset: "USDT"},
	)
	locks, err := occuplock.New(occuplock.NewMem(), occuplock.Policy{Profile: occuplock.ProfileTest, Persist: true, TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	r := &transfers.Reclaimer{Src: st, Mut: st, Locks: locks}
	res, err := r.Reclaim("t1", "w", "tok")
	if err != nil || res.Action != transfers.ActionComplete || res.State != transfers.StateCompleted {
		t.Fatalf("%+v %v", res, err)
	}
	res, err = r.Reclaim("t1", "w", "tok")
	if err != nil || res.Action != transfers.ActionSkip {
		t.Fatalf("idempotent %+v %v", res, err)
	}
	res, err = r.Reclaim("t2", "w", "tok")
	if err != nil || res.Action != transfers.ActionSkip {
		t.Fatalf("terminal %+v %v", res, err)
	}
}

func TestReclaimFailClosedAndRestart(t *testing.T) {
	st := transfers.NewMem(transfers.Rec{ID: "h1", Kind: transfers.KindTransfer, State: transfers.StateCreditPend, UserID: "u2", Asset: "BTC"}, transfers.Rec{ID: "h2", Kind: transfers.KindTransfer, State: transfers.StateHalfFrozen, UserID: "u2", Asset: "BTC"},
	)
	mem := occuplock.NewMem()
	locks, err := occuplock.New(mem, occuplock.Policy{Profile: occuplock.ProfileProd, Persist: true, TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 22, 5, 0, 0, 0, time.UTC)
	locks.WithClock(func() time.Time { return now })
	r := &transfers.Reclaimer{Src: st, Mut: nil, Locks: locks}
	if _, err := r.Reclaim("h1", "w", "tok"); !errors.Is(err, transfers.ErrStore) {
		t.Fatalf("nil mut %v", err)
	}
	r.Mut = st
	if _, err := r.Reclaim("nope", "w", "tok"); !errors.Is(err, transfers.ErrNotFound) {
		t.Fatalf("missing %v", err)
	}
	// 他人持锁：先占用 xfer 键。
	key := occuplock.Key("xfer", "u2", "h1")
	if _, err := locks.Acquire(key, "other", "ot", "xfer"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reclaim("h1", "w", "tok"); !errors.Is(err, occuplock.ErrHeld) {
		t.Fatalf("held %v", err)
	}
	if err := locks.Release(key, "other", "ot", 1); err != nil {
		t.Fatal(err)
	}
	res, err := r.Reclaim("h1", "w", "tok")
	if err != nil || res.State != transfers.StateCompleted {
		t.Fatalf("complete %+v %v", res, err)
	}
	// 重启后半成品 h2 续扫回收。
	locks2, err := occuplock.New(mem, occuplock.Policy{Profile: occuplock.ProfileProd, Persist: true, TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	locks2.WithClock(func() time.Time { return now.Add(time.Second) })
	r2 := &transfers.Reclaimer{Src: st, Mut: st, Locks: locks2}
	cur := &transfers.Cursor{}
	out, err := r2.ReclaimIncomplete(cur, 10, "w", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].ID != "h2" || out[0].State != transfers.StateRolledBack {
		t.Fatalf("restart reclaim %#v", out)
	}
}

func TestReclaimUnknownHoldFailClosed(t *testing.T) {
	st := transfers.NewMem(transfers.Rec{ID: "x", Kind: transfers.KindTransfer, State: "mystery", UserID: "u", Asset: "ETH"})
	locks, err := occuplock.New(occuplock.NewMem(), occuplock.Policy{Profile: occuplock.ProfileTest, Persist: true})
	if err != nil {
		t.Fatal(err)
	}
	r := &transfers.Reclaimer{Src: st, Mut: st, Locks: locks}
	if _, err := r.Reclaim("x", "w", "tok"); !errors.Is(err, transfers.ErrUnreclaimable) {
		t.Fatalf("hold %v", err)
	}
}

func TestLoadIncompleteNilStore(t *testing.T) {
	if _, err := transfers.LoadIncomplete(nil, nil, 1); !errors.Is(err, transfers.ErrStore) {
		t.Fatalf("%v", err)
	}
	r := &transfers.Reclaimer{}
	if _, err := r.Reclaim("a", "w", "t"); !errors.Is(err, transfers.ErrStore) {
		t.Fatalf("%v", err)
	}
}
