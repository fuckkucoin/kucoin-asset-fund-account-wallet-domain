package quotemultigate_test

import (
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/quotemultigate"
)

func TestProdPanelNeedBundle(t *testing.T) {
	p := quotemultigate.NewProdPanel()
	if !p.NeedBundle() || !p.Policy().RefuseSilentSingle() {
		t.Fatal("prod")
	}
	if quotemultigate.NewPanel(quotemultigate.DefaultPolicy(), nil).NeedBundle() {
		t.Fatal("default should allow absent bundle")
	}
	var nilP *quotemultigate.Panel
	if nilP.NeedBundle() {
		t.Fatal("nil")
	}
	if nilP.Journal() != nil {
		t.Fatal("j")
	}
	pol := nilP.Policy()
	if !pol.AllowSilentSingle {
		t.Fatal("nil policy")
	}
}

func TestPanelDecideNoCommitOnFail(t *testing.T) {
	p := quotemultigate.NewProdPanel()
	now := gateNow()
	_, err := p.Decide(quotemultigate.Bundle{Ticks: []quotemultigate.Tick{
		liveTick("a", "BTC/USDT", "1", now),
		{Venue: "b", Status: quotemultigate.StatusTimeout},
	}}, now)
	if !errors.Is(err, quotemultigate.ErrQuoteTimeout) {
		t.Fatal(err)
	}
	if p.Journal().Count() != 0 {
		t.Fatal("fail closed journal")
	}
	p.Commit("", "k", "1", quotemultigate.Fingerprint{})
	if p.Journal().Count() != 0 {
		t.Fatal("empty idem")
	}
}

func TestOrPanelAndCoalesce(t *testing.T) {
	if quotemultigate.Coalesce(" a ", "b") != " a " {
		t.Fatal("a")
	}
	if quotemultigate.Coalesce("  ", "id") != "id" {
		t.Fatal("b")
	}
	p := quotemultigate.OrPanel(nil)
	if p == nil || p.NeedBundle() {
		t.Fatal("or")
	}
	keep := quotemultigate.NewProdPanel()
	if quotemultigate.OrPanel(keep) != keep {
		t.Fatal("keep")
	}
	quotemultigate.BindClock(time.Time{})
}

func TestAcceptanceCatalogPresent(t *testing.T) {
	raw := testdata(t, "acceptance_catalog.json")
	if len(raw) < 20 {
		t.Fatal("catalog")
	}
}

func TestNewPanelZeroPolicy(t *testing.T) {
	p := quotemultigate.NewPanel(quotemultigate.Policy{}, quotemultigate.NewJournal())
	if p.Policy().MinVenues != quotemultigate.DefaultMinVenues {
		t.Fatal("zero policy filled")
	}
}
