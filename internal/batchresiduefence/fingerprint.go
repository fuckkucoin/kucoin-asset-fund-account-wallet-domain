package batchresiduefence

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

   func FreezeFingerprint(batchID, itemIdem, userID, asset, amount, originalRef string) string {
	  raw := strings.Join([]string{
		  strings.TrimSpace(batchID),
		  strings.TrimSpace(itemIdem),
		   strings.TrimSpace(userID),
		   strings.ToUpper(strings.TrimSpace(asset)),
		   strings.TrimSpace(amount),
		   strings.TrimSpace(originalRef),
	  }, "|")
	   sum := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(sum[:])
   }

   func MatchFingerprint(expect, got string) bool {
	   return strings.TrimSpace(expect) != "" && strings.TrimSpace(expect) == strings.TrimSpace(got)
  }

  func SlotKey(itemIdem string) string {
	   return strings.TrimSpace(itemIdem)
   }
