package noise

// Strategy 噪声处置表，含默认行和链覆盖。
   func Strategy() []Rule {
	   src := strategyRules()
	   out := make([]Rule, len(src))
	   copy(out, src)
	   return out
   }
