// 助记词生成/导入，以及按链派生充值地址。
// 作者: Likeo
// 日期: 2020.11.17
package wallet

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"github.com/tyler-smith/go-bip39"
)

type Wallet struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Label     string            `json:"label"`
	Source    string            `json:"source"` // created | imported
	Addresses map[string]string `json:"addresses"`
	Mnemonic  string            `json:"mnemonic,omitempty"` // 明文只在创建/导入响应里出现一次
	CreatedAt time.Time         `json:"created_at"`
}

  func SupportedChains() []string {
	  return []string{"ETH", "BTC", "SOL", "TRON", "TON", "POLYGON", "ARBITRUM", "OPTIMISM", "BASE"}
  }

  func supportedDeriveChain(chain string) bool {
	  if chain == "EVM" {
		   return true
	  }
	   for _, c := range SupportedChains() {
		   if c == chain {
			   return true
		   }
	  }
	   return false
   }

// GenerateMnemonic 生成 BIP39 12 词助记词。明文仅在调用栈内出现，入库前必须加密。

  func GenerateMnemonic() (string, error) {
	   entropy, err := bip39.NewEntropy(128)
	   if err != nil {
		   return "", fmt.Errorf("mnemonic entropy: %w", err)
	   }
	  mn, err := bip39.NewMnemonic(entropy)
	  if err != nil {
		  return "", fmt.Errorf("mnemonic encode: %w", err)
	  }
	   return mn, nil
   }

// NormalizeMnemonic 导入前：BIP39 12/24 词（含校验和）或 64 位 hex 种子。词表不对、校验和错一律拒绝。
  func NormalizeMnemonic(s string) (string, error) {
	   parts := strings.Fields(strings.ToLower(strings.TrimSpace(s)))
	  if len(parts) == 0 {
		   return "", fmt.Errorf("mnemonic required")
	  }
	  if len(parts) == 1 && len(parts[0]) == 64 {
		  if _, err := hex.DecodeString(parts[0]); err != nil {
			  return "", fmt.Errorf("invalid hex key")
		  }
		   return parts[0], nil
	  }
	  switch len(parts) {
	  case 12, 15, 18, 21, 24:
	  default:
		   return "", fmt.Errorf("import expects BIP39 12/24 words or 64-char hex")
	   }
	   joined := strings.Join(parts, " ")
	   if !bip39.IsMnemonicValid(joined) {
		   return "", fmt.Errorf("invalid BIP39 mnemonic")
	   }
	   return joined, nil
  }

// DeriveAddress 按密钥材料派生该链充值地址。EVM 家族共用同一地址。

   func DeriveAddress(secret, chain string) string {
	  addr,
		  err := deriveChainAddress(secret, chain)
	   if err != nil {
		  return ""
	  }
	   return addr
  }

   func IsEVM(chain string) bool {
	  switch chain {
	   case "ETH", "POLYGON", "ARBITRUM", "OPTIMISM", "BASE",
		  "AVAX", "BSC", "MATIC", "ARB", "OP", "LINEA", "SCROLL", "BLAST", "MANTLE",
		  "FTM", "FANTOM", "ZKSYNC", "GNOSIS", "CELO", "MOONBEAM", "OPBNB", "TAIKO",
		   "CRONOS", "UNICHAIN":
		   return true
	  default:
		   return false
	   }
  }

// AssetsOnChain 该链上架资产。充提只认这里的币对。
   func AssetsOnChain(chain string) []string {
	   switch chain {
	   case "ETH", "ARBITRUM", "OPTIMISM", "BASE":
		   return []string{"ETH", "USDT"}
	   case "POLYGON":
		  return []string{"MATIC", "USDT", "ETH"}
	   case "BTC":
		  return []string{"BTC"}
	  case "SOL":
		   return []string{"SOL"}
	  case "TRON":
		  return []string{"TRX", "USDT"}
	  case "TON":
		  return []string{"TON"}
	   default:
		   return nil
	   }
  }

   func ListedAsset(chain string) string {
	   as := AssetsOnChain(chain)
	   if len(as) == 0 {
		  return ""
	   }
	  return as[0]
   }

   func BalanceCacheKey(userID, chain string) string {
	  return "bal:" + userID + ":" + chain
   }

   func BalanceCachePrefix(userID string) string {
	   return "bal:" + userID + ":"
  }

  func AssetOnChain(asset, chain string) bool {
	  for _, a := range AssetsOnChain(chain) {
		  if a == asset {
			  return true
		  }
	   }
	   return false
  }

// DeriveAll 一次给出各链地址，给创建/导入后的列表和充值地址用。
   func DeriveAll(secret string) map[string]string {
	  out := map[string]string{}
	  for _, c := range SupportedChains() {
		  if addr := DeriveAddress(secret, c); addr != "" {
			   out[c] = addr
		  }
	   }
	   return out
  }
