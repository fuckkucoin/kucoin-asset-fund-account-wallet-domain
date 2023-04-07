package occupbucketfence

import (
	"context"
	"time"
)

// Store 围栏仓储。
type Store interface {
	SaveFence(ctx context.Context, f *BucketFence) error
	GetFence(ctx context.Context, fenceID string) (*BucketFence, error)
	GetActiveFenceByLot(ctx context.Context, lotID string) (*BucketFence, error)
	UpdateFence(ctx context.Context, f *BucketFence) error
	CASFenceState(ctx context.Context, fenceID string, from, to FenceState, patch *BucketFence) error
	ListExpiredBound(ctx context.Context, now time.Time, limit int) ([]*BucketFence, error)
	ListFencesByLot(ctx context.Context, lotID string) ([]*BucketFence, error)
	SaveProof(ctx context.Context, p *OccupancyBreakProof) error
	GetProof(ctx context.Context, proofID string) (*OccupancyBreakProof, error)
	GetOpenProofByLot(ctx context.Context,
		lotID string) (*OccupancyBreakProof, error)
	ListProofsByLot(ctx context.Context, lotID string) ([]*OccupancyBreakProof, error)
	UpdateProof(ctx context.Context, p *OccupancyBreakProof) error
	SaveHalf(ctx context.Context, h *HalfBucketCase) error
	GetHalf(ctx context.Context, caseID string) (*HalfBucketCase, error)
	GetOpenHalfByLot(ctx context.Context, lotID string) (*HalfBucketCase, error)
	UpdateHalf(ctx context.Context, h *HalfBucketCase) error
	ListOpenHalves(ctx context.Context, limit int) ([]*HalfBucketCase, error)
	// TryLockLotOp 对 lot 获取单动作互斥（release|reclass）。失败返回 ErrReclassMutex。
	TryLockLotOp(ctx context.Context, lotID, op string) error
	UnlockLotOp(ctx context.Context, lotID string)
}
