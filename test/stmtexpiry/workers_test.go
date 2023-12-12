package stmtexpiry_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/stmt"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/stmtexpiry"
)

// mockRecomputer Root 重算。

type mockRecomputer struct {
	root	string
	newHW	int
	seqs	[]int
	err	error
}

func (m *mockRecomputer) RecomputeRoot(_ context.Context, _ string, _ int) (string, int, []int, error) {
	return m.root, m.newHW, m.seqs, m.err
}

func makeExpiredLease(store *stmtexpiry.MemStore, userID string, ttlSec int) stmtexpiry.ExportLease {
	ctx := context.Background()
	bills := testBills()
	rep := stmt.Report{UserID: userID, Lines: bills, Count: len(bills)}
	exp, _ := stmt.ExportReport(rep, stmt.FormatJSON)
	man, _ := stmt.BuildManifest(userID, []stmt.Export{exp})
	now := time.Now().UTC()
	lease := stmtexpiry.NewExportLease(userID, "json", man.Root, 3, ttlSec, now.Add(-2*time.Hour), now.Add(-time.Hour))
	past := now.Add(-time.Duration(ttlSec+10) * time.Second)
	lease.State = stmtexpiry.LeaseIssued
	lease.IssuedAt = past
	lease.ExpiresAt = past.Add(time.Duration(ttlSec) * time.Second)
	proof := stmtexpiry.NewIssueProof(lease.ID, userID, man.Root, ttlSec, lease.WindowStart, lease.WindowEnd)
	proof.IssuedAt = past
	proof.ExpiresAt = lease.ExpiresAt
	lease.ProofID = proof.ID
	_ = store.InsertLease(ctx, lease)
	_ = store.InsertProof(ctx, proof)
	return lease
}

func TestTTLScanWorker_Tick(t *testing.T) {
	tests := []struct {
		name		string
		expiredCount	int
		ttlSec		int
		wantRevoked	int
	}{
		{"no-expired-leases", 0, 3600, 0},
		{"one-expired", 1, 1, 1},
		{"three-expired", 3, 1, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := stmtexpiry.NewMemStore()
			for i := 0; i < tt.expiredCount; i++ {
				makeExpiredLease(store, fmt.Sprintf("user-ttl-%d", i), tt.ttlSec)
			}
			w := stmtexpiry.NewTTLScanWorker(store, zerolog.Nop(), 100)
			ctx := context.Background()
			revoked, err := w.Tick(ctx)
			if err != nil {
				t.Fatalf("Tick error: %v", err)
			}
			if revoked != tt.wantRevoked {
				t.Errorf("revoked = %d, want %d", revoked, tt.wantRevoked)
			}
			for _, lease := range store.leases {
				if lease.State == stmtexpiry.LeaseIssued && !lease.ExpiresAt.IsZero() && lease.ExpiresAt.Before(time.Now().UTC()) {
					t.Errorf("lease %s should have been revoked", lease.ID)
				}
			}
		})
	}
}

func TestTTLScanWorker_Job(t *testing.T) {
	store := stmtexpiry.NewMemStore()
	makeExpiredLease(store, "user-job", 1)
	w := stmtexpiry.NewTTLScanWorker(store, zerolog.Nop(), 100)
	if err := w.Job(context.Background()); err != nil {
		t.Fatalf("Job error: %v", err)
	}
}

