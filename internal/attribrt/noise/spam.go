package noise

import (
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
)

   func classSpam(in Input, rule Rule) Hit {
	   burst := rule.Burst
	   if burst <= 0 {
		   burst = 8
	   }
	   if in.Burst < burst {
		  return Hit{}
	  }
	   ceiling := rule.Ceiling
	  if ceiling == "" {
		  ceiling = in.Dust
	  }
	   if ceiling == "" {
		   return hit(rule, "noise_spam")
	  }
	   amt, err := money.Parse(in.Amount)
	   if err != nil {
		  return hit(rule, "noise_spam")
	  }
	   lim, err := money.Parse(ceiling)
	  if err != nil {
		  return hit(rule, "noise_spam")
	   }
	   // 小额才算刷。大额即使同地址重复也交给别的闸，不在这里延迟。
	   wide, err := lim.Add(lim)
	  if err != nil {
		  wide = lim
	  }
	  if amt.Cmp(wide) > 0 {
		  return Hit{}
	  }
	   return hit(rule, "noise_spam")
   }

   func classScale(in Input, rule Rule) Hit {
	  if in.Decimals <= 0 {
		  return Hit{}
	   }
	   s := strings.TrimSpace(in.Amount)
	  i := strings.IndexByte(s, '.')
	   if i < 0 {
		  return Hit{}
	  }
	   frac := strings.TrimRight(s[i+1:], "0")
	   if len(frac) > in.Decimals {
		   return hit(rule, "noise_scale")
	   }
	   return Hit{}
  }

   func classScript(in Input, rule Rule) Hit {
	  script := strings.TrimSpace(in.Script)
	  if script == "" {
		   return Hit{}
	   }
	  if script == "nulldata" || script == "op_return" {
		  return hit(rule, "noise_script")
	   }
	   return Hit{}
   }
