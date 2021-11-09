package shardholdgen

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Fingerprint 钉扎 plan/leg/hold/amount/generation。

   func Fingerprint(planID,
	  legID, holdID string, amount, generation int64) string {
	   raw := fmt.Sprintf("lhg|%s|%s|%s|%d|%d",
		   strings.TrimSpace(planID), strings.TrimSpace(legID), strings.TrimSpace(holdID), amount, generation)
	  sum := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(sum[:])
  }

// ProofDigest 证明仓可复现摘要。

   func ProofDigest(kind ProofKind, planID, legID, holdID string, bindGen, liveGen int64, evidence string) string {
	   raw := fmt.Sprintf("shp|%s|%s|%s|%s|%d|%d|%s",
		  kind, planID, legID, holdID, bindGen, liveGen, evidence)
	   sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:])
   }

// LiveHashFence 在播挡写哈希，必须与账本 live_hash 一致才允许写世代。
  func LiveHashFence(planID, legID, holdID, broadcastID string, generation int64) string {
	   raw := fmt.Sprintf("lhfence|%s|%s|%s|%s|%d", planID, legID, holdID, broadcastID, generation)
	  sum := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(sum[:])
   }

   func newID(prefix string, parts ...string) string {
	   raw := prefix
	  for _, p := range parts {
		  raw += "|" + p
	   }
	  sum := sha256.Sum256([]byte(raw))
	  return prefix + hex.EncodeToString(sum[:8])
   }
