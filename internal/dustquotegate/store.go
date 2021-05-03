package dustquotegate

import (
	"context"
	"time"
)

// Store 租约与证明持久化。

type Store interface {
	SaveLease(ctx context.Context, l DustQuoteLease) error
	GetLease(ctx context.Context, leaseID string) (DustQuoteLease, error)
	GetLeaseByIdem(ctx context.Context, idem string) (DustQuoteLease, error)
	ListLeasesByUserAsset(ctx context.Context,
		user, asset string) ([]DustQuoteLease, error)
	ListOpenByAsset(ctx context.Context, asset string) ([]DustQuoteLease, error)
	ListQuotedOrBound(ctx context.Context, now time.Time, limit int) ([]DustQuoteLease, error)
	CASState(ctx context.Context, leaseID string, from, to LeaseState, patch DustQuoteLease) error
	SaveProof(ctx context.Context, p DustQuoteDriftProof) error
	GetProof(ctx context.Context,
		proofID string) (DustQuoteDriftProof, error)
	ListProofsByLease(ctx context.Context, leaseID string) ([]DustQuoteDriftProof, error)
	ListProofsByUser(ctx context.Context, user string, limit int) ([]DustQuoteDriftProof, error)
	ListProofsByKind(ctx context.Context, kind ProofKind, limit int) ([]DustQuoteDriftProof, error)
	AppendAudit(ctx context.Context, a DustQuoteAudit) error
	ListAudit(ctx context.Context,
		leaseID string) ([]DustQuoteAudit, error)
	SaveCursor(ctx context.Context, c ScanCursor) error
	GetCursor(ctx context.Context, batchID string) (ScanCursor, error)
	ListHaltedCursors(ctx context.Context) ([]ScanCursor, error)
	SaveFlip(ctx context.Context,
		e FlipEvent) error
	ListPendingFlips(ctx context.Context, limit int) ([]FlipEvent, error)
	MarkFlipDone(ctx context.Context, eventID string) error
	NextFlipEpoch(ctx context.Context, asset string) (int64, error)
	CurrentFlipEpoch(ctx context.Context, asset string) (int64, error)
}
