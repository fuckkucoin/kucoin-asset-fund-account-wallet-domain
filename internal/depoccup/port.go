package depoccup

import "context"

type RollbackPort interface {
	Unwind(ctx context.Context, id string) error
	Fail(ctx context.Context,
		id, reason string) error
}

type KYCReader interface {
	Level(ctx context.Context, userID string) (string, error)
}

type DelaySource interface {
	Snapshot(chain, hash string) (have, need int, early bool, ok bool)
}
