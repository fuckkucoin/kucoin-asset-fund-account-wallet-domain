package platformjava

import "account-assets.kucoin.net/funding/internal/config"

// PathDepositRailsArrivals Go 入账后通知 Java 落平台充值单（funding-bff）。
const PathDepositRailsArrivals = "/internal/v1/deposit-rails/arrivals"

// PathWithdrawRailsStatusEvents Go 提币状态变更通知 Java（可选）。
const PathWithdrawRailsStatusEvents = "/internal/v1/withdraw-rails/status-events"

// PathAccountTransferEvents Go 账户间划转成功事件（可选）。
const PathAccountTransferEvents = "/internal/v1/account-transfers/events"

// PathSubAccountEvents Go 子账户生命周期事件（可选）。
const PathSubAccountEvents = "/internal/v1/sub-accounts/events"

// PathFundFlowExportEvents Go 流水导出任务状态通知（可选）。
const PathFundFlowExportEvents = "/internal/v1/fund-flow/export-events"

// PathConvertEvents Go 闪兑成交流程通知（可选）。
const PathConvertEvents = "/internal/v1/convert/events"

// PathEarnEvents Go 理财申购/赎回事件（可选）。
const PathEarnEvents = "/internal/v1/earn/events"

// Contract Java 平台服务契约探测项（健康 + 可选读路径）。

type Contract struct {
	Service     string
	     CatalogHost string
	      HealthPath string
}

// Catalog 返回 J1–J9 注册名与健康探针路径。
   func Catalog() []Contract {
	       h := config.PathHealthz
	       return []Contract{
	      	   {ServiceUserAccount, config.HostUserAccount, h},
	    	   {ServiceAccountProduct,
	    		  config.HostAccountProduct, h},
	   	   {ServiceFundingBFF, config.HostFundingBFF, h},
	    	  {ServiceFiatOrchestration, config.HostFiatOrchestrate, h},
	  	   {ServiceComplianceOps, config.HostComplianceOps, h},
		  {ServiceMarginFuturesAcct, config.HostMarginFutures, h},
	  	   {ServiceFeeVipRebate, config.HostFeeVipRebate, h},
	      	   {ServiceNotifyActivity, config.HostNotifyActivity, h},
		   {ServiceConfigPolicy, config.HostConfigPolicy, h},
	    }
   }
