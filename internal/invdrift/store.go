package invdrift

import (
	"context"
	"time"
)

// Store 偏离管线持久化接口

type Store interface {
	// 批次
	InsertBatch(ctx context.Context, b DriftBatch) error
	UpdateBatch(ctx context.Context, b DriftBatch) error
	GetBatch(ctx context.Context, id string) (DriftBatch, error)
	ListBatches(ctx context.Context, limit, offset int) ([]DriftBatch, error)
	ListOpenBatches(ctx context.Context) ([]DriftBatch, error)
	// 条目
	InsertItems(ctx context.Context, items []DriftItem) error
	ListItemsByBatch(ctx context.Context, batchID string) ([]DriftItem, error)
	ListItemsByAsset(ctx context.Context, asset string, limit int) ([]DriftItem, error)
	MarkItemResolved(ctx context.Context, itemID string, resolvedAt time.Time) error
	// 封堵建议
	InsertAdvices(ctx context.Context, advices []SealAdvice) error
	GetAdvice(ctx context.Context, id string) (SealAdvice, error)
	ListAdvicesByBatch(ctx context.Context,
		batchID string) ([]SealAdvice, error)
	ListActiveAdvices(ctx context.Context) ([]SealAdvice, error)
	UpdateAdviceState(ctx context.Context, id string, state AdviceState, ackedBy string, ackedAt, expiredAt *time.Time, expireReason string) error
	// 证明
	InsertProof(ctx context.Context, p DriftProof) error
	GetProof(ctx context.Context,
		id string) (DriftProof, error)
	ListProofsByBatch(ctx context.Context, batchID string) ([]DriftProof, error)
}
