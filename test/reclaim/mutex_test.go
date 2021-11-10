package reclaim_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/reclaim"
)

func TestOutputKeyString(t *testing.T) {
	if (reclaim.OutputKey{}).String() != "" {
		t.Fatal("empty")
	}
	k := reclaim.OutputKey{Chain: "eth", TxHash: "0x1", OutputIndex: 12}
	if k.String() != "eth|0x1|12" {
		t.Fatalf("got %s", k.String())
	}
}

func TestLateCreditMutexDrain(t *testing.T) {
	m := reclaim.NewLateCreditMutex(nil)
	m.Record(reclaim.RaceEvent{EventID: "1", Kind: "occupy_vs_credit"})
	m.Record(reclaim.RaceEvent{EventID: "2", Acked: true})
	d := m.Drain()
	if len(d) != 1 || d[0].EventID != "1" {
		t.Fatalf("%+v", d)
	}
}

func TestCheckFenceLegacy(t *testing.T) {
	if !reclaim.CheckFence(0, "") {
		t.Fatal("legacy omit")
	}
	if reclaim.CheckFence(0, "t") {
		t.Fatal("epoch0 with token should fail in-process")
	}
	if !reclaim.CheckFence(1, "t") {
		t.Fatal("valid")
	}
}

func TestCaseOccupancyHelpers(t *testing.T) {
	c := reclaim.CaseView{State: reclaim.ReclaimCaseReserved}
	if !c.IsActive() || c.IsTerminal() {
		t.Fatal("reserved")
	}
	c.State = reclaim.ReclaimCaseClosed
	if !c.IsTerminal() {
		t.Fatal("closed")
	}
	o := reclaim.OccupancyView{State: reclaim.OccupancyActive, ExpiresAt: reclaim.NowUTC().Add(60e9)}
	if !o.IsOccupying(reclaim.NowUTC()) {
		t.Fatal("occupying")
	}
}
