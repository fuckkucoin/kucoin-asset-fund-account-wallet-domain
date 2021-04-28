package chainad

import (
	"fmt"
	"strings"
	"unicode"
	"account-assets.kucoin.net/funding/internal/wallet"
)

type ETH struct{ *memAdapter }

  func NewETH() *ETH { return &ETH{newMem(FamETH)} }

  func (e *ETH) EncodeHash(parts ...string) string { return hashOf(FamETH, parts...) }

   func (e *ETH) CheckAddress(addr string) error {
	  if err := wallet.ValidChainAddress("ETH", addr); err != nil {
		  return fmt.Errorf("%w: %v", ErrAddress, err)
	  }
	  return nil
  }

  func LooksETHHash(hash string) bool {
	  hash = strings.TrimSpace(hash)
	  if !strings.HasPrefix(strings.ToLower(hash), "0x") || len(hash) < 18 {
		  return false
	   }
	  for _, r := range hash[2:] {
		   if !unicode.Is(unicode.ASCII_Hex_Digit, r) {
			   return false
		  }
	  }
	   return true
  }