func TestDriftPatrolWorker_Tick(t *testing.T) {
	tests := []struct {
		name		string
		rootDrifts	bool
		wantDrifts	int
	}{
		{"no-drift", false, 0},
		{"drift-detected", true, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := stmtexpiry.NewMemStore()
			ctx := context.Background()
			bills := testBills()
			rep := stmt.Report{UserID: "user-dp", Lines: bills, Count: len(bills)}
			exp, _ := stmt.ExportReport(rep, stmt.FormatJSON)
			man, _ := stmt.BuildManifest("user-dp", []stmt.Export{exp})
			lease := stmtexpiry.NewExportLease("user-dp", "json", man.Root, 3, 3600, time.Time{}, time.Time{})
			lease.State = stmtexpiry.LeaseIssued
			lease.IssuedAt = time.Now().UTC()
			lease.ExpiresAt = time.Now().UTC().Add(time.Hour)
			proof := stmtexpiry.NewIssueProof(lease.ID, "user-dp", man.Root, 3600, time.Time{}, time.Time{})
			lease.ProofID = proof.ID
			_ = store.InsertLease(ctx, lease)
			_ = store.InsertProof(ctx, proof)
			rc := &mockRecomputer{root: man.Root,
				newHW:	3, seqs: nil}
			if tt.rootDrifts {
				rc.root = "drifted-root"
				rc.newHW = 5
				rc.seqs = []int{4, 5}
			}
			w := stmtexpiry.NewDriftPatrolWorker(store, rc, zerolog.Nop(), 50)
			drifts, err := w.Tick(ctx)
			if err != nil {
				t.Fatalf("Tick error: %v", err)
			}
			if drifts != tt.wantDrifts {
				t.Errorf("drifts = %d, want %d", drifts, tt.wantDrifts)
			}
			if tt.rootDrifts {
				l, _ := store.GetLease(ctx, lease.ID)
				if l.State != stmtexpiry.LeaseRevoked {
					t.Errorf("lease state = %q, want revoked", l.State)
				}
			}
		})
	}
}

func TestDriftPatrolWorker_SkipsExpired(t *testing.T) {
	store := stmtexpiry.NewMemStore()
	ctx := context.Background()
	bills := testBills()
	rep := stmt.Report{UserID: "user-skip", Lines: bills, Count: len(bills)}
	exp, _ := stmt.ExportReport(rep, stmt.FormatJSON)
	man, _ := stmt.BuildManifest("user-skip", []stmt.Export{exp})
	lease := stmtexpiry.NewExportLease("user-skip", "json", man.Root, 3, 1, time.Time{}, time.Time{})
	lease.State = stmtexpiry.LeaseIssued
	lease.IssuedAt = time.Now().UTC().Add(-2 * time.Hour)
	lease.ExpiresAt = time.Now().UTC().Add(-time.Hour)
	_ = store.InsertLease(ctx, lease)
	rc := &mockRecomputer{root: "drifted"}
	w := stmtexpiry.NewDriftPatrolWorker(store, rc, zerolog.Nop(), 50)
	drifts, err := w.Tick(ctx)
	if err != nil {
		t.Fatalf("Tick error: %v", err)
	}
	if drifts != 0 {
		t.Errorf("should skip expired leases, got %d drifts", drifts)
	}
}

func TestDriftPatrolWorker_RecomputeError(t *testing.T) {
	store := stmtexpiry.NewMemStore()
	ctx := context.Background()
	bills := testBills()
	rep := stmt.Report{UserID: "user-err", Lines: bills, Count: len(bills)}
	exp, _ := stmt.ExportReport(rep, stmt.FormatJSON)
	man, _ := stmt.BuildManifest("user-err", []stmt.Export{exp})
	lease := stmtexpiry.NewExportLease("user-err", "json", man.Root, 3, 3600, time.Time{}, time.Time{})
	lease.State = stmtexpiry.LeaseIssued
	lease.IssuedAt = time.Now().UTC()
	lease.ExpiresAt = time.Now().UTC().Add(time.Hour)
	_ = store.InsertLease(ctx, lease)
	rc := &mockRecomputer{err: fmt.Errorf("db connection failed")}
	w := stmtexpiry.NewDriftPatrolWorker(store, rc, zerolog.Nop(), 50)
	drifts, err := w.Tick(ctx)
	if err != nil {
		t.Fatalf("Tick error: %v", err)
	}
	if drifts != 0 {
		t.Errorf("should be 0 drifts on error, got %d", drifts)
	}
	l,
		_ := store.GetLease(ctx, lease.ID)
	if l.State != stmtexpiry.LeaseIssued {
		t.Errorf("lease should remain issued on recompute error")
	}
}

