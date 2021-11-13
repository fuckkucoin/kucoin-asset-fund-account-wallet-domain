package screenleasegate

import "strings"

   func AssertIsolation(mark string) error {
	   if strings.TrimSpace(mark) == "" {
		  return ErrIsolationViolation
	  }
	   if mark != IsolationMark {
		   return ErrIsolationViolation
	  }
	  return nil
  }

  func ForbiddenPayload(keys map[string]string) error {
	   if keys == nil {
		   return nil
	   }
	  for _, k := range []string{
		   "triple_pin_gen", "wd_triple_id", "challenge_lease_id",
		   "otp_code", "dualctl_ticket", "dual_control_gen",
	   } {
		   if v := strings.TrimSpace(keys[k]); v != "" {
			   return ErrIsolationViolation
		  }
	  }
	   return nil
  }
