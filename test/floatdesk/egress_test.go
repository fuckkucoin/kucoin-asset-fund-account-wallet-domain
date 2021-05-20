package floatdesk_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/floatdesk"
)

func TestGradeAvailTable(t *testing.T) {
	rows := []struct {
		avail, min, tgt	string
		want		floatdesk.TankLevel
	}{
		{"10", "2", "8", floatdesk.TankOK},
		{"5", "2", "8", floatdesk.TankWarn},
		{"1", "2", "8", floatdesk.TankHalt},
		{"80000", "20000", "80000", floatdesk.TankOK},
		{"20000", "20000", "80000", floatdesk.TankWarn},
		{"19999", "20000", "80000", floatdesk.TankHalt},
	}
	for _, r := range rows {
		got, err := floatdesk.GradeAvail(r.avail, r.min, r.tgt)
		if err != nil || got != r.want {
			t.Fatalf("%s/%s/%s -> %s (%v) want %s", r.avail, r.min, r.tgt, got, err, r.want)
		}
	}
}

func TestCheckInstantAllowWarnHalt(t *testing.T) {
	s := floatdesk.New()
	ok, err := s.CheckEgress("ETH", "USDT", "90000")
	if err != nil || ok.Decision != floatdesk.InstantAllow {
		t.Fatalf("allow %+v %v", ok, err)
	}
	wn, err := s.CheckEgress("ETH", "USDT", "30000")
	if err != nil || wn.Decision != floatdesk.InstantWarn {
		t.Fatalf("warn %+v %v", wn, err)
	}
	ht, err := s.CheckEgress("ETH", "USDT", "1000")
	if err != nil || ht.Decision != floatdesk.InstantHalt {
		t.Fatalf("level halt %+v %v", ht, err)
	}
	if err := s.Halt().HaltAsset("USDT"); err != nil {
		t.Fatal(err)
	}
	fl, err := s.CheckEgress("ETH", "USDT", "90000")
	if err != nil || fl.Decision != floatdesk.InstantHalt || fl.Reason != "halt_flag" {
		t.Fatalf("flag %+v %v", fl, err)
	}
	if s.Halt().Generation() == 0 || s.Halt().Fingerprint() == "" {
		t.Fatal("generation/fingerprint")
	}
}

func TestCheckEgressFromTanks(t *testing.T) {
	s := floatdesk.New()
	_ = s.Tanks().Put(floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "10", Min: "2", Target: "8"})
	ev, err := s.CheckEgressFromTanks("ETH", "ETH")
	if err != nil || ev.Decision != floatdesk.InstantAllow {
		t.Fatalf("%+v %v", ev, err)
	}
	_ = s.Tanks().Put(floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "1", Min: "2", Target: "8"})
	ev, err = s.CheckEgressFromTanks("ETH", "ETH")
	if err != nil || ev.Decision != floatdesk.InstantHalt {
		t.Fatalf("tank halt %+v %v", ev, err)
	}
}

func TestRefillObserveReadOnly(t *testing.T) {
	s := floatdesk.New()
	if s.RefillObserve().IsActive("ETH", "ETH") {
		t.Fatal("empty")
	}
	s.RefillObserve().SetActive("ETH", "ETH", true)
	if !s.RefillObserve().IsActive("ETH", "ETH") || !s.RefillObserve().AnyActive() {
		t.Fatal("active")
	}
	s.RefillObserve().SetActive("ETH", "ETH", false)
	if s.RefillObserve().IsActive("ETH", "ETH") {
		t.Fatal("cleared")
	}
}

func TestTankDigestStable(t *testing.T) {
	a := floatdesk.TankDigest(floatdesk.Tank{Chain: "eth", Native: "eth", Avail: "1", Min: "2", Target: "8"})
	b := floatdesk.TankDigest(floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "1", Min: "2", Target: "8"})
	if a != b || a == "" {
		t.Fatal("digest")
	}
	d := floatdesk.TanksDigest([]floatdesk.Tank{{Chain: "B", Native: "B"}, {Chain: "A", Native: "A"}})
	e := floatdesk.TanksDigest([]floatdesk.Tank{{Chain: "A", Native: "A"}, {Chain: "B", Native: "B"}})
	if d != e {
		t.Fatal("sort digest")
	}
}

func TestHaltFingerprintChanges(t *testing.T) {
	f := floatdesk.NewFlags()
	a := f.Fingerprint()
	_ = f.HaltChain("ETH")
	if f.Fingerprint() == a {
		t.Fatal("fp should change")
	}
	if f.Generation() != 1 {
		t.Fatalf("gen %d", f.Generation())
	}
}
