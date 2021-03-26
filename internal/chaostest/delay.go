package chaostest

import (
	"context"
	"sync"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// DelayedTip 压住确认数。里 Advance 才放行，用来链头延迟。

type DelayedTip struct {
	inner Inspector
	mu      sync.Mutex
	held    map[string]int
	release map[string]bool
}

type Inspector interface {
	Inspect(ctx context.Context,
		chain, hash string) (wallet.Observation, error)
}

   func WrapInspect(inner Inspector) *DelayedTip {
	  return &DelayedTip{inner: inner, held: map[string]int{}, release: map[string]bool{}}
  }

   func (d *DelayedTip) Hold(hash string, conf int) {
	  d.mu.Lock()
	   d.held[hash] = conf
	  d.release[hash] = false
	  d.mu.Unlock()
   }

  func (d *DelayedTip) Advance(hash string) {
	   d.mu.Lock()
	   d.release[hash] = true
	   d.mu.Unlock()
   }

  func (d *DelayedTip) Inspect(ctx context.Context, chain, hash string) (wallet.Observation, error) {
	   obs, err := d.inner.Inspect(ctx, chain, hash)
	  if err != nil {
		   return obs, err
	  }
	   d.mu.Lock()
	   defer d.mu.Unlock()
	   if conf, ok := d.held[hash]; ok && !d.release[hash] {
		   obs.Confirmations = conf
		   return obs, nil
	   }
	  return obs, nil
   }
