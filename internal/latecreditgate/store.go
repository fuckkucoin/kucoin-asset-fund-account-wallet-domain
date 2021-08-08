package latecreditgate

import (
	"context"
	"time"
)

// Store 迟贷记世代、竞态事件与证明仓存储。

type Store interface {
	GetGen(ctx context.Context, key OutputKey) (*LateCreditGeneration, error)
	GetGenByID(ctx context.Context, genID string) (*LateCreditGeneration, error)
	InsertGen(ctx context.Context, g LateCreditGeneration) error
	CASUpdateGen(ctx context.Context, genID string, expectedGen int64, expectedState GenState, next LateCreditGeneration) error
	ListGensByState(ctx context.Context,
		state GenState, limit int) ([]LateCreditGeneration, error)
	ListOccupiedExpired(ctx context.Context, now time.Time, ttl time.Duration, limit int) ([]LateCreditGeneration, error)
	InsertRaceEvent(ctx context.Context, ev RaceEvent) error
	GetRaceEvent(ctx context.Context, eventID string) (*RaceEvent, error)
	ListOpenRaceEvents(ctx context.Context, limit int) ([]RaceEvent, error)
	ListOpenRaceEventsByKey(ctx context.Context,
		key OutputKey) ([]RaceEvent, error)
	AckRaceEvent(ctx context.Context, eventID string, now time.Time) error
	InsertProof(ctx context.Context, p LateCreditRaceProof) error
	GetProof(ctx context.Context, proofID string) (*LateCreditRaceProof, error)
	ListProofs(ctx context.Context, limit int) ([]LateCreditRaceProof, error)
	ListProofsByKey(ctx context.Context, key OutputKey) ([]LateCreditRaceProof, error)
	MarkProofExported(ctx context.Context, proofID string, now time.Time) error
}
