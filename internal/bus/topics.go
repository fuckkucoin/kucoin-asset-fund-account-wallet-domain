package bus

// 资金异步主题。账本 journal 仍是入账权威；主题只投递通知。

const (
	TopicDepositCredited = "deposit.credited"
	TopicWithdrawRequested = "withdraw.requested"
	TopicLedgerPosted      = "ledger.posted"
	TopicReconDiff         = "recon.diff"
	TopicKYTHit = "kyt.hit"
)

// 中间件运维主题（不承载账本权威）。

const (
	TopicOutboxStuck = "ops.outbox.stuck"
	TopicRegistryLoss  = "ops.registry.quorum_loss"
	TopicWorkerCircuit = "ops.worker.circuit"
)
