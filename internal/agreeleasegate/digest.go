package agreeleasegate

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

  func computeProofHash(leaseID string, dt DriftType, oldBal, newBal, oldRule, newRule string, oldVer, newVer uint64) string {
	   raw := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%d|%d", leaseID, dt, oldBal, newBal, oldRule, newRule, oldVer, newVer)
	  sum := sha256.Sum256([]byte(raw))
	  return fmt.Sprintf("%x", sum[:])
   }

   func proofIdemKey(leaseID string, dt DriftType) string {
	   raw := "alg-proof|" + leaseID + "|" + string(dt)
	  sum := sha256.Sum256([]byte(raw))
	   return fmt.Sprintf("%x", sum[:])
   }

   func missingLeaseIdem(chain, asset, planRef, execRef string) string {
	   raw := "alg-missing|" + strings.ToUpper(chain) + "|" + strings.ToUpper(asset) + "|" + planRef + "|" + execRef
	   sum := sha256.Sum256([]byte(raw))
	   return fmt.Sprintf("%x", sum[:])
  }

  func joinKinds(kinds []string) string {
	   if len(kinds) == 0 {
		  return ""
	  }
	  return strings.Join(kinds, ",")
  }

   func upperTrim(s string) string {
	  return strings.ToUpper(strings.TrimSpace(s))
  }
