package invcheck

import (
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/treasury"
	"account-assets.kucoin.net/funding/internal/validate"
)

// CheckPlans 归集计划金额必须能 Parse，且不得超过热账可用。超了失败即关。
   func CheckPlans(plans []treasury.SweepPlan, led map[string]pair) error {
	   for _, p := range plans {
		  as, err := validate.Asset(p.Asset)
		   if err != nil {
			  return ErrAsset
		  }
		  amt, err := money.Parse(p.Amount)
		   if err != nil {
			  return ErrAmount
		  }
		  hot := led[as].hot
		   if amt.Cmp(hot) > 0 {
			   return ErrMismatch
		  }
	   }
	   return nil
  }