func TestProofSealWorker_Tick(t *testing.T) {
	tests := []struct {
		name		string
		hasProof	bool
		validProof	bool
		wantSealed	int
	}{
		{"valid-proof", true, true, 1},
		{"missing-proof", false, false, 0},
		{"invalid-proof", true, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := stmtexpiry.NewMemStore()
			ctx := context.Background()
			bills := testBills()
			rep := stmt.Report{UserID: "user-seal", Lines: bills, Count: len(bills)}
			exp, _ := stmt.ExportReport(rep, stmt.FormatJSON)
			man, _ := stmt.BuildManifest("user-seal", []stmt.Export{exp})
			lease := stmtexpiry.NewExportLease("user-seal", "json", man.Root, 3, 3600, time.Time{}, time.Time{})
			lease.State = stmtexpiry.LeaseIssued
			lease.IssuedAt = time.Now().UTC()
			lease.ExpiresAt = time.Now().UTC().Add(time.Hour)
			if tt.hasProof {
				proof := stmtexpiry.NewIssueProof(lease.ID, "user-seal", man.Root, 3600, time.Time{}, time.Time{})
				if !tt.validProof {
					proof.Digest = "tampered"
				}
				lease.ProofID = proof.ID
				_ = store.InsertProof(ctx, proof)
			}
			_ = store.InsertLease(ctx, lease)
			w := stmtexpiry.NewProofSealWorker(store, zerolog.Nop(), 100)
			sealed, err := w.Tick(ctx)
			if err != nil {
				t.Fatalf("Tick error: %v", err)
			}
			if sealed != tt.wantSealed {
				t.Errorf("sealed = %d, want %d", sealed, tt.wantSealed)
			}
			if !tt.hasProof || !tt.validProof {
				l, _ := store.GetLease(ctx, lease.ID)
				if l.State != stmtexpiry.LeaseRevoked {
					t.Errorf("lease should be revoked, got %q", l.State)
				}
			}
		})
	}
}

func TestProofSealWorker_Job(t *testing.T) {
	store := stmtexpiry.NewMemStore()
	ctx := context.Background()
	bills := testBills()
	rep := stmt.Report{UserID: "user-job2", Lines: bills, Count: len(bills)}
	exp, _ := stmt.ExportReport(rep, stmt.FormatJSON)
	man, _ := stmt.BuildManifest("user-job2", []stmt.Export{exp})
	lease := stmtexpiry.NewExportLease("user-job2", "json", man.Root, 3, 3600, time.Time{}, time.Time{})
	lease.State = stmtexpiry.LeaseIssued
	lease.IssuedAt = time.Now().UTC()
	lease.ExpiresAt = time.Now().UTC().Add(time.Hour)
	proof := stmtexpiry.NewIssueProof(lease.ID, "user-job2", man.Root, 3600, time.Time{}, time.Time{})
	lease.ProofID = proof.ID
	_ = store.InsertLease(ctx, lease)
	_ = store.InsertProof(ctx, proof)
	w := stmtexpiry.NewProofSealWorker(store, zerolog.Nop(), 100)
	if err := w.Job(ctx); err != nil {
		t.Fatalf("Job error: %v", err)
	}
}

// ──────────────────────────── 综合场景 ────────────────────────────

func TestScenario_IssueDriftRevokeReissue(t *testing.T) {
	eng, store := testEngine()
	ctx := context.Background()
	userID := "user-scenario"
	r1, err := eng.BuildAndIssueExport(ctx, stmtexpiry.BuildExportReq{
		UserID:	userID,
		Format:	stmt.FormatJSON,
		Bills:	testBills(),
		TTLSec:	3600,
	})
	if err != nil {
		t.Fatalf("first issue: %v", err)
	}
	if err := eng.VerifyExport(ctx, r1.Lease.ID); err != nil {
		t.Fatalf("first verify: %v", err)
	}
	dc, err := eng.DetectRootDrift(ctx, stmtexpiry.DetectDriftReq{
		LeaseID:	r1.Lease.ID,
		CurrentRoot:	"drifted-root",
		NewHighWater:	4,
		NewBillSeqs:	[]int{4},
	})
	if err != nil {
		t.Fatalf("detect drift: %v", err)
	}
	if dc == nil {
		t.Fatal("expected drift case")
	}
	if err := eng.VerifyExport(ctx, r1.Lease.ID); err == nil {
		t.Fatal("verify should fail after drift")
	}
	l1, _ := store.GetLease(ctx, r1.Lease.ID)
	if l1.State != stmtexpiry.LeaseRevoked {
		t.Errorf("first lease state = %q, want revoked", l1.State)
	}
	newBills := append(testBills(), stmt.Line{
		Bill: ledger.Bill{
			MovementID:	"MV-4",
			UserID:		userID,
			Book:		"funding",
			Subject:	"available",
			Asset:		"USDT",
			Debit:		"100",
			Seq:		4,
		},
	})
	r2, err := eng.BuildAndIssueExport(ctx, stmtexpiry.BuildExportReq{
		UserID:	userID,
		Format:	stmt.FormatJSON,
		Bills:	newBills,
		TTLSec:	3600,
	})
	if err != nil {
		t.Fatalf("second issue: %v", err)
	}
	if err := eng.VerifyExport(ctx, r2.Lease.ID); err != nil {
		t.Fatalf("second verify: %v", err)
	}
	if r1.Lease.Root == r2.Lease.Root {
		t.Error("new export should have different root")
	}
	if err := eng.ConfirmDrift(ctx, dc.ID, "recomputed"); err != nil {
		t.Fatalf("confirm drift: %v", err)
	}
	d, _ := store.GetDrift(ctx, dc.ID)
	if d.State != stmtexpiry.DriftClosed {
		t.Errorf("drift state = %q, want closed", d.State)
	}
}

