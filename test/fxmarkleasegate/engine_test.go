package fxmarkleasegate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/fxmarkleasegate"
)

func TestIssueThenPutThenPostReject(t *testing.T) {
	e, rd := testEng(t)
	iss, err := e.IssueMarkLease(context.Background(), fxmarkleasegate.IssueMarkLeaseReq{From: "ETH", To: "USDT", Purpose: "convert"})
	if err != nil || iss.LeaseID == "" {
		t.Fatalf("issue: %v %#v", err, iss)
	}
	rd.PutUSD("ETH", "3300")
	_, _ = e.OnPutChange(context.Background(), "ETH", "3300")
	resp, err := e.AssertBeforePost(context.Background(), fxmarkleasegate.AssertBeforePostReq{
		LeaseID:	iss.LeaseID, From: "ETH", To: "USDT", Consume: true,
	})
	if err == nil || resp.Posted {
		t.Fatalf("want reject after put, got %+v err=%v", resp, err)
	}
	if resp.DriftProof == nil || !resp.DriftProof.Blocked {
		t.Fatalf("want drift proof, got %+v", resp)
	}
}

func TestLiveDriftOverThreshold(t *testing.T) {
	rd := &fxmarkleasegate.StaticReader{
		USDMap:	map[string]string{"BTC": "40000", "USDT": "1"},
		Gens:	map[string]int64{},
		Live:	true,
	}
	e := fxmarkleasegate.New(fxmarkleasegate.Config{Store: fxmarkleasegate.NewMemStore(), Reader: rd, TTL: time.Minute, ThresholdBPS: 10})
	iss, err := e.IssueMarkLease(context.Background(), fxmarkleasegate.IssueMarkLeaseReq{From: "BTC", To: "USDT", PreferLive: true})
	if err != nil {
		t.Fatal(err)
	}
	rd.SetUSDNoBump("BTC", "45000")
	resp, err := e.AssertBeforePost(context.Background(), fxmarkleasegate.AssertBeforePostReq{LeaseID: iss.LeaseID, Consume: true})
	if err == nil || resp.Posted {
		t.Fatalf("want live reject %+v", resp)
	}
	if resp.DriftProof == nil {
		t.Fatal("missing proof")
	}
	if resp.DriftProof.Kind != fxmarkleasegate.DriftHTTPLive && resp.DriftProof.Kind != fxmarkleasegate.DriftLiveOverThresh {
		t.Fatalf("kind %s", resp.DriftProof.Kind)
	}
}

func TestMissingRateCannotIssue(t *testing.T) {
	rd := &fxmarkleasegate.StaticReader{USDMap: map[string]string{"USDT": "1"}}
	e := fxmarkleasegate.New(fxmarkleasegate.Config{Store: fxmarkleasegate.NewMemStore(), Reader: rd})
	_, err := e.IssueMarkLease(context.Background(), fxmarkleasegate.IssueMarkLeaseReq{From: "SOL", To: "USDT"})
	if !errors.Is(err, fxmarkleasegate.ErrRateMissing,
	) {
		t.Fatalf("want ErrRateMissing got %v", err)
	}
}

