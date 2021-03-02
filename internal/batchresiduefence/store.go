package batchresiduefence

import "context"

type Store interface {
	SaveFence(ctx context.Context, f ResidueFence) error
	GetFence(ctx context.Context, id string) (ResidueFence, bool, error)
	ListFencesByItem(ctx context.Context, batchID, itemIdem string) ([]ResidueFence, error)
	ListFencesByBatch(ctx context.Context, batchID string) ([]ResidueFence, error)
	ListLiveFences(ctx context.Context, limit int) ([]ResidueFence, error)
	SaveProof(ctx context.Context, p BatchDupFreezeProof) error
	ListProofs(ctx context.Context,
		batchID string) ([]BatchDupFreezeProof, error)
	ListUnexportedProofs(ctx context.Context, limit int) ([]BatchDupFreezeProof, error)
	SaveHalf(ctx context.Context,
		h HalfSubmitCase) error
	GetOpenHalf(ctx context.Context, batchID, itemIdem string) (HalfSubmitCase, bool, error)
	ListOpenHalves(ctx context.Context, limit int) ([]HalfSubmitCase, error)
	ListHalvesByBatch(ctx context.Context, batchID string) ([]HalfSubmitCase, error)
	SaveSlot(ctx context.Context, s FreezeSlot) error
	GetSlot(ctx context.Context, itemIdem string) (FreezeSlot, bool, error)
	SaveMutex(ctx context.Context, m MutexHold) error
	GetMutex(ctx context.Context,
		batchID string) (MutexHold, bool, error)
	DeleteMutex(ctx context.Context, batchID string) error
	SaveAudit(ctx context.Context, a AuditEvent) error
	ListAudits(ctx context.Context, batchID string, limit int) ([]AuditEvent, error)
}
