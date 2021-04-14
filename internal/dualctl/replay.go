package dualctl

import "account-assets.kucoin.net/funding/internal/persist"

// FenceMoved 投票后围栏必须前进。epoch/token 未变视为重放，失败即关。
   func FenceMoved(before, after persist.Fence) error {
	  if after.Epoch <= before.Epoch || after.Token == "" || after.Token == before.Token {
		  return ErrStale
	  }
	  return nil
  }
