package sweeprun_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/sweeprun"
)

func TestCheckAgreeTable(t *testing.T) {
	cases := []struct {
		name	string
		bal	string
		rule	sweeprun.RebalanceRuleSnap
		code	sweeprun.AgreeCode
	}{
		{
			name:	"pass",
			bal:	"100",
			rule:	sweeprun.RebalanceRuleSnap{Asset: "USDT", Waterline: "40", Kinds: []string{"hot_cold"}, Version: 1},
			code:	sweeprun.AgreeOK,
		},
		{
			name:	"below",
			bal:	"10",
			rule:	sweeprun.RebalanceRuleSnap{Asset: "USDT", Waterline: "40", Kinds: []string{"hot_cold"}, Version: 1},
			code:	sweeprun.AgreeDeniedBelow,
		},
		{
			name:	"halt",
			bal:	"100",
			rule:	sweeprun.RebalanceRuleSnap{Asset: "USDT", Waterline: "40", Kinds: []string{"hot_cold"}, HaltAgree: true},
			code:	sweeprun.AgreeDeniedHalt,
		},
		{
			name:	"no_kinds",
			bal:	"100",
			rule:	sweeprun.RebalanceRuleSnap{Asset: "USDT", Waterline: "40"},
			code:	sweeprun.AgreeDeniedKinds,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			g := sweeprun.NewAgreeBook()
			if err := g.PutShadowBal(sweeprun.ShadowBal{Chain: "ETH", Asset: "USDT", Amount: tc.bal}); err != nil {
				t.Fatal(err)
			}
			if err := g.PutRule(tc.rule); err != nil {
				t.Fatal(err)
			}
			ev, err := g.CheckAgree("ETH", "USDT")
			if err != nil {
				t.Fatal(err)
			}
			if ev.Code != tc.code {
				t.Fatalf("code %q want %q", ev.Code, tc.code)
			}
		})
	}
}

func TestFingerprintStable(t *testing.T) {
	a := sweeprun.BalFingerprint(sweeprun.ShadowBal{Chain: "eth", Asset: "usdt", Amount: "10.00"})
	b := sweeprun.BalFingerprint(sweeprun.ShadowBal{Chain: "ETH", Asset: "USDT", Amount: "10"})
	if a != b {
		t.Fatalf("%s != %s", a, b)
	}
	r1 := sweeprun.RuleFingerprint(sweeprun.RebalanceRuleSnap{Asset: "USDT", Waterline: "1", Kinds: []string{"hot_cold"}, Version: 1})
	r2 := sweeprun.RuleFingerprint(sweeprun.RebalanceRuleSnap{Asset: "USDT", Waterline: "1.0", Kinds: []string{"hot_cold"}, Version: 1})
	if r1 != r2 {
		t.Fatalf("rule %s != %s", r1, r2)
	}
}

func TestBumpRuleChangesFingerprint(t *testing.T) {
	g := sweeprun.NewAgreeBook()
	_ = g.PutRule(sweeprun.RebalanceRuleSnap{Asset: "USDT", Waterline: "10", Kinds: []string{"hot_cold"}, Version: 1})
	before, _ := g.GetRule("USDT")
	fp1 := sweeprun.RuleFingerprint(before)
	if err := g.BumpRuleVersion("USDT", "12"); err != nil {
		t.Fatal(err)
	}
	after, _ := g.GetRule("USDT")
	if after.Version != 2 {
		t.Fatal(after.Version)
	}
	if sweeprun.RuleFingerprint(after) == fp1 {
		t.Fatal("fingerprint should change")
	}
}
