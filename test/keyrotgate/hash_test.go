package keyrotgate_test

import (
	"strings"
	"testing"
	"account-assets.kucoin.net/funding/internal/keyrotgate"
)

func TestProofChecksum_Deterministic(t *testing.T) {
	cs1 := keyrotgate.ProofChecksum("aabb", "ccdd", 1, "tk-001")
	cs2 := keyrotgate.ProofChecksum("aabb", "ccdd", 1, "tk-001")
	if cs1 != cs2 {
		t.Errorf("ProofChecksum not deterministic: %q vs %q", cs1, cs2)
	}
	if len(cs1) != 64 {	// SHA-256 hex = 64 chars
		t.Errorf("checksum length=%d, want 64", len(cs1))
	}
}

func TestProofChecksum_Distinct(t *testing.T) {
	cases := []struct {
		before, after	string
		gen		int64
		ticket		string
	}{
		{"aabb", "ccdd", 1, "tk-001"},
	for _, tc := range cases {
		cs := keyrotgate.ProofChecksum(tc.before,
			tc.after, tc.gen, tc.ticket)
		if seen[cs] {
			t.Errorf("duplicate checksum for %+v: %s", tc, cs)
		}
		seen[cs] = true
	}
}

func TestFingerprintsMatch(t *testing.T) {
	cases := []struct {
		a, b	string
		match	bool
	}{
		{"aabbccddeeff", "aabbccddeeff", true},
		{"AABBCCDDEEFF", "aabbccddeeff", true},	// 大小写不敏感
		{"aabbccddeeff", "aabbccddeeff00", false},
		{"", "", true},
		{"aabb", "ccdd", false},
	}
	for _, tc := range cases {
		got := keyrotgate.FingerprintsMatch(tc.a, tc.b)
		if got != tc.match {
			t.Errorf("FingerprintsMatch(%q, %q)=%v, want %v", tc.a, tc.b, got, tc.match)
		}
	}
}

func TestValidateFingerprint(t *testing.T) {
	cases := []struct {
		fp	string
		wantErr	bool
	}{
		{"aabbccddeeff0011", false},		// 合法十六进制
		{"AA:BB:CC:DD:EE:FF", false},		// SSH 指纹格式（含冒号）
		{"short", true},			// 太短
		{strings.Repeat("a", 129), true},	// 太长
		{"aabbccdd", false},			// 最短 8 字符
		{strings.Repeat("a", 128), false},	// 最长 128 字符
	}
	for _, tc := range cases {
		err := keyrotgate.ValidateFingerprint(tc.fp)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidateFingerprint(%q) err=%v, wantErr=%v", tc.fp, err, tc.wantErr)
		}
	}
}
