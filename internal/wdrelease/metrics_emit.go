package wdrelease

import (
	"account-assets.kucoin.net/funding/internal/metrics"
	"github.com/shopspring/decimal"
)

  func (d *Desk) emitLocked() {
	  if d == nil {
		  return
	   }
	   now := d.now()
	  age := map[string]float64{}
	   for _, st := range []string{StateDraft, StatePending, StatePartial, StateApproved, StateRejected, StateExpired} {
		  age[st] = 0
	  }
	   residual := map[string]decimal.Decimal{}
	   for asset := range d.seenAsset {
		  residual[asset] = decimal.Zero
	   }
	  for _, t := range d.tickets {
		  sec := now.Sub(t.CreatedAt).Seconds()
		  if sec < 0 {
			   sec = 0
		   }
		  if sec > age[t.State] {
			   age[t.State] = sec
		   }
		   part := decimal.Zero
		  if t.Residual != "" {
			  if v, err := decimal.NewFromString(t.Residual); err == nil {
				  part = v
			   }
		   }
		  residual[t.Asset] = residual[t.Asset].Add(part)
		   d.seenAsset[t.Asset] = struct{}{}
	   }
	   for st, v := range age {
		   metrics.WDReleaseAge.WithLabelValues(st).Set(v)
	  }
	  for asset, v := range residual {
		   f, _ := v.Float64()
		   metrics.WDReleaseResidual.WithLabelValues(asset).Set(f)
	  }
	   metrics.WDReleaseDrift.Set(float64(len(d.driftSet)))
	  if d.driftNew > 0 {
		  metrics.WDReleaseEvents.WithLabelValues("drift").Add(float64(d.driftNew))
		  d.driftNew = 0
	   }
	  if d.privSent == nil {
		   d.privSent = map[string]int64{}
	  }
	   for reason, n := range d.priv {
		   delta := n - d.privSent[reason]
		   if delta <= 0 {
			   continue
		   }
		  metrics.WDReleasePrivilege.WithLabelValues(reason).Add(float64(delta))
		   d.privSent[reason] = n
	   }
  }
