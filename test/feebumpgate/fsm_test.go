package feebumpgate_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/feebumpgate"
)

func TestAllowGenTransitionTable(t *testing.T) {
	for _, e := range feebumpgate.TransitionTable() {
		if e[0] == "held" || e[0] == "retired" || e[0] == "orphaned" {
			if !feebumpgate.AllowOccTransition(feebumpgate.OccState(e[0]), feebumpgate.OccState(e[1])) {
				t.Fatalf("occ %s->%s", e[0], e[1])
			}
			continue
		}
		if !feebumpgate.AllowGenTransition(feebumpgate.GenState(e[0]), feebumpgate.GenState(e[1])) {
			t.Fatalf("gen %s->%s", e[0], e[1])
		}
	}
}

func TestIllegalGenEdges(t *testing.T) {
	for _, e := range feebumpgate.IllegalGenEdges() {
		if feebumpgate.AllowGenTransition(e[0], e[1]) {
			t.Fatalf("illegal allowed %s->%s", e[0], e[1])
		}
	}
}

func TestIllegalOccEdges(t *testing.T) {
	for _, e := range feebumpgate.IllegalOccEdges() {
		if feebumpgate.AllowOccTransition(e[0], e[1]) {
			t.Fatalf("illegal occ %s->%s", e[0], e[1])
		}
	}
}

func TestNextHelpers(t *testing.T) {
	if _, err := feebumpgate.NextOnBroadcast(feebumpgate.GenSigned); err != nil {
		t.Fatal(err)
	}
	if _, err := feebumpgate.NextOnBroadcast(feebumpgate.GenSuperseded); err == nil {
		t.Fatal("expected error")
	}
	if s, err := feebumpgate.NextOnSupersede(feebumpgate.GenSigned); err != nil || s != feebumpgate.GenSuperseded {
		t.Fatalf("%s %v", s, err)
	}
	if _, err := feebumpgate.NextOnRetire(feebumpgate.OccHeld); err != nil {
		t.Fatal(err)
	}
	if _, err := feebumpgate.NextOnOrphan(feebumpgate.OccRetired); err == nil {
		t.Fatal("expected")
	}
}

func TestStatePredicates(t *testing.T) {
	if !feebumpgate.GenFinal.Terminal() || feebumpgate.GenSigned.Terminal() {
		t.Fatal("gen terminal")
	}
	if !feebumpgate.OccRetired.Terminal() || feebumpgate.OccHeld.Terminal() {
		t.Fatal("occ terminal")
	}
	if !feebumpgate.GenSigned.CanBroadcast() || !feebumpgate.GenBroadcast.CanSupersede() {
		t.Fatal("can*")
	}
	if !feebumpgate.OrphanOpen.Open() || feebumpgate.OrphanClosed.Open() {
		t.Fatal("orphan")
	}
}
