package noise

import "strings"

  func classSelf(in Input, rule Rule) Hit {
	   from := strings.TrimSpace(in.From)
	  to := strings.TrimSpace(in.To)
	  if from == "" || to == "" {
		   return Hit{}
	  }
	   if strings.EqualFold(from, to) {
		   return hit(rule, "noise_self")
	  }
	   return Hit{}
  }

   func classSweep(in Input, rule Rule) Hit {
	  from := strings.TrimSpace(in.From)
	  if from == "" || len(in.Hot) == 0 {
		  return Hit{}
	  }
	   for _, h := range in.Hot {
		  if strings.EqualFold(strings.TrimSpace(h), from) {
			  return hit(rule, "noise_sweep")
		   }
	   }
	   return Hit{}
  }
