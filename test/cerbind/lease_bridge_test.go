package cerbind_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/cerdecidegate"
	"account-assets.kucoin.net/funding/internal/cerbind"
)

func TestDecideWithLeaseRequiredIssues(t *testing.T) {
	eng := cerbind.New(cerbind.NewMem())
	cerbind.AttachLeaseGate(eng)
	ctx := context.Background()
	got, err := eng.DecideWithLease(ctx, cerbind.PlanReq{
		Actor:	"ops-desk-11", BatchID: "batch-bridge-1",
		Asset:	"BTC", Chain: "BTC", Amount: "12", Dest: "bc1qcerdecidebridge01",
		DestRisk:	"high", Side: cerbind.SideCold, Purpose: cerbind.PurposeWithdraw, Approved: true,
	}, "idem-bridge-1", 120)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Decision.Required {
		t.Fatalf("want required %+v", got.Decision)
	}
	if got.LeaseID == "" {
		t.Fatal("missing decide_lease_id")
	}
}

func TestDecideWithLeaseExemptNoID(t *testing.T) {
	eng := cerbind.New(cerbind.NewMem())
	cerbind.AttachLeaseGate(eng)
	ctx := context.Background()
	// 小额热路径若命中豁免策略则无租约。
	got, err := eng.DecideWithLease(ctx, cerbind.PlanReq{
		Actor:	"ops-desk-12", BatchID: "batch-bridge-ex",
		Asset:	"USDT", Chain: "TRON", Amount: "1", Dest: "Tcerdecidebridge02",
		DestRisk:	"low", Side: cerbind.SideHot, Purpose: cerbind.PurposeWithdraw, Approved: true,
	}, "idem-bridge-ex", 120)
	if err != nil {
		t.Fatal(err)
	}
	if got.Decision.Required && got.LeaseID == "" {
		t.Fatal("required must mint lease")
	}
	if !got.Decision.Required && got.LeaseID != "" {
		t.Fatal("exempt must not mint force lease")
	}
}

func TestAssertExecRequiresLease(t *testing.T) {
	eng := cerbind.New(cerbind.NewMem())
	cerbind.AttachLeaseGate(eng)
	_, err := eng.AssertExecLease(context.Background(), "", "ops-desk-13", "exec", cerbind.Subject{BatchID: "b"})
	if err != cerdecidegate.ErrNoLease {
		t.Fatalf("want no lease got %v", err)
	}
}
