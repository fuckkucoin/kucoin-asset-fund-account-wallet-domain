package incdef_test

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/stucksettle"
	"account-assets.kucoin.net/funding/internal/incdef"
)

var sg0 = time.Date(2025, 11, 22, 10, 0, 0, 0, time.UTC)

func newTestServiceWithGate(t *testing.T, recoverFn stucksettle.RecoverFn) (*incdef.Service, *stucksettle.Gate) {
	t.Helper()
	store := stucksettle.NewMemStore()
	gate := stucksettle.NewGate(stucksettle.GateConfig{
		Store:		store,
		NodeID:		"test-incdef",
		Recover:	recoverFn,
	})
	svc := incdef.New()
	svc.bindStuckGate(gate)
	svc.clock = func() time.Time { return sg0 }
	return svc, gate
}

// TestGuardedRecoverStuck_Success 正常回收路径通过世代闸。

func TestGuardedRecoverStuck_Success(t *testing.T) {
	fn := func(trID string, now time.Time) (string, string, error) {
		return "recover", "bc-sg001", nil
	}
	svc,
		_ := newTestServiceWithGate(t, fn)
	svc.OpenStuck("tr-sg001", "broadcast stuck")
	rec, err := svc.GuardedRecoverStuck("tr-sg001", false)
	if err != nil {
		t.Fatalf("GuardedRecoverStuck failed: %v", err)
	}
	if rec.Action != "recover" {
		t.Errorf("expected action=recover, got %q", rec.Action)
	}
}

// TestGuardedRecoverStuck_HoldBypass Hold 态禁止旁路。
func TestGuardedRecoverStuck_HoldBypass(t *testing.T) {
	svc, _ := newTestServiceWithGate(t, nil)
	svc.OpenStuck("tr-sg002", "stuck")
	// isHoldState=true
	_, err := svc.GuardedRecoverStuck("tr-sg002", true)
	if err != incdef.ErrStuck {
		t.Errorf("expected ErrStuck (hold bypass), got %v", err)
	}
}

// TestGuardedRecoverStuck_NoGate 无 Gate 时退化为原有路径（wd 未绑定返回 ErrUnavailable）。

func TestGuardedRecoverStuck_NoGate(t *testing.T) {
	svc := incdef.New()
	svc.clock = func() time.Time { return sg0 }
	// wd 未绑定，RecoverStuck 内部返回 ErrUnavailable（无 Gate 直接透传）
	_, err := svc.GuardedRecoverStuck("tr-sg003", false)
	if err != incdef.ErrUnavailable {
		t.Errorf("expected ErrUnavailable for no-gate/no-wd path, got %v", err)
	}
}

// TestStuckGenStatus_Available Gate 绑定后查询世代状态成功。
func TestStuckGenStatus_Available(t *testing.T) {
	fn := func(trID string, now time.Time) (string, string, error) {
		return "recover", "bc-sg010", nil
	}
	svc, _ := newTestServiceWithGate(t, fn)
	svc.OpenStuck("tr-sg010", "stuck")
	svc.GuardedRecoverStuck("tr-sg010", false)
	gen, err := svc.StuckGenStatus("tr-sg010")
	if err != nil {
		t.Fatalf("StuckGenStatus failed: %v", err)
	}
	if gen.CurrentGen != 1 {
		t.Errorf("expected gen=1, got %d", gen.CurrentGen)
	}
}

// TestStuckDupCases_Available 产出双结后 StuckDupCases 返回案件。

func TestStuckDupCases_Available(t *testing.T) {
	store := stucksettle.NewMemStore()
	gate := stucksettle.NewGate(stucksettle.GateConfig{Store: store, NodeID: "test-incdef"})
	svc := incdef.New()
	svc.bindStuckGate(gate)
	// 手动在 gate engine 中产出双结
	gate.Engine().AcquireLease(stucksettle.AcquireLeaseReq{TransferID: "tr-sg020", HolderKey: "n1"}, false)
	gate.Engine().AcquireLease(stucksettle.AcquireLeaseReq{TransferID: "tr-sg020", HolderKey: "n2"}, false)
	cases, err := svc.StuckDupCases("tr-sg020")
	if err != nil {
		t.Fatalf("StuckDupCases failed: %v", err)
	}
	if len(cases) != 1 {
		t.Errorf("expected 1 dup case, got %d", len(cases))
	}
}
