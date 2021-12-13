package stmtexpiry_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/fsm"
	"account-assets.kucoin.net/funding/internal/stmtexpiry"
)

func TestExportFSM_ValidTransitions(t *testing.T) {
	tests := []struct {
		name	string
		from	string
		ev	string
		to	string
	}{
		{"built-issue-issued", string(stmtexpiry.LeaseBuilt), stmtexpiry.EvIssue, string(stmtexpiry.LeaseIssued)},
		{"built-revoke-revoked", string(stmtexpiry.LeaseBuilt), stmtexpiry.EvRevoke, string(stmtexpiry.LeaseRevoked)},
		{"issued-expire-expired", string(stmtexpiry.LeaseIssued), stmtexpiry.EvExpire, string(stmtexpiry.LeaseExpired)},
		{"issued-revoke-revoked", string(stmtexpiry.LeaseIssued), stmtexpiry.EvRevoke, string(stmtexpiry.LeaseRevoked)},
	}
	m := stmtexpiry.ExportFSM()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := m.Apply(tt.from, tt.ev)
			if err != nil {
				t.Fatalf("Apply(%q, %q) error = %v", tt.from, tt.ev, err)
			}
			if got != tt.to {
				t.Errorf("Apply(%q, %q) = %q, want %q", tt.from, tt.ev, got, tt.to)
			}
		})
	}
}

func TestExportFSM_InvalidTransitions(t *testing.T) {
	tests := []struct {
		name	string
		from	string
		ev	string
	}{
		{"built-expire", string(stmtexpiry.LeaseBuilt), stmtexpiry.EvExpire},
		{"issued-issue", string(stmtexpiry.LeaseIssued), stmtexpiry.EvIssue},
		{"expired-issue", string(stmtexpiry.LeaseExpired), stmtexpiry.EvIssue},
		{"expired-expire", string(stmtexpiry.LeaseExpired), stmtexpiry.EvExpire},
		{"expired-revoke", string(stmtexpiry.LeaseExpired), stmtexpiry.EvRevoke},
		{"revoked-issue", string(stmtexpiry.LeaseRevoked), stmtexpiry.EvIssue},
		{"revoked-expire", string(stmtexpiry.LeaseRevoked), stmtexpiry.EvExpire},
		{"revoked-revoke", string(stmtexpiry.LeaseRevoked), stmtexpiry.EvRevoke},
	}
	m := stmtexpiry.ExportFSM()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := m.Apply(tt.from, tt.ev)
			if err == nil {
				t.Errorf("Apply(%q, %q) should have failed", tt.from, tt.ev)
			}
		})
	}
}

func TestExportFSM_Walk(t *testing.T) {
	tests := []struct {
		name	string
		events	[]string
		wantEnd	string
		wantErr	bool
	}{
		{"built-issue-expire", []string{stmtexpiry.EvIssue, stmtexpiry.EvExpire}, string(stmtexpiry.LeaseExpired), false},
		{"built-issue-revoke", []string{stmtexpiry.EvIssue, stmtexpiry.EvRevoke}, string(stmtexpiry.LeaseRevoked), false},
		{"built-revoke", []string{stmtexpiry.EvRevoke}, string(stmtexpiry.LeaseRevoked), false},
		{"built-expire-fail", []string{stmtexpiry.EvExpire}, "", true},
		{"built-issue-issue-fail", []string{stmtexpiry.EvIssue, stmtexpiry.EvIssue}, "", true},
	}
	m := stmtexpiry.ExportFSM()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fsm.Walk(m, string(stmtexpiry.LeaseBuilt), tt.events...)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Walk should have failed")
				}
				return
			}
			if err != nil {
				t.Fatalf("Walk error = %v", err)
			}
			if got != tt.wantEnd {
				t.Errorf("Walk = %q, want %q", got, tt.wantEnd)
			}
		})
	}
}

func TestExportFSM_Reachable(t *testing.T) {
	tests := []struct {
		name	string
		from	string
		to	string
		reach	bool
	}{
		{"built-to-issued", string(stmtexpiry.LeaseBuilt), string(stmtexpiry.LeaseIssued), true},
		{"built-to-expired", string(stmtexpiry.LeaseBuilt), string(stmtexpiry.LeaseExpired), true},
		{"built-to-revoked", string(stmtexpiry.LeaseBuilt), string(stmtexpiry.LeaseRevoked), true},
		{"issued-to-expired", string(stmtexpiry.LeaseIssued), string(stmtexpiry.LeaseExpired), true},
		{"issued-to-revoked", string(stmtexpiry.LeaseIssued), string(stmtexpiry.LeaseRevoked), true},
		{"expired-to-built", string(stmtexpiry.LeaseExpired), string(stmtexpiry.LeaseBuilt), false},
		{"revoked-to-built", string(stmtexpiry.LeaseRevoked), string(stmtexpiry.LeaseBuilt), false},
		{"expired-to-issued", string(stmtexpiry.LeaseExpired), string(stmtexpiry.LeaseIssued), false},
	}
	m := stmtexpiry.ExportFSM()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fsm.Reachable(m, tt.from, tt.to)
			if got != tt.reach {
				t.Errorf("Reachable(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.reach)
			}
		})
	}
}

