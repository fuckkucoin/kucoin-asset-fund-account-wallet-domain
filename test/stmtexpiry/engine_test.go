package stmtexpiry_test

import (
	"context"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/stmt"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/stmtexpiry"
)

func makeBills(specs ...struct {
	mid,
	book, asset, debit, credit	string
	seq	int
}) []stmt.Line {
	lines := make([]stmt.Line, 0, len(specs))
	for _, s := range specs {
		b := ledger.Bill{
			MovementID:	s.mid,
			UserID:		"test-user",
			Book:		s.book,
			Subject:	"available",
			Asset:		s.asset,
			Debit:		s.debit,
			Credit:		s.credit,
			Seq:		s.seq,
		}
		lines = append(lines, stmt.Line{Bill: b})
	}
	return lines
}

func testBills() []stmt.Line {
	return makeBills(
		struct {
			mid, book, asset, debit, credit	string
			seq				int
		}{"MV-1", "funding", "USDT", "0", "1000", 1},
		struct {
			mid, book, asset, debit, credit	string
			seq				int
		}{"MV-2", "funding", "USDT", "200", "0", 2},
		struct {
			mid, book, asset, debit, credit	string
			seq				int
		}{"MV-3", "funding", "BTC", "0", "0.5", 3},
	)
}

func testEngine() (*stmtexpiry.Engine, *stmtexpiry.MemStore,) {
	store := stmtexpiry.NewMemStore()
	log := zerolog.Nop()
	eng := stmtexpiry.NewEngine(store, log, 3600)
	return eng, store
}

func TestEngine_BuildAndIssueExport(t *testing.T) {
	tests := []struct {
		name	string
		req	stmtexpiry.BuildExportReq
		wantErr	bool
		errMsg	string
	}{
		{
			name:	"valid-json-export",
			req: stmtexpiry.BuildExportReq{
				UserID:		"user-1",
				Format:		stmt.FormatJSON,
				Bills:		testBills(),
				WindowStart:	time.Now().UTC().Add(-24 * time.Hour),
				WindowEnd:	time.Now().UTC(),
				TTLSec:		3600,
			},
			wantErr:	false,
		},
		{
			name:	"valid-csv-export",
			req: stmtexpiry.BuildExportReq{
				UserID:		"user-2",
				Format:		stmt.FormatCSV,
				Bills:		testBills(),
				WindowStart:	time.Now().UTC().Add(-24 * time.Hour),
				WindowEnd:	time.Now().UTC(),
				TTLSec:		7200,
			},
			wantErr:	false,
		},
		{
			name:	"empty-user-rejected",
			req: stmtexpiry.BuildExportReq{
				UserID:	"",
				Bills:	testBills(),
			},
			wantErr:	true,
			errMsg:		"user_id required",
		},
		{
			name:	"no-bills-rejected",
			req: stmtexpiry.BuildExportReq{
				UserID:	"user-3",
				Bills:	nil,
			},
			wantErr:	true,
			errMsg:		"no bills",
		},
		{
			name:	"whitespace-user-rejected",
			req: stmtexpiry.BuildExportReq{
				UserID:	"   ",
				Bills:	testBills(),
			},
			wantErr:	true,
			errMsg:		"user_id required",
		},
		{
			name:	"default-format-json",
			req: stmtexpiry.BuildExportReq{
				UserID:		"user-4",
				Format:		"",
				Bills:		testBills(),
				WindowStart:	time.Now().UTC().Add(-time.Hour),
				WindowEnd:	time.Now().UTC(),
			},
			wantErr:	false,
		},
		{
			name:	"custom-ttl",
			req: stmtexpiry.BuildExportReq{
				UserID:		"user-5",
				Format:		stmt.FormatJSON,
				Bills:		testBills(),
				WindowStart:	time.Now().UTC().Add(-time.Hour),
				WindowEnd:	time.Now().UTC(),
				TTLSec:		120,
			},
			wantErr:	false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng, _ := testEngine()
			ctx := context.Background()
			result, err := eng.BuildAndIssueExport(ctx, tt.req)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error %q should contain %q", err, tt.errMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Lease.State != stmtexpiry.LeaseIssued {
				t.Errorf("lease state = %q, want issued", result.Lease.State)
			}
			if result.Lease.Root == "" {
				t.Error("root should not be empty")
			}
			if result.Proof.ID == "" {
				t.Error("proof ID should not be empty")
			}
			if result.Proof.Digest == "" {
				t.Error("proof digest should not be empty")
			}
			if result.Lease.ExpiresAt.IsZero() {
				t.Error("expires_at should be set")
			}
			if err := result.Proof.Verify(); err != nil {
				t.Errorf("proof should verify: %v", err)
			}
		})
	}
}

