package shardholdgen

import (
	"context"
	"time"
)

// Store 世代 / 证明 / 半截 / 审计。

type Store interface {
	SaveGen(ctx context.Context,
		g LegHoldGen) error
	GetGen(ctx context.Context, genID string) (LegHoldGen, error)
	GetGenByIdem(ctx context.Context, key string) (LegHoldGen, bool, error)
	GetGenByLeg(ctx context.Context, planID, legID string) (LegHoldGen, bool, error)
	UpdateGen(ctx context.Context, g LegHoldGen) error
	ListGensByState(ctx context.Context,
		state GenState, limit int) ([]LegHoldGen, error)
	ListExpiredActive(ctx context.Context, now time.Time, limit int) ([]LegHoldGen, error)
	TryLockLeg(ctx context.Context, planID, legID, owner string) (bool, error)
	UnlockLeg(ctx context.Context, planID, legID, owner string) error
	SaveProof(ctx context.Context, p ShardHoldProof) error
	GetProof(ctx context.Context, proofID string) (ShardHoldProof, error)
	ListProofs(ctx context.Context, filter ExportFilter) ([]ShardHoldProof, error)
	MarkProofsExported(ctx context.Context, ids []string, at time.Time) error
	SaveHalf(ctx context.Context, c HalfLegCase) error
	GetHalf(ctx context.Context, caseID string) (HalfLegCase, error)
	GetOpenHalfByGen(ctx context.Context,
		genID string) (HalfLegCase, bool, error)
	UpdateHalf(ctx context.Context, c HalfLegCase) error
	ListOpenHalf(ctx context.Context, limit int) ([]HalfLegCase, error)
	SaveAudit(ctx context.Context, a GenAudit) error
	ListAudit(ctx context.Context, genID string, limit int) ([]GenAudit, error)
}

// ExportFilter 证明导出过滤。

type ExportFilter struct {
	Kind           ProofKind
	GenID          string
	LegID          string
	Limit          int
	UnexportedOnly bool
}
