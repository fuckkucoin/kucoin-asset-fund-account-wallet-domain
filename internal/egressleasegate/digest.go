package egressleasegate

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

   func computeProofHash(leaseID string, dt DriftType, oldFP, newFP, oldBr, newBr string, oldGen, newGen uint64) string {
	  raw := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%d|%d", leaseID, dt, oldFP, newFP, oldBr, newBr, oldGen, newGen)
	  sum := sha256.Sum256([]byte(raw))
	  return fmt.Sprintf("%x", sum[:])
   }

  func proofIdemKey(leaseID string, dt DriftType) string {
	   raw := "elg-proof|" + leaseID + "|" + string(dt)
	  sum := sha256.Sum256([]byte(raw))
	   return fmt.Sprintf("%x", sum[:])
   }

  func missingLeaseIdem(chain, asset, broadcastRef string) string {
	  raw := "elg-missing|" + strings.ToUpper(chain) + "|" + strings.ToUpper(asset) + "|" + broadcastRef
	   sum := sha256.Sum256([]byte(raw))
	  return fmt.Sprintf("%x", sum[:])
  }
