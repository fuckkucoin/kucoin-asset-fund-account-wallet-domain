package fxonce_test

import (
	"strings"
	"testing"
	"account-assets.kucoin.net/funding/internal/fxonce"
)

// ────────────────────────────────────────────────────────────────────────────
// MakeEffectKey
// ────────────────────────────────────────────────────────────────────────────

func TestMakeEffectKey_Deterministic(t *testing.T) {
	key1 := fxonce.MakeEffectKey("dep-0001", 1)
	key2 := fxonce.MakeEffectKey("dep-0001", 1)
	if key1 != key2 {
		t.Errorf("MakeEffectKey not deterministic: %q != %q", key1, key2)
	}
}

func TestMakeEffectKey_DiffGeneration(t *testing.T) {
	key1 := fxonce.MakeEffectKey("dep-0001", 1)
	key2 := fxonce.MakeEffectKey("dep-0001", 2)
	if key1 == key2 {
		t.Error("MakeEffectKey: different generations must produce different keys")
	}
}

func TestMakeEffectKey_DiffBusinessKey(t *testing.T) {
	key1 := fxonce.MakeEffectKey("dep-0001", 1)
	key2 := fxonce.MakeEffectKey("dep-0002", 1)
	if key1 == key2 {
		t.Error("MakeEffectKey: different business keys must produce different keys")
	}
}

func TestMakeEffectKey_Length(t *testing.T) {
	key := fxonce.MakeEffectKey("any-key", 99)
	if len(key) != 32 {
		t.Errorf("MakeEffectKey: expected length 32, got %d (%q)", len(key), key)
	}
}

func TestMakeEffectKey_IsHex(t *testing.T) {
	key := fxonce.MakeEffectKey("test-key", 1)
	for i, c := range key {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("MakeEffectKey: non-hex char %q at index %d in %q", c, i, key)
		}
	}
}

func TestMakeEffectKey_ColonSeparation(t *testing.T) {
	// "foo:1" と "foo:" + gen=1 的等价性
	// "fo" + "o:1" vs "foo" + ":1" 应不同 business_key 不同 gen
	k1 := fxonce.MakeEffectKey("fo", 11)
	k2 := fxonce.MakeEffectKey("foo", 1)
	// 不能碰撞：不同输入必须不同
	if k1 == k2 {
		t.Error("MakeEffectKey: collision between different (businessKey, gen) pairs")
	}
}

// ────────────────────────────────────────────────────────────────────────────
// ValidateBusinessKey
// ────────────────────────────────────────────────────────────────────────────

var businessKeyTests = []struct {
	name	string
	key	string
	wantErr	bool
}{
	{"valid short", "dep-001", false},
	{"valid uuid", "550e8400-e29b-41d4-a716-446655440000", false},
	{"empty", "", true},
	{"too long 192 chars", strings.Repeat("x", 192), true},
	{"exactly 191 chars", strings.Repeat("y", 191), false},
}

func TestValidateBusinessKey(t *testing.T) {
	for _, tc := range businessKeyTests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := fxonce.ValidateBusinessKey(tc.key)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateBusinessKey(%q) err=%v, wantErr=%v", tc.key, err, tc.wantErr)
			}
		})
	}
}

// ────────────────────────────────────────────────────────────────────────────
// ValidateEffectKey
// ────────────────────────────────────────────────────────────────────────────

func TestValidateEffectKey_Valid(t *testing.T) {
	k := fxonce.MakeEffectKey("dep-001", 1)
	if err := fxonce.ValidateEffectKey(k); err != nil {
		t.Errorf("ValidateEffectKey(%q) unexpected error: %v", k, err)
	}
}

func TestValidateEffectKey_WrongLength(t *testing.T) {
	cases := []string{"", "abc", strings.Repeat("a", 31), strings.Repeat("a", 33)}
	for _, c := range cases {
		if err := fxonce.ValidateEffectKey(c); err != fxonce.ErrBadEffectKey {
			t.Errorf("ValidateEffectKey(%q) want ErrBadEffectKey, got %v", c, err)
		}
	}
}

func TestValidateEffectKey_NonHex(t *testing.T) {
	key := strings.Repeat("g", 32)	// 'g' is not valid hex
	if err := fxonce.ValidateEffectKey(key); err != fxonce.ErrBadEffectKey {
		t.Errorf("ValidateEffectKey with non-hex chars: want ErrBadEffectKey, got %v", err)
	}
}

// ────────────────────────────────────────────────────────────────────────────
// IsFundKind / AllFundKinds
// ────────────────────────────────────────────────────────────────────────────

func TestIsFundKind_KnownKinds(t *testing.T) {
	for _, fk := range fxonce.AllFundKinds() {
		if !fxonce.IsFundKind(string(fk)) {
			t.Errorf("IsFundKind(%q) = false, want true", fk)
		}
	}
}

func TestIsFundKind_UnknownKinds(t *testing.T) {
	nonFund := []string{
		"audit.log",
		"notify.email",
		"deposit.credited",
		"withdraw.confirmed",
		"",
		"CREDIT_INTENT",	// 大小写敏感
	}
	for _, k := range nonFund {
		if fxonce.IsFundKind(k) {
			t.Errorf("IsFundKind(%q) = true, want false", k)
		}
	}
}

func TestAllFundKinds_NoDuplicates(t *testing.T) {
	seen := map[string]bool{}
	for _, fk := range fxonce.AllFundKinds() {
		if seen[string(fk)] {
			t.Errorf("AllFundKinds: duplicate kind %q", fk)
		}
		seen[string(fk)] = true
	}
}

func TestAllFundKinds_MinCount(t *testing.T) {
	if n := len(fxonce.AllFundKinds()); n < 4 {
		t.Errorf("AllFundKinds: expected at least 4 kinds, got %d", n)
	}
}

// ────────────────────────────────────────────────────────────────────────────
// validDualActors（通过包内访问）
// ────────────────────────────────────────────────────────────────────────────

func TestValidDualActors(t *testing.T) {
	cases := []struct {
		actors	string
		want	bool
	}{
		{"alice+bob", true},
		{"alice+alice", false},	// 同人不算双人
		{"alice+", false},
		{"+bob", false},
		{"", false},
		{"alice", false},
		{"alice+bob+carol", true},	// 包含 + 但前两段不同，视为合法（取第一个 +）
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.actors, func(t *testing.T) {
			got := fxonce.ValidDualActors(tc.actors)
			if got != tc.want {
				t.Errorf("validDualActors(%q) = %v, want %v", tc.actors, got, tc.want)
			}
		})
	}
}
