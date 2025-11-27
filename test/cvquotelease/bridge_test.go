package cvquotelease_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/cvfund"
	"account-assets.kucoin.net/funding/internal/cvquotelease"
)

func TestAttachBookStrictBareAndFill(t *testing.T) {
	marks := cvquotelease.NewMemoryMarks()
	marks.Put("BTC", "USDT", "42000")
	eng := cvquotelease.New(cvquotelease.Config{Marks: marks, Strict: true, Store: cvquotelease.NewMemStore()})
	_, _ = eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: "BTC", To: "USDT", Rate: "42000"})
	b := cvquotelease.AttachBook(cvfund.New(), eng)
	_, err := b.PlaceQuoted("u", "BTC", "USDT", "1", "42000", "funding", "", false)
	if err == nil {
		t.Fatal("strict bare must fail")
	}
	q, err := b.Quote(context.Background(), "u", "BTC", "USDT", "0.1", "funding")
	if err != nil {
		t.Fatal(err)
	}
	if q.LeaseID == "" {
		t.Fatal("quote must issue lease")
	}
	o, err := b.Execute(context.Background(), "u", q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if o.ID == "" {
		t.Fatal("fill")
	}
}

func TestAttachBookMarkDrift(t *testing.T) {
	marks := cvquotelease.NewMemoryMarks()
	marks.Put("BTC", "USDT", "42000")
	eng := cvquotelease.New(cvquotelease.Config{Marks: marks, Strict: true, Store: cvquotelease.NewMemStore()})
	_, _ = eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: "BTC", To: "USDT", Rate: "42000"})
	b := cvquotelease.AttachBook(cvfund.New(),
		eng)
	q, err := b.QuoteWithLease(context.Background(), "u", "BTC", "USDT", "0.2", "funding")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eng.MarkPut(context.Background(), cvquotelease.MarkPutReq{From: "BTC", To: "USDT", Rate: "44000"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ExecuteWithLease(context.Background(), "u", q.ID, q.LeaseID); err == nil {
		t.Fatal("drift must reject")
	}
}
