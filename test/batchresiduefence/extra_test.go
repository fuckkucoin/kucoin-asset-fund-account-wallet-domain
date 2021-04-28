package batchresiduefence_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/batchresiduefence"
)

func TestScanBatchRetryable(t *testing.T) {
	v := batchresiduefence.NewStaticViewer()
	putFailed(v, "b-sc", "i-sc", "u-1", "USDT", "1")
	e := testEng(t, v)
	mustIssue(t, e, "b-sc", "i-sc", "ops-a", "u-1", "USDT", "1")
	rows, err := e.ScanBatch(context.Background(), "b-sc", []batchresiduefence.BatchItemView{
		{ItemIdem: "i-sc", Phase: batchresiduefence.PhaseFailed},
		{ItemIdem: "i-sub", Phase: batchresiduefence.PhaseSubmitted},
	})
	if err != nil {
		t.Fatal(err)
	}
	if batchresiduefence.CountRetryable(rows) != 1 {
		t.Fatalf("%#v", rows)
	}
}

func TestProofHelpers(t *testing.T) {
	p := batchresiduefence.NewDupProof("id", "b", "i", "w", "l", "a", "fp", time.Date(2021, 12, 18, 0, 0, 0, 0, time.UTC))
	if err := batchresiduefence.ProofValid(p); err != nil {
		t.Fatal(err)
	}
	if batchresiduefence.ProofValid(batchresiduefence.BatchDupFreezeProof{}) == nil {
		t.Fatal("empty")
	}
	if batchresiduefence.UnexportedCount([]batchresiduefence.BatchDupFreezeProof{p}) != 1 {
		t.Fatal("unexp")
	}
	if len(batchresiduefence.ProofsForItem([]batchresiduefence.BatchDupFreezeProof{p}, "i")) != 1 {
		t.Fatal("item")
	}
}

func TestManualClock(t *testing.T) {
	c := batchresiduefence.NewManualClock(time.Date(2021,
		12, 1, 0, 0, 0, 0, time.UTC))
	t0 := c.Now()
	c.Advance(time.Hour)
	if !c.Now().After(t0) {
		t.Fatal("advance")
	}
	c.Set(t0)
	if !batchresiduefence.ExpiredAt(t0.Add(time.Second), t0) {
		t.Fatal("expired")
	}
}

func TestAuditFilter(t *testing.T) {
	v := batchresiduefence.NewStaticViewer()
	putFailed(v, "b-au", "i-au", "u-1", "USDT", "1")
	e := testEng(t, v)
	mustIssue(t, e, "b-au", "i-au", "ops-a", "u-1", "USDT", "1")
	rows, err := e.ListAudits(context.Background(), "b-au", 20)
	if err != nil || len(rows) == 0 {
		t.Fatalf("%v %d", err, len(rows))
	}
	if batchresiduefence.CountEvent(rows, batchresiduefence.AuditIssueFence) < 1 {
		t.Fatal("issue audit")
	}
	if batchresiduefence.HasDupAudit(rows) {
		t.Fatal("no dup")
	}
	_, ok := batchresiduefence.LastAudit(rows)
	if !ok {
		t.Fatal("last")
	}
	_ = batchresiduefence.FilterAudits(rows, batchresiduefence.AuditIssueFence,
	)
}

func TestSlotHelpers(t *testing.T) {
	s := batchresiduefence.FreezeSlot{FenceID: "f1", Status: batchresiduefence.FreezeSlotActive}
	if !batchresiduefence.SameSlotFence(s, "f1") || !batchresiduefence.SlotConflicts(s, "f2") {
		t.Fatal("slot")
	}
}
