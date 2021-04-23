package dustpath_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/dustquotegate"
	"account-assets.kucoin.net/funding/internal/dustpath"
)

func TestPathConvertIssuesLease(t *testing.T) {
	s := dustpath.New()
	got, err := s.Convert(context.Background(), dustpath.ConvertReq{
		UserID:	"user-path-001", Asset: "BTC", Amount: "0.00005", IdempotencyKey: "path-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.LeaseID == "" {
		t.Fatal("dust_quote_lease_id required")
	}
}

func TestPathQuoteThenRateFlip(t *testing.T) {
	s := dustpath.New()
	ctx := context.Background()
	lease, err := s.QuoteEngine().QuoteWithLease(ctx, dustquotegate.QuoteReq{
		UserID:	"user-path-002", Asset: "BTC", Amount: "0.00005",
		Actor:	"dustpath-ops", IdempotencyKey: "path-q2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Dust().PutRate("BTC", "61000"); err != nil {
		t.Fatal(err)
	}
	_, err = s.Convert(ctx, dustpath.ConvertReq{
		UserID:	"user-path-002", Asset: "BTC", Amount: "0.00005",
		IdempotencyKey:	"path-2", DustQuoteLeaseID: lease.LeaseID,
	})
	if err == nil {
		t.Fatal("old lease after PutRate must fail")
	}
}
