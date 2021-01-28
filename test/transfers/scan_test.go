package transfers_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/transfers"
)

func TestLoadIncompleteUsesCursor(t *testing.T) {
	st := transfers.NewMem(transfers.Rec{ID: "1", Kind: transfers.KindDeposit, State: transfers.StatePending}, transfers.Rec{ID: "2", Kind: transfers.KindDeposit, State: transfers.StatePending}, transfers.Rec{ID: "3", Kind: transfers.KindDeposit, State: transfers.StateCompleted},
	)
	cur := &transfers.Cursor{}
	rows, err := transfers.LoadIncomplete(st, cur, 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	cur.Advance("1")
	rows, err = transfers.LoadIncomplete(st, cur, 10)
	if err != nil || len(rows) != 1 || rows[0].ID != "2" {
		t.Fatalf("cursor rows=%v err=%v", rows, err)
	}
}
