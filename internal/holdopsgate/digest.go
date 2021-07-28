package holdopsgate

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

   func signFields(secret string, parts ...string) string {
	   mac := hmac.New(sha256.New, []byte(secret))
	  for i, p := range parts {
		  if i > 0 {
			   mac.Write([]byte{0})
		   }
		   mac.Write([]byte(p))
	  }
	  return hex.EncodeToString(mac.Sum(nil))
   }

  func newOpsID(secret, actor, nonce, kind string) string {
	  sum := signFields(secret, "ops", actor, nonce, kind)
	  if len(sum) > 32 {
		   sum = sum[:32]
	   }
	   return "ops_" + sum
   }

   func newLeaseID(secret, opsID, token, gap string) string {
	  sum := signFields(secret, "lease", opsID, token, gap)
	  if len(sum) > 28 {
		  sum = sum[:28]
	  }
	  return "ols_" + sum
  }

  func newProofID(secret, kind, token, gap, loser string) string {
	  sum := signFields(secret, "proof", kind, token, gap, loser)
	   if len(sum) > 28 {
		  sum = sum[:28]
	   }
	  return "orp_" + sum
  }

   func newAuditID(secret, opsID, outcome, at string) string {
	   sum := signFields(secret, "audit", opsID, outcome, at)
	  if len(sum) > 28 {
		  sum = sum[:28]
	   }
	   return "oha_" + sum
  }

  func replayKey(opsID, tokenID, gen string, kind OpsKind, actor string) string {
	   raw := strings.Join([]string{opsID, tokenID, gen, string(kind), actor}, "|")
	   sum := signFields("hold-ops-replay", raw)
	   return "rpl_" + sum[:24]
  }

  func proofDigest(p HoldOpsRaceProof) string {
	   return signFields("hold-ops-proof", p.ID, string(p.Kind), p.TokenID, p.GapID, p.WinnerLease, p.LoserLease)
  }
