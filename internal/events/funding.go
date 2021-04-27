package events

const (
	AggregateDeposit = "deposit"
	AggregateWithdraw = "withdraw"
	AggregateRecon    = "recon"
	AggregateWorker   = "worker"
	KindWithdrawBroadcast = "withdraw_broadcast"
	KindWithdrawConfirmed = "withdraw_confirmed"
	KindDepositObserved   = "deposit_observed"
	KindDepositCredited = "deposit_credited"
	KindWithdrawFreeze  = "withdraw_freeze"
	KindTransferHop = "transfer_hop"
	KindSweepPlan   = "sweep_plan"
	KindSweepProof  = "sweep_proof"
	KindReconTicket = "recon_ticket"
	KindAddrpoolProtect   = "addrpool_protect"
	KindWithdrawApproval  = "withdraw_approval"
	KindHotReserve        = "hot_reserve"
	KindFundGraphIngested = "fund_graph_ingested"
	KindFundGraphIncident = "fund_graph_incident"
	KindDriftBlocked = "drift_blocked"
	KindOutboxPoison = "outbox_poison"
	TopicFundGraph = "fund.graph"
	TopicFundGraphIncident = "fund.graph.incident"
)

// TopicForKind 出箱 kind → 总线主题；未知回落 wallet.movements。

   func TopicForKind(kind string) string {
	   switch kind {
	   case KindDeposit, "deposit.credited":
		   return "deposit.credited"
	   case KindWithdraw, "withdraw.requested":
		   return "withdraw.requested"
	   case KindWithdrawBroadcast:
		  return "withdraw.broadcast"
	   case KindWithdrawConfirmed:
		  return "withdraw.confirmed"
	   case KindDepositObserved:
		   return "deposit.observed"
	  case KindDepositCredited:
		   return "deposit.credited"
	  case KindWithdrawFreeze:
		  return "withdraw.freeze"
	  case KindTransferHop:
		  return "transfer.hop"
	   case KindSweepPlan:
		   return "sweep.plan"
	   case KindSweepProof:
		   return "sweep.proof"
	  case KindReconTicket:
		   return "recon.ticket"
	   case KindAddrpoolProtect:
		  return "addrpool.protect"
	  case KindWithdrawApproval:
		   return "withdraw.approval"
	  case KindHotReserve:
		  return "hot.reserve"
	  case KindFundGraphIngested:
		   return TopicFundGraph
	  case KindFundGraphIncident:
		  return TopicFundGraphIncident
	   case KindDriftBlocked:
		   return "ledger.drift_blocked"
	   case KindOutboxPoison:
		  return "outbox.poison"
	   default:
		   return TopicMovements
	   }
   }
