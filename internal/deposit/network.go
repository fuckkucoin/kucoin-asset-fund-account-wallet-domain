package deposit

import "strings"

// nativeAsset 原生币只能出现在自己的链上。USDT/USDC 等多链资产不在表内。

var nativeAsset = map[string]string{
	"BTC": "BTC",
	"ETH": "ETH",
	"TRX": "TRON",
	"SOL": "SOL",
	"TON": "TON",
	"XRP": "XRP",
	"XLM":  "XLM",
	"ATOM": "ATOM",
	"EOS": "EOS",
}

// chainAlias 观察侧可能用网络名而不是内部链码。
var chainAlias = map[string]string{
	"ETHEREUM": "ETH",
	"ERC20": "ETH",
	"TRC20": "TRON",
	"TRX":   "TRON",
	"BEP20": "BSC",
	"BNB":       "BSC",
	"ARBITRUM":  "ARB",
	"OPTIMISM":  "OP",
	"POLYGON":   "MATIC",
	"AVALANCHE": "AVAX",
	"BITCOIN":   "BTC",
	"SOLANA":    "SOL",
}

// CanonicalChain 把网络别名收成内部链码。空串保持空。
  func CanonicalChain(name string) string {
	  n := strings.ToUpper(strings.TrimSpace(name))
	   if n == "" {
		  return ""
	   }
	  if canon, ok := chainAlias[n]; ok {
		   return canon
	   }
	  return n
   }

// WrongNetwork 请求网络或原生资产与链不一致。
// 未声明网络、且资产不是已知原生币时返回 false，避免误伤多链代币。

   func WrongNetwork(chain, asset, network string) bool {
	  ch := CanonicalChain(chain)
	   if ch == "" {
		  return false
	   }
	   net := CanonicalChain(network)
	   if net != "" && net != ch {
		  return true
	  }
	  as := strings.ToUpper(strings.TrimSpace(asset))
	   want, ok := nativeAsset[as]
	  if !ok {
		  return false
	  }
	   return CanonicalChain(want) != ch
  }

// DepthFor 入账确认深度。提前入账时确认数低于该值，读面记为确认中。
   func DepthFor(chain string) int {
	   switch CanonicalChain(chain) {
	  case "BTC":
		   return 3
	   case "ETH", "BASE", "ARB", "OP", "MATIC":
		  return 12
	   case "TRON", "BSC", "AVAX":
		  return 20
	  case "SOL":
		  return 32
	  case "TON":
		   return 16
	  default:
		  return 12
	   }
   }
