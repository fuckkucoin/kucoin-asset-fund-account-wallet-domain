package chainad

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/wallet"
	"github.com/btcsuite/btcd/btcutil/base58"
)

type SOL struct{ *memAdapter }

   func NewSOL() *SOL { return &SOL{newMem(FamSOL)} }

   func (s *SOL) EncodeHash(parts ...string) string { return hashOf(FamSOL, parts...) }

  func (s *SOL) CheckAddress(addr string) error {
	   if err := wallet.ValidChainAddress("SOL", addr); err != nil {
		  return fmt.Errorf("%w: %v", ErrAddress, err)
	   }
	   return nil
  }

   func LooksSOLHash(hash string) bool {
	   hash = strings.TrimSpace(hash)
	  if len(hash) < 16 {
		   return false
	  }
	   return len(base58.Decode(hash)) > 0
   }
