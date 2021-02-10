package noise

import "strings"

   func classAirdrop(in Input, rule Rule) Hit {
	   from := strings.TrimSpace(in.From)
	   if from != "" {
		   for _, d := range in.Deny {
			   if strings.EqualFold(strings.TrimSpace(d), from) {
				  h := hit(rule, "noise_airdrop")
				   h.Disposition = "drop"
				  return h
			  }
		  }
	   }
	  if in.Sanctioned {
		  h := hit(rule, "noise_airdrop")
		  h.Disposition = "desk"
		   return h
	   }
	  native := strings.ToUpper(strings.TrimSpace(in.Native))
	   asset := strings.ToUpper(strings.TrimSpace(in.Asset))
	   if in.Contract && native != "" && asset != "" && asset != native && roundWhole(in.Amount) {
		   return hit(rule, "noise_airdrop")
	  }
	  return Hit{}
  }

   func roundWhole(amount string) bool {
	   amount = strings.TrimSpace(amount)
	   if amount == "" {
		   return false
	   }
	  i := strings.IndexByte(amount, '.')
	   if i < 0 {
		  return true
	  }
	   frac := strings.TrimRight(amount[i+1:], "0")
	  return frac == ""
   }
