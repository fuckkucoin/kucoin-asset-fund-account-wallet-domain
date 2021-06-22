package flow

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/httpapi"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/wallet"
	"github.com/shopspring/decimal"
)

func TestFundOrdersListAndDetail(t *testing.T) {
	rt, err := app.OpenMem()
	if err != nil {
		t.Fatal(err)
	}
	h := httpapi.Router(rt)
	tok := rt.Cfg.Auth.BearerTokens[0]
	uid := rt.Cfg.SeedUserID
	call := func(method, path string, body any) *httptest.ResponseRecorder {
		var req *http.Request
		if body != nil {
			raw, _ := json.Marshal(body)
			req = httptest.NewRequest(method, path, bytes.NewReader(raw))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("X-User-Id", uid)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	ext := wallet.DeriveAddress(ledger.ExtWithdrawUser, "ETH")
	wd := call(http.MethodPost, "/api/v3/withdrawals", map[string]any{
		"currency": "USDT", "amount": "0.11", "chain": "ETH", "toAddress": ext,
		"clientOid": "fund-ord-wd-1", "withdrawType": "ADDRESS", "tradePassword": "123456",
	})
	if wd.Code != 200 {
		t.Fatalf("withdraw %d %s", wd.Code, wd.Body.String())
	}
	list := call(http.MethodGet, "/api/v1/fund/orders?kind=withdraw&pageSize=5", nil)
	if list.Code != 200 {
		t.Fatalf("list %d %s", list.Code, list.Body.String())
	}
	var env struct {
		Data struct {
			Items []struct {
				Kind    string `json:"kind"`
				OrderID string `json:"orderId"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Data.Items) == 0 {
		t.Fatal("expected withdraw fund order")
	}
	id := env.Data.Items[0].OrderID
	detail := call(http.MethodGet, "/api/v1/fund/orders/"+id, nil)
	if detail.Code != 200 {
		t.Fatalf("detail %d %s", detail.Code, detail.Body.String())
	}
	to := rt.Wallet.AddressFor(httptest.NewRequest(http.MethodGet, "/", nil).Context(), uid, "ETH")
	hash, err := rt.Wallet.InjectInbound("ETH", to, "USDT", decimal.RequireFromString("1"))
	if err != nil {
		t.Fatal(err)
	}
	if w := call(http.MethodPost, "/v1/deposits/observe", map[string]any{
		"user_id": uid, "chain": "ETH", "tx_hash": hash, "asset": "USDT",
		"idempotency_key": "fund-ord-dep-1", "wait": true,
	}); w.Code != 200 {
		t.Fatalf("observe %d %s", w.Code, w.Body.String())
	}
	all := call(http.MethodGet, "/api/v1/fund/orders?kind=all&pageSize=20", nil)
	if all.Code != 200 {
		t.Fatalf("all %d %s", all.Code, all.Body.String())
	}
}
