package dualctl

import "account-assets.kucoin.net/funding/internal/persist"

// GuardReplayAdvance 法医作业启动前核对围栏。未前进不得把作业标成 running。

  func GuardReplayAdvance(before, after persist.Fence) error {
	   return FenceMoved(before, after)
  }

// ReplaySubject 双控单挂到作业号上，避免和划转单混在一个 subject。
   func ReplaySubject(jobID string) string {
	   return "audit-replay:" + jobID
   }
