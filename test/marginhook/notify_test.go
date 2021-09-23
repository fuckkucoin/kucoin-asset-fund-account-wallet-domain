package marginhook_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/marginhook"
)

func TestNotifyDedup(t *testing.T) {
	s := marginhook.New()
	n := 0
	s.OnBalanceChange(func(marginhook.BalanceChange,) { n++ })
	s.NotifyBalance(marginhook.BalanceChange{UserID: "u", LoanID: "L1", Asset: "USDT", Kind: marginhook.ChangeInterestPost, EntryID: "e1", Amount: "1"})
	s.NotifyBalance(marginhook.BalanceChange{UserID: "u", LoanID: "L1", Asset: "USDT", Kind: marginhook.ChangeInterestPost, EntryID: "e1", Amount: "1"})
	s.NotifyBalance(marginhook.BalanceChange{UserID: "u", LoanID: "L1", Asset: "USDT", Kind: marginhook.ChangeRepayOffset, EntryID: "e2", Amount: "2"})
	if n != 2 || s.BalanceCallCount(marginhook.ChangeInterestPost) != 1 || s.BalanceCallCount("") != 2 {
		t.Fatalf("n=%d log=%d", n, s.BalanceCallCount(""))
	}
}
