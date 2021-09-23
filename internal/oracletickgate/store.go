package oracletickgate

import (
	"context"
	"time"
)

// Store 内存/SQL 共用契约。
type Store interface {
	InsertLease(ctx context.Context, l TickLease) error
	GetLease(ctx context.Context, leaseID string) (*TickLease, error)
	CASLease(ctx context.Context, leaseID string, expectedGen int64, expectedState LeaseState, next TickLease) error
	ListLeasesByState(ctx context.Context, state LeaseState, limit int) ([]TickLease, error)
	ListOpenLeasesByPair(ctx context.Context, pair string) ([]TickLease, error)
	ListExpiredOpen(ctx context.Context, now time.Time, limit int) ([]TickLease, error)
	ListOpenLeases(ctx context.Context, limit int) ([]TickLease, error)
	GetProof(ctx context.Context,
		id string) (*TickDriftProof, error)
	ListProofs(ctx context.Context, limit int) ([]TickDriftProof, error)
	ListProofsByLease(ctx context.Context, leaseID string) ([]TickDriftProof, error)
	MarkProofExported(ctx context.Context, id string, now time.Time) error
	InsertAudit(ctx context.Context, a AuditEvent) error
	ListAuditsByLease(ctx context.Context, leaseID string, limit int) ([]AuditEvent, error)
	ListAuditsByPair(ctx context.Context,
		pair string, limit int) ([]AuditEvent, error)
	InsertWrite(ctx context.Context, w FundWriteRecord) error
	GetWriteByLease(ctx context.Context,
		leaseID string) (*FundWriteRecord, error)
	PutIdem(ctx context.Context, key, resultID string) error
	GetIdem(ctx context.Context, key string) (string, bool, error)
	Snapshot() StoreSnapshot
	Restore(StoreSnapshot)
}

// StoreSnapshot 重启复放。

type StoreSnapshot struct {
	Leases []TickLease `json:"leases"`
	Generations []TickGeneration `json:"generations"`
	Proofs      []TickDriftProof `json:"proofs"`
	Audits []AuditEvent      `json:"audits"`
	Writes []FundWriteRecord `json:"writes"`
	Idem   map[string]string `json:"idem"`
}
