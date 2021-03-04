package assetworker_test

import (
	"context"
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/transfers"
	"account-assets.kucoin.net/funding/internal/assetworker"
)

func TestFT03AttachDepositHitsHandler(t *testing.T) {
	p := assetworker.NewPump()
	hit := ""
	if err := p.AttachDeposit(func(_ context.Context, job assetworker.Job,) error {
		hit = job.ID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := p.Handle(context.Background(), assetworker.Job{ID: "d1", Kind: assetworker.KindDeposit}); err != nil {
		t.Fatal(err)
	}
	if hit != "d1" {
		t.Fatalf("handler not called, hit=%q", hit)
	}
}

func TestFT04AttachWithdrawHitsHandler(t *testing.T) {
	p := assetworker.NewPump()
	hit := ""
	if err := p.AttachWithdraw(func(_ context.Context, job assetworker.Job,) error {
		hit = job.ID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := p.Handle(context.Background(), assetworker.Job{ID: "w1", Kind: assetworker.KindWithdraw}); err != nil {
		t.Fatal(err)
	}
	if hit != "w1" {
		t.Fatalf("handler not called, hit=%q", hit)
	}
}

func TestFT05PumpWithoutAttachNoSilentSuccess(t *testing.T) {
	p := assetworker.NewPump()
	err := p.Handle(context.Background(), assetworker.Job{ID: "d1", Kind: assetworker.KindDeposit})
	if !errors.Is(err, assetworker.ErrNotAttached) {
		t.Fatalf("want ErrNotAttached got %v", err)
	}
	err = p.Handle(context.Background(), assetworker.Job{ID: "w1", Kind: assetworker.KindWithdraw})
	if !errors.Is(err, assetworker.ErrNotAttached) {
		t.Fatalf("want ErrNotAttached got %v", err)
	}
}

func TestFT06DuplicateAttachError(t *testing.T) {
	p := assetworker.NewPump()
	h := func(context.Context, assetworker.Job,) error { return nil }
	if err := p.AttachDeposit(h); err != nil {
		t.Fatal(err)
	}
	if err := p.AttachDeposit(h); !errors.Is(err, assetworker.ErrDupAttach) {
		t.Fatalf("want ErrDupAttach got %v", err)
	}
	if err := p.AttachWithdraw(h); err != nil {
		t.Fatal(err)
	}
	if err := p.AttachWithdraw(h); !errors.Is(err, assetworker.ErrDupAttach) {
		t.Fatalf("want ErrDupAttach got %v", err)
	}
}

func TestFT07WriteIncompleteTransferNewInstanceCallsHandler(t *testing.T) {
	st := transfers.NewMem(transfers.Rec{ID: "t-open", Kind: assetworker.KindDeposit, State: transfers.StatePending})
	p := assetworker.NewPump()
	hit := 0
	if err := p.AttachDeposit(func(context.Context, assetworker.Job,) error { hit++; return nil }); err != nil {
		t.Fatal(err)
	}
	n, err := assetworker.RestoreTransfers(context.Background(), st, p)
	if err != nil || n != 1 || hit != 1 {
		t.Fatalf("n=%d hit=%d err=%v", n, hit, err)
	}
}

func TestFT08CompletedTransferSkippedOnRestore(t *testing.T) {
	st := transfers.NewMem(transfers.Rec{ID: "t-done", Kind: assetworker.KindDeposit, State: transfers.StateCompleted})
	p := assetworker.NewPump()
	hit := 0
	_ = p.AttachDeposit(func(context.Context, assetworker.Job,) error { hit++; return nil })
	n,
		err := assetworker.RestoreTransfers(context.Background(), st, p)
	if err != nil || n != 0 || hit != 0 {
		t.Fatalf("n=%d hit=%d err=%v", n, hit, err)
	}
}

func TestFT09MixedStateRestoreIncompleteSubsetOnly(t *testing.T) {
	st := transfers.NewMem(
		transfers.Rec{ID: "a", Kind: assetworker.KindDeposit, State: transfers.StatePending},
		transfers.Rec{ID: "b", Kind: assetworker.KindWithdraw, State: transfers.StateCompleted},
		transfers.Rec{ID: "c", Kind: assetworker.KindWithdraw, State: transfers.StateProcessing},
		transfers.Rec{ID: "d", Kind: assetworker.KindDeposit, State: "credited"},
	)
	p := assetworker.NewPump()
	var got []string
	h := func(_ context.Context, job assetworker.Job,) error {
		got = append(got, job.ID)
		return nil
	}
	_ = p.AttachDeposit(h)
	_ = p.AttachWithdraw(h)
	n, err := assetworker.RestoreTransfers(context.Background(), st, p)
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v got=%v", n, err, got)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Fatalf("subset %v", got)
	}
}

func TestFT14IncompleteAADepositsRestoreEntryCalled(t *testing.T) {
	st := &assetworker.MemAA{Rows: []assetworker.AAClaim{
		{Chain: "ETH", TxHash: "0x1", Phase: assetworker.PhaseObserving, TransferID: "tr-1"},
	}}
	hit := 0
	n, err := assetworker.RestoreAADeposits(context.Background(), st, func(_ context.Context, c assetworker.AAClaim,) error {
		hit++
		if c.TransferID != "tr-1" {
			t.Fatalf("claim %+v", c)
		}
		return nil
	})
	if err != nil || n != 1 || hit != 1 {
		t.Fatalf("n=%d hit=%d err=%v", n, hit, err)
	}
}

func TestFT15TerminalAADepositsSkippedOnRestore(t *testing.T) {
	st := &assetworker.MemAA{Rows: []assetworker.AAClaim{
		{Chain: "ETH", TxHash: "0x2", Phase: assetworker.PhaseCredited, TransferID: "tr-2"},
		{Chain: "ETH", TxHash: "0x3", Phase: assetworker.PhaseRejected},
	}}
	hit := 0
	n, err := assetworker.RestoreAADeposits(context.Background(), st, func(context.Context, assetworker.AAClaim,) error {
		hit++
		return nil
	})
	if err != nil || n != 0 || hit != 0 {
		t.Fatalf("n=%d hit=%d err=%v", n, hit, err)
	}
}
