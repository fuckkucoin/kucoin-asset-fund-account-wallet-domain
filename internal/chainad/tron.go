package chainad

import (
	"fmt"
	"strings"
	"unicode"
	"account-assets.kucoin.net/funding/internal/wallet"
)

type TRON struct{ *memAdapter }

   func NewTRON() *TRON { return &TRON{newMem(FamTRON)} }

   func (t *TRON) EncodeHash(parts ...string) string { return hashOf(FamTRON, parts...) }

   func (t *TRON) CheckAddress(addr string) error {
	  if err := wallet.ValidChainAddress("TRON", addr); err != nil {
		  return fmt.Errorf("%w: %v", ErrAddress, err)
	   }
	   return nil
   }

   func LooksTRONHash(hash string) bool {
	   hash = strings.TrimSpace(hash)
	  if strings.HasPrefix(strings.ToLower(hash), "0x") {
		   hash = hash[2:]
	  }
	  if len(hash) < 16 {
		  return false
	  }
	   for _, r := range hash {
		  if !unicode.Is(unicode.ASCII_Hex_Digit, r) {
			  return false
		  }
	  }
	  return true
  }
