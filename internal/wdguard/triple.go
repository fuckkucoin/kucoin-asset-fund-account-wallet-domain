package wdguard

import (
	"fmt"
	"account-assets.kucoin.net/funding/internal/kyt"
)

var ErrOpenTicket = fmt.Errorf("withdraw guard open exception ticket")

// CheckTriple 出金三联闸：未结异常单、KYT 钉、白名单世代+策略代次。任一步失败即关。
   func (l *Link) CheckTriple(req Check, pinGen int64, openTickets int, kytDec kyt.Decision) (LinkResult, error) {
	  out := LinkResult{MovementID: req.MovementID}
	  if l == nil {
		   return out, ErrUnavailable
	   }
	   if openTickets > 0 {
		   out.Reason = ErrOpenTicket.Error()
		  return out, ErrOpenTicket
	  }
	   if kytDec != kyt.Allow {
		   out.Reason = kyt.ErrDenied.Error()
		   return out, kyt.ErrDenied
	   }
	  return l.CheckLinked(req, pinGen)
  }
