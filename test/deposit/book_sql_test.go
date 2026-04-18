package deposit_test

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"account-assets.kucoin.net/funding/internal/deposit"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "deposit-book")
}

func TestHTCasesFixturePresent(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join(testdataDir(t), "ht-cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Cases []struct {
			ID string `json:"id"`
		}
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Cases) < 14 {
		t.Fatalf("want 14 cases got %d", len(doc.Cases))
	}
}

func TestHT01ProdMemBookRefused(t *testing.T) {
	t.Parallel()
	if err := deposit.RefuseMemBook(true, true, false); !errors.Is(err, deposit.ErrMemBook) {
		t.Fatalf("H-T01: %v", err)
	}
	b := deposit.NewBook().MarkProduction(true)
	if err := b.PutErr(deposit.Run{ID: "r1", Chain: "ETH", TxHash: "h", Step: deposit.StepAccepted, Amount: "1"}); !errors.Is(err, deposit.ErrMemBook) {
		t.Fatalf("prod put: %v", err)
	}
	if _, err := (&deposit.Service{}).Recover(context.Background(), b, 8); !errors.Is(err, deposit.ErrMemBook) {
		t.Fatalf("prod recover: %v", err)
	}
}

func TestHT02DevMemBookOK(t *testing.T) {
	t.Parallel()
	if err := deposit.RefuseMemBook(false, true, false); err != nil {
		t.Fatal(err)
	}
	b := deposit.NewBook()
	r := deposit.Run{ID: "dev-1", Chain: "BTC", TxHash: "hx", OutputIndex: 0, Step: deposit.StepAccepted, Amount: "2", TransferID: "tr"}
	if err := b.PutErr(r); err != nil {
		t.Fatal(err)
	}
	got, ok := b.Get("dev-1")
	if !ok || got.Amount != "2" {
		t.Fatalf("%+v", got)
	}
	if p := b.Pending(8); len(p) != 1 {
		t.Fatalf("pending %d", len(p))
	}
}

func TestHT03ProdBindSQLPutGet(t *testing.T) {
	t.Parallel()
	tab := deposit.NewBookTable()
	b := deposit.NewBook().MarkProduction(true).BindSQLTable(tab)
	if err := deposit.RefuseMemBook(true, false, b.SQLBound()); err != nil {
		t.Fatal(err)
	}
	want := deposit.Run{ID: "p1", Chain: "ETH", TxHash: "tx-p", OutputIndex: 2, UserID: "u1", Asset: "USDT", Step: deposit.StepAccepted, Amount: "9.1", TransferID: "tr-p", FromAddress: "0xfrom"}
	if err := b.PutErr(want); err != nil {
		t.Fatal(err)
	}
	got, ok := b.Get("p1")
	if !ok || got.Amount != want.Amount || got.OutputIndex != 2 || got.TransferID != "tr-p" || got.Chain != "ETH" {
		t.Fatalf("H-T03 %+v", got)
	}
	row, err := tab.Get(context.Background(), "ETH", "tx-p", 2)
	if err != nil || row.RunID != "p1" || row.Amount != "9.1" {
		t.Fatalf("sql row %+v %v", row, err)
	}
}

func TestHT04ByHashMultiVout(t *testing.T) {
	t.Parallel()
	b := deposit.NewBook().BindSQLTable(deposit.NewBookTable())
	a := deposit.Run{ID: "va", Chain: "BTC", TxHash: "tx-same", OutputIndex: 0, Step: deposit.StepAccepted, Amount: "1.5", TransferID: "tr-a"}
	c := deposit.Run{ID: "vb", Chain: "BTC", TxHash: "tx-same", OutputIndex: 1, Step: deposit.StepConfirmed, Amount: "0.25", TransferID: "tr-b"}
	if err := b.PutErr(a); err != nil {
		t.Fatal(err)
	}
	if err := b.PutErr(c); err != nil {
		t.Fatal(err)
	}
	g0, ok := b.ByHashOut("BTC", "tx-same", 0)
	if !ok || g0.ID != "va" || g0.Amount != "1.5" {
		t.Fatalf("vout0 %+v", g0)
	}
	g1, ok := b.ByHashOut("BTC", "tx-same", 1)
	if !ok || g1.ID != "vb" || g1.Amount != "0.25" {
		t.Fatalf("vout1 %+v", g1)
	}
	if g0.ID == g1.ID {
		t.Fatal("H-T04 多 vout 串单")
	}
}

func TestHT05AdvanceIllegalKeepsStep(t *testing.T) {
	t.Parallel()
	tab := deposit.NewBookTable()
	b := deposit.NewBook().BindSQLTable(tab)
	r := deposit.Run{ID: "adv", Chain: "ETH", TxHash: "hx", Step: deposit.StepCredited, Amount: "1", TransferID: "t"}
	if err := b.PutErr(r); err != nil {
		t.Fatal(err)
	}
	if err := deposit.Advance(deposit.StepCredited, deposit.StepAccepted); err == nil {
		t.Fatal("want illegal")
	}
	got, _ := b.Get("adv")
	if got.Step != deposit.StepCredited {
		t.Fatalf("step mutated %s", got.Step)
	}
	row, _ := tab.Get(context.Background(), "ETH", "hx", 0)
	if row.Step != string(deposit.StepCredited) {
		t.Fatalf("sql step %s", row.Step)
	}
}

