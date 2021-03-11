package cerdecidegate_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/cerdecidegate"
)

func TestLineDigestStable(t *testing.T) {
	a := cerdecidegate.LineFact{BatchID: "b1", Asset: "btc", Chain: "btc", Amount: "1", Dest: "d", DestRisk: "HIGH", Side: "COLD", Purpose: "Withdraw"}
	b := cerdecidegate.LineFact{BatchID: "b1", Asset: "BTC", Chain: "BTC", Amount: "1", Dest: "d", DestRisk: "high", Side: "cold", Purpose: "withdraw"}
	if cerdecidegate.LineDigest(a) != cerdecidegate.LineDigest(b) {
		t.Fatal("normalize should stabilize digest")
	}
	b.Amount = "2"
	if cerdecidegate.LineDigest(a) == cerdecidegate.LineDigest(b) {
		t.Fatal("amount change must change digest")
	}
}

func TestCompareLivePriority(t *testing.T) {
	base := cerdecidegate.PolicyFingerprint{PolicyID: "p", ForceCold: true, Threshold: 3, Shares: 5, DestRisk: "medium", Required: true, Digest: "x", Generation: 1}
	live := base
	live.Generation = 2
	live.Digest = "y"
	if err := cerdecidegate.CompareLive(base, live); err != cerdecidegate.ErrGenStale {
		t.Fatalf("gen: %v", err)
	}
	live = base
	live.DestRisk = "critical"
	live.Digest = "y"
	if err := cerdecidegate.CompareLive(base, live); err != cerdecidegate.ErrRiskRaise {
		t.Fatalf("risk: %v", err)
	}
	live = base
	live.ForceCold = false
	live.Required = false
	live.Digest = "y"
	if err := cerdecidegate.CompareLive(base, live); err != cerdecidegate.ErrForceFlip {
		t.Fatalf("force: %v", err)
	}
	live = base
	live.Threshold = 5
	live.Digest = "y"
	if err := cerdecidegate.CompareLive(base, live); err != cerdecidegate.ErrThresholdDrift {
		t.Fatalf("th: %v", err)
	}
}

func TestPackDigestChanges(t *testing.T) {
	a := cerdecidegate.PackDigest("p", true, 3, 5, "high", "one", true)
	b := cerdecidegate.PackDigest("p", true, 4, 5, "high", "one", true)
	if a == b {
		t.Fatal("threshold must change pack digest")
	}
}
