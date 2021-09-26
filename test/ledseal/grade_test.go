package ledseal_test

import (
	"testing"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/ledseal"
)

// parseMoneyMust 解析金额，失败则 panic（仅用于夹具）。
func parseMoneyMust(s string) money.Amount {
	a, err := money.ParseBook(s)
	if err != nil {
		panic("parseMoneyMust: " + s + ": " + err.Error())
	}
	return a
}

func TestGradeSeverity_TableDriven(t *testing.T) {
	type tc struct {
		name		string
		aBefore		string
		aAfter		string
		fBefore		string
		fAfter		string
		wantSeverity	ledseal.DriftSeverity
	}
	cases := []tc{
		// ── none（无变化）──────────────────────────────────────────────────────
		{
			name:		"none: 完全相同 USDT 1000",
			aBefore:	"1000", aAfter: "1000",
			fBefore:	"0", fAfter: "0",
			wantSeverity:	ledseal.SeverityNone,
		},
		{
			name:		"none: 均为零",
			aBefore:	"0", aAfter: "0",
			fBefore:	"0", fAfter: "0",
			wantSeverity:	ledseal.SeverityNone,
		},
		{
			name:		"none: 冻结相同",
			aBefore:	"500", aAfter: "500",
			fBefore:	"100", fAfter: "100",
			wantSeverity:	ledseal.SeverityNone,
		},
		// ── minor（微小变化，delta/total < 0.01%）─────────────────────────────
		{
			name:		"minor: USDT +0.01 on 100",
			aBefore:	"100", aAfter: "100.01",
			fBefore:	"0", fAfter: "0",
			wantSeverity:	ledseal.SeverityMinor,
		},
		{
			name:		"minor: USDT +0.001 on 10000",
			aBefore:	"10000", aAfter: "10000.001",
			fBefore:	"0", fAfter: "0",
			wantSeverity:	ledseal.SeverityMinor,
		},
		{
			name:		"minor: BTC 从 0 到 0.000001（绝对值极小，但相对率100% → critical）",
			aBefore:	"0", aAfter: "0.000001",
			fBefore:	"0", fAfter: "0",
			wantSeverity:	ledseal.SeverityCritical,	// denom=0.000001, rel=100% → critical
		},
		{
			name:		"minor: 仅冻结微变 +0.01 on 100",
			aBefore:	"100", aAfter: "100",
			fBefore:	"10", fAfter: "10.01",
			wantSeverity:	ledseal.SeverityMinor,
		},
		{
			name:		"minor: USDT -0.01（负 delta）",
			aBefore:	"100.01", aAfter: "100",
			fBefore:	"0", fAfter: "0",
			wantSeverity:	ledseal.SeverityMinor,
		},
		{
			name:		"minor: 绝对值 0.5，相对率 0.005% on 10000",
			aBefore:	"10000", aAfter: "10000.5",
			fBefore:	"0", fAfter: "0",
			wantSeverity:	ledseal.SeverityMinor,
		},
		// ── major（中等变化，delta/total ∈ [0.01%, 1%)）────────────────────────
		{
			name:		"major: USDT +1 on 100（绝对>=1，相对0.99% → major）",
			aBefore:	"100", aAfter: "101",
			fBefore:	"0", fAfter: "0",
			wantSeverity:	ledseal.SeverityMajor,	// rel=1/101≈0.99% < critRelThreshold
		},
		{
			name:		"major: USDT +5 on 10000（绝对 5>=1，相对 0.05% → major）",
			aBefore:	"10000", aAfter: "10005",
			fBefore:	"0", fAfter: "0",
			wantSeverity:	ledseal.SeverityMajor,
		},
		{
			name:		"major: BTC +0.01 on 100（绝对 0.01<1 → minor，不是 major）",
			aBefore:	"100", aAfter: "100.01",
			fBefore:	"0", fAfter: "0",
			wantSeverity:	ledseal.SeverityMinor,
		},
		{
			name:		"major: USDT +2 on 1000（绝对 2>=1，相对 0.2% → major）",
			aBefore:	"1000", aAfter: "1002",
			fBefore:	"0", fAfter: "0",
			wantSeverity:	ledseal.SeverityMajor,
		},
		{
			name:		"major: 可用+冻结合计变化（各 3）on 10000，相对 0.06% → major",
			aBefore:	"5000", aAfter: "5003",
			fBefore:	"5000", fAfter: "5003",
			wantSeverity:	ledseal.SeverityMajor,
		},
		// ── critical（严重变化，delta/total >= 1% 或绝对值 >= 10000）──────────
		{
			name:		"critical: USDT +50000（绝对 >= 10000）",
			aBefore:	"100", aAfter: "50100",
			fBefore:	"0", fAfter: "0",
			wantSeverity:	ledseal.SeverityCritical,
		},
		{
			name:		"critical: USDT +10000 精确触达绝对阈值",
			aBefore:	"0", aAfter: "10000",
			fBefore:	"0", fAfter: "0",
			wantSeverity:	ledseal.SeverityCritical,
		},
		{
			name:		"critical: 相对率 1% USDT +10 on 1000（rel=10/1010≈0.99% → major）",
			aBefore:	"1000", aAfter: "1010",
			fBefore:	"0", fAfter: "0",
			wantSeverity:	ledseal.SeverityMajor,	// rel=10/1010≈0.99% < 1%, delta=10 >= 1 → major
		},
		{
			name:		"critical: 相对率 100% BTC 从 1 → 2",
			aBefore:	"1", aAfter: "2",
			fBefore:	"0", fAfter: "0",
			wantSeverity:	ledseal.SeverityCritical,
		},
		{
			name:		"critical: 冻结爆增 +5000 on 1000（绝对 >= 1000 -> 相对 500%）",
			aBefore:	"1000", aAfter: "1000",
			fBefore:	"0", fAfter: "5000",
			wantSeverity:	ledseal.SeverityCritical,
		},
		{
			name:		"critical: 大量资金消失（绝对 -20000）",
			aBefore:	"20000", aAfter: "0",
			fBefore:	"0", fAfter: "0",
			wantSeverity:	ledseal.SeverityCritical,
		},
		{
			name:		"critical: 相对率刚好 1%（+100 on 10000）",
			aBefore:	"10000", aAfter: "10100",
			fBefore:	"0", fAfter: "0",
			wantSeverity:	ledseal.SeverityMajor,	// rel=100/10100≈0.99% < 1%, 绝对100>=1 → major
		},
		// ── ：从零新增 ───────────────────────────────────────────────────
		{
			name:		"critical: 从零到 0.5（denom=0.5, rel=100% → critical）",
			aBefore:	"0", aAfter: "0.5",
			fBefore:	"0", fAfter: "0",
			wantSeverity:	ledseal.SeverityCritical,
		},
		{
			name:		"critical: 从零到 10000（绝对 >= 10000 → critical）",
			aBefore:	"0", aAfter: "10000",
			fBefore:	"0", fAfter: "0",
			wantSeverity:	ledseal.SeverityCritical,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ab := parseMoneyMust(tc.aBefore)
			aa := parseMoneyMust(tc.aAfter)
			fb := parseMoneyMust(tc.fBefore)
			fa := parseMoneyMust(tc.fAfter)
			got := ledseal.GradeSeverity(ab, aa, fb, fa)
			if got != tc.wantSeverity {
				t.Errorf("gradeSeverity(%s→%s, %s→%s) = %s, want %s",
					tc.aBefore, tc.aAfter, tc.fBefore, tc.fAfter, got, tc.wantSeverity)
			}
		})
	}
}

