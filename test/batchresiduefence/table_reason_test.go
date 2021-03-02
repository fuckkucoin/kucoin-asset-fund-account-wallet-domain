package batchresiduefence_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/batchresiduefence"
)

func TestAllReasonsMapped(t *testing.T) {
	codes := []string{batchresiduefence.ReasonDupFreeze, batchresiduefence.ReasonNoFence, batchresiduefence.ReasonFenceExpired, batchresiduefence.ReasonFenceReleased, batchresiduefence.ReasonItemSubmitted, batchresiduefence.ReasonItemSkipped, batchresiduefence.ReasonItemNotFailed, batchresiduefence.ReasonSubmitRetryMutex, batchresiduefence.ReasonHalfCutNeedsResume, batchresiduefence.ReasonSecondFullFreeze, batchresiduefence.ReasonGenMismatch, batchresiduefence.ReasonFingerprintDrift, batchresiduefence.ReasonBatchMismatch, batchresiduefence.ReasonFailClosed,
	}
	for _, c := range codes {
		if _, ok := batchresiduefence.FindReason(c); !ok {
			t.Fatalf("unmapped %s", c)
		}
	}
}

func TestTablesNamed(t *testing.T) {
	want := []string{
		"wdbatch_residue_fences", "wdbatch_dup_freeze_proofs", "wdbatch_half_submit_cases",
		"wdbatch_freeze_slots", "wdbatch_residue_fence_audits", "wdbatch_submit_retry_mutex",
	}
	got := batchresiduefence.TableCatalog()
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s != %s", got[i], want[i])
		}
	}
}

func TestCopyHelpers(t *testing.T) {
	f := batchresiduefence.ResidueFence{ID: "1", State: batchresiduefence.FenceIssued}
	if f.Copy().ID != "1" {
		t.Fatal("fence copy")
	}
	p := batchresiduefence.BatchDupFreezeProof{ID: "p"}
	if p.Copy().ID != "p" {
		t.Fatal("proof copy")
	}
	h := batchresiduefence.HalfSubmitCase{ID: "h"}
	if h.Copy().ID != "h" {
		t.Fatal("half copy")
	}
}

func TestPolicyReasonStrings(t *testing.T) {
	for _, d := range []batchresiduefence.PolicyDecision{batchresiduefence.PolicyRejectDup, batchresiduefence.PolicyRejectPhase, batchresiduefence.PolicyRejectFence, batchresiduefence.PolicyRejectExpired, batchresiduefence.PolicyRejectMutex, batchresiduefence.PolicyAllowResume, batchresiduefence.PolicyAllowFirst,
	} {
		_ = batchresiduefence.PolicyReason(d)
		_ = batchresiduefence.PolicyRejects(d)
	}
}

func TestCanFlags(t *testing.T) {
	if !batchresiduefence.CanFreeze(batchresiduefence.FenceIssued) || batchresiduefence.CanFreeze(batchresiduefence.FenceFrozen) {
		t.Fatal("freeze")
	}
	if !batchresiduefence.CanRelease(batchresiduefence.FenceFrozen) || batchresiduefence.CanRelease(batchresiduefence.FenceReleased) {
		t.Fatal("release")
	}
	if !batchresiduefence.CanExpire(batchresiduefence.FenceIssued) || batchresiduefence.CanExpire(batchresiduefence.FenceExpired) {
		t.Fatal("expire")
	}
	if !batchresiduefence.CanResumeHalf(batchresiduefence.HalfOpen) || batchresiduefence.CanResumeHalf(batchresiduefence.HalfResumed) {
		t.Fatal("resume")
	}
	if !batchresiduefence.CanAbandonHalf(batchresiduefence.HalfOpen) || batchresiduefence.CanIssueNextGen(batchresiduefence.FenceIssued) {
		t.Fatal("nextgen/abandon")
	}
	if !batchresiduefence.CanIssueNextGen(batchresiduefence.FenceReleased) {
		t.Fatal("released next")
	}
}
