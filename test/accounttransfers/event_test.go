package accounttransfers_test

import (
	"testing"

	"account-assets.kucoin.net/funding/internal/accounttransfers"
)

func TestNormalizeEvent(t *testing.T) {
	ev := accounttransfers.NormalizeTransferEvent(accounttransfers.TransferEvent{
		OrderID: "o1", UID: "u1", Currency: "usdt", Amount: "1",
		FromAccountType: "main", ToAccountType: "trade",
		TransferType: "internal", ClientOid: "c1",
	})
	if ev.Currency != "USDT" || ev.FromAccountType != "MAIN" || ev.EventID == "" || ev.OccurredAt == "" {
		t.Fatalf("normalize: %+v", ev)
	}
}
