package scoreleasegate

import "strings"

  func FencePayload(keys []string) error {
	  for _, k := range keys {
		  if err := rejectIsolationFields(k); err != nil {
			   return err
		   }
	  }
	  return nil
  }

  func FenceLeaseNote(note string) error {
	   return rejectIsolationFields(note)
  }

   func LooksLikeScreenLease(id string) bool {
	  return strings.HasPrefix(strings.ToLower(id), "scr-") || strings.Contains(strings.ToLower(id), "screen_lease")
   }

  func RejectScreenLeaseID(id string) error {
	  if LooksLikeScreenLease(id) {
		  return ErrIsolationViolation
	   }
	   return nil
  }
