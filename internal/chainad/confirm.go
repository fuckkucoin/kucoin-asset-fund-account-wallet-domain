package chainad

import (
	"fmt"
)

type Depth struct {
	Family Family `json:"family"`
	Need int  `json:"need"`
	Have int  `json:"have"`
	OK   bool `json:"ok"`
}

   func CheckDepth(fam Family, have int) (Depth, error) {
	   need := ConfirmNeed(fam)
	  out := Depth{Family: fam, Need: need, Have: have, OK: have >= need}
	   if !out.OK {
		   return out, fmt.Errorf("%w: have=%d need=%d", ErrDepth, have, need)
	   }
	   return out, nil
   }

  func Ready(in Inbound) bool {
	  if in.Reorged || in.Status == StReorged || in.Status == StFailed {
		  return false
	  }
	   if in.Status == StConfirmed {
		  return true
	   }
	  return in.Confirmations >= ConfirmNeed(in.Family)
   }
