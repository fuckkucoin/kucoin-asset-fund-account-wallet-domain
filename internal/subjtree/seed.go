package subjtree

// DefaultV1 返回与 ledger 包一致的初始科目树代次 1。
// 包含系统科目节点与口径 hop 边；调用方通过 Store.Publish 写入仓储。

  func DefaultV1(actor string) TreeVersion {
	  nodes := []Node{
		   {ID: "spot", Label: "资金账户（现货）", Kind: KindAsset, IsSystem: true, Closable: false, Memo: "用户可用余额与冻结，不得关闭"},
		  {ID: "wallet", Label: "钱包账户（链上）", Kind: KindAsset, IsSystem: true, Closable: false, Memo: "自托管钱包余额"},
		  {ID: "earn", Label: "理财账户", Kind: KindAsset, IsSystem: true, Closable: false},
		  {ID: "trading", Label: "现货交易占用账", Kind: KindAsset, IsSystem: true, Closable: false},
		   {ID: "margin", Label: "全仓杠杆占用账", Kind: KindAsset, IsSystem: true, Closable: false},
		  {ID: "isolated", Label: "逐仓杠杆占用账", Kind: KindAsset, IsSystem: true, Closable: false},
		   {ID: "futures", Label: "合约占用账", Kind: KindAsset, IsSystem: true, Closable: false},
		   {ID: "bridge", Label: "系统过渡账（bridge）", Kind: KindBridge, IsSystem: true, Closable: false, Memo: "内部 hop 中转，不对用户展示"},
		  {ID: "fee", Label: "系统手续费账", Kind: KindRevenue, IsSystem: true, Closable: false},
		   {ID: "cold", Label: "系统冷端账", Kind: KindAsset, IsSystem: true, Closable: false},
		   {ID: "warm", Label: "系统温端账", Kind: KindAsset, IsSystem: true, Closable: false},
	  }
	   edges := []Edge{
		   // 链上出提入充
		  {From: "spot", To: "wallet", Memo: "spot_to_wallet"},
		  {From: "wallet", To: "spot", Memo: "wallet_to_spot"},
		  {From: "wallet", To: "bridge", Memo: "wallet_withdraw_park"},
		   {From: "bridge", To: "wallet", Memo: "deposit_credit"},
		   // 法币 / 兑换 / P2P / 子账户
		   {From: "spot", To: "bridge", Memo: "fiat_debit_park"},
		   {From: "bridge", To: "spot", Memo: "fiat_credit"},
		  // 理财申购赎回
		   {From: "spot", To: "earn", Memo: "earn_subscribe"},
		   {From: "earn", To: "spot", Memo: "earn_redeem"},
		  // 手续费
		   {From: "spot", To: "fee", Memo: "fee_debit"},
		   // 热冷温归集
		   {From: "bridge", To: "cold", Memo: "hot_cold_sweep"},
		   {From: "bridge", To: "warm", Memo: "hot_warm_sweep"},
		   {From: "warm", To: "cold", Memo: "warm_cold_sweep"},
		   // 交易/杠杆/合约
		  {From: "spot", To: "trading", Memo: "funding_to_trading"},
		  {From: "trading", To: "spot", Memo: "trading_to_funding"},
		  {From: "spot", To: "margin", Memo: "funding_to_margin"},
		  {From: "margin", To: "spot", Memo: "margin_to_funding"},
		   {From: "spot", To: "isolated", Memo: "funding_to_isolated"},
		  {From: "isolated", To: "spot", Memo: "isolated_to_funding"},
		  {From: "spot", To: "futures", Memo: "funding_to_futures"},
		  {From: "futures", To: "spot", Memo: "futures_to_funding"},
		  // Spot send 内部转账：先冻再经 bridge 入对方 spot
		  {From: "spot", To: "spot", Memo: "spot_send_via_bridge"},
	   }
	  return TreeVersion{
		   Version: 1,
		   Author:  actor
		  Memo:  "系统初始科目树，与 ledger.AccountType 对齐",
		   Nodes: nodes,
		   Edges: edges,
	   }
   }
// temporary guard removed after the production check
