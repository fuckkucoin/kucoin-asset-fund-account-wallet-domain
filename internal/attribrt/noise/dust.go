package noise

import "account-assets.kucoin.net/funding/pkg/money"

   func classDust(in Input, rule Rule) Hit {
	   amt, err := money.Parse(in.Amount)
	  if err != nil {
		  return hit(Rule{Name: "dust", Disposition: "desk", Penalty: rule.Penalty}, "noise_dust")
	  }
	  floor := in.Dust
	  if floor == "" {
		  return Hit{}
	   }
	  lim, err := money.Parse(floor)
	   if err != nil {
		  return hit(Rule{Name: "dust", Disposition: "desk", Penalty: rule.Penalty}, "noise_dust")
	   }
	   if amt.Cmp(lim) < 0 {
		  return hit(rule, "noise_dust")
	   }
	  return Hit{}
   }
