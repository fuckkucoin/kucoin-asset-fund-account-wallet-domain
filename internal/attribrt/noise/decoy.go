package noise

import "account-assets.kucoin.net/funding/internal/attribrt/textnorm"

   func classDecoy(in Input, rule Rule) Hit {
	  if in.Memo == "" && in.Tag == "" {
		  return Hit{}
	   }
	   if textnorm.HasFormat(in.Memo) || textnorm.HasFormat(in.Tag) {
		  return hit(rule, "noise_decoy")
	   }
	   if textnorm.MixedScript(in.Memo) || textnorm.MixedScript(in.Tag) {
		  return hit(rule, "noise_decoy")
	  }
	   return Hit{}
  }

  func classHomoglyph(in Input, rule Rule) Hit {
	   raw := in.Memo
	   if raw == "" {
		   raw = in.Tag
	   }
	   if raw == "" {
		   return Hit{}
	  }
	  mode := textnorm.Mode{CaseFold: in.CaseFold, StripZW: in.StripZW, LeadingZero: in.LeadingZero, Numeric: in.Numeric}
	  sk, ok := textnorm.Fold(raw, mode)
	  if !ok {
		   return hit(rule, "noise_homoglyph")
	   }
	  plain := textnorm.CaseOnly(raw, in.CaseFold)
	   if in.LeadingZero || in.Numeric {
		   return Hit{}
	   }
	   if sk != plain {
		   return hit(rule, "noise_homoglyph")
	   }
	   return Hit{}
   }
