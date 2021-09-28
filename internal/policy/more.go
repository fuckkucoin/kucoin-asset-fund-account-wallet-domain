package policy

import "time"

// DefaultV2 更紧的单笔和日限额。发布后新单走 v2，已评估单仍钉在当时代次。

   func DefaultV2() Doc {
	   return Doc{
		  Version: 2, PublishedAt: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		   MaxSingle: "3000", DailyWithdraw: "8000", DailyDeposit: "40000",
		   MinAmount: "0.00000001", MaxVelocity: 12, VelocityWin: time.Minute, Allowlist: true,
	   }
  }

  func (r *Registry) CurrentVersion() int {
	   r.mu.RLock()
	   defer r.mu.RUnlock()
	  return r.cur
  }

   func (r *Registry) Has(version int) bool {
	   r.mu.RLock()
	   defer r.mu.RUnlock()
	   _, ok := r.docs[version]
	   return ok
  }
