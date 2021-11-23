package stucksettle_test

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/stucksettle"
)

var pt0 = time.Date(2025, 11, 22, 10, 0, 0, 0, time.UTC)

func makeTestLease(trID string, gen int64) *stucksettle.RecoverLease {
	return &stucksettle.RecoverLease{
		LeaseID:	"test-lease-" + trID,
		TransferID:	trID,
		Generation:	gen,
		State:		stucksettle.LeaseActing,
		HolderKey:	"test-node",
		OpenedAt:	pt0,
		ActingAt:	pt0,
		ExpiresAt:	pt0.Add(120 * time.Second),
		CreatedAt:	pt0,
		UpdatedAt:	pt0,
	}
}

func TestBuildEffectProof_Success(t *testing.T) {
	tests := []struct {
		name		string
		trID		string
		gen		int64
		action		string
		ledgerRef	string
	}{
		{"recover_with_ref", "tr-p001", 1, "recover", "bc-001"},
		{"fee_bump", "tr-p002", 2, "fee_bump", "bc-002"},
		{"fail_no_ref", "tr-p003", 3, "fail", ""},
		{"noop", "tr-p004", 1, "noop", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lease := makeTestLease(tc.trID, tc.gen)
			proof, err := stucksettle.BuildEffectProof(lease, tc.action, tc.ledgerRef, "ops-node", pt0)
			if err != nil {
				t.Fatalf("BuildEffectProof failed: %v", err)
			}
			if proof == nil {
				t.Fatal("proof is nil")
			}
			if !stucksettle.VerifyProof(proof) {
				t.Error("checksum invalid")
			}
			if proof.TransferID != tc.trID {
				t.Errorf("transfer_id mismatch: want %s got %s", tc.trID, proof.TransferID)
			}
			if proof.Generation != tc.gen {
				t.Errorf("generation mismatch: want %d got %d", tc.gen, proof.Generation)
			}
			if proof.Action != tc.action {
				t.Errorf("action mismatch: want %s got %s", tc.action, proof.Action)
			}
		})
	}
}

func TestBuildEffectProof_InvalidInput(t *testing.T) {
	lease := makeTestLease("tr-p010", 1)
	tests := []struct {
		name		string
		action		string
		issuedBy	string
		wantErr		bool
	}{
		{"empty_action", "", "ops", true},
		{"empty_issued_by", "recover", "", true},
		{"valid", "recover", "ops", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := stucksettle.BuildEffectProof(lease, tc.action, "ref", tc.issuedBy, pt0)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestVerifyProof_Tampered(t *testing.T) {
	lease := makeTestLease("tr-p020", 1)
	proof, _ := stucksettle.BuildEffectProof(lease, "recover", "bc-001", "ops", pt0)
	// 篡改 action
	proof.Action = "fee_bump"
	if stucksettle.VerifyProof(proof) {
		t.Error("tampered proof should fail verification")
	}
}

func TestVerifyProof_NilLease(t *testing.T) {
	_, err := stucksettle.BuildEffectProof(nil, "recover", "bc-001", "ops", pt0)
	if err != stucksettle.ErrUnavailable {
		t.Errorf("expected ErrUnavailable for nil lease, got %v", err)
	}
}

func TestVerifyProof_NilProof(t *testing.T) {
	if stucksettle.VerifyProof(nil) {
		t.Error("nil proof should not verify")
	}
}

func TestSummarize(t *testing.T) {
	lease := makeTestLease("tr-p030", 1)
	proof, _ := stucksettle.BuildEffectProof(lease, "recover", "bc-001", "ops", pt0)
	sum := stucksettle.Summarize(proof)
	if sum.EffectProofID != proof.EffectProofID {
		t.Error("effect_proof_id mismatch")
	}
	if !sum.Valid {
		t.Error("expected valid=true")
	}
	// 篡改后 Valid=false
	proof.LedgerRef = "tampered"
	sum2 := stucksettle.Summarize(proof)
	if sum2.Valid {
		t.Error("expected valid=false for tampered proof")
	}
}

func TestIntentOf(t *testing.T) {
	intent := stucksettle.IntentOf("tr-001", 3)
	expected := "RecoverStuck:tr-001:gen=3"
	if intent != expected {
		t.Errorf("expected %q, got %q", expected, intent)
	}
}
