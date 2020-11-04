package fsm

// 账本单据边。failed 不是了结：冻结未回滚时对账还得看见。

const (
	TrInitiated  = "initiated"
	TrSpotFrozen = "spot_frozen"
	TrWalletLocked   = "wallet_locked"
	TrChainSubmitted = "chain_submitted"
	TrConfirmed = "confirmed"
	TrCredited  = "credited"
	TrFailed = "failed"
	TrRolledBack = "rolled_back"
	TrHold = "hold"
	TrReorged = "reorged"
	EvFreezeSpot = "freeze_spot"
	EvLockWallet = "lock_wallet"
	EvSubmit   = "submit_chain"
	EvConfirm  = "confirm"
	EvCredit   = "credit"
	EvFail     = "fail"
	EvRollback = "rollback"
	EvHold     = "hold"
	EvReorgUnwind = "reorg_unwind"
)

  func Transfer() *Machine[string, string] {
	   return New([]Edge[string, string]{
		   {TrInitiated, EvFreezeSpot, TrSpotFrozen},
		  {TrInitiated, EvLockWallet, TrWalletLocked},
		   {TrInitiated, EvHold, TrHold},
		   {TrInitiated, EvFail, TrFailed},
		   {TrSpotFrozen, EvSubmit, TrChainSubmitted},
		  {TrSpotFrozen, EvCredit, TrCredited},
		  {TrSpotFrozen,
			  EvFail, TrFailed},
		   {TrSpotFrozen, EvRollback, TrRolledBack},
		  {TrWalletLocked, EvSubmit, TrChainSubmitted},
		   {TrWalletLocked, EvCredit, TrCredited},
		   {TrWalletLocked,
			  EvFail, TrFailed},
		  {TrWalletLocked, EvRollback, TrRolledBack},
		  {TrChainSubmitted, EvConfirm, TrConfirmed},
		  {TrChainSubmitted, EvFail, TrFailed},
		   {TrConfirmed, EvCredit, TrCredited},
		  {TrConfirmed, EvFail, TrFailed},
		   {TrHold, EvCredit, TrCredited},
		  {TrHold, EvFail, TrFailed},
		  {TrHold, EvReorgUnwind, TrReorged},
		  {TrFailed, EvRollback, TrRolledBack},
		  {TrCredited, EvReorgUnwind, TrReorged},
	   },
		  TrCredited, TrRolledBack, TrReorged)
   }

// Closed 了结态：credited / rolled_back / reorged。failed 仍 Open。

  func TransferClosed(s string) bool {
	  switch s {
	  case TrCredited,
		   TrRolledBack, TrReorged:
		  return true
	   default:
		  return false
	  }
  }
