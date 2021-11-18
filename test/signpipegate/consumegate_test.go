package signpipegate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/signpipegate"
)

func TestSealFenceMismatch(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	l, err := svc.ArmRetry(ctx, signpipegate.ArmReq{
		MovementID:	"mv-fn", JobID: "job-fn", Chain: "ETH", To: "0xfn",
		Amount:	"1.000000000000000000", Nonce: "n-fn", Actor: "sig-fn",
		FenceEpoch:	2, FenceToken: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Enter(ctx, l.ID); err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.Seal(ctx, signpipegate.SealReq{LeaseID: l.ID, Actor: "sig-fn", SigHex: "aa", FenceEpoch: 9, FenceToken: "stale"})
	if err != signpipegate.ErrFenceStale {
		t.Fatalf("%v", err)
	}
}

func TestSealEmptySig(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	l, err := svc.ArmRetry(ctx, armReq("mv-es", "job-es", "ETH", "0xes", "1.000000000000000000", "n-es", "sig-es"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Enter(ctx, l.ID); err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.Seal(ctx, signpipegate.SealReq{LeaseID: l.ID, Actor: "sig-es"})
	if err != signpipegate.ErrSignerNil {
		t.Fatalf("%v", err)
	}
}

func TestEngineSealPath(t *testing.T) {
	svc := newTestSvc()
	eng := signpipegate.NewEngine(svc)
	l, art, err := eng.SealPath(context.Background(), armReq("mv-en", "job-en", "ETH", "0xen", "1.000000000000000000", "n-en", "sig-en"), "ccdd")
	if err != nil || art == nil || l.Status != signpipegate.LeaseSealed {
		t.Fatalf("%v %+v", err, l)
	}
	if err := eng.AssertNoDouble(context.Background(), "mv-en"); err != nil {
		t.Fatal(err)
	}
}

func TestClassifyObserve(t *testing.T) {
	k, c, _ := signpipegate.ClassifyObserve(&signpipegate.ObservedJob{State: "failed", Retries: 3}, time.Now().UTC())
	if k != signpipegate.FaultRetryExhausted || c != signpipegate.CloseExhausted {
		t.Fatalf("%s %s", k, c)
	}
	k, c, _ = signpipegate.ClassifyObserve(&signpipegate.ObservedJob{State: "signed"}, time.Now().UTC())
	if k != signpipegate.FaultDualBroadcast || c != signpipegate.CloseDoubleGuard {
		t.Fatalf("%s %s", k, c)
	}
}
