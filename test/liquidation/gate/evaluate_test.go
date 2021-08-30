package gate_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/freeze"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/margin"
	"account-assets.kucoin.net/funding/internal/liquidation/gate"
)

func fixture(rateBPS int) (margin.Params, *freeze.Adapter, *gate.LedgerLockAdapter, *ledger.MemStore) {
	st := ledger.NewMemStore()
	params := margin.Params{MaintenanceRateBPS: rateBPS}
	fr := freeze.NewAdapter(account.New(nil))
	locks := gate.NewLedgerLockAdapter(st)
	return params, fr, locks, st
}

func seedLock(t *testing.T, st *ledger.MemStore, user, asset, avail string) {
	t.Helper()
	st.Seed(user,
		ledger.P2PDebit.SourceBook(), asset, avail)
}

func TestDT01AboveMaintenanceOKNoFreeze(t *testing.T) {
	params,
		fr, locks, st := fixture(500)
	seedLock(t, st, "u1", "USDT", "10000")
	g := gate.New(gate.Config{Params: params, LockAsset: "USDT"}, fr, locks)
	acc := margin.Account{ID: "u1", Equity: "2000", LockAsset: "USDT", Positions: PositionAcc("BTC", "1")}
	d, err := g.Evaluate(acc, map[string]string{"BTC": "10000"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Verdict != margin.VerdictOK {
		t.Fatalf("verdict=%s", d.Verdict)
	}
	if fr.FreezeCalls("u1") != 0 || fr.Frozen("u1") {
		t.Fatal("must not freeze above maintenance")
	}
	if locks.PostingCount("u1") != 0 {
		t.Fatal("must not lock above maintenance")
	}
}

func TestDT02BoundaryEqualityOK(t *testing.T) {
	params, fr, locks, st := fixture(500)
	seedLock(t, st, "u2", "USDT", "10000")
	g := gate.New(gate.Config{Params: params, LockAsset: "USDT"}, fr, locks)
	// required = 1 * 10000 * 500 / 10000 = 500
	acc := margin.Account{ID: "u2", Equity: "500", LockAsset: "USDT", Positions: PositionAcc("BTC", "1")}
	d, err := g.Evaluate(acc, map[string]string{"BTC": "10000"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Verdict != margin.VerdictOK {
		t.Fatalf("equality policy must be OK, got %s required=%s", d.Verdict, d.Required)
	}
	if fr.Frozen("u2") {
		t.Fatal("equality must not freeze")
	}
}

func TestDT03BelowBreachFreezeOnce(t *testing.T) {
	params, fr, locks, st := fixture(500)
	seedLock(t, st, "u3", "USDT", "10000")
	g := gate.New(gate.Config{Params: params, LockAsset: "USDT"}, fr, locks)
	acc := margin.Account{ID: "u3", Equity: "499", LockAsset: "USDT", Positions: PositionAcc("BTC", "1")}
	d, err := g.Evaluate(acc, map[string]string{"BTC": "10000"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Verdict != margin.VerdictBreach {
		t.Fatalf("verdict=%s", d.Verdict)
	}
	if fr.FreezeCalls("u3") != 1 || !fr.Frozen("u3") {
		t.Fatalf("freeze once, calls=%d frozen=%v", fr.FreezeCalls("u3"), fr.Frozen("u3"))
	}
	if fr.LastReason("u3") != freeze.ReasonMaintenanceBreach {
		t.Fatalf("reason=%s", fr.LastReason("u3"))
	}
	if locks.PostingCount("u3") != 1 {
		t.Fatalf("lock postings=%d", locks.PostingCount("u3"))
	}
}

func TestDT04ReEvaluateIdempotentNoDoubleLock(t *testing.T) {
	params, fr, locks, st := fixture(500)
	seedLock(t, st, "u4", "USDT", "10000")
	g := gate.New(gate.Config{Params: params, LockAsset: "USDT"}, fr, locks)
	acc := margin.Account{ID: "u4", Equity: "100", LockAsset: "USDT", Positions: PositionAcc("BTC", "1")}
	px := map[string]string{"BTC": "10000"}
	if _, err := g.Evaluate(acc, px); err != nil {
		t.Fatal(err)
	}
	d2, err := g.Evaluate(acc, px)
	if err != nil {
		t.Fatal(err)
	}
	if !d2.Already || fr.FreezeCalls("u4") != 1 {
		t.Fatalf("already=%v calls=%d", d2.Already, fr.FreezeCalls("u4"))
	}
	if locks.PostingCount("u4") != 1 {
		t.Fatalf("double lock postings=%d", locks.PostingCount("u4"))
	}
}

func TestDT05GatedAddPositionReject(t *testing.T) {
	params, fr, locks, st := fixture(500)
	seedLock(t, st, "u5", "USDT", "10000")
	g := gate.New(gate.Config{Params: params, LockAsset: "USDT"}, fr, locks)
	acc := margin.Account{ID: "u5", Equity: "1", LockAsset: "USDT", Positions: PositionAcc("BTC", "1")}
	if _, err := g.Evaluate(acc, map[string]string{"BTC": "10000"}); err != nil {
		t.Fatal(err)
	}
	if err := g.Allow("u5", gate.OpAddPosition); err == nil {
		t.Fatal("add_position must reject")
	}
}

func TestDT06GatedWithdrawReject(t *testing.T) {
	params, fr, locks, st := fixture(500)
	seedLock(t, st, "u6", "USDT", "10000")
	g := gate.New(gate.Config{Params: params, LockAsset: "USDT"}, fr, locks)
	acc := margin.Account{ID: "u6", Equity: "1", LockAsset: "USDT", Positions: PositionAcc("BTC", "1")}
	if _, err := g.Evaluate(acc, map[string]string{"BTC": "10000"}); err != nil {
		t.Fatal(err)
	}
	if err := g.Allow("u6", gate.OpExternalWithdraw); err == nil {
		t.Fatal("external_withdraw must reject")
	}
	if err := g.Allow("u6", gate.OpReducePosition); err != nil {
		t.Fatalf("reduce should pass: %v", err)
	}
}

func TestDT07MultiAssetCombined(t *testing.T) {
	params, fr, locks, st := fixture(1000)
	seedLock(t, st, "u7", "USDT", "10000")
	g := gate.New(gate.Config{Params: params, LockAsset: "USDT"}, fr, locks)
	acc := margin.Account{
		ID:	"u7", Equity: "19", LockAsset: "USDT",
		Positions:	[]margin.Position{{Asset: "BTC", Qty: "1"}, {Asset: "ETH", Qty: "10"}},
	}
	d, err := g.Evaluate(acc, map[string]string{"BTC": "100", "ETH": "10"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Required != "20" {
		t.Fatalf("required=%s want=20", d.Required)
	}
	if d.Verdict != margin.VerdictBreach {
		t.Fatalf("verdict=%s", d.Verdict)
	}
}

func TestDT08LedgerSubjectLockDirection(t *testing.T) {
	params, fr, locks, st := fixture(500)
	seedLock(t, st, "u8", "USDT", "10000")
	g := gate.New(gate.Config{Params: params, LockAsset: "USDT"}, fr, locks)
	acc := margin.Account{ID: "u8", Equity: "1", LockAsset: "USDT", Positions: PositionAcc("BTC", "1")}
	if _, err := g.Evaluate(acc, map[string]string{"BTC": "10000"}); err != nil {
		t.Fatal(err)
	}
	lines := locks.LastLines("u8")
	if len(lines) != 2 {
		t.Fatalf("lines=%d", len(lines))
	}
	if lines[0].Memo != "freeze_available" || !lines[0].Debit.IsPositive() || !lines[0].Credit.IsZero() {
		t.Fatalf("line0 %#v", lines[0])
	}
	if lines[1].Memo != "freeze_frozen" || !lines[1].Credit.IsPositive() || !lines[1].Debit.IsZero() {
		t.Fatalf("line1 %#v", lines[1])
	}
	if lines[0].AccountType != ledger.P2PDebit.SourceBook() {
		t.Fatalf("book=%s", lines[0].AccountType)
	}
	assertLockGolden(t, lines)
}

func TestDT09ReleaseUnmetStayFrozen(t *testing.T) {
	params,
		fr, locks, st := fixture(500)
	seedLock(t, st, "u9", "USDT", "10000")
	g := gate.New(gate.Config{Params: params, LockAsset: "USDT"}, fr, locks)
	acc := margin.Account{ID: "u9", Equity: "1", LockAsset: "USDT", Positions: PositionAcc("BTC", "1")}
	px := map[string]string{"BTC": "10000"}
	if _, err := g.Evaluate(acc, px); err != nil {
		t.Fatal(err)
	}
	if err := g.TryRelease(acc, px); err == nil {
		t.Fatal("must stay frozen")
	}
	if !fr.Frozen("u9") || !g.Blocked("u9") {
		t.Fatal("must remain frozen")
	}
	acc.Equity = "10000"
	if err := g.TryRelease(acc, px); err == nil {
		t.Fatal("must not mis-unfreeze even if equity recovers")
	}
	if !fr.Frozen("u9") {
		t.Fatal("unfreeze product must not run")
	}
}

func TestDT10PriceInjectCrossesThreshold(t *testing.T) {
	params, fr, locks, st := fixture(500)
	seedLock(t, st, "u10", "USDT", "10000")
	g := gate.New(gate.Config{Params: params, LockAsset: "USDT"}, fr, locks)
	acc := margin.Account{ID: "u10", Equity: "500", LockAsset: "USDT", Positions: PositionAcc("BTC", "1")}
	d1, err := g.Evaluate(acc, map[string]string{"BTC": "10000"})
	if err != nil {
		t.Fatal(err)
	}
	if d1.Verdict != margin.VerdictOK {
		t.Fatalf("start=%s", d1.Verdict)
	}
	d2, err := g.Evaluate(acc, map[string]string{"BTC": "20000"})
	if err != nil {
		t.Fatal(err)
	}
	if d2.Verdict != margin.VerdictBreach {
		t.Fatalf("after price=%s required=%s", d2.Verdict, d2.Required)
	}
	if !fr.Frozen("u10") {
		t.Fatal("must freeze after crossing")
	}
}

func TestEvaluateMissingPriceFailClosed(t *testing.T) {
	params, fr, locks, st := fixture(500)
	seedLock(t, st, "u12", "USDT", "10000")
	g := gate.New(gate.Config{Params: params, LockAsset: "USDT"}, fr, locks)
	acc := margin.Account{ID: "u12", Equity: "10000", LockAsset: "USDT", Positions: PositionAcc("BTC", "1")}
	if _, err := g.Evaluate(acc, map[string]string{}); err == nil {
		t.Fatal("missing price must fail closed")
	}
	if fr.Frozen("u12") || locks.PostingCount("u12") != 0 {
		t.Fatal("must not freeze or lock when price missing")
	}
}

func TestThresholdFromAccountFieldNotLiteral(t *testing.T) {
	_, fr, locks, st := fixture(0)
	seedLock(t, st, "u11", "USDT", "10000")
	g := gate.New(gate.Config{Params: margin.Params{}, LockAsset: "USDT"}, fr, locks)
	acc := margin.Account{
		ID:	"u11", Equity: "4", LockAsset: "USDT",
		MaintenanceRateBPS:	1000,
		Positions:		PositionAcc("AAA", "1"),
	}
	d, err := g.Evaluate(acc, map[string]string{"AAA": "10"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Required != "1" || d.Verdict != margin.VerdictOK {
		t.Fatalf("got required=%s verdict=%s", d.Required, d.Verdict)
	}
}

func PositionAcc(asset,
	qty string) []margin.Position {
	return []margin.Position{{Asset: asset,
		Qty:	qty}}
}

func assertLockGolden(t *testing.T, lines []ledger.JournalEntry) {
	t.Helper()
	path := filepath.Join("..", "..", "..", "testdata", "golden", "liqgate", "lock_posting_direction.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var want []struct {
		Memo		string	`json:"memo"`
		DebitPositive	bool	`json:"debit_positive"`
		CreditZero	bool	`json:"credit_zero"`
		DebitZero	bool	`json:"debit_zero"`
		CreditPos	bool	`json:"credit_positive"`
		AccountType	string	`json:"account_type"`
	}
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if len(want) != len(lines) {
		t.Fatalf("golden lines %d got %d", len(want), len(lines))
	}
	for i, w := range want {
		if lines[i].Memo != w.Memo || string(lines[i].AccountType) != w.AccountType {
			t.Fatalf("line %d memo/book", i)
		}
		if w.DebitPositive && !lines[i].Debit.IsPositive() {
			t.Fatalf("line %d debit", i)
		}
		if w.CreditZero && !lines[i].Credit.IsZero() {
			t.Fatalf("line %d credit", i)
		}
		if w.DebitZero && !lines[i].Debit.IsZero() {
			t.Fatalf("line %d debit zero", i)
		}
		if w.CreditPos && !lines[i].Credit.IsPositive() {
			t.Fatalf("line %d credit pos", i)
		}
	}
}
