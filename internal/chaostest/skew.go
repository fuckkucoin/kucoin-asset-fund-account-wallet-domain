package chaostest

import (
	"context"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// HeightSkew 把观察高度往回拨。里用来链头落后，不是改金额。
type HeightSkew struct {
	inner Inspector
	blocks int64
}

   func WrapSkew(inner Inspector, blocks int64) *HeightSkew {
	  if blocks < 0 {
		  blocks = 0
	   }
	  return &HeightSkew{inner: inner, blocks: blocks}
  }

  func (s *HeightSkew) Inspect(ctx context.Context, chain, hash string) (wallet.Observation, error) {
	   obs,
		  err := s.inner.Inspect(ctx, chain, hash)
	  if err != nil {
		  return obs, err
	   }
	   if s.blocks > 0 && obs.Height > s.blocks {
		   obs.Height -= s.blocks
	   } else if s.blocks > 0 {
		   obs.Height = 0
	  }
	   return obs, nil
  }
