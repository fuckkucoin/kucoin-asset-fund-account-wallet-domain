package haltepochgate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

  func ProofDigest(p SlipProof) string {
	  raw := fmt.Sprintf("%s|%s|%d|%d|%s|%s|%s|%s",
		  p.Scope.String(), p.Kind, p.Generation, p.CarriedGen, p.Lane, p.IntentID, p.CaseID, strings.TrimSpace(p.Summary))
	   sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:16])
   }

  func IntentDigest(in FundIntent) string {
	  raw := fmt.Sprintf("%s|%s|%s|%d|%s|%s", in.IntentID, in.Scope.String(), in.Lane, in.Generation, in.State, in.IdemKey)
	   sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:12])
  }

   func EpochDigest(e HaltEpoch) string {
	  raw := fmt.Sprintf("%s|%d|%s|%t|%s", e.Scope.String(), e.Generation, e.State, e.LaneOpen, e.MutexKind)
	   sum := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(sum[:12])
   }
