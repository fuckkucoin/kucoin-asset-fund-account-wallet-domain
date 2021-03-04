package balproofgate_test

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/balproofgate"
)

func TestBuildSnapshotAndHMAC(t *testing.T) {
	view := balproofgate.LiveView{Available: "10", Frozen: "2", ObserveGen: 5, At: time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)}
	snap, err := balproofgate.BuildSnapshot(view)
	if err != nil || snap.Total != "12" || snap.RootDigest == "" {
		t.Fatalf("%v %+v", err, snap)
	}
	can := balproofgate.LeaseCanonical("u", "main", "usdt", "n", snap, view.At, 1)
	sig := balproofgate.SignHMAC("s", can)
	if err := balproofgate.VerifyHMAC("s", can, sig); err != nil {
		t.Fatal(err)
	}
	if err := balproofgate.VerifyHMAC("s", can, "dead"); err != balproofgate.ErrSig {
		t.Fatal(err)
	}
}

func TestClassifyDriftTable(t *testing.T) {
	base := balproofgate.LiveSnapshotRoot{Available: "10", Frozen: "2", ObserveGen: 3}
	cases := []struct {
		avail, frozen	string
		obs		int64
		want		balproofgate.DriftKind
	}{
		{"10", "2", 3, ""},
		{"11", "2", 4, balproofgate.DriftAvailCredit},
		{"9", "2", 4, balproofgate.DriftAvailDebit},
		{"10", "3", 4, balproofgate.DriftFrozenUp},
		{"10", "1", 4, balproofgate.DriftFrozenDown},
		{"10", "2", 8, balproofgate.DriftObserveGen},
	}
	for _, c := range cases {
		got := balproofgate.ClassifyDrift(base, balproofgate.LiveView{Available: c.avail, Frozen: c.frozen, ObserveGen: c.obs})
		if got != c.want {
			t.Fatalf("%s/%s/%d got %s want %s", c.avail, c.frozen, c.obs, got, c.want)
		}
	}
}

func TestFingerprintStable(t *testing.T) {
	a := balproofgate.FingerprintObserveGen("10", "0")
	b := balproofgate.FingerprintObserveGen("10", "0")
	c := balproofgate.FingerprintObserveGen("11", "0")
	if a != b || a == c || a == 0 {
		t.Fatalf("%d %d %d", a, b, c)
	}
}

func TestNormalizeIssue(t *testing.T) {
	got, err := balproofgate.NormalizeIssue(balproofgate.IssueReq{UserID: " u1 ", Book: " main ", Asset: "usdt", Nonce: " n1 ", TTLSec: 99999})
	if err != nil {
		t.Fatal(err)
	}
	if got.Asset != "USDT" || got.TTLSec != 3600 {
		t.Fatalf("%+v", got)
	}
}
