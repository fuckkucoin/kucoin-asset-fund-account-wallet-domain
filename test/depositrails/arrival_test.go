package depositrails_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/depositrails"
	"account-assets.kucoin.net/funding/internal/fundingbff"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/svcdial"
	"account-assets.kucoin.net/funding/internal/svcreg"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
)

func TestPostArrivalInternalDial(t *testing.T) {
	var got depositrails.ArrivalRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/deposit-rails/arrivals" {
			http.Error(w, "path", http.StatusNotFound)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"code":"200000","data":{"orderId":"CD-1","state":"CREDITED","notified":true}}`))
	}))
	defer srv.Close()

	reg := svcreg.NewMem()
	_ = reg.Register(context.Background(), svcreg.Instance{
		Name: "funding-bff-service",
		Addr: srv.URL,
	})
	bff := fundingbff.NewClient(svcdial.New(reg, zerolog.Nop()), &config.Config{Profile: "dev"}, false)
	req := depositrails.ArrivalRequest{
		UID: "u1", Chain: "ETH", Asset: "USDT", Amount: "10",
		TxHash: "0xabc", Address: "0xdep", Confirmations: 12, Credited: true,
	}
	data, err := depositrails.PostArrival(context.Background(), bff, req)
	if err != nil {
		t.Fatal(err)
	}
	if data.OrderID != "CD-1" || got.TxHash != "0xabc" {
		t.Fatalf("data=%+v got=%+v", data, got)
	}
}

func TestNotifierSkipsMemProfile(t *testing.T) {
	n := depositrails.NewNotifier(fundingbff.NewClient(nil, nil, true), zerolog.Nop())
	tr := &ledger.Transfer{
		UserID: "u1", Direction: ledger.WalletDeposit, State: ledger.StateCredited,
		Asset: "USDT", Chain: "ETH", TxHash: "h", Amount: decimal.NewFromInt(1),
	}
	if err := n.NotifyArrival(context.Background(), tr, 1); err != nil {
		t.Fatal(err)
	}
}
