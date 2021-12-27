package wdrelease

import "account-assets.kucoin.net/funding/internal/withdraw"

   func releaseAsk(in Input, ticketID, amount string) withdraw.ReleaseAsk {
	   return withdraw.ReleaseAsk{
		   UserID: in.UserID,
		   Asset:  in.Asset, Amount: amount, Chain: in.Chain, Dest: in.Dest,
		  Idem: in.Idem + ":frz:" + amount, TicketID: ticketID, Risk: in.Risk, DestClass: in.DestClass,
	   }
  }