func TestScenario_TTLExpiryAndReissue(t *testing.T) {
	store := stmtexpiry.NewMemStore()
	eng := stmtexpiry.NewEngine(store, zerolog.Nop(), 1)
	ctx := context.Background()
	r, err := eng.BuildAndIssueExport(ctx, stmtexpiry.BuildExportReq{
		UserID:	"user-ttl-scenario",
		Format:	stmt.FormatJSON,
		Bills:	testBills(),
		TTLSec:	1,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	time.Sleep(2 * time.Second)
	w := stmtexpiry.NewTTLScanWorker(store, zerolog.Nop(), 100)
	revoked, err := w.Tick(ctx)
	if err != nil {
		t.Fatalf("ttl scan: %v", err)
	}
	if revoked != 1 {
		t.Errorf("revoked = %d, want 1", revoked)
	}
	if err := eng.VerifyExport(ctx, r.Lease.ID); err == nil {
		t.Fatal("verify should fail after TTL expiry")
	}
	r2, err := eng.BuildAndIssueExport(ctx, stmtexpiry.BuildExportReq{
		UserID:	"user-ttl-scenario",
		Format:	stmt.FormatJSON,
		Bills:	testBills(),
		TTLSec:	3600,
	})
	if err != nil {
		t.Fatalf("reissue: %v", err)
	}
	if err := eng.VerifyExport(ctx, r2.Lease.ID); err != nil {
		t.Fatalf("reissue verify: %v", err)
	}
}

func TestScenario_ConcurrentExportMutex(t *testing.T) {
	store := stmtexpiry.NewMemStore()
	eng := stmtexpiry.NewEngine(store, zerolog.Nop(), 3600)
	ctx := context.Background()
	store.mu.Lock()
	store.mutexes["export:user-mutex"] = "other-lease-id"
	store.mu.Unlock()
	_, err := eng.BuildAndIssueExport(ctx, stmtexpiry.BuildExportReq{
		UserID:	"user-mutex",
		Format:	stmt.FormatJSON,
		Bills:	testBills(),
		TTLSec:	3600,
	})
	if err == nil {
		t.Fatal("expected mutex rejection")
	}
	if !strings.Contains(err.Error(), "concurrent export") {
		t.Errorf("error should mention concurrent: %v", err)
	}
}

func TestScenario_ManualRevokeVerify(t *testing.T) {
	eng, store := testEngine()
	ctx := context.Background()
	r, err := eng.BuildAndIssueExport(ctx, stmtexpiry.BuildExportReq{
		UserID:	"user-manual",
		Format:	stmt.FormatJSON,
		Bills:	testBills(),
		TTLSec:	3600,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if err := eng.VerifyExport(ctx, r.Lease.ID); err != nil {
		t.Fatalf("pre-revoke verify: %v", err)
	}
	if err := eng.RevokeExport(ctx, r.Lease.ID, "compliance review", "ops_admin"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := eng.VerifyExport(ctx, r.Lease.ID); err == nil {
		t.Fatal("verify should fail after manual revoke")
	}
	revs, _ := store.ListRevocationsByLease(ctx, r.Lease.ID)
	if len(revs) == 0 {
		t.Error("should have revocation record")
	}
	if revs[0].Kind != stmtexpiry.RevokeManual {
		t.Errorf("kind = %q, want manual", revs[0].Kind)
	}
	if revs[0].Actor != "ops_admin" {
		t.Errorf("actor = %q, want ops_admin", revs[0].Actor)
	}
}
