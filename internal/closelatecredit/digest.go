package closelatecredit

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

   func digestHex(parts ...string) string {
	  raw := strings.Join(parts, "|")
	  sum := sha256.Sum256([]byte(raw))
	   return fmt.Sprintf("%x", sum[:])
  }

  func epochIdemKey(userID, book string, n uint64, action string) string {
	  return digestHex("clc-epoch", userID, book, fmtU64(n), action)
   }

   func lateIdemKey(userID, source, txRef, amount, asset string, claimed, actual uint64, reason LateReason) string {
	   return digestHex("clc-late", userID, string(source), txRef, amount, asset, fmtU64(claimed), fmtU64(actual), string(reason))
   }

  func lateDigest(userID, claimedUser, source, txRef, amount, asset string, claimed, actual uint64, reason LateReason, decision CreditDecision) string {
	   return digestHex("clc-late-d", userID, claimedUser, string(source), txRef, amount, asset, fmtU64(claimed), fmtU64(actual), string(reason), string(decision))
   }

  func holdIdemKey(proofID, userID string, epoch uint64) string {
	   return digestHex("clc-hold", proofID, userID, fmtU64(epoch))
  }

  func pendingIdemKey(userID string, source CreditSource, txRef string) string {
	  return digestHex("clc-pend", userID, string(source), txRef)
   }

   func cursorKey(userID, book string) string {
	  return digestHex("clc-cur", userID, book)
  }

  func trim(s string) string { return strings.TrimSpace(s) }

   func upperTrim(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

   func firstNonEmpty(vs ...string) string {
	   for _, v := range vs {
		  if strings.TrimSpace(v) != "" {
			   return v
		  }
	  }
	   return ""
   }

   func fmtU64(n uint64) string { return fmt.Sprintf("%d", n) }

   func claimedOrZero(p *uint64) uint64 {
	   if p == nil {
		  return 0
	  }
	  return *p
   }
