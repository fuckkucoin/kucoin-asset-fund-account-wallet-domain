package config_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/config"
)

func TestServiceDialURLAddsListenPort(t *testing.T) {
	got := config.ServiceDialURL("https://ledger-transfer-freeze-reconciliation.kucoin.net", ":18480")
	want := "http://ledger-transfer-freeze-reconciliation.kucoin.net:18480"
	if got != want {
		t.Fatalf("got %s", got)
	}
	got = config.ServiceDialURL("http://ledger.kucoin.net:9443", ":18480")
	if got != "http://ledger.kucoin.net:9443" {
		t.Fatalf("explicit port %s", got)
	}
}
