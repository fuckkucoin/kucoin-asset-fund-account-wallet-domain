package margin_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/margin"
)

func TestMaintenanceRequiredTable(t *testing.T) {
	params := margin.Params{MaintenanceRateBPS: 500}
	cases := []struct {
		name	string
		pos	margin.Position
		px	string
		p	margin.Params
		want	string
	}{
		{name: "rate_only", pos: margin.Position{Asset: "BTC", Qty: "2"}, px: "10000", p: params, want: "1000"},
		{name: "pos_rate_override", pos: margin.Position{Asset: "BTC", Qty: "2", MaintenanceRateBPS: 1000}, px: "10000", p: params, want: "2000"},
		{name: "amount_floor", pos: margin.Position{Asset: "ETH", Qty: "1"}, px: "100", p: margin.Params{MaintenanceRateBPS: 100, MaintenanceAmount: "50"}, want: "50"},
		{name: "zero_qty_floor", pos: margin.Position{Asset: "ETH", Qty: "0"}, px: "100", p: margin.Params{MaintenanceAmount: "12"}, want: "12"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := margin.MaintenanceRequired(tc.pos, tc.px, tc.p)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("required=%s want=%s", got, tc.want)
			}
		})
	}
}

func TestMaintenanceRequiredRejectsMissingThreshold(t *testing.T) {
	_, err := margin.MaintenanceRequired(margin.Position{Asset: "BTC", Qty: "1"}, "10", margin.Params{})
	if err == nil {
		t.Fatal("expected missing threshold error")
	}
}

func TestMaintenanceRequiredRejectsMissingPrice(t *testing.T) {
	_, err := margin.MaintenanceRequired(margin.Position{Asset: "BTC", Qty: "1"}, "", margin.Params{MaintenanceRateBPS: 500})
	if err == nil {
		t.Fatal("missing price must fail closed")
	}
	_, err = margin.MaintenanceRequired(margin.Position{Asset: "BTC", Qty: "1"}, "0", margin.Params{MaintenanceRateBPS: 500})
	if err == nil {
		t.Fatal("zero price must fail closed")
	}
}

func TestCombinedMaintenance(t *testing.T) {
	params := margin.Params{MaintenanceRateBPS: 1000}
	got, err := margin.CombinedMaintenance(
		[]margin.Position{{Asset: "BTC", Qty: "1"}, {Asset: "ETH", Qty: "10"}},
		map[string]string{"BTC": "100", "ETH": "10"},
		params,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != "20" {
		t.Fatalf("combined=%s want=20", got)
	}
}

func TestMarginRatioAndEquality(t *testing.T) {
	r, err := margin.MarginRatio("100", "100")
	if err != nil {
		t.Fatal(err)
	}
	if r != "1" {
		t.Fatalf("ratio=%s", r)
	}
	v, err := margin.CompareThreshold("100", "100")
	if err != nil || v != margin.VerdictOK {
		t.Fatalf("equality should be OK got %s err=%v", v, err)
	}
	v, err = margin.CompareThreshold("99", "100")
	if err != nil || v != margin.VerdictBreach {
		t.Fatalf("below should breach got %s err=%v", v, err)
	}
	v, err = margin.CompareThreshold("10", "0")
	if err != nil || v != margin.VerdictOK {
		t.Fatalf("zero required OK got %s err=%v", v, err)
	}
}
