package kcfund_test

import (
	"net/http"
	"testing"
	"account-assets.kucoin.net/funding/internal/kcfund"
)

func TestBffProductPath(t *testing.T) {
	cases := []struct {
		method	string
		path	string
		want	bool
	}{
		{http.MethodGet, "/api/v1/user-info", true},
		{http.MethodPost, "/api/v1/withdrawals", false},
		{http.MethodGet, "/api/v1/accounts", false},
		{http.MethodGet, "/api/v1/margin/account", true},
		{http.MethodPost, "/api/v1/margin/borrow", false},
		{http.MethodGet, "/api/v1/account-preference", true},
		{http.MethodPut, "/api/v1/account-preference", true},
		{http.MethodPost, "/api/v1/accounts/open-all", true},
		{http.MethodPost, "/api/v1/accounts/inner-transfer", false},
		{http.MethodGet, "/api/v1/earn/products", true},
		{http.MethodGet, "/api/v1/p2p/summary", true},
		{http.MethodPost, "/api/v1/earn/orders", true},
		{http.MethodGet, "/api/v1/base-fee", false},
		{http.MethodGet, "/api/v1/trade-fees", false},
		{http.MethodGet, "/api/v1/withdrawals/fee", false},
		{http.MethodGet, "/api/ua/v1/account/mode", false},
		{http.MethodPost, "/api/ua/v1/account/mode", false},
		{http.MethodGet, "/api/ua/v1/unified/account/leverage", false},
		{http.MethodPost, "/api/ua/v1/unified/account/leverage", false},
		{http.MethodGet, "/api/ua/v1/margin/borrow/info", false},
		{http.MethodGet, "/api/ua/v1/account/interest-limits", false},
	}
	for _, tc := range cases {
		got := kcfund.BffProductPath(tc.method, tc.path)
		if got != tc.want {
			t.Errorf("%s %s: got %v want %v", tc.method, tc.path, got, tc.want)
		}
	}
}
