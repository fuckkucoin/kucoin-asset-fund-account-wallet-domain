package signpipegate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/signpipegate"
)

func TestWorkerPropagateFault(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	fc := &signpipegate.FaultCase{
		ID:	"f1", LeaseID: "l", MovementID: "mv", JobID: "j",
		Kind:	signpipegate.FaultSigner, Status: signpipegate.FaultInjected, Operator: "ops",
		IsolationMark:	signpipegate.IsolationMark, InjectedAt: time.Now().UTC(),
	}
	if err := svc.store.SaveFault(ctx, fc); err != nil {
		t.Fatal(err)
	}
	rep, err := signpipegate.NewWorker(svc).Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.FaultObserved == 0 {
		t.Fatalf("%+v", rep)
	}
	got, err := svc.GetFault(ctx, "f1")
	if err != nil || got.Status != signpipegate.FaultObserved {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestWorkerNil(t *testing.T) {
	w := signpipegate.NewWorker(nil)
	if _, err := w.Tick(context.Background()); err != signpipegate.ErrUnavailable {
		t.Fatalf("%v", err)
	}
}
