package recycleepoch

import (
	"context"
	"time"
)

// Store 回收世代、观察绑定、错户证明与观察记录。

type Store interface {
	GetEpoch(ctx context.Context,
		slot SlotKey) (*RecycleEpoch, error)
	GetEpochByID(ctx context.Context,
		epochID string) (*RecycleEpoch, error)
	InsertEpoch(ctx context.Context, e RecycleEpoch) error
	CASUpdateEpoch(ctx context.Context, epochID string, expectedGen int64, expectedState EpochState, next RecycleEpoch) error
	ListEpochsByState(ctx context.Context, state EpochState, limit int) ([]RecycleEpoch, error)
	ListCoolingElapsed(ctx context.Context, now time.Time, limit int) ([]RecycleEpoch, error)
	GetBind(ctx context.Context, slot SlotKey) (*ObserveBind, error)
	GetBindByID(ctx context.Context,
		bindID string) (*ObserveBind, error)
	UpsertBind(ctx context.Context,
		b ObserveBind) error
	RevokeBind(ctx context.Context, slot SlotKey, reason string, now time.Time) error
	ListBindsByUser(ctx context.Context, userID string, limit int) ([]ObserveBind, error)
	InsertProof(ctx context.Context, p MiscreditProof) error
	GetProof(ctx context.Context, proofID string) (*MiscreditProof, error)
	ListOpenProofs(ctx context.Context, limit int) ([]MiscreditProof, error)
	ListProofsBySlot(ctx context.Context,
		slot SlotKey) ([]MiscreditProof, error)
	AckProof(ctx context.Context, proofID string, now time.Time) error
	MarkProofExported(ctx context.Context, proofID string, now time.Time) error
	InsertObserve(ctx context.Context, r ObserveRecord) error
	GetObserveByIdem(ctx context.Context, idemKey string) (*ObserveRecord, error)
	ListObservesBySlot(ctx context.Context, slot SlotKey, limit int) ([]ObserveRecord, error)
	InsertAudit(ctx context.Context, ev AuditEvent) error
	ListAudit(ctx context.Context, slot SlotKey, limit int) ([]AuditEvent, error)
}
