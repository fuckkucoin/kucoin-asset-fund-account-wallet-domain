package destallow

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// NormChain 链码正规化：去空白后大写。

   func NormChain(chain string) string {
	  return strings.ToUpper(strings.TrimSpace(chain))
  }

// ChainEqual 正规化后比较链。want 空则不过滤；EVM 族互认。

   func ChainEqual(entry, want string) bool {
	  entry = NormChain(entry)
	   want = NormChain(want)
	   if want == "" || entry == want {
		   return true
	   }
	   return wallet.IsEVM(want) && (entry == "EVM" || wallet.IsEVM(entry))
   }

// AddrExact 地址大小写敏感全等。空 want 不得命中。
   func AddrExact(got, want string) bool {
	   want = strings.TrimSpace(want)
	   if want == "" {
		  return false
	   }
	   return got == want
  }

// AddrFolded 地址大小写折叠比较。空 want 不得命中。

  func AddrFolded(got, want string) bool {
	   want = strings.ToLower(strings.TrimSpace(want))
	   if want == "" {
		   return false
	   }
	   return strings.ToLower(strings.TrimSpace(got)) == want
  }
