package quotemultigate_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
	"account-assets.kucoin.net/funding/internal/quotemultigate"
)

func TestJournalRestartSameFingerprint(t *testing.T) {
	now := gateNow()
	b := quotemultigate.Bundle{
		Key:	"BTC/USDT",
		Idem:	"u-reentry-1",
		Ticks: []quotemultigate.Tick{
			liveTick("spot_alpha", "BTC/USDT", "64000", now),
			liveTick("spot_beta", "BTC/USDT", "64000", now),
		},
	}
	p1 := quotemultigate.NewPanel(quotemultigate.DefaultPolicy(), quotemultigate.NewJournal())
	d1, err := p1.Decide(b, now)
	if err != nil {
		t.Fatal(err)
	}
	p1.Commit(b.Idem, b.Key, "64000", d1.Fingerprint)
	dir := t.TempDir()
	path := filepath.Join(dir, "journal.json")
	if err := p1.Journal().Save(path); err != nil {
		t.Fatal(err)
	}
	j2, err := quotemultigate.LoadJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	p2 := quotemultigate.NewPanel(quotemultigate.DefaultPolicy(), j2)
	d2, err := p2.Decide(b, now)
	if err != nil {
		t.Fatal(err)
	}
	if !d2.Reentry {
		t.Fatal("expect reentry")
	}
	if j2.Count() != 1 {
		t.Fatal(j2.Count())
	}
}

func TestJournalReentryDivergeFingerprint(t *testing.T) {
	now := gateNow()
	p := quotemultigate.NewPanel(quotemultigate.DefaultPolicy(), quotemultigate.NewJournal())
	b1 := quotemultigate.Bundle{
		Idem:	"u-reentry-1",
		Ticks: []quotemultigate.Tick{
			liveTick("spot_alpha", "BTC/USDT", "64000", now),
			liveTick("spot_beta", "BTC/USDT", "64000", now),
		},
	}
	d, err := p.Decide(b1, now)
	if err != nil {
		t.Fatal(err)
	}
	p.Commit(b1.Idem, "BTC/USDT", "64000", d.Fingerprint)
	b2 := b1
	b2.Ticks = []quotemultigate.Tick{
		liveTick("spot_alpha", "BTC/USDT", "64100", now),
		liveTick("spot_beta", "BTC/USDT", "64100", now),
	}
	if _, err := p.Decide(b2, now); !errors.Is(err, quotemultigate.ErrQuoteDiverge) {
		t.Fatalf("%v", err)
	}
}

func TestLoadJournalMissingAndSeed(t *testing.T) {
	j, err := quotemultigate.LoadJournal(filepath.Join("..", "..", "testdata", "quotemulti", "journal_reentry.json"))
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := j.Lookup("u-reentry-1")
	if !ok || !rec.Applied {
		t.Fatal(rec)
	}
	raw := testdata(t, "journal_reentry.json")
	var rows []quotemultigate.Record
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatal(len(rows))
	}
	miss, err := quotemultigate.LoadJournal(filepath.Join("..", "..", "testdata", "quotemulti", "no_such.json"))
	if err != nil || miss.Count() != 0 {
		t.Fatal(err)
	}
	empty, err := quotemultigate.LoadJournal("")
	if err != nil || empty.Count() != 0 {
		t.Fatal(err)
	}
	if rec, ok := quotemultigate.NewJournal().Lookup(""); ok || rec.Applied {
		t.Fatal("empty idem")
	}
}

func TestJournalCommitGuards(t *testing.T) {
	var j *quotemultigate.Journal
	j.Commit(quotemultigate.Record{Idem: "x"})
	if j.Count() != 0 {
		t.Fatal("nil")
	}
	if err := j.Save(filepath.Join(t.TempDir(), "x.json")); err != nil {
		t.Fatal(err)
	}
	n := quotemultigate.NewJournal()
	n.Commit(quotemultigate.Record{})
	if n.Count() != 0 {
		t.Fatal("skip empty")
	}
	_ = quote.DefaultSwapTTL
	_ = decimal.Zero
	if _, err := os.Stat(filepath.Join("..", "..", "testdata", "quotemulti", "acceptance_catalog.json")); err != nil {
		t.Fatal(err)
	}
}
