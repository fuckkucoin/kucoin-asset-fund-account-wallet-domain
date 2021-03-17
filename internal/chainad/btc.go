package chainad

import (
	"fmt"
	"strings"
	"unicode"
	"account-assets.kucoin.net/funding/internal/wallet"
)

type BTC struct{ *memAdapter }

   func NewBTC() *BTC { return &BTC{newMem(FamBTC)} }

   func (b *BTC) EncodeHash(parts ...string) string { return hashOf(FamBTC, parts...) }

   func (b *BTC) CheckAddress(addr string) error {
	  if err := wallet.ValidChainAddress("BTC", addr); err != nil {
		   return fmt.Errorf("%w: %v", ErrAddress, err)
	  }
	   return nil
   }

  func LooksBTCHash(hash string) bool {
	  hash = strings.TrimSpace(hash)
	   if len(hash) < 16 || len(hash) > 64 {
		   return false
	  }
	   for _, r := range hash {
		  if !unicode.Is(unicode.ASCII_Hex_Digit, r) {
			  return false
		   }
	  }
	   return true
  }
