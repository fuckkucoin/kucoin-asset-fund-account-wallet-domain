package custodyhalt_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
)

func TestViewEpochScope(t *testing.T) {
	v := custodyhalt.ViewEpochScope(custodyhalt.ScopeGlobal, "", "")
	if v.Key != custodyhalt.KeyGlobal {
		t.Fatalf("key %s", v.Key)
	}
	v = custodyhalt.ViewEpochScope(custodyhalt.ScopeChain, "eth", "")
	if v.Key != "chain:ETH" {
		t.Fatalf("chain key %s", v.Key)
	}
	if custodyhalt.LaneEpochHint(custodyhalt.LaneWithdraw) != "withdraw" {
		t.Fatal(custodyhalt.LaneEpochHint(custodyhalt.LaneWithdraw))
	}
	if custodyhalt.LaneEpochHint(custodyhalt.LaneDeposit) != "deposit" {
		t.Fatal()
	}
}
