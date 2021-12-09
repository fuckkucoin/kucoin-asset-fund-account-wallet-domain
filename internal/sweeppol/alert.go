package sweeppol

import (
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/hotinv"
)

var ErrInvAlert = fmt.Errorf("sweep blocked by inventory alert")

// PlanGuarded 窗口内出计划。库存不可读或金额不可解析失败即关；超 cap 才允许归集。
   func (b *Book) PlanGuarded(inv []hotinv.Snapshot, sch Schedule, now time.Time, alerts []hotinv.Alert) ([]Plan, error) {
	   for _, a := range alerts {
		  if a.Kind == hotinv.AlertUnread || a.Kind == hotinv.AlertUnparsable {
			   return nil, fmt.Errorf("%w: %s", ErrInvAlert, a.Kind)
		   }
	  }
	  return b.PlanAt(inv, sch, now)
   }
