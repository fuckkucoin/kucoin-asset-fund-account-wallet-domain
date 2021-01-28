package ledger_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/shopspring/decimal"
)

type swapSeal struct{}

func (swapSeal) Encrypt(plain []byte) ([]byte, error) {
	out := make([]byte, len(plain))
	for i, b := range plain {
		out[i] = b ^ 0x5a
	}
	return out, nil
}

func (swapSeal) Decrypt(blob []byte) ([]byte, error) {
	return swapSeal{}.Encrypt(blob)
}

func TestSealRoundTripLeavesCallerPlain(t *testing.T) {
	s := ledger.NewSQLStore(nil)
	s.SetSeal(swapSeal{})
	tr := &ledger.Transfer{MemoHint: "memo-1", TagHint: "9", TagNameHint: "destination_tag"}
	memo, tag, name, err := s.SealHints(tr.MemoHint, tr.TagHint, tr.TagNameHint)
	if err != nil {
		t.Fatal(err)
	}
	prefix := ledger.SealPrefix()
	if memo == tr.MemoHint || !bytes.HasPrefix([]byte(memo), []byte(prefix)) {
		t.Fatalf("stored memo %s", memo)
	}
	if tr.MemoHint != "memo-1" {
		t.Fatalf("caller mutated %s", tr.MemoHint)
	}
	got := &ledger.Transfer{MemoHint: memo, TagHint: tag, TagNameHint: name}
	s.OpenHints(got)
	if got.MemoHint != "memo-1" || got.TagHint != "9" || got.TagNameHint != "destination_tag" {
		t.Fatalf("opened %+v", got)
	}
	plain := &ledger.Transfer{MemoHint: "legacy"}
	s.OpenHints(plain)
	if plain.MemoHint != "legacy" {
		t.Fatalf("legacy %s", plain.MemoHint)
	}
}

func TestCountSinceSkipsOtherPool(t *testing.T) {
	m := ledger.NewMemStore()
	now := time.Now().UTC()
	ctx := context.Background()
	for _, tr := range []*ledger.Transfer{
		{ID: "a", IdempotencyKey: "a", UserID: "u", Direction: ledger.WalletWithdraw, Asset: "USDT", Amount: decimal.NewFromInt(1), State: ledger.StateChainSubmitted, CreatedAt: now},
		{ID: "b", IdempotencyKey: "b", UserID: "u", Direction: ledger.WalletDeposit, Asset: "USDT", Amount: decimal.NewFromInt(1), State: ledger.StateCredited, CreatedAt: now},
		{ID: "c", IdempotencyKey: "c", UserID: "u", Direction: ledger.WalletWithdraw, Asset: "USDT", Amount: decimal.NewFromInt(1), State: ledger.StateFailed, CreatedAt: now},
	} {
		if err := m.InsertTransfer(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}
	n, err := m.CountSince(ctx, "u", ledger.WalletWithdraw, now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("count %d", n)
	}
}
