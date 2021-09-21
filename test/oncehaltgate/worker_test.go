package oncehaltgate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/oncehaltgate"
)

func TestRevokeScanner_ScanOnce_NotHalted(t *testing.T) {
	gate, _ := newTestGate()
	scanner := oncehaltgate.NewRevokeScanner(gate,
		0)
	rep := scanner.ScanOnce(context.Background())
	if rep.InFlight != 0 {
		t.Errorf("not halted: in_flight want 0, got %d", rep.InFlight)
	}
}

func TestRevokeScanner_ScanOnce_Halted(t *testing.T) {
	gate, _ := newTestGate()
	gate.Switch(context.Background(), oncehaltgate.SwitchRequest{On: true, Actor: "ops"})
	scanner := oncehaltgate.NewRevokeScanner(gate, 0)
	rep := scanner.ScanOnce(context.Background())
	if rep.Generation == 0 {
		t.Error("halted: generation want > 0")
	}
}

func TestRevokeScanner_StartStop(t *testing.T) {
	gate, _ := newTestGate()
	scanner := oncehaltgate.NewRevokeScanner(gate, 50*time.Millisecond)
	ctx,
		cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	scanner.Start(ctx)
	time.Sleep(120 * time.Millisecond)
	scanner.Stop()
}

func TestDoubleMarkPatrol_PatrolOnce_NoCases(t *testing.T) {
	gate, _ := newTestGate()
	patrol := oncehaltgate.NewDoubleMarkPatrol(gate, 0)
	rep := patrol.PatrolOnce(context.Background())
	if rep.DoubleMarks != 0 {
		t.Errorf("no double marks want 0, got %d", rep.DoubleMarks)
	}
}

func TestDoubleMarkPatrol_PatrolOnce_WithDoubleMark(t *testing.T) {
	gate, svc := newTestGate()
	ev, _ := svc.Enqueue(context.Background(), "dm-1", "transfer", `{"x":1}`)
	// 注入一个 double_mark case。
	svc.HaltGenMgrRecordDoubleMark("dm-1", ev.ID, "other-id", ev.Epoch, ev.Token)
	patrol := oncehaltgate.NewDoubleMarkPatrol(gate,
		0)
	rep := patrol.PatrolOnce(context.Background())
	if rep.DoubleMarks != 1 {
		t.Errorf("want 1 double mark, got %d", rep.DoubleMarks)
	}
}

func TestWorkers_StartStop(t *testing.T) {
	gate, _ := newTestGate()
	w := oncehaltgate.NewWorkers(gate, 50*time.Millisecond, 100*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	w.Start(ctx)
	time.Sleep(120 * time.Millisecond)
	w.Stop()
	status := w.Status()
	_ = status.RevokeLast
	_ = status.PatrolLast
}

func TestWorkers_IdempotentStart(t *testing.T) {
	gate, _ := newTestGate()
	w := oncehaltgate.NewWorkers(gate, 0, 0)
	ctx := context.Background()
	w.Start(ctx)
	w.Start(ctx)	// idempotent
	w.Stop()
}

type workerTestCase struct {
	name	string
	run	func(t *testing.T)
}

var workerTests = []workerTestCase{
	{
		name:	"revoke_scanner_nil_gate",
		run: func(t *testing.T) {
			scanner := oncehaltgate.NewRevokeScanner(nil, 0)
			rep := scanner.ScanOnce(context.Background())
			if rep.InFlight != 0 {
				t.Error("nil gate: in_flight want 0")
			}
		},
	},
	{
		name:	"patrol_nil_gate",
		run: func(t *testing.T) {
			patrol := oncehaltgate.NewDoubleMarkPatrol(nil, 0)
			rep := patrol.PatrolOnce(context.Background())
			if rep.DoubleMarks != 0 {
				t.Error("nil gate: double_marks want 0")
			}
		},
	},
	{
		name:	"workers_nil_safe",
		run: func(t *testing.T) {
			var w *oncehaltgate.Workers
			w.Start(context.Background())
			w.Stop()
			st := w.Status()
			_ = st
		},
	},
}

func TestWorkers_TableDriven(t *testing.T) {
	for _, tc := range workerTests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t)
		})
	}
}
