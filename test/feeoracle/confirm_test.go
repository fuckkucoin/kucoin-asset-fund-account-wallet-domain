package feeoracle_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
	"account-assets.kucoin.net/funding/internal/feeoracle"
)

func testdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "quoterefuse", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func baseNow() time.Time {
	return time.Date(2022, 5, 12, 10, 0, 0, 0, time.UTC)
}

func TestG_T01_EmptyFeeNoLedger(t *testing.T) {
	raw := testdata(t, "golden_gt01_fee_empty.json")
	var doc struct {
		WantErr		string	`json:"want_err"`
		LedgerWrites	int	`json:"ledger_writes"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	ld := &feeoracle.RecLedger{}
	clk := quote.FixedClock{T: baseNow()}
	b := feeoracle.NewFeeBook(clk, quote.DefaultFeeTTL, ld)
	_, err := b.ConfirmWithdrawFee(context.Background(), feeoracle.ConfirmReq{
		UserID:	"u1", Asset: "USDT", Chain: "ETH", Gross: decimal.RequireFromString("100"), Now: baseNow(),
	})
	if err == nil || err.Error() != doc.WantErr {
		t.Fatalf("err %v want %s", err, doc.WantErr)
	}
	if ld.Count() != doc.LedgerWrites {
		t.Fatalf("writes %d", ld.Count())
	}
}

func TestG_T02_StaleFee(t *testing.T) {
	raw := testdata(t, "golden_gt02_fee_stale.json")
	var doc struct {
		BPS	int	`json:"bps"`
		AgeNS	int64	`json:"age_ns"`
		TTLNS	int64	`json:"ttl_ns"`
		WantErr	string	`json:"want_err"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	asOf := baseNow()
	now := asOf.Add(time.Duration(doc.AgeNS))
	ld := &feeoracle.RecLedger{}
	b := feeoracle.NewFeeBook(quote.FixedClock{T: now}, time.Duration(doc.TTLNS), ld)
	if err := b.Put(feeoracle.FeeQuote{Asset: "USDT", Chain: "ETH", BPS: doc.BPS, AsOf: asOf, TTL: time.Duration(doc.TTLNS), Source: quote.SourceMarket}); err != nil {
		t.Fatal(err)
	}
	_, err := b.ConfirmWithdrawFee(context.Background(), feeoracle.ConfirmReq{
		UserID:	"u1", Asset: "USDT", Chain: "ETH", Gross: decimal.RequireFromString("100"), Now: now,
	})
	if !errors.Is(err, feeoracle.ErrQuoteStale) {
		t.Fatalf("got %v", err)
	}
	if err.Error() != doc.WantErr {
		t.Fatalf("msg %v", err)
	}
	if ld.Count() != 0 {
		t.Fatal("ledger")
	}
}

func TestG_T03_FreshFee(t *testing.T) {
	raw := testdata(t, "golden_gt03_fee_fresh.json")
	var doc struct {
		BPS		int	`json:"bps"`
		TTLNS		int64	`json:"ttl_ns"`
		Gross		string	`json:"gross"`
		LedgerWrites	int	`json:"ledger_writes"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	now := baseNow()
	ld := &feeoracle.RecLedger{}
	b := feeoracle.NewFeeBook(quote.FixedClock{T: now}, time.Duration(doc.TTLNS), ld)
	if err := b.Put(feeoracle.FeeQuote{Asset: "USDT", Chain: "ETH", BPS: doc.BPS, AsOf: now, TTL: time.Duration(doc.TTLNS), Source: quote.SourceMarket}); err != nil {
		t.Fatal(err)
	}
	got, err := b.ConfirmWithdrawFee(context.Background(), feeoracle.ConfirmReq{
		UserID:	"u1", Asset: "USDT", Chain: "ETH", Gross: decimal.RequireFromString(doc.Gross), Now: now, IdemKey: "k1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Applied || ld.Count() != doc.LedgerWrites {
		t.Fatalf("%+v writes=%d", got, ld.Count())
	}
	if got.Source != quote.SourceMarket {
		t.Fatal(got.Source)
	}
}

func TestG_T11_FeeBeforeAfterExpire(t *testing.T) {
	asOf := baseNow()
	sc := &quote.StepClock{Base: asOf}
	ld := &feeoracle.RecLedger{}
	b := feeoracle.NewFeeBook(sc, quote.DefaultFeeTTL, ld)
	if err := b.Put(feeoracle.FeeQuote{Asset: "USDT", BPS: 10, AsOf: asOf, TTL: quote.DefaultFeeTTL, Source: quote.SourceMarket}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ConfirmWithdrawFee(context.Background(), feeoracle.ConfirmReq{
		UserID:	"u1", Asset: "USDT", Gross: decimal.RequireFromString("50"), IdemKey: "a",
	}); err != nil {
		t.Fatal(err)
	}
	sc.Advance(quote.DefaultFeeTTL + time.Second)
	if _, err := b.ConfirmWithdrawFee(context.Background(), feeoracle.ConfirmReq{
		UserID:	"u1", Asset: "USDT", Gross: decimal.RequireFromString("50"), IdemKey: "b",
	}); !errors.Is(err, feeoracle.ErrQuoteStale) {
		t.Fatalf("got %v", err)
	}
	if ld.Count() != 1 {
		t.Fatalf("writes %d", ld.Count())
	}
}

func TestFeeIsolationByAsset(t *testing.T) {
	now := baseNow()
	ld := &feeoracle.RecLedger{}
	b := feeoracle.NewFeeBook(quote.FixedClock{T: now},
		quote.DefaultFeeTTL, ld)
	if err := b.Put(feeoracle.FeeQuote{Asset: "BTC", Chain: "BTC", BPS: 5, AsOf: now, TTL: quote.DefaultFeeTTL, Source: quote.SourceMarket}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.GetFee("ETH", "ETH", now); !errors.Is(err, feeoracle.ErrQuoteEmpty) {
		t.Fatalf("%v", err)
	}
	if _, err := b.GetFee("BTC", "BTC", now); err != nil {
		t.Fatal(err)
	}
}

func TestFeeHouseDebugNotConfirm(t *testing.T) {
	now := baseNow()
	ld := &feeoracle.RecLedger{}
	b := feeoracle.NewFeeBook(quote.FixedClock{T: now}, quote.DefaultFeeTTL, ld)
	dbg := feeoracle.HouseDebugFee("USDT", 10, now)
	if err := b.Put(dbg); err != nil {
		t.Fatal(err)
	}
	_, err := b.ConfirmWithdrawFee(context.Background(), feeoracle.ConfirmReq{
		UserID:	"u1", Asset: "USDT", Gross: decimal.RequireFromString("10"),
	})
	if !errors.Is(err, quote.ErrHouseOnConfirm) {
		t.Fatalf("%v", err)
	}
	if ld.Count() != 0 {
		t.Fatal("writes")
	}
}

func TestGetFeeEmptyBook(t *testing.T) {
	var b *feeoracle.Book
	if _, err := b.GetFee("USDT", "", baseNow()); !errors.Is(err, feeoracle.ErrQuoteEmpty) {
		t.Fatal(err)
	}
	b = feeoracle.NewFeeBook(quote.FixedClock{T: baseNow()}, quote.DefaultFeeTTL, nil)
	if _, err := b.GetFee("", "", baseNow()); err != feeoracle.ErrAssetRequired {
		t.Fatal(err)
	}
}

func TestConfirmRejectsBadGross(t *testing.T) {
	now := baseNow()
	b := feeoracle.NewFeeBook(quote.FixedClock{T: now}, quote.DefaultFeeTTL, &feeoracle.RecLedger{})
	_ = b.Put(feeoracle.FeeQuote{Asset: "USDT", BPS: 10, AsOf: now, TTL: quote.DefaultFeeTTL, Source: quote.SourceMarket})
	if _, err := b.ConfirmWithdrawFee(context.Background(), feeoracle.ConfirmReq{UserID: "u", Asset: "USDT", Gross: decimal.Zero}); err != feeoracle.ErrAmountInvalid {
		t.Fatal(err)
	}
}

func TestMaskedEmptyFeeQuote(t *testing.T) {
	now := baseNow()
	ld := &feeoracle.RecLedger{}
	b := feeoracle.NewFeeBook(quote.FixedClock{T: now},
		quote.DefaultFeeTTL, ld)
	if err := b.Put(feeoracle.FeeQuote{Asset: "USDT", Source: quote.SourceMarket}); err != nil {
		t.Fatal(err)
	}
	_, err := b.ConfirmWithdrawFee(context.Background(), feeoracle.ConfirmReq{
		UserID:	"u1", Asset: "USDT", Gross: decimal.RequireFromString("10"),
	})
	if !errors.Is(err, quote.ErrSourceMasked) && !errors.Is(err, feeoracle.ErrQuoteEmpty) {
		t.Fatalf("%v", err)
	}
	if ld.Count() != 0 {
		t.Fatal("writes")
	}
}
