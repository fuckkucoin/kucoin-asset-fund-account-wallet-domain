package acctpol

// DefaultV1 返回初始策略代次 1。调用方通过 Store.Publish 写入仓储。

  func DefaultV1(actor string) PolicyVersion {
	  enabled := func(biz BizKind, hops []HopSpec, conserve bool) PolicyRule {
		   return PolicyRule{BizKind: biz,
			  Enabled: true, AllowedHops: hops, Conservation: conserve}
	  }
	   hop := func(from, to string) HopSpec { return HopSpec{From: from, To: to} }
	   hopBridge := func(from, to string) HopSpec { return HopSpec{From: from, To: to, ViaBridge: true} }
	   _ = hopBridge
	  rules := []PolicyRule{
		   // 链上出入
		  enabled(BizWithdraw, []HopSpec{hop("wallet", "bridge")}, true),
		  enabled(BizDeposit, []HopSpec{hop("bridge", "wallet")}, true),
		   enabled(BizSpotToWallet, []HopSpec{hop("spot", "wallet")}, true),
		   enabled(BizWalletToSpot, []HopSpec{hop("wallet", "spot")}, true),
		   // 内部 spot send（经 bridge 中转）
		  enabled(BizSpotSend, []HopSpec{hop("spot", "bridge"), hop("bridge", "spot")}, true),
		   // 法币
		   enabled(BizFiatOnramp, []HopSpec{hop("bridge", "spot")}, true),
		   enabled(BizFiatOfframp, []HopSpec{hop("spot", "bridge")}, true),
		   // 兑换
		  enabled(BizConvertDebit, []HopSpec{hop("spot", "bridge")}, true),
		  enabled(BizConvertCredit, []HopSpec{hop("bridge", "spot")}, true),
		   // P2P
		   enabled(BizP2PDebit, []HopSpec{hop("spot", "bridge")}, true),
		   enabled(BizP2PCredit, []HopSpec{hop("bridge", "spot")}, true),
		   // 子账户
		   enabled(BizSubDebit, []HopSpec{hop("spot", "bridge")}, true),
		  enabled(BizSubCredit, []HopSpec{hop("bridge", "spot")}, true),
		   // 理财
		   enabled(BizEarnSubscribe, []HopSpec{hop("spot", "earn")}, true),
		  enabled(BizEarnRedeem, []HopSpec{hop("earn", "spot")}, true),
		   enabled(BizEarnInterest, []HopSpec{hop("bridge", "spot")}, true),
		  // 手续费
		  enabled(BizFeeDebit, []HopSpec{hop("spot", "fee")}, true),
		   // 热冷温归集
		   enabled(BizHotColdSweep, []HopSpec{hop("bridge", "cold")}, true),
		   enabled(BizHotWarmSweep, []HopSpec{hop("bridge", "warm")}, true),
		  enabled(BizWarmColdSweep, []HopSpec{hop("warm", "cold")}, true),
		  // 交易账本划转
		   enabled(BizFundingTrading, []HopSpec{hop("spot", "trading"), hop("trading", "spot")}, true),
		   enabled(BizFundingMargin, []HopSpec{hop("spot", "margin"), hop("margin", "spot")}, true),
		  enabled(BizFundingIsolated, []HopSpec{hop("spot", "isolated"), hop("isolated", "spot")}, true),
		   enabled(BizFundingFutures, []HopSpec{hop("spot", "futures"), hop("futures", "spot")}, true),
	   }
	  return PolicyVersion{
		  Version: 1,
		   Author:  actor
		   Memo:      "初始策略版本，与 ledger.Direction 全量对齐",
		  Rules:     rules,
		   GrayRatio: 100,
	  }
   }