func TestHT06PendingNonTerminal(t *testing.T) {
	t.Parallel()
	b := deposit.NewBook()
	for _, r := range []deposit.Run{
		{ID: "a", Chain: "E", TxHash: "1", Step: deposit.StepAccepted},
		{ID: "c", Chain: "E", TxHash: "2", Step: deposit.StepCredited},
		{ID: "u", Chain: "E", TxHash: "3", Step: deposit.StepUnwound},
		{ID: "f", Chain: "E", TxHash: "4", Step: deposit.StepFailed},
		{ID: "h", Chain: "E", TxHash: "5", Step: deposit.StepHeld},
		{ID: "k", Chain: "E", TxHash: "6", Step: deposit.StepConfirmed},
	} {
		b.Put(r)
	}
	p := b.Pending(20)
	seen := map[deposit.Step]int{}
	for _, r := range p {
		seen[r.Step]++
		if deposit.TerminalStep(r.Step) {
			t.Fatalf("terminal in pending %s", r.Step)
		}
	}
	if seen[deposit.StepAccepted] != 1 || seen[deposit.StepHeld] != 1 || seen[deposit.StepConfirmed] != 1 || len(p) != 3 {
		t.Fatalf("%+v", seen)
	}
}

func TestHT07RestartNonTerminal(t *testing.T) {
	t.Parallel()
	shared := map[string]sqlrepo.DepositClaim{}
	a := deposit.NewBook().BindSQLTable(deposit.OpenBookTable(shared))
	want := deposit.Run{ID: "rs", Chain: "BTC", TxHash: "rh", OutputIndex: 3, Amount: "4.4", TransferID: "tr-rs", Step: deposit.StepAccepted, UserID: "u", Asset: "BTC"}
	if err := a.PutErr(want); err != nil {
		t.Fatal(err)
	}
	b := deposit.NewBook().BindSQLTable(deposit.OpenBookTable(shared))
	if err := b.Hydrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, ok := b.Get("rs")
	if !ok || got.Step != deposit.StepAccepted || got.Amount != "4.4" || got.TransferID != "tr-rs" || got.OutputIndex != 3 {
		t.Fatalf("H-T07 %+v ok=%v", got, ok)
	}
	if len(b.Pending(8)) != 1 {
		t.Fatal("pending lost")
	}
}

func TestHT08RestartTerminalNotPending(t *testing.T) {
	t.Parallel()
	shared := map[string]sqlrepo.DepositClaim{}
	a := deposit.NewBook().BindSQLTable(deposit.OpenBookTable(shared))
	for _, st := range []deposit.Step{deposit.StepCredited, deposit.StepUnwound, deposit.StepFailed} {
		_ = a.PutErr(deposit.Run{ID: string(st), Chain: "ETH", TxHash: string(st), Step: st, Amount: "1", TransferID: "t"})
	}
	b := deposit.NewBook().BindSQLTable(deposit.OpenBookTable(shared))
	if err := b.Hydrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p := b.Pending(20); len(p) != 0 {
		t.Fatalf("H-T08 pending %+v", p)
	}
	if _, ok := b.Get(string(deposit.StepCredited)); !ok {
		t.Fatal("credited should remain gettable")
	}
}

