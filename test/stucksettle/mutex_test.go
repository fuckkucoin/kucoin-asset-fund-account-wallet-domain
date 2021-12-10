package stucksettle_test

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/stucksettle"
)

var mt0 = time.Date(2025, 11, 22, 10, 0, 0, 0, time.UTC)

func TestMutexStore_TryHold_Success(t *testing.T) {
	m := stucksettle.NewMutexStore()
	lock, err := m.TryHold("tr-m001", "node-A", mt0)
	if err != nil {
		t.Fatalf("TryHold failed: %v", err)
	}
	if lock.State != stucksettle.ALockHeld {
		t.Errorf("expected Held, got %q", lock.State)
	}
}

func TestMutexStore_TryHold_Conflict(t *testing.T) {
	m := stucksettle.NewMutexStore()
	m.TryHold("tr-m002", "node-A", mt0)
	_, err := m.TryHold("tr-m002", "node-B", mt0)
	if err != stucksettle.ErrAdvanceLocked {
		t.Errorf("expected ErrAdvanceLocked, got %v", err)
	}
}

func TestMutexStore_TryHold_Idempotent(t *testing.T) {
	m := stucksettle.NewMutexStore()
	l1, _ := m.TryHold("tr-m003", "node-A", mt0)
	l2, err := m.TryHold("tr-m003", "node-A", mt0)	// 同节点重入
	if err != nil {
		t.Fatalf("idempotent TryHold returned error: %v", err)
	}
	if l1.LockID != l2.LockID {
		t.Error("idempotent should return same lock")
	}
}

func TestMutexStore_TryHold_AutoExpire(t *testing.T) {
	m := stucksettle.NewMutexStore()
	m.TryHold("tr-m004", "node-A", mt0)
	// 推进时钟超过 TTL
	future := mt0.Add(stucksettle.DefaultAdvanceLockTTL + time.Second)
	// 新节点应能获锁（超时自动释放）
	lock, err := m.TryHold("tr-m004", "node-B", future)
	if err != nil {
		t.Fatalf("expected success after expiry, got %v", err)
	}
	if lock.HolderNode != "node-B" {
		t.Errorf("expected node-B holder, got %q", lock.HolderNode)
	}
}

func TestMutexStore_Release(t *testing.T) {
	m := stucksettle.NewMutexStore()
	m.TryHold("tr-m005", "node-A", mt0)
	err := m.Release("tr-m005", "node-A", mt0)
	if err != nil {
		t.Fatalf("Release failed: %v", err)
	}
	// 检查 CheckRecoverAllowed 通过
	err2 := m.CheckRecoverAllowed("tr-m005", false, mt0)
	if err2 != nil {
		t.Errorf("expected nil after release, got %v", err2)
	}
}

func TestMutexStore_Release_WrongNode(t *testing.T) {
	m := stucksettle.NewMutexStore()
	m.TryHold("tr-m006", "node-A", mt0)
	err := m.Release("tr-m006", "node-B", mt0)
	if err != stucksettle.ErrAdvanceLocked {
		t.Errorf("expected ErrAdvanceLocked for wrong node release, got %v", err)
	}
}

func TestMutexStore_CheckRecoverAllowed_HoldState(t *testing.T) {
	m := stucksettle.NewMutexStore()
	// isHoldState=true 即使无锁也拒绝
	err := m.CheckRecoverAllowed("tr-m007", true, mt0)
	if err != stucksettle.ErrHoldBypass {
		t.Errorf("expected ErrHoldBypass, got %v", err)
	}
}

func TestMutexStore_CheckRecoverAllowed_Locked(t *testing.T) {
	m := stucksettle.NewMutexStore()
	m.TryHold("tr-m008", "node-A", mt0)
	err := m.CheckRecoverAllowed("tr-m008", false, mt0)
	if err != stucksettle.ErrAdvanceLocked {
		t.Errorf("expected ErrAdvanceLocked, got %v", err)
	}
}

func TestMutexStore_SweepExpired(t *testing.T) {
	m := stucksettle.NewMutexStore()
	m.TryHold("tr-m009", "node-A", mt0)
	m.TryHold("tr-m010", "node-B", mt0)
	n := m.SweepExpired(mt0.Add(stucksettle.DefaultAdvanceLockTTL + time.Second))
	if n != 2 {
		t.Errorf("expected 2 expired swept, got %d", n)
	}
	// 之后 CheckRecoverAllowed 应通过
	for _, tr := range []string{"tr-m009", "tr-m010"} {
		if err := m.CheckRecoverAllowed(tr, false, mt0.Add(2*stucksettle.DefaultAdvanceLockTTL)); err != nil {
			t.Errorf("expected nil after sweep for %s, got %v", tr, err)
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// FSM 转换表
// ──────────────────────────────────────────────────────────────────────────────

func TestLeaseTransition_Legal(t *testing.T) {
	legal := []struct{ from, to stucksettle.RecoverLeaseState }{
		{stucksettle.LeaseOpen, stucksettle.LeaseActing},
		{stucksettle.LeaseOpen, stucksettle.LeaseExpired},
		{stucksettle.LeaseActing, stucksettle.LeaseRecovered},
		{stucksettle.LeaseActing, stucksettle.LeaseDupBlocked},
		{stucksettle.LeaseActing, stucksettle.LeaseExpired},
	}
	for _, tc := range legal {
		if err := stucksettle.LeaseTransition(tc.from, tc.to); err != nil {
			t.Errorf("legal transition %q→%q returned error: %v", tc.from, tc.to, err)
		}
	}
}

func TestLeaseTransition_Illegal(t *testing.T) {
	illegal := []struct{ from, to stucksettle.RecoverLeaseState }{
		{stucksettle.LeaseRecovered, stucksettle.LeaseActing},
		{stucksettle.LeaseDupBlocked, stucksettle.LeaseRecovered},
		{stucksettle.LeaseExpired, stucksettle.LeaseActing},
		{stucksettle.LeaseOpen, stucksettle.LeaseRecovered},
	}
	for _, tc := range illegal {
		if err := stucksettle.LeaseTransition(tc.from, tc.to); err == nil {
			t.Errorf("illegal transition %q→%q should have returned error", tc.from, tc.to)
		}
	}
}

func TestDupCaseTransition(t *testing.T) {
	legal := []struct {
		from, to stucksettle.DupRecoverCaseState
	}{
		{stucksettle.DupCaseOpen, stucksettle.DupCaseProven},
		{stucksettle.DupCaseOpen, stucksettle.DupCaseClosed},
		{stucksettle.DupCaseProven, stucksettle.DupCaseClosed},
	}
	for _, tc := range legal {
		if err := stucksettle.DupCaseTransition(tc.from, tc.to); err != nil {
			t.Errorf("legal dup case transition %q→%q: %v", tc.from, tc.to, err)
		}
	}
	// Closed → anything: error
	if err := stucksettle.DupCaseTransition(stucksettle.DupCaseClosed, stucksettle.DupCaseOpen); err == nil {
		t.Error("Closed→Open should be illegal")
	}
}
