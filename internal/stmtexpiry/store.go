package stmtexpiry

import (
	"context"
	"time"
)

// Store 持久化层接口。SQL 实现见 sqlstore.go。
type Store interface {
	// ── 租约 ──
	InsertLease(ctx context.Context, l ExportLease) error
	GetLease(ctx context.Context, id string) (ExportLease, error)
	UpdateLeaseState(ctx context.Context, id string, from, to LeaseState, updatedAt time.Time) error
	SetLeaseIssued(ctx context.Context, id, proofID string, issuedAt, expiresAt time.Time) error
	SetLeaseRevoked(ctx context.Context,
		id, revokedBy string) error
	ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]ExportLease, error)
	ListActiveLeasesByUser(ctx context.Context,
		userID string) ([]ExportLease, error)
	ListIssuedLeases(ctx context.Context, limit int) ([]ExportLease, error)
	// ── 漂移 ──
	InsertDrift(ctx context.Context, d RootDriftCase) error
	GetDrift(ctx context.Context, id string) (RootDriftCase, error)
	UpdateDriftState(ctx context.Context, id string, from, to DriftState) error
	ListOpenDrifts(ctx context.Context, limit int) ([]RootDriftCase, error)
	ListDriftsByLease(ctx context.Context, leaseID string) ([]RootDriftCase, error)
	// ── 撤销 ──
	InsertRevocation(ctx context.Context, r Revocation) error
	GetRevocation(ctx context.Context, id string) (Revocation, error)
	ListRevocationsByLease(ctx context.Context, leaseID string) ([]Revocation, error)
	// ── 证明 ──
	InsertProof(ctx context.Context, p IssueProof) error
	GetProof(ctx context.Context, id string) (IssueProof, error)
	GetProofByLease(ctx context.Context, leaseID string) (IssueProof, error)
	// ── 互斥 ──
	TryAcquireExportMutex(ctx context.Context, userID string, leaseID string, ttl time.Duration) (bool, error)
	ReleaseExportMutex(ctx context.Context,
		userID string, leaseID string) error
}
