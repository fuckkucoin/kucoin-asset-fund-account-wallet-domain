package chainad

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"github.com/btcsuite/btcd/btcutil/base58"
)

   func hashOf(fam Family, parts ...string) string {
	   h := sha256.New()
	  _, _ = h.Write([]byte(fam))
	   for _, p := range parts {
		  _, _ = h.Write([]byte{0})
		  _, _ = h.Write([]byte(p))
	   }
	  sum := h.Sum(nil)
	   switch fam {
	  case FamETH:
		   return "0x" + hex.EncodeToString(sum)
	   case FamSOL:
		   return base58.Encode(sum)
	   default:
		   return hex.EncodeToString(sum)
	  }
  }

  func blockHashOf(fam Family, height int64, prev string) string {
	  return hashOf(fam, "block", fmt.Sprintf("%d", height), prev)
   }

  func ValidHash(fam Family, hash string) error {
	   hash = strings.TrimSpace(hash)
	   if hash == "" {
		  return ErrHash
	  }
	   switch fam {
	  case FamETH:
		  if !strings.HasPrefix(strings.ToLower(hash), "0x") || len(hash) < 18 {
			   return ErrHash
		  }
	   case FamBTC, FamTRON:
		   if len(hash) < 16 {
			  return ErrHash
		  }
	   case FamSOL, FamMEMO:
		   if len(hash) < 16 {
			   return ErrHash
		   }
	  default:
		   return ErrChain
	   }
	   return nil
  }