func TestHT09HT10Claim(t *testing.T) {
	t.Parallel()
	tab := deposit.NewBookTable()
	ctx := context.Background()
	first, err := tab.Claim(ctx, "ETH", "hx", 0, "u1", "USDT", "1", "tr1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = tab.Claim(ctx, "ETH", "hx", 0, "u2", "USDT", "1", "tr2")
	if !errors.Is(err, sqlrepo.ErrTaken) {
		t.Fatalf("H-T09 %v", err)
	}
	again, err := tab.Claim(ctx, "ETH", "hx", 0, "u1", "USDT", "1", "tr1")
	if err != nil || again.UserID != first.UserID || again.TransferID != first.TransferID {
		t.Fatalf("H-T10 %+v %v", again, err)
	}
}

func TestHT11TwoVoutNotMerged(t *testing.T) {
	t.Parallel()
	tab := deposit.NewBookTable()
	ctx := context.Background()
	if _, err := tab.Claim(ctx, "BTC", "tx-same", 0, "u1", "BTC", "1.5", "tr-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := tab.Claim(ctx, "BTC", "tx-same", 1, "u1", "BTC", "0.25", "tr-b"); err != nil {
		t.Fatal(err)
	}
	rows, err := tab.ListByHash(ctx, "BTC", "tx-same")
	if err != nil || len(rows) != 2 {
		t.Fatalf("H-T11 %d %v", len(rows), err)
	}
	seen := map[int]string{}
	for _, r := range rows {
		seen[r.OutputIndex] = r.TransferID
	}
	if seen[0] != "tr-a" || seen[1] != "tr-b" {
		t.Fatalf("%+v", seen)
	}
}

func TestHT13ProdAttachUnboundUnhealthy(t *testing.T) {
	t.Parallel()
	rt := &app.Runtime{
		Cfg:	&config.Config{Profile: "prod", Auth: config.AuthConfig{Kind: "iam"}},
		Mem:	true,
	}
	svc := deposit.Attach(rt, deposit.New(nil, nil))
	h := svc.Runs().Health()
	if h.OK || h.SQLBound {
		t.Fatalf("H-T13 healthy %+v", h)
	}
	if !errors.Is(svc.Runs().AttachError(), deposit.ErrMemBook) {
		t.Fatalf("attach %v", svc.Runs().AttachError())
	}
}

func TestHT14RecoverNewInstance(t *testing.T) {
	t.Parallel()
	shared := map[string]sqlrepo.DepositClaim{}
	a := deposit.NewBook().BindSQLTable(deposit.OpenBookTable(shared))
	_ = a.PutErr(deposit.Run{ID: "rec", Chain: "ETH", TxHash: "hr", Step: deposit.StepAccepted, Amount: "2", TransferID: "tr-rec"})
	b := deposit.NewBook().BindSQLTable(deposit.OpenBookTable(shared))
	if err := b.Hydrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	called := 0
	svc := &deposit.Service{recoverHook: func(ctx context.Context, id string) error {
		called++
		if id != "tr-rec" {
			t.Fatalf("id %s", id)
		}
		r, _ := b.Get("rec")
		r.Step = deposit.StepConfirmed
		return b.PutErr(r)
	}}
	n, err := svc.Recover(context.Background(), b, 8)
	if err != nil || n != 1 || called != 1 {
		t.Fatalf("H-T14 n=%d called=%d err=%v", n, called, err)
	}
	got, _ := b.Get("rec")
	if got.Step != deposit.StepConfirmed {
		t.Fatalf("step %s", got.Step)
	}
}

func TestGoldenCSVAndConcurrency(t *testing.T) {
	t.Parallel()
	f, err := os.Open(filepath.Join(testdataDir(t), "golden-runs.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rd := csv.NewReader(f)
	rows, err := rd.ReadAll()
	if err != nil || len(rows) < 2 {
		t.Fatal(err)
	}
	shared := map[string]sqlrepo.DepositClaim{}
	b := deposit.NewBook().BindSQLTable(deposit.OpenBookTable(shared))
	var wg sync.WaitGroup
	errCh := make(chan error, len(rows))
	for i, rec := range rows[1:] {
		i, rec := i, rec
		wg.Add(1)
		go func() {
			defer wg.Done()
			vout, _ := strconv.Atoi(rec[3])
			err := b.PutErr(deposit.Run{
				ID:	rec[0],
				Chain:	rec[1], TxHash: rec[2], OutputIndex: vout,
				Step:	deposit.Step(rec[4]), Amount: rec[5], TransferID: rec[6], UserID: rec[7], Asset: rec[8],
			})
			if err != nil {
				errCh <- err
			}
			_ = i
		}()
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		t.Fatal(e)
	}
	hyd := deposit.NewBook().BindSQLTable(deposit.OpenBookTable(shared))
	if err := hyd.Hydrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	p := hyd.Pending(20)
	for _, r := range p {
		if deposit.TerminalStep(r.Step) {
			t.Fatalf("terminal %s", r.ID)
		}
	}
}

func TestHydrateFaultInjection(t *testing.T) {
	t.Parallel()
	tab := deposit.NewBookTable()
	tab.FailNextList()
	b := deposit.NewBook().MarkProduction(true).BindSQLTable(tab)
	if err := b.Hydrate(context.Background()); err == nil {
		t.Fatal("want hydrate fault")
	}
	if b.Health().OK {
		t.Fatal("prod hydrate fault must not be healthy")
	}
}

func TestAdvanceTable(t *testing.T) {
	t.Parallel()
	ok := [][2]deposit.Step{{deposit.StepObserved, deposit.StepAccepted}, {deposit.StepAccepted, deposit.StepConfirmed}, {deposit.StepConfirmed, deposit.StepCredited}, {deposit.StepCredited, deposit.StepUnwound}, {deposit.StepHeld, deposit.StepAccepted}}
	for _, e := range ok {
		if err := deposit.Advance(e[0], e[1]); err != nil {
			t.Fatalf("%s→%s %v", e[0], e[1], err)
		}
	}
	bad := [][2]deposit.Step{{deposit.StepCredited, deposit.StepAccepted}, {deposit.StepFailed, deposit.StepAccepted}, {deposit.StepUnwound, deposit.StepCredited}}
	for _, e := range bad {
		if err := deposit.Advance(e[0], e[1]); err == nil {
			t.Fatalf("want reject %s→%s", e[0], e[1])
		}
	}
}

func TestSQLFixtureMentionsNaturalKey(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join(testdataDir(t), "aa_deposits_fixture.sql"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, "tx-same") || !strings.Contains(s, "output_index") {
		t.Fatal(s)
	}
}