// TestSeverityRank 验证严重度排序顺序。
func TestSeverityRank(t *testing.T) {
	order := []ledseal.DriftSeverity{ledseal.SeverityNone, ledseal.SeverityMinor, ledseal.SeverityMajor, ledseal.SeverityCritical}
	for i := 1; i < len(order); i++ {
		if ledseal.SeverityRank(order[i]) <= ledseal.SeverityRank(order[i-1]) {
			t.Errorf("severityRank(%s)=%d should be > severityRank(%s)=%d",
				order[i], ledseal.SeverityRank(order[i]), order[i-1], ledseal.SeverityRank(order[i-1]))
		}
	}
}

// TestSignedDecStr 验证带符号字符串整理。
func TestSignedDecStr(t *testing.T) {
	cases := []struct {
		input	string
		want	string
	}{
		{"0", "0"},
		{"1.5", "+1.5"},
		{"-1.5", "-1.5"},
		{"10000", "+10000"},
		{"-0.001", "-0.001"},
	}
	for _, tc := range cases {
		d, _ := decimal.NewFromString(tc.input)
		got := ledseal.SignedDecStr(d)
		if got != tc.want {
			t.Errorf("signedDecStr(%s) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestMaxSeverity_EmptyAndSingleAndMixed 验证 maxSeverity。
func TestMaxSeverity_EmptyAndSingleAndMixed(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if s := ledseal.MaxSeverity(nil); s != ledseal.SeverityNone {
			t.Errorf("empty: want none, got %s", s)
		}
	})
	t.Run("single_minor", func(t *testing.T) {
		rows := []ledseal.DriftRow{{Severity: ledseal.SeverityMinor}}
		if s := ledseal.MaxSeverity(rows); s != ledseal.SeverityMinor {
			t.Errorf("single_minor: want minor, got %s", s)
		}
	})
	t.Run("mixed_takes_highest", func(t *testing.T) {
		rows := []ledseal.DriftRow{
			{Severity: ledseal.SeverityMinor},
			{Severity: ledseal.SeverityMajor},
			{Severity: ledseal.SeverityMinor},
		}
		if s := ledseal.MaxSeverity(rows); s != ledseal.SeverityMajor {
			t.Errorf("mixed: want major, got %s", s)
		}
	})
	t.Run("has_critical", func(t *testing.T) {
		rows := []ledseal.DriftRow{
			{Severity: ledseal.SeverityMinor},
			{Severity: ledseal.SeverityCritical},
			{Severity: ledseal.SeverityMajor},
		}
		if s := ledseal.MaxSeverity(rows); s != ledseal.SeverityCritical {
			t.Errorf("has_critical: want critical, got %s", s)
		}
	})
}
