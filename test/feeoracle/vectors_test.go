package feeoracle_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
	"account-assets.kucoin.net/funding/internal/feeoracle"
)

func TestCutTable(t *testing.T) {
	rows := []struct {
		name	string
		gross	string
		bps	int
		flat	string
		ok	bool
		fee	string
	}{
		{"bps10", "100", 10, "0", true, "0.1"},
		{"flat", "100", 0, "1.5", true, "1.5"},
		{"flat_over", "1", 0, "2", false, "0"},
		{"zero", "10", 0, "0", false, "0"},
	}
	for _, row := range rows {
		fee, net, err := feeoracle.Cut(decimal.RequireFromString(row.gross), feeoracle.FeeQuote{
			BPS:	row.bps, Flat: decimal.RequireFromString(row.flat),
		})
		if row.ok {
			if err != nil || fee.String() != row.fee {
				t.Fatalf("%s %v %s", row.name, err, fee)
			}
			if !net.Add(fee).Equal(decimal.RequireFromString(row.gross)) {
				t.Fatalf("%s conserve", row.name)
			}
		} else if err == nil {
			t.Fatalf("%s want err", row.name)
		}
	}
}

func TestGetFeeStaleThenFreshOtherChain(t *testing.T) {
	now := baseNow()
	b := feeoracle.NewFeeBook(quote.FixedClock{T: now}, quote.DefaultFeeTTL, &feeoracle.RecLedger{})
	_ = b.Put(feeoracle.FeeQuote{Asset: "USDT", Chain: "TRX", BPS: 8, AsOf: now.Add(-time.Hour), TTL: time.Second, Source: quote.SourceMarket})
	_ = b.Put(feeoracle.FeeQuote{Asset: "USDT", Chain: "ETH", BPS: 8, AsOf: now, TTL: quote.DefaultFeeTTL, Source: quote.SourceMarket})
	if _, err := b.GetFee("USDT", "TRX", now); !errors.Is(err, feeoracle.ErrQuoteStale) {
		t.Fatal(err)
	}
	if _, err := b.GetFee("USDT", "ETH", now); err != nil {
		t.Fatal(err)
	}
}

func TestLedgerFailDoesNotApplyFlag(t *testing.T) {
	now := baseNow()
	ld := &feeoracle.RecLedger{Fail: errors.New("store down")}
	b := feeoracle.NewFeeBook(quote.FixedClock{T: now}, quote.DefaultFeeTTL, ld)
	_ = b.Put(feeoracle.FeeQuote{Asset: "USDT", BPS: 10, AsOf: now, TTL: quote.DefaultFeeTTL, Source: quote.SourceMarket})
	got, err := b.ConfirmWithdrawFee(context.Background(), feeoracle.ConfirmReq{
		UserID:	"u", Asset: "USDT", Gross: decimal.RequireFromString("20"), IdemKey: "x",
	})
	if err == nil || got.Applied {
		t.Fatal(got, err)
	}
}

func TestDeleteFeeKey(t *testing.T) {
	now := baseNow()
	b := feeoracle.NewFeeBook(quote.FixedClock{T: now},
		quote.DefaultFeeTTL, nil)
	_ = b.Put(feeoracle.FeeQuote{Asset: "USDT", BPS: 1, AsOf: now, TTL: quote.DefaultFeeTTL, Source: quote.SourceMarket})
	b.Delete("USDT", "")
	if _, err := b.GetFee("USDT", "", now); !errors.Is(err, feeoracle.ErrQuoteEmpty) {
		t.Fatal(err)
	}
}
