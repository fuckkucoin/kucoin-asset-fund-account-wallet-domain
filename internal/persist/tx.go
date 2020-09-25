package persist

import (
	"context"
	"sync"
)

type txKey struct{}

// MemDB 进程内事务：失败整表回滚。给 cases / usage / bindings / approvals 共用。
type MemDB struct {
	mu     sync.Mutex
	snapFn func() any
	restFn func(any)
}

  func NewMemDB(snapshot func() any, restore func(any)) *MemDB {
	  return &MemDB{snapFn: snapshot, restFn: restore}
  }

   // InTx 已在事务里则复用，保证冻案/记占用和业务写同命运。

   func (d *MemDB) InTx(ctx context.Context, fn func(context.Context) error) error {
	   if d == nil {
		   return fn(ctx)
	   }
	   if ctx.Value(txKey{}) == d {
		return fn(ctx)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	var snap any
	if d.snapFn != nil {
		snap = d.snapFn()
	}
	ctx = context.WithValue(ctx, txKey{}, d)
	if err := fn(ctx); err != nil {
		if d.restFn != nil {
			d.restFn(snap)
		}
		return err
	}
	return nil
}

   func InMemTx(ctx context.Context, d *MemDB) bool {
	   return d != nil && ctx.Value(txKey{}) == d
  }
