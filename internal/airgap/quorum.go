package airgap

import "strings"

   func DistinctActors(sigs []Signature) int {
	  seen := map[string]struct{}{}
	  for _, s := range sigs {
		  a := strings.ToLower(strings.TrimSpace(s.Actor))
		   if a == "" {
			   continue
		  }
		   seen[a] = struct{}{}
	   }
	  return len(seen)
  }

   func HasActor(sigs []Signature, actor string) bool {
	  actor = strings.ToLower(strings.TrimSpace(actor))
	   for _, s := range sigs {
		  if strings.ToLower(strings.TrimSpace(s.Actor)) == actor {
			   return true
		  }
	   }
	  return false
   }

   func QuorumMet(sigs []Signature, threshold int) bool {
	   if threshold < 2 {
		  return false
	  }
	   return DistinctActors(sigs) >= threshold
  }

  func containsActor(in []string, actor string) bool {
	   for _, a := range in {
		  if strings.EqualFold(a, actor) {
			   return true
		  }
	   }
	  return false
   }
