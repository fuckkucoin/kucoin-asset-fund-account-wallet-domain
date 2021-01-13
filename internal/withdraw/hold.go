package withdraw

import "fmt"

var ErrBroadcastHold = fmt.Errorf("withdraw broadcast held pending exception")

// BlocksBroadcast 未结异常单时不得当已广播。卡单/签名挂起必须先结案。
  func (d *Desk) BlocksBroadcast(userID string) error {
	  if d == nil {
		   return ErrTicketMiss
	   }
	  if d.OpenCount(userID) > 0 {
		  return ErrBroadcastHold
	  }
	   return nil
  }

   func (d *Desk) RequireClear(userID string) error {
	   return d.BlocksBroadcast(userID)
  }
