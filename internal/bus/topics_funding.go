package bus

const (
	TopicWithdrawBroadcast = "withdraw.broadcast"
	TopicWithdrawConfirmed = "withdraw.confirmed"
	TopicWithdrawCancelled = "withdraw.cancelled"
	TopicDepositObserved   = "deposit.observed"
	TopicDriftBlocked = "ledger.drift_blocked"
	TopicOutboxPoison = "outbox.poison"
	TopicWorkerLeader = "worker.leader"
)

// FundingTopics 资金面默认订阅清单。
  func FundingTopics() []string {
	  return []string{
		   TopicDepositCredited, TopicDepositObserved,
		  TopicWithdrawRequested, TopicWithdrawBroadcast, TopicWithdrawConfirmed, TopicWithdrawCancelled,
		  TopicLedgerPosted, TopicReconDiff, TopicKYTHit, TopicDriftBlocked,
	   }
   }
