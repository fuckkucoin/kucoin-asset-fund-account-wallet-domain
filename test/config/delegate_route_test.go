package config_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/config"
)

func TestMatchDelegateRoute(t *testing.T) {
	cases := []struct {
		method	string
		path	string
		key	string
	}{
		{"GET", "/api/v1/accounts", "query_gateway"},
		{"POST", "/v1/fundcircuit/trip", "risk_portal"},
		{"GET", "/v1/fiatjnl/entries", "ledger"},
		{"POST", "/v1/notify/enqueue", "notify"},
		{"GET", "/v1/accounts/UA1/balances", "account_book"},
		{"POST", "/v1/chain/tokens", "deposit_withdraw"},
		{"GET", "/v1/assets/campaign-holds", "query_gateway"},
	}
	for _, tc := range cases {
		got, ok := config.MatchDelegateRoute(tc.method, tc.path)
		if !ok || got.HostKey != tc.key {
			t.Fatalf("%s %s: ok=%v key=%q", tc.method, tc.path, ok, got.HostKey)
		}
		if config.DelegateHostName(got.HostKey) == "" {
			t.Fatalf("missing host for key %q", got.HostKey)
		}
	}
}
