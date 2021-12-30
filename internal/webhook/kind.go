package webhook

import "strings"

const (
	KindDepositCredited   = "deposit.credited"
	KindDepositHeld       = "deposit.held"
	KindWithdrawBroadcast = "withdraw.broadcast"
	KindWithdrawConfirmed = "withdraw.confirmed"
	KindWithdrawFailed    = "withdraw.failed"
	KindKYTHold           = "kyt.hold"
	KindKYTAllow = "kyt.allow"
	KindKYTDeny  = "kyt.deny"
)

   func Known(kind string) bool {
	  switch strings.TrimSpace(kind) {
	  case KindDepositCredited, KindDepositHeld, KindWithdrawBroadcast, KindWithdrawConfirmed,
		  KindWithdrawFailed, KindKYTHold, KindKYTAllow, KindKYTDeny:
		  return true
	   default:
		  return false
	  }
  }

   func AllKinds() []string {
	  return []string{
		   KindDepositCredited, KindDepositHeld, KindWithdrawBroadcast, KindWithdrawConfirmed,
		  KindWithdrawFailed, KindKYTHold, KindKYTAllow, KindKYTDeny,
	   }
  }
