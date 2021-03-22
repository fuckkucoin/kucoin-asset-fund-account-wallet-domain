package balproofgate_test

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/balproofgate"
)

func TestEvaluateConsumeGate(t *testing.T) {
	now := time.Date(2026, 9, 21, 15, 0, 0, 0, time.UTC)
	lease := balproofgate.ProofLease{
		Status:		balproofgate.LeaseBound,
		Generation:	2,
		ExpiresAt:	now.Add(time.Minute),
		Snapshot:	balproofgate.LiveSnapshotRoot{Available: "5", Frozen: "0", ObserveGen: 1},
	}
	live := balproofgate.LiveView{Available: "5", Frozen: "0", ObserveGen: 1}
	cases := []struct {
		name	string
		revoked	bool
		gen	int64
		live	balproofgate.LiveView
		block	bool
		lease	balproofgate.ProofLease
		err	error
		allow	bool
	}{
		{"ok", false, 2, live, false, lease, nil, true},
		{"revoked", true, 2, live, false, lease, balproofgate.ErrRevoked, false},
		{"stale", false, 3, live, false, lease, balproofgate.ErrStaleGen, false},
		{"drift", false, 2, balproofgate.LiveView{Available: "9", Frozen: "0", ObserveGen: 2}, false, lease, balproofgate.ErrLiveDrift, false},
		{"block", false, 2, live, true, lease, balproofgate.ErrDriftBlocked, false},
		{"ttl", false, 2, live, false, func() balproofgate.ProofLease { l := lease; l.ExpiresAt = now.Add(-time.Second); return l }(), balproofgate.ErrLeaseExpired, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := balproofgate.EvaluateConsumeGate(c.lease, now, c.revoked, c.gen, c.live, c.block)
			if v.Allow != c.allow || v.Err != c.err {
				t.Fatalf("%+v", v)
			}
		})
	}
	if !balproofgate.MustDenyConsume(lease.Snapshot, balproofgate.LiveView{Available: "6", Frozen: "0", ObserveGen: 2}) {
		t.Fatal("must deny")
	}
}
