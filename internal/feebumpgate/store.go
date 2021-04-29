package feebumpgate

import (
	"context"
	"time"
)

// Store 持久化。
type Store interface {
	SaveGen(ctx context.Context, g FeeBumpGen) error
	GetGen(ctx context.Context,
		genID string) (FeeBumpGen, error)
	GetGenByIdem(ctx context.Context, idem string) (FeeBumpGen, error)
	LatestGen(ctx context.Context, transferID string) (FeeBumpGen, error)
	ListGens(ctx context.Context, transferID string) ([]FeeBumpGen, error)
	ListActiveGens(ctx context.Context, limit int) ([]FeeBumpGen, error)
	CASGen(ctx context.Context, genID string, from, to GenState, patch FeeBumpGen) error
	SaveOcc(ctx context.Context, o BroadcastOccupancy) error
	GetOcc(ctx context.Context, occID string) (BroadcastOccupancy, error)
	GetOccByGen(ctx context.Context,
		genID string) (BroadcastOccupancy, error)
	ListHeldByTransfer(ctx context.Context, transferID string) ([]BroadcastOccupancy, error)
	ListHeldExpired(ctx context.Context, now time.Time, limit int) ([]BroadcastOccupancy, error)
	CASOcc(ctx context.Context, occID string, from, to OccState, patch BroadcastOccupancy) error
	CountHeld(ctx context.Context, transferID string) (int, error)
	SaveProof(ctx context.Context, p RbfOrphanProof) error
	GetProof(ctx context.Context, proofID string) (RbfOrphanProof, error)
	ListProofsByTransfer(ctx context.Context, transferID string) ([]RbfOrphanProof, error)
	ListOpenOrphans(ctx context.Context, limit int) ([]RbfOrphanProof, error)
	ListProofsByKind(ctx context.Context, kind ProofKind, limit int) ([]RbfOrphanProof, error)
	CASProofState(ctx context.Context, proofID string, from, to OrphanState, patch RbfOrphanProof) error
	AppendAudit(ctx context.Context, a GenAudit) error
	ListAudit(ctx context.Context, genID string) ([]GenAudit, error)
	NextGeneration(ctx context.Context, transferID string) (int64, error)
	CurrentGeneration(ctx context.Context,
		transferID string) (int64, error)
}

// Signer 抬费规范串签章适配。custody 只读挂钩，本包不改 SignFeeBump 内核。
type Signer interface {
	SignCanonical(canon string) ([]byte, error)
}
