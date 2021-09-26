package notify

import "strings"

const (
	KindDepositCredited   = "deposit.credited"
	KindDepositHeld       = "deposit.held"
	KindWithdrawBroadcast = "withdraw.broadcast"
	KindWithdrawConfirmed = "withdraw.confirmed"
	KindWithdrawFailed    = "withdraw.failed"
	KindFreeze = "account.frozen"
	KindEarnAccrual = "earn.accrual"
	KindEarnRedeem  = "earn.redeemed"
	KindSnapshotTaken     = "snapshot.taken"
	KindReconTicket       = "recon.ticket"
	KindFiatCredited      = "fiat.credited"
	KindP2PHold           = "p2p.hold"
	KindConvertFilled     = "convert.filled"
	KindWithdrawChallenge = "withdraw.challenge"
)

   func KnownKind(kind string) bool {
	   switch strings.TrimSpace(kind) {
	   case KindDepositCredited, KindDepositHeld, KindWithdrawBroadcast, KindWithdrawConfirmed,
		  KindWithdrawFailed,
		   KindFreeze, KindEarnAccrual, KindEarnRedeem, KindSnapshotTaken, KindReconTicket,
		  KindFiatCredited, KindP2PHold, KindConvertFilled, KindWithdrawChallenge:
		   return true
	  default:
		   return false
	  }
   }

  func AllKinds() []string {
	   return []string{
		   KindDepositCredited, KindDepositHeld, KindWithdrawBroadcast, KindWithdrawConfirmed,
		   KindWithdrawFailed,
		  KindFreeze, KindEarnAccrual, KindEarnRedeem, KindSnapshotTaken, KindReconTicket,
		  KindFiatCredited, KindP2PHold, KindConvertFilled, KindWithdrawChallenge,
	   }
   }
