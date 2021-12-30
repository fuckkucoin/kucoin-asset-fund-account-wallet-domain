package wdtriplegate_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/wdtriplegate"
)

// TestDetectorProve Prove 产出 TripleDriftCase。
func TestDetectorProve(t *testing.T) {
	store := wdtriplegate.NewMemStore()
	d := wdtriplegate.NewDetector(store).WithClock(fixedClock(testNow))
	dc, err := d.Prove(wdtriplegate.DetectReq{
		MovementID:		"mv-det-01",
		TripleLeaseID:		"tl-det-01",
		DriftKind:		wdtriplegate.DriftListGen,
		LeaseListGen:		10,
		CurrentListGen:		11,
		LeasePolicyVer:		3,
		CurrentPolicyVer:	3,
		LeaseKYTPin:		"allow",
		CurrentKYTPin:		"allow",
	})
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	if dc.State != wdtriplegate.DriftOpen {
		t.Errorf("state: %q want open", dc.State)
	}
	if dc.ProofChecksum == "" {
		t.Error("proof_checksum should not be empty")
	}
	if dc.DriftKind != wdtriplegate.DriftListGen {
		t.Errorf("drift kind: %q want list_gen", dc.DriftKind)
	}
}

// TestDetectorProveIdempotent 同 movement_id+drift_kind+lease_id 重复调用幂等。

func TestDetectorProveIdempotent(t *testing.T) {
	store := wdtriplegate.NewMemStore()
	d := wdtriplegate.NewDetector(store).WithClock(fixedClock(testNow))
	req := wdtriplegate.DetectReq{
		MovementID:	"mv-det-idem",
		TripleLeaseID:	"tl-det-idem",
		DriftKind:	wdtriplegate.DriftPolicyVer,
		LeasePolicyVer:	5, CurrentPolicyVer: 6,
	}
	dc1, err1 := d.Prove(req)
	if err1 != nil {
		t.Fatalf("first prove: %v", err1)
	}
	dc2, err2 := d.Prove(req)
	if err2 != nil {
		t.Fatalf("second prove: %v", err2)
	}
	if dc1.DriftCaseID != dc2.DriftCaseID {
		t.Errorf("idempotent: got different IDs %q vs %q", dc1.DriftCaseID, dc2.DriftCaseID)
	}
}

// TestDetectorAttachProof Open→Proven 正常流程。
func TestDetectorAttachProof(t *testing.T) {
	store := wdtriplegate.NewMemStore()
	d := wdtriplegate.NewDetector(store).WithClock(fixedClock(testNow))
	dc, _ := d.Prove(wdtriplegate.DetectReq{
		MovementID:	"mv-det-02", TripleLeaseID: "tl-det-02",
		DriftKind:	wdtriplegate.DriftKYTPin, LeaseKYTPin: "allow", CurrentKYTPin: "deny",
	})
	proven, err := d.AttachProof(dc.DriftCaseID, "sha256checksum-abc")
	if err != nil {
		t.Fatalf("attach proof: %v", err)
	}
	if proven.State != wdtriplegate.DriftProven {
		t.Errorf("state: %q want proven", proven.State)
	}
	if proven.ProofChecksum != "sha256checksum-abc" {
		t.Errorf("checksum: %q want sha256checksum-abc", proven.ProofChecksum)
	}
}

// TestDetectorConfirmClose 双人确认关闭 Proven→Closed。
func TestDetectorConfirmClose(t *testing.T) {
	tests := []struct {
		name		string
		confirmedBy	string
		coConfirmedBy	string
		wantErr		error
	}{
		{
			name:		"valid dual confirm",
			confirmedBy:	"ops-alice", coConfirmedBy: "ops-bob",
			wantErr:	nil,
		},
		{
			name:		"same person",
			confirmedBy:	"ops-alice", coConfirmedBy: "ops-alice",
			wantErr:	wdtriplegate.ErrDualConfirm,
		},
		{
			name:		"empty confirmed_by",
			confirmedBy:	"", coConfirmedBy: "ops-bob",
			wantErr:	wdtriplegate.ErrDualConfirm,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := wdtriplegate.NewMemStore()
			d := wdtriplegate.NewDetector(store).WithClock(fixedClock(testNow))
			dc, _ := d.Prove(wdtriplegate.DetectReq{
				MovementID:	"mv-det-confirm-" + tt.name, TripleLeaseID: "tl-cc",
				DriftKind:	wdtriplegate.DriftListGen, LeaseListGen: 1, CurrentListGen: 2,
			})
			_, _ = d.AttachProof(dc.DriftCaseID, "checksum")
			_, err := d.ConfirmClose(wdtriplegate.ConfirmDriftCaseReq{
				DriftCaseID:	dc.DriftCaseID,
				ConfirmedBy:	tt.confirmedBy,
				CoConfirmedBy:	tt.coConfirmedBy,
				Note:		"test close",
			})
			if err != tt.wantErr {
				t.Errorf("ConfirmClose err=%v want=%v", err, tt.wantErr)
			}
		})
	}
}

// TestDetectorReConfirmTerminal 已关闭案件二次确认返回 ErrDriftCaseTerminal。
func TestDetectorReConfirmTerminal(t *testing.T) {
	store := wdtriplegate.NewMemStore()
	d := wdtriplegate.NewDetector(store).WithClock(fixedClock(testNow))
	dc, _ := d.Prove(wdtriplegate.DetectReq{
		MovementID:	"mv-det-term", TripleLeaseID: "tl-term",
		DriftKind:	wdtriplegate.DriftListGen, LeaseListGen: 5, CurrentListGen: 6,
	})
	_, _ = d.ConfirmClose(wdtriplegate.ConfirmDriftCaseReq{
		DriftCaseID:	dc.DriftCaseID,
		ConfirmedBy:	"ops-alice",
		CoConfirmedBy:	"ops-bob",
	})
	_, err := d.ConfirmClose(wdtriplegate.ConfirmDriftCaseReq{
		DriftCaseID:	dc.DriftCaseID,
		ConfirmedBy:	"ops-charlie",
		CoConfirmedBy:	"ops-dave",
	})
	if err != wdtriplegate.ErrDriftCaseTerminal {
		t.Errorf("re-confirm closed: got %v want ErrDriftCaseTerminal", err)
	}
}
