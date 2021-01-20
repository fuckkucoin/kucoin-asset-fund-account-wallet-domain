package withdraw_test

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
	"account-assets.kucoin.net/funding/internal/withdraw"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "wd-outbound")
}

func TestATCasesFixturePresent(t *testing.T) {
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
	if len(doc.Cases) < 16 {
		t.Fatalf("want 16 cases got %d", len(doc.Cases))
	}
}

func TestRefuseMemTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name	string
		prod	bool
		mem	bool
		bound	bool
		wantErr	bool
	}{
		{"A-T01 prod mem unbound", true, true, false, true},
		{"A-T02 dev mem unbound", false, true, false, false},
		{"prod sql bound", true, false, true, false},
		{"prod no mem unbound", true, false, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := withdraw.RefuseMemBook(tc.prod,
				tc.mem, tc.bound)
			if tc.wantErr && !errors.Is(err, withdraw.ErrMemBook) {
				t.Fatalf("err=%v", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAT01ProdMemBookRefused(t *testing.T) {
	t.Parallel()
	b := withdraw.NewBook().MarkProduction(true)
	if err := b.PutErr(withdraw.Run{ID: "r1", TransferID: "tr1", Step: withdraw.StepFrozen, Amount: "1"}); !errors.Is(err, withdraw.ErrMemBook) {
		t.Fatalf("prod put: %v", err)
	}
	if _, err := (&withdraw.Service{}).Recover(context.Background(), b, 8); !errors.Is(err, withdraw.ErrMemBook) {
		t.Fatalf("prod recover: %v", err)
	}
}

func TestAT02DevMemBookOK(t *testing.T) {
	t.Parallel()
	b := withdraw.NewBook()
	r := withdraw.Run{ID: "dev-1", TransferID: "tr-dev", Step: withdraw.StepFrozen, Amount: "2", Idem: "idem-dev", DebitOnce: true}
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

func TestAT03ProdBindSQLPutGet(t *testing.T) {
	t.Parallel()
	tab := withdraw.NewBookTable()
	b := withdraw.NewBook().MarkProduction(true).BindSQLTable(tab)
	if err := withdraw.RefuseMemBook(true, false, b.SQLBound()); err != nil {
		t.Fatal(err)
	}
	want := withdraw.Run{ID: "p1", TransferID: "tr-p", UserID: "u1", Asset: "USDT", Step: withdraw.StepFrozen, Amount: "9.1", Idem: "idem-p", Chain: "ETH", DebitOnce: true}
	if err := b.PutErr(want); err != nil {
		t.Fatal(err)
	}
	got, ok := b.ByTransfer("tr-p")
	if !ok || got.Amount != want.Amount || got.Idem != "idem-p" || got.DebitOnce != true {
		t.Fatalf("A-T03 %+v", got)
	}
	row, err := tab.Get(context.Background(), "tr-p")
	if err != nil || row.RunID != "p1" || row.Amount != "9.1" {
		t.Fatalf("sql row %+v %v", row, err)
	}
}

func TestAT04IdemConflict(t *testing.T) {
	t.Parallel()
	b := withdraw.NewBook().BindSQLTable(withdraw.NewBookTable())
	a := withdraw.Run{ID: "va", TransferID: "tr-a", Idem: "same", Step: withdraw.StepFrozen, UserID: "u1", DebitOnce: true}
	c := withdraw.Run{ID: "vb", TransferID: "tr-b", Idem: "same", Step: withdraw.StepFrozen, UserID: "u1", DebitOnce: true}
	if err := b.PutErr(a); err != nil {
		t.Fatal(err)
	}
	if err := b.PutErr(c); !errors.Is(err, withdraw.ErrIdemTaken) && !errors.Is(err, sqlrepo.ErrTaken) {
		t.Fatalf("A-T04 %v", err)
	}
}

func TestAT05AdvanceIllegalKeepsStep(t *testing.T) {
	t.Parallel()
	tab := withdraw.NewBookTable()
	b := withdraw.NewBook().BindSQLTable(tab)
	r := withdraw.Run{ID: "adv", TransferID: "t", Step: withdraw.StepConfirmed, Amount: "1", DebitOnce: true}
	if err := b.PutErr(r); err != nil {
		t.Fatal(err)
	}
	if err := withdraw.Advance(withdraw.StepConfirmed, withdraw.StepFrozen); err == nil {
		t.Fatal("want illegal")
	}
	if err := b.AdvancePersist("adv", withdraw.StepFrozen); err == nil {
		t.Fatal("want persist reject")
	}
	got, _ := b.Get("adv")
	if got.Step != withdraw.StepConfirmed {
		t.Fatalf("step mutated %s", got.Step)
	}
	row, _ := tab.Get(context.Background(), "t")
	if row.Step != string(withdraw.StepConfirmed) {
		t.Fatalf("sql step %s", row.Step)
	}
}

func TestAT06PendingNonTerminal(t *testing.T) {
	t.Parallel()
	b := withdraw.NewBook()
	for _, r := range []withdraw.Run{
		{ID: "a", TransferID: "1", Step: withdraw.StepFrozen},
		{ID: "c", TransferID: "2", Step: withdraw.StepConfirmed},
		{ID: "f", TransferID: "3", Step: withdraw.StepFailed, FailSealed: true},
func TestAT07RestartFrozenNoRedebit(t *testing.T) {
	t.Parallel()
	shared := map[string]sqlrepo.WDOutboundRun{}
	a := withdraw.NewBook().BindSQLTable(withdraw.OpenBookTable(shared))
	want := withdraw.Run{ID: "rs", TransferID: "tr-rs", Amount: "4.4", Step: withdraw.StepFrozen, UserID: "u", Asset: "USDT", Idem: "idem-rs", DebitOnce: true}
	if err := a.PutErr(want); err != nil {
		t.Fatal(err)
	}
	b := withdraw.NewBook().BindSQLTable(withdraw.OpenBookTable(shared))
	if err := b.Hydrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, ok := b.ByTransfer("tr-rs")
	if !ok || got.Step != withdraw.StepFrozen || got.Amount != "4.4" || !got.DebitOnce {
		t.Fatalf("A-T07 %+v ok=%v", got, ok)
	}
	again, ok := b.ResumeByIdem("idem-rs")
	if !ok || again.TransferID != "tr-rs" || !again.DebitOnce {
		t.Fatalf("resume %+v", again)
	}
	if len(b.Pending(8)) != 1 {
		t.Fatal("pending lost")
	}
}

func TestAT08RestartTerminalNotPending(t *testing.T) {
	t.Parallel()
	shared := map[string]sqlrepo.WDOutboundRun{}
	a := withdraw.NewBook().BindSQLTable(withdraw.OpenBookTable(shared))
	_ = a.PutErr(withdraw.Run{ID: "cf", TransferID: "t-cf", Step: withdraw.StepConfirmed, DebitOnce: true, Idem: "i-cf"})
	_ = a.FailSeal(withdraw.Run{ID: "ff", TransferID: "t-ff", Step: withdraw.StepFrozen, DebitOnce: true, Idem: "i-ff"}, "closed")
	b := withdraw.NewBook().BindSQLTable(withdraw.OpenBookTable(shared))
	if err := b.Hydrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p := b.Pending(20); len(p) != 0 {
		t.Fatalf("A-T08 pending %+v", p)
	}
	if _, ok := b.ByTransfer("t-cf"); !ok {
		t.Fatal("confirmed should remain gettable")
	}
	sealed, _ := b.ByTransfer("t-ff")
	if !sealed.FailSealed || sealed.Step != withdraw.StepFailed {
		t.Fatalf("seal lost %+v", sealed)
	}
}

func TestAT09AT10Claim(t *testing.T) {
	t.Parallel()
	tab := withdraw.NewBookTable()
	ctx := context.Background()
	first, err := tab.Claim(ctx, "tr1", "idem-1", "u1", "USDT", "1", "ETH")
	if err != nil {
		t.Fatal(err)
	}
	_, err = tab.Claim(ctx, "tr2", "idem-1", "u1", "USDT", "1", "ETH")
	if !errors.Is(err, sqlrepo.ErrTaken) {
		t.Fatalf("A-T09 %v", err)
	}
	again, err := tab.Claim(ctx, "tr1", "idem-1", "u1", "USDT", "1", "ETH")
	if err != nil || again.UserID != first.UserID || again.TransferID != first.TransferID {
		t.Fatalf("A-T10 %+v %v", again, err)
	}
}

func TestFailSealAndCompensateTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name		string
		step		withdraw.Step
		wantComp	string
		wantErr		bool
		afterConfirm	bool
	}{
		{"A-T11 frozen", withdraw.StepFrozen, "unfreeze_required", false, false},
		{"A-T12 submitted", withdraw.StepSubmitted, "submitted_live", false, true},
		{"A-T15 confirmed", withdraw.StepConfirmed, "", true, false},
		{"signed", withdraw.StepSigned, "unfreeze_required", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tab := withdraw.NewBookTable()
			b := withdraw.NewBook().BindSQLTable(tab)
			r := withdraw.Run{ID: "fs-" + string(tc.step), TransferID: "tr-" + string(tc.step), Step: tc.step, DebitOnce: true, Idem: "idem-" + string(tc.step)}
			if err := b.PutErr(r); err != nil {
				t.Fatal(err)
			}
			err := b.FailSeal(r, "step_fault")
			if tc.wantErr {
				if err == nil {
					t.Fatal("want fail-seal refuse")
				}
				got, _ := b.Get(r.ID)
				if got.Step != withdraw.StepConfirmed {
					t.Fatalf("confirmed mutated %s", got.Step)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, _ := b.ByTransfer(r.TransferID)
			if !got.FailSealed || got.Step != withdraw.StepFailed || got.Compensate != tc.wantComp {
				t.Fatalf("%+v", got)
			}
			if err := b.AdvancePersist(r.ID, withdraw.StepConfirmed); !errors.Is(err, withdraw.ErrFailSealed) {
				t.Fatalf("advance after seal %v", err)
			}
			if tc.afterConfirm {
				if err := b.Compensate(got, "unfreeze"); err == nil {
					t.Fatal("submitted_live must not silent unfreeze")
				}
			} else {
				if err := b.Compensate(got, "unfrozen"); err != nil {
					t.Fatal(err)
				}
				again, _ := b.ByTransfer(r.TransferID)
				if again.Compensate != "unfrozen" || again.Step != withdraw.StepFailed {
					t.Fatalf("%+v", again)
				}
			}
			row,
				_ := tab.Get(context.Background(), r.TransferID)
			if !row.FailSealed || row.Step != string(withdraw.StepFailed) {
				t.Fatalf("sql %+v", row)
			}
		})
	}
}

func TestAT13ProdAttachUnboundUnhealthy(t *testing.T) {
	t.Parallel()
	rt := &app.Runtime{
		Cfg:	&config.Config{Profile: "prod", Auth: config.AuthConfig{Kind: "iam"}},
		Mem:	true,
	}
	svc := withdraw.Attach(rt, withdraw.New(nil))
	h := svc.Runs().Health()
	if h.OK || h.SQLBound {
		t.Fatalf("A-T13 healthy %+v", h)
	}
	if !errors.Is(svc.Runs().AttachError(), withdraw.ErrMemBook) {
		t.Fatalf("attach %v", svc.Runs().AttachError())
	}
	if _, err := svc.Request(context.Background(), withdraw.Request{UserID: "u", Asset: "USDT", Chain: "ETH", ToAddress: "0x1"}); !errors.Is(err, withdraw.ErrMemBook) {
		t.Fatalf("request %v", err)
	}
}

func TestAT14RecoverNewInstance(t *testing.T) {
	t.Parallel()
	shared := map[string]sqlrepo.WDOutboundRun{}
	a := withdraw.NewBook().BindSQLTable(withdraw.OpenBookTable(shared))
	_ = a.PutErr(withdraw.Run{ID: "rec", TransferID: "tr-rec", Step: withdraw.StepFrozen, Amount: "2", DebitOnce: true, Idem: "idem-rec"})
	b := withdraw.NewBook().BindSQLTable(withdraw.OpenBookTable(shared))
	if err := b.Hydrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	called := 0
	svc := &withdraw.Service{book: b, recoverHook: func(ctx context.Context, id string) error {
		called++
		if id != "tr-rec" {
			t.Fatalf("id %s", id)
		}
		r, _ := b.ByTransfer("tr-rec")
		r.Step = withdraw.StepSubmitted
		return b.PutErr(r)
	}}
	n, err := svc.Recover(context.Background(), b, 8)
	if err != nil || n != 1 || called != 1 {
		t.Fatalf("A-T14 n=%d called=%d err=%v", n, called, err)
	}
	got, _ := b.ByTransfer("tr-rec")
	if got.Step != withdraw.StepSubmitted || !got.DebitOnce {
		t.Fatalf("step %s debit=%v", got.Step, got.DebitOnce)
	}
}

func TestPersistFaultNoSilentSuccess(t *testing.T) {
	t.Parallel()
	tab := withdraw.NewBookTable()
	tab.FailNextWrite()
	b := withdraw.NewBook().MarkProduction(true).BindSQLTable(tab)
	err := b.PutErr(withdraw.Run{ID: "w", TransferID: "tr-w", Step: withdraw.StepFrozen, DebitOnce: true, Idem: "idem-w"})
	if err == nil {
		t.Fatal("want persist fault")
	}
}

func TestHydrateFaultInjection(t *testing.T) {
	t.Parallel()
	tab := withdraw.NewBookTable()
	tab.FailNextList()
	b := withdraw.NewBook().MarkProduction(true).BindSQLTable(tab)
	if err := b.Hydrate(context.Background()); err == nil {
		t.Fatal("want hydrate fault")
	}
	if b.Health().OK {
		t.Fatal("prod hydrate fault must not be healthy")
	}
}

func TestAdvanceTable(t *testing.T) {
	t.Parallel()
	ok := [][2]withdraw.Step{
		{withdraw.StepRequested, withdraw.StepFrozen}, {withdraw.StepFrozen, withdraw.StepAllowlisted}, {withdraw.StepAllowlisted, withdraw.StepDecrypted},
		{withdraw.StepDecrypted, withdraw.StepKYT}, {withdraw.StepKYT, withdraw.StepSigned}, {withdraw.StepKYT, withdraw.StepReview},
		{withdraw.StepReview, withdraw.StepSigned,
		}, {withdraw.StepSigned, withdraw.StepSubmitted}, {withdraw.StepSubmitted, withdraw.StepConfirmed},
		{withdraw.StepFrozen, withdraw.StepFailed},
	}
	for _, e := range ok {
		if err := withdraw.Advance(e[0], e[1]); err != nil {
			t.Fatalf("%s→%s %v", e[0], e[1], err)
		}
	}
	bad := [][2]withdraw.Step{{withdraw.StepConfirmed, withdraw.StepFrozen}, {withdraw.StepFailed, withdraw.StepFrozen}, {withdraw.StepKYT, withdraw.StepSubmitted}}
	for _, e := range bad {
		if err := withdraw.Advance(e[0], e[1]); err == nil {
			t.Fatalf("want reject %s→%s", e[0], e[1])
		}
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
	shared := map[string]sqlrepo.WDOutboundRun{}
	b := withdraw.NewBook().BindSQLTable(withdraw.OpenBookTable(shared))
	var wg sync.WaitGroup
	errCh := make(chan error, len(rows))
	for _, rec := range rows[1:] {
		rec := rec
		wg.Add(1)
		go func() {
			defer wg.Done()
			debit, _ := strconv.Atoi(rec[8])
			seal, _ := strconv.Atoi(rec[9])
			err := b.PutErr(withdraw.Run{
				ID:	rec[0], TransferID: rec[1], UserID: rec[2], Step: withdraw.Step(rec[3]),
				Idem:	rec[4],
				Asset:	rec[5], Amount: rec[6], Chain: rec[7],
				DebitOnce:	debit == 1, FailSealed: seal == 1,
			})
			if err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		t.Fatal(e)
	}
	hyd := withdraw.NewBook().BindSQLTable(withdraw.OpenBookTable(shared))
	if err := hyd.Hydrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	p := hyd.Pending(40)
	for _, r := range p {
		if r.Step == withdraw.StepConfirmed || r.FailSealed {
			t.Fatalf("terminal %s", r.ID)
		}
	}
}

func TestSQLFixtureMentionsNaturalKey(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join(testdataDir(t), "aa_wd_outbound_fixture.sql"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, "transfer_id") || !strings.Contains(s, "idem-frozen") || !strings.Contains(s, "fail_sealed") {
		t.Fatal(s)
	}
}

func TestCompensateRequiresSeal(t *testing.T) {
	t.Parallel()
	b := withdraw.NewBook()
	r := withdraw.Run{ID: "c1", TransferID: "tr-c", Step: withdraw.StepFrozen, DebitOnce: true}
	if err := b.Compensate(r, "x"); err == nil {
		t.Fatal("want require seal")
	}
}

func TestConcurrentFailSeal(t *testing.T) {
	t.Parallel()
	tab := withdraw.NewBookTable()
	b := withdraw.NewBook().BindSQLTable(tab)
	r := withdraw.Run{ID: "conc", TransferID: "tr-conc", Step: withdraw.StepFrozen, DebitOnce: true, Idem: "idem-conc"}
	if err := b.PutErr(r); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.FailSeal(r, "race")
		}()
	}
	wg.Wait()
	got, _ := b.ByTransfer("tr-conc")
	if !got.FailSealed || got.Step != withdraw.StepFailed {
		t.Fatalf("%+v", got)
	}
}
