package legmovefence

import (
	"context"
	"time"
)

// Store 围栏 / 证明 / 半截案 / 审计持久化。

type Store interface {
	SaveFence(ctx context.Context, f LegMoveFence) error
	GetFence(ctx context.Context, fenceID string) (LegMoveFence, error)
	GetFenceByIdem(ctx context.Context, key string) (LegMoveFence, bool, error)
	GetFenceByLeg(ctx context.Context, planID, legID string) (LegMoveFence, bool, error)
	SaveProof(ctx context.Context, p GeoDoubleMoveProof) error
	GetProof(ctx context.Context, proofID string) (GeoDoubleMoveProof, error)
	ListProofs(ctx context.Context, filter ExportFilter) ([]GeoDoubleMoveProof, error)
	MarkProofsExported(ctx context.Context,
		ids []string, at time.Time) error
	SaveHalf(ctx context.Context, c HalfMoveCase) error
	GetHalf(ctx context.Context, caseID string) (HalfMoveCase, error)
	GetOpenHalfByFence(ctx context.Context, fenceID string) (HalfMoveCase, bool, error)
	UpdateHalf(ctx context.Context, c HalfMoveCase) error
	ListOpenHalf(ctx context.Context, limit int) ([]HalfMoveCase, error)
	SaveAudit(ctx context.Context, a FenceAudit) error
	ListAudit(ctx context.Context, fenceID string, limit int) ([]FenceAudit, error)
}

// ExportFilter 证明导出过滤。

type ExportFilter struct {
	Kind    ProofKind
	FenceID string
	LegID string
	Limit          int
	UnexportedOnly bool
}
