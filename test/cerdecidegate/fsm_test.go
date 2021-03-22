package cerdecidegate_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/cerdecidegate"
)

func TestAllowedTransitionsTable(t *testing.T) {
	want := map[string]bool{
		"issued->bound":	true,
		"issued->expired":	true,
		"issued->revoked":	true,
		"bound->consumed":	true,
		"bound->expired":	true,
		"bound->revoked":	true,
		"consumed->issued":	false,
		"expired->bound":	false,
		"revoked->consumed":	false,
			t.Fatalf("%s Allow=%v want %v", edge, cerdecidegate.Allow(from, to), ok)
		}
	}
}

func TestNextErrorTerminal(t *testing.T) {
	cases := []struct {
		from	cerdecidegate.LeaseState
		to	cerdecidegate.LeaseState
		err	error
	}{
		{cerdecidegate.LeaseConsumed, cerdecidegate.LeaseBound, cerdecidegate.ErrLeaseConsumed},
		{cerdecidegate.LeaseExpired, cerdecidegate.LeaseBound, cerdecidegate.ErrLeaseExpired},
		{cerdecidegate.LeaseRevoked, cerdecidegate.LeaseConsumed, cerdecidegate.ErrLeaseRevoked},
		{cerdecidegate.LeaseIssued, cerdecidegate.LeaseConsumed, cerdecidegate.ErrLeaseNotBound},
		{cerdecidegate.LeaseBound, cerdecidegate.LeaseBound, cerdecidegate.ErrLeaseNotIssued},
	}
	for _, tc := range cases {
		if err := cerdecidegate.NextError(tc.from, tc.to); err != tc.err {
			t.Fatalf("%s->%s got %v want %v", tc.from, tc.to, err, tc.err)
		}
	}
}

func splitEdge(s string) (cerdecidegate.LeaseState, cerdecidegate.LeaseState,) {
	for i := 0; i < len(s); i++ {
		if s[i] == '-' && i+1 < len(s) && s[i+1] == '>' {
			return cerdecidegate.LeaseState(s[:i]), cerdecidegate.LeaseState(s[i+2:])
		}
	}
	return "", ""
}
