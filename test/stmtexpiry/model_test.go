package stmtexpiry_test

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/stmtexpiry"
)

func TestLeaseState_Terminal(t *testing.T) {
	tests := []struct {
		name		string
		state		stmtexpiry.LeaseState
		terminal	bool
	}{
		{"built-not-terminal", stmtexpiry.LeaseBuilt, false},
		{"issued-not-terminal", stmtexpiry.LeaseIssued, false},
		{"expired-terminal", stmtexpiry.LeaseExpired, true},
		{"revoked-terminal", stmtexpiry.LeaseRevoked, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.Terminal(); got != tt.terminal {
				t.Errorf("LeaseState(%q).Terminal() = %v, want %v", tt.state, got, tt.terminal)
			}
		})
	}
}

func TestLeaseState_Valid(t *testing.T) {
	tests := []struct {
		name	string
		state	stmtexpiry.LeaseState
		valid	bool
	}{
		{"built", stmtexpiry.LeaseBuilt, true},
		{"issued", stmtexpiry.LeaseIssued, true},
		{"expired", stmtexpiry.LeaseExpired, true},
		{"revoked", stmtexpiry.LeaseRevoked, true},
		{"unknown", stmtexpiry.LeaseState("unknown"), false},
		{"empty", stmtexpiry.LeaseState(""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.Valid(); got != tt.valid {
				t.Errorf("LeaseState(%q).Valid() = %v, want %v", tt.state, got, tt.valid)
			}
		})
	}
}

func TestDriftState_Terminal(t *testing.T) {
	tests := []struct {
		name		string
		state		stmtexpiry.DriftState
		terminal	bool
	}{
		{"open-not-terminal", stmtexpiry.DriftOpen, false},
		{"proven-not-terminal", stmtexpiry.DriftProven, false},
		{"closed-terminal", stmtexpiry.DriftClosed, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.Terminal(); got != tt.terminal {
				t.Errorf("DriftState(%q).Terminal() = %v, want %v", tt.state, got, tt.terminal)
			}
		})
	}
}

func TestNewExportLease(t *testing.T) {
	now := time.Now().UTC()
	wStart := now.Add(-24 * time.Hour)
	lease := stmtexpiry.NewExportLease("user1", "json", "root-abc", 10, 3600, wStart, now)
	if lease.UserID != "user1" {
		t.Errorf("UserID = %q, want user1", lease.UserID)
	}
	if lease.State != stmtexpiry.LeaseBuilt {
		t.Errorf("State = %q, want built", lease.State)
	}
	if lease.HighWater != 10 {
		t.Errorf("HighWater = %d, want 10", lease.HighWater)
	}
	if lease.Root != "root-abc" {
		t.Errorf("Root = %q, want root-abc", lease.Root)
	}
	if lease.TTLSec != 3600 {
		t.Errorf("TTLSec = %d, want 3600", lease.TTLSec)
	}
	if lease.ID == "" {
		t.Error("ID should not be empty")
	}
}

func TestExportLease_Issue(t *testing.T) {
	tests := []struct {
		name	string
		initial	stmtexpiry.LeaseState
		proofID	string
		wantErr	bool
	}{
		{"issue-from-built", stmtexpiry.LeaseBuilt, "proof-1", false},
		{"issue-from-issued", stmtexpiry.LeaseIssued, "proof-2", true},
		{"issue-from-expired", stmtexpiry.LeaseExpired, "proof-3", true},
		{"issue-from-revoked", stmtexpiry.LeaseRevoked, "proof-4", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lease := stmtexpiry.NewExportLease("u", "json", "r", 1, 60, time.Time{}, time.Time{})
			lease.State = tt.initial
			err := lease.Issue(tt.proofID)
			if (err != nil) != tt.wantErr {
				t.Errorf("Issue() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				if lease.State != stmtexpiry.LeaseIssued {
					t.Errorf("State after issue = %q, want issued", lease.State)
				}
				if lease.ProofID != tt.proofID {
					t.Errorf("ProofID = %q, want %q", lease.ProofID, tt.proofID)
				}
				if lease.ExpiresAt.IsZero() {
					t.Error("ExpiresAt should be set")
				}
			}
		})
	}
}

