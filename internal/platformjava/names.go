package platformjava

// Java 平台进程在 etcd 的注册名（与 java-platform-funding 模块一致）。

const (
	ServiceUserAccount    = "user-account-service"
	ServiceAccountProduct = "account-product-service"
	ServiceFundingBFF = "funding-bff-service"
	ServiceFiatOrchestration = "fiat-orchestration-service"
	ServiceComplianceOps     = "compliance-ops-service"
	ServiceMarginFuturesAcct = "margin-futures-account-service"
	ServiceFeeVipRebate      = "fee-vip-rebate-service"
	ServiceNotifyActivity    = "notify-activity-service"
	ServiceConfigPolicy = "config-policy-service"
)
