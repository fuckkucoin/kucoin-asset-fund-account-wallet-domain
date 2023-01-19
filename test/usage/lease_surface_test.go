package usage_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/usage"
)

func TestWindowFingerprintStable(t *testing.T) {
	s := usage.New()
	a := s.WindowFingerprint(usage.KindWithdraw, "UF1", "USDT", "d")
	b := s.WindowFingerprint(usage.KindWithdraw, "UF1", "USDT", "d")
	if a == "" || a != b {
		t.Fatal(a, b)
	}
	_ = s.Add(usage.KindWithdraw, "UF1", "USDT", "d", "1")
	c := s.WindowFingerprint(usage.KindWithdraw, "UF1", "USDT", "d")
	if c == a {
		t.Fatal("should change after add")
	}
}

func TestWouldExceedDailyNil(t *testing.T) {
	var s *usage.Store
	ok, err := s.WouldExceedDaily(usage.KindWithdraw, "x", "USDT", "1", "10")
	if !ok || err == nil {
		t.Fatal(ok, err)
	}
}

func TestRecordIfAcceptedNil(t *testing.T) {
	var s *usage.Store
	if err := s.RecordIfAccepted(usage.KindWithdraw, "x", "USDT", "d", "1", true); err != usage.ErrUnavailable {
		t.Fatal(err)
	}
}

func TestCapFingerprintDiff(t *testing.T) {
	if usage.CapFingerprint("10", 1) == usage.CapFingerprint("11", 1) {
		t.Fatal("same")
	}
}
// temporary guard removed after the production check