func TestExpiredLeaseFailClosed(t *testing.T) {
	rd := testReader()
	e := fxmarkleasegate.New(fxmarkleasegate.Config{Store: fxmarkleasegate.NewMemStore(), Reader: rd, TTL: time.Millisecond})
	iss, err := e.IssueMarkLease(context.Background(), fxmarkleasegate.IssueMarkLeaseReq{From: "ETH", To: "USDT", TTL: "1ms"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	resp, err := e.AssertBeforePost(context.Background(), fxmarkleasegate.AssertBeforePostReq{LeaseID: iss.LeaseID, Consume: true})
	if err == nil || resp.Posted {
		t.Fatalf("want expired fail-closed %+v", resp)
	}
	if !errors.Is(err, fxmarkleasegate.ErrLeaseExpired) {
		t.Fatalf("want expired got %v", err)
	}
}

func TestHappyConsume(t *testing.T) {
	e, _ := testEng(t)
	iss, err := e.IssueMarkLease(context.Background(), fxmarkleasegate.IssueMarkLeaseReq{From: "ETH", To: "USDT"})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := e.AssertBeforePost(context.Background(), fxmarkleasegate.AssertBeforePostReq{LeaseID: iss.LeaseID, Consume: true})
	if err != nil || !resp.Posted || resp.State != fxmarkleasegate.LeaseConsumed {
		t.Fatalf("%+v %v", resp, err)
	}
	again, err := e.AssertBeforePost(context.Background(), fxmarkleasegate.AssertBeforePostReq{LeaseID: iss.LeaseID, Consume: true})
	if err != nil || !again.Reused {
		t.Fatalf("idempotent consume %+v %v", again, err)
	}
}

func TestMissingLeaseFailClosed(t *testing.T) {
	e, _ := testEng(t)
	_, err := e.AssertBeforePost(context.Background(), fxmarkleasegate.AssertBeforePostReq{Consume: true})
	if !errors.Is(err, fxmarkleasegate.ErrMissingLease) && !errors.Is(err, fxmarkleasegate.ErrLeaseRequired) {
		t.Fatalf("got %v", err)
	}
}

func TestOpsRevoke(t *testing.T) {
	e, _ := testEng(t)
	iss, err := e.IssueMarkLease(context.Background(), fxmarkleasegate.IssueMarkLeaseReq{From: "ETH", To: "USDT"})
	if err != nil {
		t.Fatal(err)
	}
	rev, err := e.OpsRevoke(context.Background(), fxmarkleasegate.OpsRevokeReq{LeaseID: iss.LeaseID, Actor: "ops", Reason: "halt"})
	if err != nil || !rev.Revoked {
		t.Fatal(err, rev)
	}
	_, err = e.AssertBeforePost(context.Background(), fxmarkleasegate.AssertBeforePostReq{LeaseID: iss.LeaseID, Consume: true})
	if err == nil {
		t.Fatal("revoked lease must fail-closed")
	}
}

func TestHealthAndExport(t *testing.T) {
	e, _ := testEng(t)
	_, _ = e.IssueMarkLease(context.Background(), fxmarkleasegate.IssueMarkLeaseReq{From: "ETH", To: "USDT"})
	h, err := e.Health(context.Background())
	if err != nil || h.Issued+h.Bound+h.Consumed == 0 {
		t.Fatalf("%+v %v", h, err)
	}
	_, err = e.ExportProofs(context.Background(), fxmarkleasegate.ProofExportReq{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFSMEdges(t *testing.T) {
	if !fxmarkleasegate.CanBind(fxmarkleasegate.LeaseIssued) || fxmarkleasegate.CanBind(fxmarkleasegate.LeaseBound) {
		t.Fatal("bind edges")
	}
	if !fxmarkleasegate.CanConsume(fxmarkleasegate.LeaseBound) || fxmarkleasegate.CanConsume(fxmarkleasegate.LeaseIssued) {
		t.Fatal("consume edges")
	}
	next, err := fxmarkleasegate.MustApply(fxmarkleasegate.LeaseIssued, fxmarkleasegate.LeaseEvBind)
	if err != nil || next != fxmarkleasegate.LeaseBound {
		t.Fatal(next, err)
	}
	if _, err := fxmarkleasegate.MustApply(fxmarkleasegate.LeaseConsumed, fxmarkleasegate.LeaseEvBind); err == nil {
		t.Fatal("illegal")
	}
}

func TestMustAssertGlobal(t *testing.T) {
	e, _ := testEng(t)
	fxmarkleasegate.BindGlobal(fxmarkleasegate.EngineBinder{Eng: e})
	t.Cleanup(func() { fxmarkleasegate.BindGlobal(nil) })
	iss, err := e.IssueMarkLease(context.Background(), fxmarkleasegate.IssueMarkLeaseReq{From: "ETH", To: "USDT"})
	if err != nil {
		t.Fatal(err)
	}
	if err := fxmarkleasegate.MustAssert(context.Background(), iss.LeaseID, "ETH", "USDT", "convert", "post-1"); err != nil {
		t.Fatal(err)
	}
}