func TestEngine_BuildAndIssueExport_Idempotent(t *testing.T) {
	eng, _ := testEngine()
	ctx := context.Background()
	req := stmtexpiry.BuildExportReq{
		UserID:		"user-idem",
		Format:		stmt.FormatJSON,
		Bills:		testBills(),
		WindowStart:	time.Now().UTC().Add(-time.Hour),
		WindowEnd:	time.Now().UTC(),
		TTLSec:		3600,
	}
	r1, err := eng.BuildAndIssueExport(ctx, req)
	if err != nil {
		t.Fatalf("first issue: %v", err)
	}
	r2, err := eng.BuildAndIssueExport(ctx, req)
	if err != nil {
		t.Fatalf("second issue: %v", err)
	}
	if r1.Lease.ID == r2.Lease.ID {
		t.Error("different issues should have different lease IDs")
	}
	if r1.Lease.Root != r2.Lease.Root {
		t.Error("same bills should produce same root")
	}
}

func TestEngine_DetectRootDrift(t *testing.T) {
	tests := []struct {
		name		string
		newRoot		string
		sameRoot	bool
		wantDrift	bool
		wantRevoke	bool
	}{
		{
			name:		"drift-detected",
			newRoot:	"new-root-abc",
			sameRoot:	false,
			wantDrift:	true,
			wantRevoke:	true,
		},
		{
			name:		"no-drift-same-root",
			sameRoot:	true,
			wantDrift:	false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng, store := testEngine()
			ctx := context.Background()
			result, err := eng.BuildAndIssueExport(ctx, stmtexpiry.BuildExportReq{
				UserID:	"user-drift",
				Format:	stmt.FormatJSON,
				Bills:	testBills(),
				TTLSec:	3600,
			})
			if err != nil {
				t.Fatalf("issue: %v", err)
			}
			currentRoot := tt.newRoot
			if tt.sameRoot {
				currentRoot = result.Lease.Root
			}
			dc, err := eng.DetectRootDrift(ctx, stmtexpiry.DetectDriftReq{
				LeaseID:	result.Lease.ID,
				CurrentRoot:	currentRoot,
				NewHighWater:	result.Lease.HighWater + 1,
				NewBillSeqs:	[]int{result.Lease.HighWater + 1},
			})
			if err != nil {
				t.Fatalf("detect: %v", err)
			}
			if tt.wantDrift {
				if dc == nil {
					t.Fatal("expected drift case")
				}
				if dc.OriginalRoot != result.Lease.Root {
					t.Errorf("original root mismatch")
				}
				if dc.CurrentRoot != tt.newRoot {
					t.Errorf("current root mismatch")
				}
				if dc.State != stmtexpiry.DriftProven {
					t.Errorf("drift state = %q, want proven", dc.State)
				}
				lease, _ := store.GetLease(ctx, result.Lease.ID)
				if lease.State != stmtexpiry.LeaseRevoked {
					t.Errorf("lease should be revoked, got %q", lease.State)
				}
			} else {
				if dc != nil {
					t.Error("expected no drift case")
				}
			}
		})
	}
}

