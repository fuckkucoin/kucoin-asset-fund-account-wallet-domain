package batchresiduefence_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/batchresiduefence"
)

func TestRetryPolicyMatrix(t *testing.T) {
	for _, c := range batchresiduefence.RetryMatrix() {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			got := batchresiduefence.DecidePolicy(batchresiduefence.PolicyInput{
				Phase:	c.Phase, Fence: c.Fence, HasFence: c.HasFence,
				HalfOpen:	c.HalfOpen, SlotActive: c.Slot, SameFence: c.Same,
				MutexKind:	c.Mutex, Expired: c.Expired,
			})
			if got != c.Want {
				t.Fatalf("got %s want %s", got, c.Want)
			}
			if batchresiduefence.PolicyRejects(got) == (c.Want == batchresiduefence.PolicyAllowFirst || c.Want == batchresiduefence.PolicyAllowResume) {
				if batchresiduefence.PolicyRejects(got) && (c.Want == batchresiduefence.PolicyAllowFirst || c.Want == batchresiduefence.PolicyAllowResume) {
					t.Fatalf("reject mismatch %s", got)
				}
			}
			_ = batchresiduefence.PolicyReason(got)
		})
	}
}

func TestExhaustPhaseFenceHalf(t *testing.T) {
	n := 0
	for _, ph := range batchresiduefence.AllPhases() {
		for _, fs := range batchresiduefence.AllFenceStates() {
			for _, half := range []bool{false, true} {
				for _, slot := range []bool{false, true} {
					for _, same := range []bool{false, true} {
						for _, mu := range []string{"", batchresiduefence.MutexSubmit, batchresiduefence.MutexRetry} {
							in := batchresiduefence.PolicyInput{
								Phase:	ph, Fence: fs, HasFence: true,
								HalfOpen:	half, SlotActive: slot, SameFence: same, MutexKind: mu,
								Expired:	fs == batchresiduefence.FenceExpired,
							}
							d := batchresiduefence.DecidePolicy(in)
							if d == "" {
								t.Fatalf("empty decision %#v", in)
							}
							n++
						}
func TestFSMEdges(t *testing.T) {
	cases := []struct {
		from	batchresiduefence.FenceState
		ev	string
		ok	bool
		to	batchresiduefence.FenceState
	}{
		{batchresiduefence.FenceIssued, batchresiduefence.FenceEvFreeze, true, batchresiduefence.FenceFrozen},
		{batchresiduefence.FenceIssued, batchresiduefence.FenceEvExpire, true, batchresiduefence.FenceExpired},
		{batchresiduefence.FenceIssued, batchresiduefence.FenceEvRelease, true, batchresiduefence.FenceReleased},
		{batchresiduefence.FenceFrozen, batchresiduefence.FenceEvRelease, true, batchresiduefence.FenceReleased},
		{batchresiduefence.FenceFrozen, batchresiduefence.FenceEvExpire, true, batchresiduefence.FenceExpired},
		{batchresiduefence.FenceReleased, batchresiduefence.FenceEvFreeze, false, batchresiduefence.FenceReleased},
		{batchresiduefence.FenceExpired, batchresiduefence.FenceEvRelease, false, batchresiduefence.FenceExpired},
	}
	for _, c := range cases {
		next, err := batchresiduefence.MustApplyFence(c.from, c.ev)
		if c.ok {
			if err != nil || next != c.to {
				t.Fatalf("%s+%s got %s %v", c.from, c.ev, next, err)
			}
		} else if err == nil {
			t.Fatalf("expected fail %s+%s", c.from, c.ev)
		}
	}
	if _, err := batchresiduefence.MustApplyHalf(batchresiduefence.HalfOpen, batchresiduefence.HalfEvResume); err != nil {
		t.Fatal(err)
	}
	if _, err := batchresiduefence.MustApplyHalf(batchresiduefence.HalfOpen, batchresiduefence.HalfEvAbandon); err != nil {
		t.Fatal(err)
	}
	if _, err := batchresiduefence.MustApplyHalf(batchresiduefence.HalfResumed, batchresiduefence.HalfEvAbandon); err == nil {
		t.Fatal("terminal")
	}
	if !batchresiduefence.TerminalFence(batchresiduefence.FenceReleased) || !batchresiduefence.TerminalHalf(batchresiduefence.HalfAbandoned) {
		t.Fatal("terminal flags")
	}
	if !batchresiduefence.CanRetryHold(batchresiduefence.FenceIssued) || batchresiduefence.CanRetryHold(batchresiduefence.FenceExpired) {
		t.Fatal("hold flags")
	}
}

func TestFingerprintStable(t *testing.T) {
	a := batchresiduefence.FreezeFingerprint("b", "i", "u", "usdt", "1.0", "r")
	b := batchresiduefence.FreezeFingerprint("b", "i", "u", "USDT", "1.0", "r")
	if a != b || a == "" {
		t.Fatalf("fp %s %s", a, b)
	}
	if batchresiduefence.MatchFingerprint("", a) {
		t.Fatal("empty must not match")
	}
}

func TestValidateRejects(t *testing.T) {
	if _, err := batchresiduefence.RequireBatchID(""); err == nil {
		t.Fatal("batch")
	}
	if _, err := batchresiduefence.RequireItemIdem(""); err == nil {
		t.Fatal("idem")
	}
	if _, err := batchresiduefence.RequireActor(""); err == nil {
		t.Fatal("actor")
	}
	if _, err := batchresiduefence.RequireAmount("0"); err == nil {
		t.Fatal("amt")
	}
	if _, err := batchresiduefence.RequireAmount("x"); err == nil {
		t.Fatal("amt x")
	}
	if err := batchresiduefence.ForbidRetryPhase(batchresiduefence.PhaseSubmitted); err != batchresiduefence.ErrItemSubmitted {
		t.Fatal(err)
	}
	if err := batchresiduefence.ForbidRetryPhase(batchresiduefence.PhaseSkipped); err != batchresiduefence.ErrItemSkipped {
		t.Fatal(err)
	}
	if err := batchresiduefence.ForbidRetryPhase(batchresiduefence.PhaseFailed); err != nil {
		t.Fatal(err)
	}
}

func TestReasonCatalogFailClosed(t *testing.T) {
	for _, r := range batchresiduefence.ReasonCatalog() {
		if !r.FailClosed {
			t.Fatalf("%s must fail closed", r.Code)
		}
	}
}
