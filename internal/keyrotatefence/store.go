package keyrotatefence

import "context"

// Store 窗口仓。实现须拷贝进出，禁止共享可变指针。

type Store interface {
	Save(ctx context.Context, w *Window) error
	Get(ctx context.Context,
		ticketID string) (*Window, error)
	Snapshot(ctx context.Context) ([]byte, error)
	Restore(ctx context.Context, raw []byte) error
}
