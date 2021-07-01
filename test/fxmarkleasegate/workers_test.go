package fxmarkleasegate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/fxmarkleasegate"
)

func TestExpireScan(t *testing.T) {
	rd := testReader()
	e := fxmarkleasegate.New(fxmarkleasegate.Config{Store: fxmarkleasegate.NewMemStore(), Reader: rd, TTL: time.Millisecond})
	if _, err := e.IssueMarkLease(context.Background(), fxmarkleasegate.IssueMarkLeaseReq{From: "ETH", To: "USDT", TTL: "1ms"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(4 * time.Millisecond)
	n, err := e.ExpireScan(context.Background(), 10)
	if err != nil || n == 0 {
		t.Fatalf("expired=%d err=%v", n, err)
	}
	h,
		_ := e.Health(context.Background())
	if h.Expired == 0 {
		t.Fatalf("%+v", h)
	}
}

func TestPutRevokePumpAndPatrol(t *testing.T) {
	e, rd := testEng(t)
	if _, err := e.IssueMarkLease(context.Background(), fxmarkleasegate.IssueMarkLeaseReq{From: "ETH", To: "USDT"}); err != nil {
		t.Fatal(err)
	}
	rd.PutUSD("ETH", "9999")
	n, err := e.OnPutChange(context.Background(), "ETH", "9999")
	if err != nil || n == 0 {
		t.Fatalf("put revoke %d %v", n, err)
	}
	iss2, err := e.IssueMarkLease(context.Background(), fxmarkleasegate.IssueMarkLeaseReq{From: "BTC", To: "USDT", PreferLive: true})
	if err != nil {
		t.Fatal(err)
	}
	_ = iss2
	rd.Live = true
	rd.SetUSDNoBump("BTC", "80000")
	n, err = e.PatrolLiveDrift(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("patrol should revoke drifted live lease")
	}
	job := fxmarkleasegate.NewExpireScanJob(e, fxmarkleasegate.WorkerConfig{BatchSize: 5}, nopLog())
	if err := job(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := fxmarkleasegate.NewPutRevokePumpJob(e, fxmarkleasegate.WorkerConfig{}, nopLog())(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := fxmarkleasegate.NewLiveDriftPatrolJob(e, fxmarkleasegate.WorkerConfig{}, nopLog())(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestFingerprintStable(t *testing.T) {
	a := fxmarkleasegate.ComputeSourcePrint("eth", "usdt", "3000", "kv", 1)
	b := fxmarkleasegate.ComputeSourcePrint("ETH", "USDT", "3000.0", "kv", 1)
	if !fxmarkleasegate.FingerprintsEqual(a, b) {
		t.Fatalf("%s vs %s", a, b)
	}
	if fxmarkleasegate.CombineGeneration(3, 2) != 3 {
		t.Fatal("gen")
	}
}
