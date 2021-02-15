package noise

import "strings"

  func classPoison(in Input, rule Rule) Hit {
	   to := strings.TrimSpace(in.To)
	   if to == "" || len(in.Look) == 0 {
		  return Hit{}
	   }
	   for _, raw := range in.Look {
		   known := strings.TrimSpace(raw)
		  if known == "" {
			  continue
		  }
		  if strings.EqualFold(known, to) {
			  return Hit{}
		  }
		   if lookalike(known, to) {
			  return hit(rule, "noise_poison")
		   }
	  }
	  return Hit{}
  }

// lookalike 等长、首 4 尾 4 相同、汉明距离 1 到 4。
   func lookalike(a, b string) bool {
	   if len(a) != len(b) || len(a) < 12 {
		  return false
	  }
	  if !strings.EqualFold(a[:4], b[:4]) || !strings.EqualFold(a[len(a)-4:], b[len(b)-4:]) {
		   return false
	  }
	   dist := 0
	   for i := 0; i < len(a); i++ {
		  if a[i]|0x20 != b[i]|0x20 {
			   dist++
			  if dist > 4 {
				  return false
			  }
		  }
	  }
	  return dist >= 1 && dist <= 4
  }
