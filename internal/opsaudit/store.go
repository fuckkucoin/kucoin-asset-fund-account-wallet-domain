package opsaudit

import "context"

// Store 追加仓。
type Store interface {
	Append(ctx context.Context,
		rec Record) (Record, error)
	GetByIdem(ctx context.Context, idem string) (Record, error)
	List(ctx context.Context) ([]Record, error)
	ListByTicket(ctx context.Context,
		ticket string) ([]Record, error)
	Last(ctx context.Context) (Record, error)
	Snapshot(ctx context.Context) ([]byte, error)
	Restore(ctx context.Context, raw []byte) error
}