func TestEngine_DetectRootDrift_TerminalLease(t *testing.T) {
	eng, store := testEngine()
	ctx := context.Background()
	result, err := eng.BuildAndIssueExport(ctx, stmtexpiry.BuildExportReq{
		UserID:	"user-term",
		Format:	stmt.FormatJSON,
		Bills:	testBills(),
		TTLSec:	3600,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if err := store.SetLeaseRevoked(ctx, result.Lease.ID, "test"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	dc, err := eng.DetectRootDrift(ctx, stmtexpiry.DetectDriftReq{
		LeaseID:	result.Lease.ID,
		CurrentRoot:	"new-root",
	})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if dc != nil {
		t.Error("should not detect drift on terminal lease")
	}
}

func TestEngine_ConfirmDrift(t *testing.T) {
	eng, store := testEngine()
	ctx := context.Background()
	result, err := eng.BuildAndIssueExport(ctx, stmtexpiry.BuildExportReq{
		UserID:	"user-confirm",
		Format:	stmt.FormatJSON,
		Bills:	testBills(),
		TTLSec:	3600,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	dc,
		err := eng.DetectRootDrift(ctx, stmtexpiry.DetectDriftReq{
		LeaseID:	result.Lease.ID,
		CurrentRoot:	"different-root",
	})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if err := eng.ConfirmDrift(ctx, dc.ID, "verified by ops"); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	d, _ := store.GetDrift(ctx, dc.ID)
	if d.State != stmtexpiry.DriftClosed {
		t.Errorf("drift state = %q, want closed", d.State)
	}
}

func TestEngine_RevokeExport(t *testing.T) {
	tests := []struct {
		name		string
		preRevoke	bool
		wantErr		bool
	}{
		{"revoke-active", false, false},
		{"revoke-already-revoked", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng, store := testEngine()
			ctx := context.Background()
			result, err := eng.BuildAndIssueExport(ctx, stmtexpiry.BuildExportReq{
				UserID:	"user-rev",
				Format:	stmt.FormatJSON,
				Bills:	testBills(),
				TTLSec:	3600,
			})
			if err != nil {
				t.Fatalf("issue: %v", err)
			}
			if tt.preRevoke {
				_ = store.SetLeaseRevoked(ctx, result.Lease.ID, "pre-test")
			}
			err = eng.RevokeExport(ctx, result.Lease.ID, "test reason", "admin")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("revoke: %v", err)
			}
			lease, _ := store.GetLease(ctx, result.Lease.ID)
			if lease.State != stmtexpiry.LeaseRevoked {
				t.Errorf("state = %q, want revoked", lease.State)
			}
			revs, _ := store.ListRevocationsByLease(ctx, result.Lease.ID)
			if len(revs) == 0 {
				t.Error("should have revocation record")
			}
		})
	}
}

func TestEngine_VerifyExport(t *testing.T) {
	tests := []struct {
		name	string
		setup	func(eng *stmtexpiry.Engine, store *stmtexpiry.MemStore, leaseID string)
		wantErr	bool
	}{
		{
			name:		"valid-issued",
			setup:		func(eng *stmtexpiry.Engine, store *stmtexpiry.MemStore, leaseID string) {},
			wantErr:	false,
		},
		{
			name:	"revoked-rejected",
			setup: func(eng *stmtexpiry.Engine, store *stmtexpiry.MemStore, leaseID string) {
				_ = store.SetLeaseRevoked(context.Background(), leaseID, "test")
			},
			wantErr:	true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng, store := testEngine()
			ctx := context.Background()
			result, err := eng.BuildAndIssueExport(ctx, stmtexpiry.BuildExportReq{
				UserID:	"user-verify",
				Format:	stmt.FormatJSON,
				Bills:	testBills(),
				TTLSec:	3600,
			})
			if err != nil {
				t.Fatalf("issue: %v", err)
			}
			tt.setup(eng, store, result.Lease.ID)
			err = eng.VerifyExport(ctx, result.Lease.ID)
			if (err != nil) != tt.wantErr {
				t.Errorf("VerifyExport() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEngine_GetExportProof(t *testing.T) {
	eng, _ := testEngine()
	ctx := context.Background()
	result, err := eng.BuildAndIssueExport(ctx, stmtexpiry.BuildExportReq{
		UserID:	"user-proof",
		Format:	stmt.FormatJSON,
		Bills:	testBills(),
		TTLSec:	3600,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	proof,
		err := eng.GetExportProof(ctx, result.Lease.ID)
	if err != nil {
		t.Fatalf("get proof: %v", err)
	}
	if proof.ID != result.Proof.ID {
		t.Errorf("proof ID mismatch: got %q, want %q", proof.ID, result.Proof.ID)
	}
	if proof.Root != result.Lease.Root {
		t.Errorf("root mismatch")
	}
}

func TestEngine_HighWater(t *testing.T) {
	tests := []struct {
		name	string
		seqs	[]int
		wantHW	int
	}{
		{"single-seq", []int{1}, 1},
		{"ascending", []int{1, 2, 3, 4, 5}, 5},
		{"out-of-order", []int{3, 1, 5, 2, 4}, 5},
		{"duplicates", []int{1, 1, 3, 3}, 3},
		{"large-gap", []int{1, 100}, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bills []stmt.Line
			for _, seq := range tt.seqs {
				b := ledger.Bill{
					MovementID:	"MV",
					UserID:		"u",
					Book:		"funding",
					Subject:	"available",
					Asset:		"USDT",
					Credit:		"100",
					Seq:		seq,
				}
				bills = append(bills, stmt.Line{Bill: b})
			}
			got := stmtexpiry.MaxSeq(bills)
			if got != tt.wantHW {
				t.Errorf("maxSeq = %d, want %d", got, tt.wantHW)
			}
		})
	}
}
