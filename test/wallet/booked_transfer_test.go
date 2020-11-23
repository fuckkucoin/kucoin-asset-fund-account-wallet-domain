package wallet_test

import (
	"testing"

	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/wallet"
	"github.com/shopspring/decimal"
)

const erc20TransferTopic0 = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"

func TestMatchBookedTransferUsesRegistration(t *testing.T) {
	contract := "0x1111111111111111111111111111111111111112"
	to := "0x2222222222222222222222222222222222222222"
	logs := []wallet.ReceiptLog{{
		Address: contract,
		Topics: []string{
			erc20TransferTopic0,
			"0x0000000000000000000000003333333333333333333333333333333333333333",
			"0x0000000000000000000000002222222222222222222222222222222222222222",
		},
		Data: "0x00000000000000000000000000000000000000000000000000000000000f4240",
	}}
	tokens := map[string]config.TokenSpec{
		"BONK": {Contract: contract, Decimals: 6},
	}
	asset, amt, recv, ok := wallet.MatchBookedTransfer(logs, tokens)
	if !ok {
		t.Fatal("expected match")
	}
	if asset != "BONK" || recv != to {
		t.Fatalf("asset %s recv %s", asset, recv)
	}
	if !amt.Equal(decimal.RequireFromString("1")) {
		t.Fatalf("amount %s", amt)
	}
}
