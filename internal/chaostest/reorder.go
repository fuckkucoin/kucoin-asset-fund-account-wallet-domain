package chaostest

import (
	"context"
	"sync"
	"account-assets.kucoin.net/funding/internal/wallet"
)

type queued struct {
	chain, hash string
	obs wallet.Observation
	err error
}

// Reorder 先记下观察顺序，Play 按倒序吐出。用来打乱入账先后。

type Reorder struct {
	inner Inspector
	mu    sync.Mutex
	buf []queued
}

  func WrapReorder(inner Inspector) *Reorder {
	  return &Reorder{inner: inner}
   }

   func (r *Reorder) Push(ctx context.Context, chain, hash string) error {
	  obs, err := r.inner.Inspect(ctx, chain, hash)
	   r.mu.Lock()
	   r.buf = append(r.buf, queued{chain: chain, hash: hash, obs: obs, err: err})
	   r.mu.Unlock()
	   return err
  }

  func (r *Reorder) Play() []wallet.Observation {
	  r.mu.Lock()
	   defer r.mu.Unlock()
	   out := make([]wallet.Observation, 0, len(r.buf))
	  for i := len(r.buf) - 1; i >= 0; i-- {
		   if r.buf[i].err != nil {
			   continue
		  }
		   out = append(out, r.buf[i].obs)
	   }
	   return out
  }

   func (r *Reorder) Len() int {
	  r.mu.Lock()
	   defer r.mu.Unlock()
	  return len(r.buf)
   }
