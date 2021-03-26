package chaostest

import (
	"context"
	"sync"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// DuplicateObserve 同一 hash 第二次 Inspect 仍返回原文。用来打重复观察，不是换金额。
type DuplicateObserve struct {
	inner Inspector
	mu    sync.Mutex
	seen  map[string]int
}

  func WrapDup(inner Inspector) *DuplicateObserve {
	  return &DuplicateObserve{inner: inner, seen: map[string]int{}}
  }

   func (d *DuplicateObserve) Times(hash string) int {
	   d.mu.Lock()
	  defer d.mu.Unlock()
	  return d.seen[hash]
  }

   func (d *DuplicateObserve) Inspect(ctx context.Context, chain, hash string) (wallet.Observation, error) {
	  obs, err := d.inner.Inspect(ctx, chain, hash)
	  d.mu.Lock()
	   d.seen[hash]++
	  d.mu.Unlock()
	   return obs, err
  }
