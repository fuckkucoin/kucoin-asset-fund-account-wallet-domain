package keyrotgate_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/keyrotgate"
)

func TestLeaseTransition(t *testing.T) {
	cases := []struct {
		name	string
		from	keyrotgate.LeaseState
		to	keyrotgate.LeaseState
		wantErr	bool
	}{
		// 合法路径
		{"open→approved", keyrotgate.LeaseOpen, keyrotgate.LeaseApproved, false},
		{"open→blocked", keyrotgate.LeaseOpen, keyrotgate.LeaseBlocked, false},
		{"approved→executing", keyrotgate.LeaseApproved, keyrotgate.LeaseExecuting, false},
		{"approved→blocked", keyrotgate.LeaseApproved, keyrotgate.LeaseBlocked, false},
		{"executing→done", keyrotgate.LeaseExecuting, keyrotgate.LeaseDone, false},
		{"executing→blocked", keyrotgate.LeaseExecuting, keyrotgate.LeaseBlocked, false},
		// 非法路径
		{"open→executing", keyrotgate.LeaseOpen, keyrotgate.LeaseExecuting, true},
		{"open→done", keyrotgate.LeaseOpen, keyrotgate.LeaseDone, true},
		{"approved→done", keyrotgate.LeaseApproved, keyrotgate.LeaseDone, true},
		{"done→approved (终态)", keyrotgate.LeaseDone, keyrotgate.LeaseApproved, true},
		{"blocked→open (终态)", keyrotgate.LeaseBlocked, keyrotgate.LeaseOpen, true},
		{"done→blocked (终态)", keyrotgate.LeaseDone, keyrotgate.LeaseBlocked, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := keyrotgate.LeaseTransition(tc.from,
				tc.to)
			if (err != nil) != tc.wantErr {
				t.Errorf("LeaseTransition(%q→%q) err=%v, wantErr=%v", tc.from, tc.to, err, tc.wantErr)
			}
		})
	}
}

func TestLeaseIsTerminal(t *testing.T) {
	cases := []struct {
		state		keyrotgate.LeaseState
		terminal	bool
	}{
		{keyrotgate.LeaseOpen, false},
		{keyrotgate.LeaseApproved, false},
		{keyrotgate.LeaseExecuting, false},
		{keyrotgate.LeaseDone, true},
		{keyrotgate.LeaseBlocked, true},
	}
	for _, tc := range cases {
		t.Run(string(tc.state), func(t *testing.T) {
			if got := keyrotgate.LeaseIsTerminal(tc.state); got != tc.terminal {
				t.Errorf("LeaseIsTerminal(%q)=%v, want %v", tc.state, got, tc.terminal)
			}
		})
	}
}

func TestDriftCaseTransition(t *testing.T) {
	cases := []struct {
		name	string
		from	keyrotgate.DriftCaseState
		to	keyrotgate.DriftCaseState
		wantErr	bool
	}{
		{"open→proven", keyrotgate.DriftOpen, keyrotgate.DriftProven, false},
		{"open→closed", keyrotgate.DriftOpen, keyrotgate.DriftClosed, false},
		{"proven→closed", keyrotgate.DriftProven, keyrotgate.DriftClosed, false},
		{"closed→open (终态)", keyrotgate.DriftClosed, keyrotgate.DriftOpen, true},
		{"proven→open (无逆)", keyrotgate.DriftProven, keyrotgate.DriftOpen, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := keyrotgate.DriftCaseTransition(tc.from, tc.to)
			if (err != nil) != tc.wantErr {
				t.Errorf("DriftCaseTransition(%q→%q) err=%v, wantErr=%v", tc.from, tc.to, err, tc.wantErr)
			}
		})
	}
}

func TestSignBindTransition(t *testing.T) {
	cases := []struct {
		name	string
		from	keyrotgate.SignBindState
		to	keyrotgate.SignBindState
		wantErr	bool
	}{
		{"active→released", keyrotgate.SignBindActive, keyrotgate.SignBindReleased, false},
		{"active→expired", keyrotgate.SignBindActive, keyrotgate.SignBindExpired, false},
		{"active→rejected", keyrotgate.SignBindActive, keyrotgate.SignBindRejected, false},
		{"released→active (终态)", keyrotgate.SignBindReleased, keyrotgate.SignBindActive, true},
		{"expired→released (终态)", keyrotgate.SignBindExpired, keyrotgate.SignBindReleased, true},
		{"rejected→active (终态)", keyrotgate.SignBindRejected, keyrotgate.SignBindActive, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := keyrotgate.SignBindTransition(tc.from, tc.to)
			if (err != nil) != tc.wantErr {
				t.Errorf("SignBindTransition(%q→%q) err=%v, wantErr=%v", tc.from, tc.to, err, tc.wantErr)
			}
		})
	}
}
