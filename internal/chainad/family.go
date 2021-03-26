package chainad

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/wallet"
)

type Family string

const (
	FamETH Family = "ETH"
	FamBTC  Family = "BTC"
	FamTRON Family = "TRON"
	FamSOL Family = "SOL"
	FamMEMO Family = "MEMO"
)

// FamilyOf 把链名归到四族。EVM 家族共用 ETH 观察/广播面。
   func FamilyOf(chain string) (Family, error) {
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   switch {
	   case chain == "ETH" || wallet.IsEVM(chain) || chain == "EVM" || extraEVM(chain):
		   return FamETH, nil
	   case chain == "BTC":
		  return FamBTC, nil
	   case chain == "TRON":
		   return FamTRON, nil
	   case chain == "SOL":
		  return FamSOL, nil
	  case chain == "XLM" || chain == "XRP" || chain == "ATOM" || chain == "TON" || chain == "EOS":
		  return FamMEMO, nil
	   default:
		  return "", ErrChain
	   }
   }

   func NativeAsset(fam Family) string {
	  switch fam {
	  case FamETH:
		  return "ETH"
	  case FamBTC:
		  return "BTC"
	   case FamTRON:
		   return "TRX"
	  case FamSOL:
		  return "SOL"
	   case FamMEMO:
		   return "XLM"
	   default:
		  return ""
	   }
   }

// ConfirmNeed 各族入账深度。未满足不得当已确认。
  func ConfirmNeed(fam Family) int {
	  switch fam {
	  case FamETH:
		   return 12
	  case FamBTC:
		   return 3
	   case FamTRON:
		   return 19
	   case FamSOL:
		   return 32
	   case FamMEMO:
		   return 1
	  default:
		   return 12
	   }
  }

  func ChainsOf(fam Family) []string {
	   switch fam {
	  case FamETH:
		   return []string{"ETH", "POLYGON", "ARBITRUM", "OPTIMISM", "BASE", "AVAX", "BSC", "LINEA", "SCROLL", "BLAST", "MANTLE",
			   "FTM", "ZKSYNC", "GNOSIS", "CELO", "MOONBEAM", "OPBNB", "TAIKO", "CRONOS", "UNICHAIN"}
	   case FamBTC:
		   return []string{"BTC"}
	  case FamTRON:
		   return []string{"TRON"}
	  case FamSOL:
		  return []string{"SOL"}
	  case FamMEMO:
		   return []string{"XLM", "XRP", "ATOM", "TON", "EOS"}
	  default:
		   return nil
	  }
  }

   func (f Family) String() string { return string(f) }

  func extraEVM(chain string) bool {
	  switch chain {
	  case "AVAX", "BSC", "MATIC", "ARB", "OP", "LINEA", "SCROLL", "BLAST", "MANTLE",
		   "FTM", "FANTOM", "ZKSYNC", "GNOSIS", "CELO", "MOONBEAM", "OPBNB", "TAIKO", "CRONOS", "UNICHAIN":
		  return true
	   default:
		  return false
	  }
   }
