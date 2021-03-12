package cerdecidegate

import (
	"context"
	"time"
)

// Store 租约与证明持久化。
type Store interface {
	SaveLease(ctx context.Context, l DecideLease) error
	GetLease(ctx context.Context, leaseID string) (DecideLease, error)
	GetLeaseByIdem(ctx context.Context, idem string) (DecideLease, error)
	ListOpenByScope(ctx context.Context, scope string) ([]DecideLease, error)
	ListOpenByPolicy(ctx context.Context, policyID string) ([]DecideLease, error)
	ListOpenByBatch(ctx context.Context, batchID string) ([]DecideLease, error)
	ListIssuedOrBound(ctx context.Context, now time.Time, limit int) ([]DecideLease, error)
	CASState(ctx context.Context,
		leaseID string, from, to LeaseState, patch DecideLease) error
	SaveProof(ctx context.Context,
		p CerDriftProof) error
	GetProof(ctx context.Context, proofID string) (CerDriftProof, error)
	ListProofsByLease(ctx context.Context, leaseID string) ([]CerDriftProof, error)
	ListProofsByBatch(ctx context.Context,
		batchID string, limit int) ([]CerDriftProof, error)
	ListProofsByKind(ctx context.Context, kind ProofKind, limit int) ([]CerDriftProof, error)
	AppendAudit(ctx context.Context, a DecideAudit) error
	ListAudit(ctx context.Context,
		leaseID string) ([]DecideAudit, error)
	SaveFlip(ctx context.Context, e FlipEvent) error
	ListPendingFlips(ctx context.Context, limit int) ([]FlipEvent, error)
	MarkFlipDone(ctx context.Context, eventID string) error
	NextGeneration(ctx context.Context, scope string) (int64, error)
	CurrentGeneration(ctx context.Context,
		scope string) (int64, error)
}

// LiveSource 活策略/风险只读源。由 cerbind 适配，本包不改 Decide 公式。

type LiveSource interface {
	LiveDecide(ln LineFact) (LiveView, error)
}
