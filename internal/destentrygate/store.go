package destentrygate

import (
	"context"
	"time"
)

// Store 持久化操作。

type Store interface {
	SaveLease(ctx context.Context, lease DestEntryLease) error
	GetLease(ctx context.Context, leaseID string) (DestEntryLease, error)
	GetLeaseByIdempotencyKey(ctx context.Context, key string) (DestEntryLease, bool, error)
	UpdateLease(ctx context.Context, lease DestEntryLease) error
	ListLeasesByState(ctx context.Context,
		state LeaseState, limit int) ([]DestEntryLease, error)
	ListExpiredIssued(ctx context.Context, now time.Time, limit int) ([]DestEntryLease, error)
	ListUnconsumed(ctx context.Context, limit int) ([]DestEntryLease, error)
	ListByEntry(ctx context.Context, entryID string, limit int) ([]DestEntryLease, error)
	ListRevocableByEntryGen(ctx context.Context,
		entryID string, maxGen int64, limit int) ([]DestEntryLease, error)
	SaveGen(ctx context.Context, gen EntryGeneration) error
	GetGen(ctx context.Context, entryID string) (EntryGeneration, error)
	SaveProof(ctx context.Context,
		proof DestEntryDriftProof) error
	GetProof(ctx context.Context,
		proofID string) (DestEntryDriftProof, error)
	ListProofs(ctx context.Context, filter ExportProofFilter) ([]DestEntryDriftProof, error)
	MarkProofsExported(ctx context.Context, ids []string, at time.Time) error
	SaveAudit(ctx context.Context, row DestEntryAudit) error
	ListAuditByLease(ctx context.Context, leaseID string, limit int) ([]DestEntryAudit, error)
}

// EntrySurface destallow 只读观察面。不回写冷却编排 / destlife / 三联 ListGen。

type EntrySurface interface {
	WhitelistOnly(ctx context.Context, userID string) bool
	Snapshot(ctx context.Context, userID, chain, addr string) (LiveEntry, error)
	ListExpiredTemp(ctx context.Context, now time.Time, limit int) ([]LiveEntry, error)
	ListExpiredGlass(ctx context.Context, now time.Time, limit int) ([]LiveEntry, error)
}

// WithdrawSink 出金提交挂钩。本包不改写出金主链公式，只在复验通过后回调。

type WithdrawSink interface {
	Submit(ctx context.Context, userID, chain, addr, leaseID, idem string) (withdrawRef string, err error)
}
