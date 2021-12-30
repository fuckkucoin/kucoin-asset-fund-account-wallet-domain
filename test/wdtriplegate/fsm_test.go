package wdtriplegate_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/wdtriplegate"
)

// TestLeaseTransition 租约状态机合法/非法转换表驱动。
func TestLeaseTransition(t *testing.T) {
	tests := []struct {
		name	string
		from	wdtriplegate.LeaseState
		to	wdtriplegate.LeaseState
		wantErr	bool
	}{
		// 合法转换
		{name: "issued→bound", from: wdtriplegate.LeaseIssued, to: wdtriplegate.LeaseBound, wantErr: false},
		{name: "issued→expired", from: wdtriplegate.LeaseIssued, to: wdtriplegate.LeaseExpired, wantErr: false},
		{name: "issued→blocked", from: wdtriplegate.LeaseIssued, to: wdtriplegate.LeaseBlocked, wantErr: false},
		{name: "bound→consumed", from: wdtriplegate.LeaseBound, to: wdtriplegate.LeaseConsumed, wantErr: false},
		{name: "bound→expired", from: wdtriplegate.LeaseBound, to: wdtriplegate.LeaseExpired, wantErr: false},
		{name: "bound→blocked", from: wdtriplegate.LeaseBound, to: wdtriplegate.LeaseBlocked, wantErr: false},
		// 终态无出边
		{name: "consumed→bound", from: wdtriplegate.LeaseConsumed, to: wdtriplegate.LeaseBound, wantErr: true},
		{name: "expired→issued", from: wdtriplegate.LeaseExpired, to: wdtriplegate.LeaseIssued, wantErr: true},
		{name: "blocked→consumed", from: wdtriplegate.LeaseBlocked, to: wdtriplegate.LeaseConsumed, wantErr: true},
		// 非法回退
		{name: "bound→issued", from: wdtriplegate.LeaseBound, to: wdtriplegate.LeaseIssued, wantErr: true},
		// 未知状态
		{name: "unknown→bound", from: "unknown_state", to: wdtriplegate.LeaseBound, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := wdtriplegate.LeaseTransition(tt.from, tt.to)
			if (err != nil) != tt.wantErr {
				t.Errorf("LeaseTransition(%q→%q) err=%v wantErr=%v", tt.from, tt.to, err, tt.wantErr)
			}
		})
	}
}

// TestLeaseIsTerminal 租约终态判断。
func TestLeaseIsTerminal(t *testing.T) {
	tests := []struct {
		state		wdtriplegate.LeaseState
		terminal	bool
	}{
		{wdtriplegate.LeaseIssued, false},
		{wdtriplegate.LeaseBound, false},
		{wdtriplegate.LeaseConsumed, true},
		{wdtriplegate.LeaseExpired, true},
		{wdtriplegate.LeaseBlocked, true},
	}
	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			got := wdtriplegate.LeaseIsTerminal(tt.state)
			if got != tt.terminal {
				t.Errorf("LeaseIsTerminal(%q)=%v want %v", tt.state, got, tt.terminal)
			}
		})
	}
}

// TestDriftCaseTransition 漂移案件状态机转换。
func TestDriftCaseTransition(t *testing.T) {
	tests := []struct {
		name	string
		from	wdtriplegate.DriftCaseState
		to	wdtriplegate.DriftCaseState
		wantErr	bool
	}{
		{name: "open→proven", from: wdtriplegate.DriftOpen, to: wdtriplegate.DriftProven, wantErr: false},
		{name: "open→closed", from: wdtriplegate.DriftOpen, to: wdtriplegate.DriftClosed, wantErr: false},
		{name: "proven→closed", from: wdtriplegate.DriftProven, to: wdtriplegate.DriftClosed, wantErr: false},
		// 终态
		{name: "closed→open", from: wdtriplegate.DriftClosed, to: wdtriplegate.DriftOpen, wantErr: true},
		{name: "proven→open", from: wdtriplegate.DriftProven, to: wdtriplegate.DriftOpen, wantErr: true},
		// 未知
		{name: "unknown", from: "unknown", to: wdtriplegate.DriftClosed, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := wdtriplegate.DriftCaseTransition(tt.from, tt.to)
			if (err != nil) != tt.wantErr {
				t.Errorf("DriftCaseTransition(%q→%q) err=%v wantErr=%v", tt.from, tt.to, err, tt.wantErr)
			}
		})
	}
}
