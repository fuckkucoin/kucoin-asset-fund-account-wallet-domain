package cvquotelease_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/cvquotelease"
)

func TestQuoteExecuteMatrix(t *testing.T) {
	type row struct {
		user, from, to, amt, book, rate	string
		wantQuoteErr			error
		doMarkBump			bool
		wantExecErr			error
	}
	rows := []row{
		{"u1", "BTC", "USDT", "0.01", "funding", "42000", nil, false, nil},
		{"u2", "ETH", "USDT", "1", "trading", "3000", nil, false, nil},
		{"u3", "KCS", "USDT", "5", "both", "12.5", nil, false, nil},
		{"u4", "BTC", "USDT", "0.02", "funding", "42000", nil, true, cvquotelease.ErrLeaseRevoked},
		{"u5", "ETH", "USDT", "0.3", "funding", "3000", nil, true, cvquotelease.ErrLeaseRevoked},
		{"", "BTC", "USDT", "1", "funding", "42000", cvquotelease.ErrUserRequired, false, nil},
		{"u6", "BTC", "BTC", "1", "funding", "1", cvquotelease.ErrSameAsset, false, nil},
		{"u7", "BTC", "USDT", "0", "funding", "42000", cvquotelease.ErrAmountInvalid, false, nil},
		{"u8", "BTC", "USDT", "x", "funding", "42000", cvquotelease.ErrAmountInvalid, false, nil},
		{"u9", "BTC", "USDT", "1", "spot", "42000", cvquotelease.ErrBookInvalid, false, nil},
		{"u10", "BTC", "USDT", "1", "funding", "42000", nil, false, nil},
		{"u11", "ETH", "USDT", "2", "both", "3000", nil, false, nil},
		{"u12", "KCS", "USDT", "9", "trading", "12.5", nil, false, nil},
		{"u13", "BTC", "USDT", "0.001", "both", "42000", nil, true, cvquotelease.ErrLeaseRevoked},
		{"u14", "ETH", "USDT", "4", "trading", "3000", nil, true, cvquotelease.ErrLeaseRevoked},
		{"u15", "BTC", "USDT", "3", "funding", "42000", nil, false, nil},
		{"u16", "ETH", "USDT", "0.01", "funding", "3000", nil, false, nil},
		{"u17", "KCS", "USDT", "1.1", "funding", "12.5", nil, false, nil},
		{"u18", "BTC", "USDT", "0.4", "trading", "42000", nil, false, nil},
		{"u19", "ETH", "USDT", "0.4", "both", "3000", nil, false, nil},
		{"u20", "KCS", "USDT", "20", "both", "12.5", nil, false, nil},
		{"u21", "BTC", "USDT", "-2", "funding", "42000", cvquotelease.ErrAmountInvalid, false, nil},
		{"u22", "", "USDT", "1", "funding", "42000", cvquotelease.ErrAssetRequired, false, nil},
		{"u23", "BTC", "", "1", "funding", "42000", cvquotelease.ErrAssetRequired, false, nil},
		{"u24", "BTC", "USDT", "1", "FUNDING", "42000", nil, false, nil},
		{"u25", "btc", "usdt", "1", "funding", "42000", nil, false, nil},
	}
	eng, marks := testEng(t)
	for _, r := range rows {
		r := r
		t.Run(r.user+"/"+r.from+r.to+r.book, func(t *testing.T) {
			if r.from != "" && r.to != "" && r.from != r.to && r.rate != "" {
				marks.Put(r.from, r.to, r.rate)
				_, _ = eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: r.from, To: r.to, Rate: r.rate})
			}
			q, err := eng.QuoteWithLease(context.Background(), cvquotelease.QuoteWithLeaseReq{
				UserID:	r.user, From: r.from, To: r.to, Amount: r.amt, Book: r.book,
			})
			if r.wantQuoteErr != nil {
				if !errors.Is(err, r.wantQuoteErr) {
					t.Fatalf("quote err %v want %v", err, r.wantQuoteErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if r.doMarkBump {
				_, _ = eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: r.from, To: r.to, Rate: r.rate + "1"})
			}
			ex, err := eng.ExecuteWithLease(context.Background(), cvquotelease.ExecuteWithLeaseReq{
				UserID:	r.user, QuoteID: q.QuoteID, LeaseID: q.LeaseID,
			})
			if r.wantExecErr != nil {
				if !errors.Is(err, r.wantExecErr) && !errors.Is(err, cvquotelease.ErrMarkGenDrift) {
					t.Fatalf("exec err %v want %v proof=%v", err, r.wantExecErr, ex.DriftProof)
				}
				return
			}
			if err != nil || !ex.Filled {
				t.Fatalf("fill %+v %v", ex, err)
			}
		})
	}
}

func TestFailClosedReasons(t *testing.T) {
	now := time.Now().UTC()
	if k, err := cvquotelease.FailClosedReason(nil, now, 1, false); k != cvquotelease.DriftMissingLease || err == nil {
		t.Fatal("missing")
	}
	l := &cvquotelease.CvQuoteLease{ID: "1", State: cvquotelease.LeaseIssued, ExpiresAt: now.Add(-time.Second), MarkGen: 1}
	if k, err := cvquotelease.FailClosedReason(l, now, 1, true); k != cvquotelease.DriftExpired || err == nil {
		t.Fatal("exp")
	}
	l.ExpiresAt = now.Add(time.Minute)
	l.State = cvquotelease.LeaseRevoked
	if k, err := cvquotelease.FailClosedReason(l, now, 1, true); k != cvquotelease.DriftRevoked || err == nil {
		t.Fatal("rev")
	}
	l.State = cvquotelease.LeaseIssued
	if k, err := cvquotelease.FailClosedReason(l, now, 2, true); k != cvquotelease.DriftMarkGen || err == nil {
		t.Fatal("gen")
	}
	if !cvquotelease.MustBlock(cvquotelease.DriftMarkGen) || cvquotelease.MustBlock("") {
		t.Fatal("block")
	}
}

func TestPairRules(t *testing.T) {
	if !cvquotelease.BookAllowed("funding") || cvquotelease.BookAllowed("margin") {
		t.Fatal("book")
	}
	if !cvquotelease.PairAllowed("BTC", "USDT") || cvquotelease.PairAllowed("BTC", "BTC") {
		t.Fatal("pair")
	}
	if cvquotelease.SourceName("") != "mark_put" {
		t.Fatal("source")
	}
}
