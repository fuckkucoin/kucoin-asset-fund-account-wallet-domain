package stucksettle_test

import (
	"fmt"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/stucksettle"
)

var gt0 = time.Date(2025, 11, 22, 10, 0, 0, 0, time.UTC)

func newTestGate(t *testing.T, recoverFn stucksettle.RecoverFn,) *stucksettle.Gate {
	t.Helper()
	store := stucksettle.NewMemStore()
	gate := stucksettle.NewGate(stucksettle.GateConfig{
		Store:		store,
		NodeID:		"test-node",
		Recover:	recoverFn,
	}).WithClock(fixedClock(gt0))
	return gate
}

// TestGuardedRecover_Success 正常回收路径：返回有效证明。
func TestGuardedRecover_Success(t *testing.T) {
	fn := func(trID string, now time.Time) (string, string, error) {
		return "recover", "bc-ref-001", nil
	}
	gate := newTestGate(t, fn)
	proof, err := gate.GuardedRecover("tr-g001", "node1:try1", false)
	if err != nil {
		t.Fatalf("GuardedRecover failed: %v", err)
	}
	if proof == nil {
		t.Fatal("proof is nil")
	}
	if !stucksettle.VerifyProof(proof) {
		t.Error("proof checksum invalid")
	}
}

// TestGuardedRecover_DupBlocked 相同 transfer_id 第二个不同持有者被封堵。

func TestGuardedRecover_DupBlocked(t *testing.T) {
	callCount := 0
	fn := func(trID string, now time.Time) (string, string, error) {
		callCount++
		return "recover", "bc-ref-002", nil
	}
	store := stucksettle.NewMemStore()
	gate := stucksettle.NewGate(stucksettle.GateConfig{Store: store, NodeID: "test-node", Recover: fn}).
		WithClock(fixedClock(gt0))
		// 第一次正常获取租约
	gate.engine.AcquireLease(stucksettle.AcquireLeaseReq{TransferID: "tr-g002", HolderKey: "node1"}, false)
	// 第二次不同 holderKey → DupBlocked
	_, err := gate.GuardedRecover("tr-g002", "node2", false)
	if err != stucksettle.ErrDupBlocked {
		t.Errorf("expected ErrDupBlocked, got %v", err)
	}
	if callCount != 0 {
		t.Errorf("recover fn should not be called for dup, called %d times", callCount)
	}
}

// TestGuardedRecover_HoldBypass Hold 态禁止旁路。

func TestGuardedRecover_HoldBypass(t *testing.T) {
	gate := newTestGate(t, nil)
	_, err := gate.GuardedRecover("tr-g003", "node1", true /* isHoldState */)
	if err != stucksettle.ErrHoldBypass {
		t.Errorf("expected ErrHoldBypass, got %v", err)
	}
}

// TestGuardedRecover_AdvanceMutex Advance 持锁后 Recover 被阻塞。
func TestGuardedRecover_AdvanceMutex(t *testing.T) {
	gate := newTestGate(t, nil)
	if err := gate.HoldAdvance("tr-g004"); err != nil {
		t.Fatalf("HoldAdvance failed: %v", err)
	}
	_, err := gate.GuardedRecover("tr-g004", "node1", false)
	if err != stucksettle.ErrAdvanceLocked {
		t.Errorf("expected ErrAdvanceLocked, got %v", err)
	}
}

// TestGuardedRecover_AdvanceMutexRelease Advance 释放后 Recover 允许。

func TestGuardedRecover_AdvanceMutexRelease(t *testing.T) {
	fn := func(trID string, now time.Time) (string, string, error) {
		return "recover", "bc-005", nil
	}
	gate := newTestGate(t, fn)
	gate.HoldAdvance("tr-g005")
	gate.ReleaseAdvance("tr-g005")
	proof, err := gate.GuardedRecover("tr-g005", "node1", false)
	if err != nil {
		t.Fatalf("expected success after release, got %v", err)
	}
	if proof == nil {
		t.Error("expected proof, got nil")
	}
}

// TestGuardedRecover_ActionError 回收动作失败不固化证明。
func TestGuardedRecover_ActionError(t *testing.T) {
	fn := func(trID string, now time.Time) (string, string, error) {
		return "", "", fmt.Errorf("withdraw service unavailable")
	}
	gate := newTestGate(t, fn)
	proof, err := gate.GuardedRecover("tr-g006", "node1", false)
	if err == nil {
		t.Fatal("expected error for failed action")
	}
	if proof != nil {
		t.Error("proof should be nil for failed action")
	}
	// 证明未写入：GetEffectProof 应返回 ErrProofMissing
	_, proofErr := gate.engine.GetEffectProof("tr-g006")
	if proofErr != stucksettle.ErrProofMissing {
		t.Errorf("expected ErrProofMissing after failed action, got %v", proofErr)
	}
}

// TestExportProof_RoundTrip 证明导出包含完整信息。
func TestExportProof_RoundTrip(t *testing.T) {
	fn := func(trID string, now time.Time) (string, string, error) {
		return "fee_bump", "bc-007", nil
	}
	gate := newTestGate(t, fn)
	gate.GuardedRecover("tr-g007", "node1", false)
	export, err := gate.ExportProof(stucksettle.ProofExportReq{TransferID: "tr-g007"})
	if err != nil {
		t.Fatalf("ExportProof failed: %v", err)
	}
	if export.Proof.Action != "fee_bump" {
		t.Errorf("expected action=fee_bump, got %q", export.Proof.Action)
	}
	if export.Proof.TransferID != "tr-g007" {
		t.Errorf("expected transfer_id=tr-g007, got %q", export.Proof.TransferID)
	}
	if !stucksettle.VerifyProof(&export.Proof) {
		t.Error("exported proof checksum invalid")
	}
}

// TestConfirmDupCase_ViaGate 通过 Gate 接口确认双结案件。

func TestConfirmDupCase_ViaGate(t *testing.T) {
	store := stucksettle.NewMemStore()
	gate := stucksettle.NewGate(stucksettle.GateConfig{Store: store, NodeID: "test-node"}).WithClock(fixedClock(gt0))
	// 产出双结
	gate.engine.AcquireLease(stucksettle.AcquireLeaseReq{TransferID: "tr-g010", HolderKey: "n1"}, false)
	gate.engine.AcquireLease(stucksettle.AcquireLeaseReq{TransferID: "tr-g010", HolderKey: "n2"}, false)
	cases, _ := gate.DupCasesByTransfer("tr-g010")
	if len(cases) == 0 {
		t.Fatal("no dup case produced")
	}
	result, err := gate.ConfirmDupCase(stucksettle.ConfirmDupCaseReq{
		DupCaseID:	cases[0].DupCaseID,
		ConfirmedBy:	"alice",
		CoConfirmedBy:	"bob",
		Note:		"ok",
	})
	if err != nil {
		t.Fatalf("ConfirmDupCase via gate failed: %v", err)
	}
	if result.State != stucksettle.DupCaseClosed {
		t.Errorf("expected Closed, got %q", result.State)
	}
}
