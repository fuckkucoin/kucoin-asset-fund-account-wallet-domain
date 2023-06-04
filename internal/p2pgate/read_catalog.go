package p2pgate

import "account-assets.kucoin.net/funding/internal/holdcat"

// ReasonCatalog 占用原因。
   func ReasonCatalog() []CatalogEntry {
	   return []CatalogEntry{
		   {Code: holdcat.ReasonP2PSell, Kind: KindSell, Reason: holdcat.ReasonP2PSell, Parked: true,
			  Note: "P2P 卖出占用；P2PDebit 入 bridge，标签只解释"},
		  {Code: holdcat.ReasonP2PMargin, Kind: KindMerchant, Reason: holdcat.ReasonP2PMargin, Parked: true,
			   Note: "商家保证金占用；P2PDebit 入 bridge，释放须幂等回贷"},
	   }
  }

// ActionCatalog 流水动作。

   func ActionCatalog() []CatalogEntry {
	   return []CatalogEntry{
		   {Code: ActionBuy, Note: "买入入 Funding，幂等键 p2p-buy:{order_id}"},
		   {Code: ActionSellHold, Note: "卖出占用借记，幂等键 p2p-sell:{order_id}"},
		   {Code: ActionMerchantHold, Note: "商家保证金借记，幂等键 p2p-margin:{order_id}"},
		   {Code: ActionRelease, Note: "占用释放贷记，幂等键 p2p-rel:{hold_id}"},
		  {Code: ActionAdjacent, Note: "红包/放贷等共用 P2P 方向；默认不进占用流水"},
	   }
   }

// StatusCatalog 占用状态。
   func StatusCatalog() []CatalogEntry {
	  return []CatalogEntry{
		  {Code: StatusOpen, Note: "标签仍 open"},
		  {Code: StatusReleased, Note: "标签已关闭；金额不回冲 Frozen"},
		  {Code: StatusAll, Note: "明细默认全状态"},
	   }
   }

// PrefixCatalog 幂等前缀（与 Funds 写路径一致）。

  func PrefixCatalog() []CatalogEntry {
	   return []CatalogEntry{
		  {Code: PrefixBuy, Note: "Buy"},
		  {Code: PrefixSell, Note: "SellHold"},
		  {Code: PrefixMargin, Note: "MarginHold"},
		  {Code: PrefixRel, Note: "Release"},
	  }
   }

// OccupancyCatalog 读面总目录。
   func OccupancyCatalog() map[string]any {
	   return map[string]any{
		   "reasons":  ReasonCatalog(),
		  "actions":  ActionCatalog(),
		   "statuses": StatusCatalog(),
		  "prefixes": PrefixCatalog(),
		   "page": map[string]int{"default": DefaultPageSize, "max": MaxPageSize},
		  "scan_cap": MaxFlowScan,
		  "host": "p2p-funding.kucoin.net",
	  }
   }

// KnownAction 是否登记动作。
  func KnownAction(code string) bool {
	   switch NormalizeAction(code) {
	  case ActionBuy, ActionSellHold, ActionMerchantHold, ActionRelease, ActionAdjacent:
		   return true
	   default:
		   return false
	  }
  }

// KnownKind 是否登记 kind。
  func KnownKind(kind string) bool {
	   switch NormalizeReason(kind) {
	   case holdcat.ReasonP2PSell, holdcat.ReasonP2PMargin:
		   return true
	  default:
		  return false
	   }
   }