func TestDriftFSM_ValidTransitions(t *testing.T) {
	tests := []struct {
		name	string
		from	string
		ev	string
		to	string
	}{
		{"open-prove-proven", string(stmtexpiry.DriftOpen), stmtexpiry.EvProve, string(stmtexpiry.DriftProven)},
		{"proven-close-closed", string(stmtexpiry.DriftProven), stmtexpiry.EvClose, string(stmtexpiry.DriftClosed)},
	}
	m := stmtexpiry.DriftFSM()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := m.Apply(tt.from, tt.ev)
			if err != nil {
				t.Fatalf("Apply(%q, %q) error = %v", tt.from, tt.ev, err)
			}
			if got != tt.to {
				t.Errorf("Apply(%q, %q) = %q, want %q", tt.from, tt.ev, got, tt.to)
			}
		})
	}
}

func TestDriftFSM_InvalidTransitions(t *testing.T) {
	tests := []struct {
		name	string
		from	string
		ev	string
	}{
		{"open-close", string(stmtexpiry.DriftOpen), stmtexpiry.EvClose},
		{"proven-prove", string(stmtexpiry.DriftProven), stmtexpiry.EvProve},
		{"closed-prove", string(stmtexpiry.DriftClosed), stmtexpiry.EvProve},
		{"closed-close", string(stmtexpiry.DriftClosed), stmtexpiry.EvClose},
	}
	m := stmtexpiry.DriftFSM()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := m.Apply(tt.from, tt.ev)
			if err == nil {
				t.Errorf("Apply(%q, %q) should have failed", tt.from, tt.ev)
			}
		})
	}
}

func TestDriftFSM_Walk(t *testing.T) {
	tests := []struct {
		name	string
		events	[]string
		wantEnd	string
		wantErr	bool
	}{
		{"open-prove-close", []string{stmtexpiry.EvProve, stmtexpiry.EvClose}, string(stmtexpiry.DriftClosed), false},
		{"open-close-fail", []string{stmtexpiry.EvClose}, "", true},
		{"open-prove-prove-fail", []string{stmtexpiry.EvProve, stmtexpiry.EvProve}, "", true},
	}
	m := stmtexpiry.DriftFSM()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fsm.Walk(m, string(stmtexpiry.DriftOpen), tt.events...)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Walk should have failed")
				}
				return
			}
			if err != nil {
				t.Fatalf("Walk error = %v", err)
			}
			if got != tt.wantEnd {
				t.Errorf("Walk = %q, want %q", got, tt.wantEnd)
			}
		})
	}
}

func TestValidateLeaseTransition(t *testing.T) {
	tests := []struct {
		name	string
		from	stmtexpiry.LeaseState
		ev	string
		wantTo	stmtexpiry.LeaseState
		wantErr	bool
	}{
		{"built-issue", stmtexpiry.LeaseBuilt, stmtexpiry.EvIssue, stmtexpiry.LeaseIssued, false},
		{"built-revoke", stmtexpiry.LeaseBuilt, stmtexpiry.EvRevoke, stmtexpiry.LeaseRevoked, false},
		{"issued-expire", stmtexpiry.LeaseIssued, stmtexpiry.EvExpire, stmtexpiry.LeaseExpired, false},
		{"issued-revoke", stmtexpiry.LeaseIssued, stmtexpiry.EvRevoke, stmtexpiry.LeaseRevoked, false},
		{"expired-issue", stmtexpiry.LeaseExpired, stmtexpiry.EvIssue, stmtexpiry.LeaseExpired, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := stmtexpiry.ValidateLeaseTransition(tt.from, tt.ev)
			if tt.wantErr {
				if err == nil {
					t.Error("should error")
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if got != tt.wantTo {
				t.Errorf("got %q, want %q", got, tt.wantTo)
			}
		})
	}
}

func TestValidateDriftTransition(t *testing.T) {
	tests := []struct {
		name	string
		from	stmtexpiry.DriftState
		ev	string
		wantTo	stmtexpiry.DriftState
		wantErr	bool
	}{
		{"open-prove", stmtexpiry.DriftOpen, stmtexpiry.EvProve, stmtexpiry.DriftProven, false},
		{"proven-close", stmtexpiry.DriftProven, stmtexpiry.EvClose, stmtexpiry.DriftClosed, false},
		{"open-close", stmtexpiry.DriftOpen, stmtexpiry.EvClose, stmtexpiry.DriftOpen, true},
		{"closed-prove", stmtexpiry.DriftClosed, stmtexpiry.EvProve, stmtexpiry.DriftClosed, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := stmtexpiry.ValidateDriftTransition(tt.from, tt.ev)
			if tt.wantErr {
				if err == nil {
					t.Error("should error")
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if got != tt.wantTo {
				t.Errorf("got %q, want %q", got, tt.wantTo)
			}
		})
	}
}
