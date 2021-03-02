package batchresiduefence_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/batchresiduefence"
)

func TestExpandedRetryFixtures(t *testing.T) {
	cells := batchresiduefence.ExpandedRetryFixtures()
	if len(cells) < 60 {
		t.Fatalf("thin %d", len(cells))
	}
	for _, c := range cells {
		got := batchresiduefence.DecidePolicy(batchresiduefence.PolicyInput{
			Phase:	c.Phase, Fence: c.Fence, HasFence: c.HasFence,
			HalfOpen:	c.HalfOpen, SlotActive: c.Slot, SameFence: c.Same,
			MutexKind:	c.Mutex, Expired: c.Expired,
		})
		if got != c.Want {
			t.Fatalf("%s got %s want %s", c.Name, got, c.Want)
		}
	}
	if batchresiduefence.CountRejectCells(cells) == 0 || batchresiduefence.CountAllowCells(cells) == 0 {
		t.Fatal("unbalanced")
	}
}