func TestExportLease_Expire(t *testing.T) {
	tests := []struct {
		name	string
		initial	stmtexpiry.LeaseState
		wantErr	bool
	}{
		{"expire-from-issued", stmtexpiry.LeaseIssued, false},
		{"expire-from-built", stmtexpiry.LeaseBuilt, true},
		{"expire-from-expired", stmtexpiry.LeaseExpired, true},
		{"expire-from-revoked", stmtexpiry.LeaseRevoked, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lease := stmtexpiry.NewExportLease("u", "json", "r", 1, 60, time.Time{}, time.Time{})
			lease.State = tt.initial
			err := lease.Expire()
			if (err != nil) != tt.wantErr {
				t.Errorf("Expire() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExportLease_Revoke(t *testing.T) {
	tests := []struct {
		name	string
		initial	stmtexpiry.LeaseState
		wantErr	bool
	}{
		{"revoke-from-built", stmtexpiry.LeaseBuilt, false},
		{"revoke-from-issued", stmtexpiry.LeaseIssued, false},
		{"revoke-from-expired", stmtexpiry.LeaseExpired, true},
		{"revoke-from-revoked", stmtexpiry.LeaseRevoked, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lease := stmtexpiry.NewExportLease("u", "json", "r", 1, 60, time.Time{}, time.Time{})
			lease.State = tt.initial
			err := lease.Revoke("test")
			if (err != nil) != tt.wantErr {
				t.Errorf("Revoke() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExportLease_IsExpired(t *testing.T) {
	tests := []struct {
		name		string
		state		stmtexpiry.LeaseState
		expiresAt	time.Time
		want		bool
	}{
		{"issued-not-expired", stmtexpiry.LeaseIssued, time.Now().UTC().Add(time.Hour), false},
		{"issued-expired", stmtexpiry.LeaseIssued, time.Now().UTC().Add(-time.Hour), true},
		{"built-not-checked", stmtexpiry.LeaseBuilt, time.Now().UTC().Add(-time.Hour), false},
		{"revoked-not-checked", stmtexpiry.LeaseRevoked, time.Now().UTC().Add(-time.Hour), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lease := stmtexpiry.NewExportLease("u", "json", "r", 1, 60, time.Time{}, time.Time{})
			lease.State = tt.state
			lease.ExpiresAt = tt.expiresAt
			if got := lease.IsExpired(); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewRootDriftCase(t *testing.T) {
	dc := stmtexpiry.NewRootDriftCase("lease1", "user1", "root-old", "root-new", 5, 8, []int{6, 7, 8})
	if dc.LeaseID != "lease1" {
		t.Errorf("LeaseID = %q, want lease1", dc.LeaseID)
	}
	if dc.State != stmtexpiry.DriftOpen {
		t.Errorf("State = %q, want open", dc.State)
	}
	if dc.OriginalRoot != "root-old" {
		t.Errorf("OriginalRoot = %q", dc.OriginalRoot)
	}
	if dc.CurrentRoot != "root-new" {
		t.Errorf("CurrentRoot = %q", dc.CurrentRoot)
	}
	if len(dc.DriftBillSeqs) != 3 {
		t.Errorf("DriftBillSeqs len = %d, want 3", len(dc.DriftBillSeqs))
	}
}

func TestRootDriftCase_Prove(t *testing.T) {
	tests := []struct {
		name	string
		initial	stmtexpiry.DriftState
		wantErr	bool
	}{
		{"prove-from-open", stmtexpiry.DriftOpen, false},
		{"prove-from-proven", stmtexpiry.DriftProven, true},
		{"prove-from-closed", stmtexpiry.DriftClosed, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc := stmtexpiry.NewRootDriftCase("l", "u", "a", "b", 1, 2, nil)
			dc.State = tt.initial
			err := dc.Prove()
			if (err != nil) != tt.wantErr {
				t.Errorf("Prove() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRootDriftCase_Close(t *testing.T) {
	tests := []struct {
		name	string
		initial	stmtexpiry.DriftState
		wantErr	bool
	}{
		{"close-from-proven", stmtexpiry.DriftProven, false},
		{"close-from-open", stmtexpiry.DriftOpen, true},
		{"close-from-closed", stmtexpiry.DriftClosed, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc := stmtexpiry.NewRootDriftCase("l", "u", "a", "b", 1, 2, nil)
			dc.State = tt.initial
			err := dc.Close("resolved")
			if (err != nil) != tt.wantErr {
				t.Errorf("Close() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIssueProof_Verify(t *testing.T) {
	now := time.Now().UTC()
	wStart := now.Add(-time.Hour)
	wEnd := now
	proof := stmtexpiry.NewIssueProof("lease1", "user1", "root-abc", 3600, wStart, wEnd)
	if err := proof.Verify(); err != nil {
		t.Errorf("valid proof should verify: %v", err)
	}
	tampered := proof
	tampered.Digest = "tampered"
	if err := tampered.Verify(); err == nil {
		t.Error("tampered proof should fail verify")
	}
	empty := proof
	empty.Digest = ""
	if err := empty.Verify(); err == nil {
		t.Error("empty digest should fail verify")
	}
}

func TestIssueProof_Expired(t *testing.T) {
	tests := []struct {
		name		string
		expiresAt	time.Time
		want		bool
	}{
		{"not-expired", time.Now().UTC().Add(time.Hour), false},
		{"expired", time.Now().UTC().Add(-time.Hour), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := stmtexpiry.IssueProof{ExpiresAt: tt.expiresAt}
			if got := p.Expired(); got != tt.want {
				t.Errorf("Expired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVerifyExportIntegrity(t *testing.T) {
	now := time.Now().UTC()
	wStart := now.Add(-time.Hour)
	wEnd := now
	validProof := stmtexpiry.NewIssueProof("lease1", "user1", "root", 3600, wStart, wEnd)
	tests := []struct {
		name	string
		lease	stmtexpiry.ExportLease
		proof	*stmtexpiry.IssueProof
		wantErr	bool
	}{
		{
			name:	"valid-issued-with-proof",
			lease: stmtexpiry.ExportLease{
				ID:	"l1", State: stmtexpiry.LeaseIssued,
				ExpiresAt:	now.Add(time.Hour),
			},
			proof:		&validProof,
			wantErr:	false,
		},
		{
			name:	"terminal-expired",
			lease: stmtexpiry.ExportLease{
				ID:	"l2", State: stmtexpiry.LeaseExpired,
			},
			proof:		&validProof,
			wantErr:	true,
		},
		{
			name:	"terminal-revoked",
			lease: stmtexpiry.ExportLease{
				ID:	"l3", State: stmtexpiry.LeaseRevoked,
			},
			proof:		&validProof,
			wantErr:	true,
		},
		{
			name:	"not-issued-built",
			lease: stmtexpiry.ExportLease{
				ID:	"l4", State: stmtexpiry.LeaseBuilt,
			},
			proof:		&validProof,
			wantErr:	true,
		},
		{
			name:	"issued-but-expired-time",
			lease: stmtexpiry.ExportLease{
				ID:	"l5", State: stmtexpiry.LeaseIssued,
				ExpiresAt:	now.Add(-time.Hour),
			},
			proof:		&validProof,
			wantErr:	true,
		},
		{
			name:	"issued-nil-proof",
			lease: stmtexpiry.ExportLease{
				ID:	"l6", State: stmtexpiry.LeaseIssued,
				ExpiresAt:	now.Add(time.Hour),
			},
			proof:		nil,
			wantErr:	true,
		},
		{
			name:	"issued-tampered-proof",
			lease: stmtexpiry.ExportLease{
				ID:	"l7", State: stmtexpiry.LeaseIssued,
				ExpiresAt:	now.Add(time.Hour),
			},
			proof: func() *stmtexpiry.IssueProof {
				p := validProof
				p.Digest = "bad"
				return &p
			}(),
			wantErr:	true,
		},
		{
			name:	"issued-proof-expired",
			lease: stmtexpiry.ExportLease{
				ID:	"l8", State: stmtexpiry.LeaseIssued,
				ExpiresAt:	now.Add(time.Hour),
			},
			proof: func() *stmtexpiry.IssueProof {
				p := validProof
				p.ExpiresAt = now.Add(-time.Hour)
				return &p
			}(),
			wantErr:	true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := stmtexpiry.VerifyExportIntegrity(tt.lease, tt.proof)
			if (err != nil) != tt.wantErr {
				t.Errorf("VerifyExportIntegrity() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewRevocation(t *testing.T) {
	rev := stmtexpiry.NewRevocation("lease1", "user1", stmtexpiry.RevokeTTLExpiry, "expired", "worker")
	if rev.LeaseID != "lease1" {
		t.Errorf("LeaseID = %q", rev.LeaseID)
	}
	if rev.Kind != stmtexpiry.RevokeTTLExpiry {
		t.Errorf("Kind = %q", rev.Kind)
	}
	if rev.ID == "" {
		t.Error("ID should not be empty")
	}
}
