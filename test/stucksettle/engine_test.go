package stucksettle_test

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/stucksettle"
)

var t0 = time.Date(2025, 11, 22, 10, 0, 0, 0, time.UTC)

func fixedClock(t time.Time) func() time.Time	{ return func() time.Time { return t } }

func newTestEngine(t *testing.T) (*stucksettle.Engine, *stucksettle.MemStore,) {
	t.Helper()
	store := stucksettle.NewMemStore()
	mutex := stucksettle.NewMutexStore()
	eng := stucksettle.NewEngine(store, mutex).WithClock(fixedClock(t0))
	return eng, store
}

// ──────────────────────────────────────────────────────────────────────────────
// AcquireLease
// ──────────────────────────────────────────────────────────────────────────────

func TestAcquireLease_FirstTime(t *testing.T) {
	eng, _ := newTestEngine(t)
	req := stucksettle.AcquireLeaseReq{TransferID: "tr-001", HolderKey: "node1:try1"}
	lease, err := eng.AcquireLease(req, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lease.State != stucksettle.LeaseActing {
		t.Errorf("expected Acting, got %q", lease.State)
	}
	if lease.Generation != 1 {
		t.Errorf("expected generation=1, got %d", lease.Generation)
	}
}

func TestAcquireLease_Idempotent(t *testing.T) {
	eng, _ := newTestEngine(t)
	req := stucksettle.AcquireLeaseReq{TransferID: "tr-002", HolderKey: "node1:try1"}
	l1, _ := eng.AcquireLease(req, false)
	l2, err := eng.AcquireLease(req, false)
	if err != nil {
		t.Fatalf("idempotent call returned error: %v", err)
	}
	if l1.LeaseID != l2.LeaseID {
		t.Errorf("idempotent call returned different lease: %s vs %s", l1.LeaseID, l2.LeaseID)
	}
}

func TestAcquireLease_DupBlocked(t *testing.T) {
	eng, store := newTestEngine(t)
	req1 := stucksettle.AcquireLeaseReq{TransferID: "tr-003", HolderKey: "node1:try1"}
	req2 := stucksettle.AcquireLeaseReq{TransferID: "tr-003", HolderKey: "node2:try1"}	// 不同持有者
	_,
		err1 := eng.AcquireLease(req1, false)
	if err1 != nil {
		t.Fatalf("first acquire failed: %v", err1)
	}
	dupLease, err2 := eng.AcquireLease(req2, false)
	if err2 != stucksettle.ErrDupBlocked {
		t.Fatalf("expected ErrDupBlocked, got %v", err2)
	}
	if dupLease == nil || dupLease.State != stucksettle.LeaseDupBlocked {
		t.Errorf("dup lease state expected DupBlocked, got %v", dupLease)
	}
	// DupRecoverCase 应被产出
	cases, _ := store.DupCasesByTransfer("tr-003")
	if len(cases) != 1 {
		t.Errorf("expected 1 dup case, got %d", len(cases))
	}
	if cases[0].State != stucksettle.DupCaseOpen {
		t.Errorf("dup case state expected Open, got %q", cases[0].State)
	}
}

func TestAcquireLease_HoldBypass(t *testing.T) {
	eng, _ := newTestEngine(t)
	req := stucksettle.AcquireLeaseReq{TransferID: "tr-004", HolderKey: "node1:try1"}
	_, err := eng.AcquireLease(req, true)	// isHoldState=true
	if err != stucksettle.ErrHoldBypass {
		t.Errorf("expected ErrHoldBypass, got %v", err)
	}
}

func TestAcquireLease_AdvanceLocked(t *testing.T) {
	eng, _ := newTestEngine(t)
	mutex := eng.mutex
	_, err := mutex.TryHold("tr-005", "advance-node", t0)
	if err != nil {
		t.Fatalf("TryHold failed: %v", err)
	}
	req := stucksettle.AcquireLeaseReq{TransferID: "tr-005", HolderKey: "node1:try1"}
	_, err = eng.AcquireLease(req, false)
	if err != stucksettle.ErrAdvanceLocked {
		t.Errorf("expected ErrAdvanceLocked, got %v", err)
	}
}

func TestAcquireLease_InvalidInput(t *testing.T) {
	tests := []struct {
		name	string
		req	stucksettle.AcquireLeaseReq
	}{
		{"empty transfer_id", stucksettle.AcquireLeaseReq{TransferID: "", HolderKey: "k"}},
		{"empty holder_key", stucksettle.AcquireLeaseReq{TransferID: "tr", HolderKey: ""}},
	}
	eng, _ := newTestEngine(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := eng.AcquireLease(tc.req, false)
			if err != stucksettle.ErrInvalidInput {
				t.Errorf("expected ErrInvalidInput, got %v", err)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// CommitRecover
// ──────────────────────────────────────────────────────────────────────────────

func TestCommitRecover_Success(t *testing.T) {
	eng, _ := newTestEngine(t)
	req := stucksettle.AcquireLeaseReq{TransferID: "tr-010", HolderKey: "node1:try1"}
	lease, _ := eng.AcquireLease(req, false)
	proof, err := eng.CommitRecover(stucksettle.CommitRecoverReq{
		LeaseID:	lease.LeaseID,
		Action:		"recover",
		LedgerRef:	"bc-001",
		IssuedBy:	"ops-worker",
	})
	if err != nil {
		t.Fatalf("CommitRecover failed: %v", err)
	}
	if proof == nil {
		t.Fatal("proof is nil")
	}
	if !stucksettle.VerifyProof(proof) {
		t.Error("proof checksum invalid")
	}
	if proof.Generation != 1 {
		t.Errorf("expected generation=1, got %d", proof.Generation)
	}
	// 租约应转为 Recovered
	updated, _ := eng.store.GetLease(lease.LeaseID)
	if updated.State != stucksettle.LeaseRecovered {
		t.Errorf("expected Recovered, got %q", updated.State)
	}
}

func TestCommitRecover_ProofMissing_EmptyAction(t *testing.T) {
	eng, _ := newTestEngine(t)
	req := stucksettle.AcquireLeaseReq{TransferID: "tr-011", HolderKey: "node1:try1"}
	lease, _ := eng.AcquireLease(req, false)
	_, err := eng.CommitRecover(stucksettle.CommitRecoverReq{
		LeaseID:	lease.LeaseID,
		Action:		"",	// 空 action
		LedgerRef:	"bc-001",
		IssuedBy:	"ops-worker",
	})
	if err != stucksettle.ErrProofMissing {
		t.Errorf("expected ErrProofMissing, got %v", err)
	}
}

func TestCommitRecover_LeaseExpired(t *testing.T) {
	store := stucksettle.NewMemStore()
	mutex := stucksettle.NewMutexStore()
	pastClock := fixedClock(t0)
	eng := stucksettle.NewEngine(store, mutex).WithClock(pastClock)
	req := stucksettle.AcquireLeaseReq{TransferID: "tr-012", HolderKey: "node1:try1", TTLSeconds: 1}
	lease, _ := eng.AcquireLease(req, false)
	// 推进时钟 > TTL
	futureClock := fixedClock(t0.Add(5 * time.Second))
	eng.WithClock(futureClock)
	_, err := eng.CommitRecover(stucksettle.CommitRecoverReq{
		LeaseID:	lease.LeaseID,
		Action:		"recover",
		LedgerRef:	"bc-001",
		IssuedBy:	"ops-worker",
	})
	if err != stucksettle.ErrLeaseExpired {
		t.Errorf("expected ErrLeaseExpired, got %v", err)
	}
}

func TestCommitRecover_TerminalLease(t *testing.T) {
	eng, _ := newTestEngine(t)
	req := stucksettle.AcquireLeaseReq{TransferID: "tr-013", HolderKey: "node1:try1"}
	lease, _ := eng.AcquireLease(req, false)
	// 第一次 Commit
	eng.CommitRecover(stucksettle.CommitRecoverReq{
		LeaseID:	lease.LeaseID, Action: "recover", LedgerRef: "bc-001", IssuedBy: "ops",
	})
	// 第二次 Commit 应返回 ErrLeaseTerminal
	_, err := eng.CommitRecover(stucksettle.CommitRecoverReq{
		LeaseID:	lease.LeaseID, Action: "recover", LedgerRef: "bc-001", IssuedBy: "ops",
	})
	if err != stucksettle.ErrLeaseTerminal {
		t.Errorf("expected ErrLeaseTerminal, got %v", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// GetEffectProof — 证明缺失不标 Recovered
// ──────────────────────────────────────────────────────────────────────────────

func TestGetEffectProof_Missing(t *testing.T) {
	eng, _ := newTestEngine(t)
	_, err := eng.GetEffectProof("tr-no-recover")
	if err != stucksettle.ErrProofMissing {
		t.Errorf("expected ErrProofMissing, got %v", err)
	}
}

func TestGetEffectProof_AfterCommit(t *testing.T) {
	eng, _ := newTestEngine(t)
	req := stucksettle.AcquireLeaseReq{TransferID: "tr-020", HolderKey: "node1:try1"}
	lease, _ := eng.AcquireLease(req, false)
	eng.CommitRecover(stucksettle.CommitRecoverReq{
		LeaseID:	lease.LeaseID, Action: "fail", LedgerRef: "", IssuedBy: "ops",
	})
	proof, err := eng.GetEffectProof("tr-020")
	if err != nil {
		t.Fatalf("expected proof, got error: %v", err)
	}
	if proof.Action != "fail" {
		t.Errorf("expected action=fail, got %q", proof.Action)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// ConfirmDupCase
// ──────────────────────────────────────────────────────────────────────────────

func TestConfirmDupCase_DualConfirm(t *testing.T) {
	eng, store := newTestEngine(t)
	// 产出一个 DupRecoverCase
	r1 := stucksettle.AcquireLeaseReq{TransferID: "tr-030", HolderKey: "node1"}
	r2 := stucksettle.AcquireLeaseReq{TransferID: "tr-030", HolderKey: "node2"}
	eng.AcquireLease(r1, false)
	eng.AcquireLease(r2, false)	// → ErrDupBlocked
	cases, _ := store.DupCasesByTransfer("tr-030")
	if len(cases) == 0 {
		t.Fatal("no dup case produced")
	}
	caseID := cases[0].DupCaseID
	// 双人确认
	result, err := eng.ConfirmDupCase(stucksettle.ConfirmDupCaseReq{
		DupCaseID:	caseID,
		ConfirmedBy:	"ops-alice",
		CoConfirmedBy:	"ops-bob",
		Note:		"verified no double spend",
	})
	if err != nil {
		t.Fatalf("ConfirmDupCase failed: %v", err)
	}
	if result.State != stucksettle.DupCaseClosed {
		t.Errorf("expected Closed, got %q", result.State)
	}
}

func TestConfirmDupCase_SameActor(t *testing.T) {
	eng, store := newTestEngine(t)
	r1 := stucksettle.AcquireLeaseReq{TransferID: "tr-031", HolderKey: "node1"}
	r2 := stucksettle.AcquireLeaseReq{TransferID: "tr-031", HolderKey: "node2"}
	eng.AcquireLease(r1, false)
	eng.AcquireLease(r2, false)
	cases, _ := store.DupCasesByTransfer("tr-031")
	_, err := eng.ConfirmDupCase(stucksettle.ConfirmDupCaseReq{
		DupCaseID:	cases[0].DupCaseID,
		ConfirmedBy:	"ops-alice",
		CoConfirmedBy:	"ops-alice",	// 相同
	})
	if err != stucksettle.ErrInvalidActor {
		t.Errorf("expected ErrInvalidActor, got %v", err)
	}
}

func TestConfirmDupCase_AlreadyClosed(t *testing.T) {
	eng, store := newTestEngine(t)
	r1 := stucksettle.AcquireLeaseReq{TransferID: "tr-032", HolderKey: "node1"}
	r2 := stucksettle.AcquireLeaseReq{TransferID: "tr-032", HolderKey: "node2"}
	eng.AcquireLease(r1, false)
	eng.AcquireLease(r2, false)
	cases, _ := store.DupCasesByTransfer("tr-032")
	caseID := cases[0].DupCaseID
	eng.ConfirmDupCase(stucksettle.ConfirmDupCaseReq{
		DupCaseID:	caseID,
		ConfirmedBy:	"alice",
		CoConfirmedBy:	"bob",
	})
	_, err := eng.ConfirmDupCase(stucksettle.ConfirmDupCaseReq{
		DupCaseID:	caseID,
		ConfirmedBy:	"alice",
		CoConfirmedBy:	"bob",
	})
	if err != stucksettle.ErrCaseAlreadyClosed {
		t.Errorf("expected ErrCaseAlreadyClosed, got %v", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// StuckGeneration
// ──────────────────────────────────────────────────────────────────────────────

func TestGenBump_Monotonic(t *testing.T) {
	eng, _ := newTestEngine(t)
	for i := 1; i <= 5; i++ {
		g, err := eng.store.BumpGen("tr-040", t0)
		if err != nil {
			t.Fatalf("BumpGen[%d] failed: %v", i, err)
		}
		if g.CurrentGen != int64(i) {
			t.Errorf("expected gen=%d, got %d", i, g.CurrentGen)
		}
	}
}

func TestGenStatus_NotCreated(t *testing.T) {
	eng, _ := newTestEngine(t)
	gen, err := eng.GetOrCreateGen("tr-new")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gen.CurrentGen != 0 {
		t.Errorf("expected gen=0 for new transfer, got %d", gen.CurrentGen)
	}
}
