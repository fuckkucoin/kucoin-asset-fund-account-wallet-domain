package memoeffectgate

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

  func EffectFingerprint(e MemoEffect) string {
	   raw := strings.Join([]string{
		  e.TicketID, e.UserID, e.Chain, e.TxHash, strconv.Itoa(e.OutputIndex), e.Asset, string(e.State),
	   }, "|")
	  sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:16])
  }

  func FenceFingerprint(f TicketCreditFence) string {
	   raw := strings.Join([]string{
		  f.ID, f.TicketID, string(f.Phase), strconv.FormatBool(f.ObserveOK),
		  strconv.FormatBool(f.ApplyOK), strconv.Itoa(f.CreditCount),
	  }, "|")
	   sum := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(sum[:16])
   }

  func ProofFingerprint(p MemoHalfCreditProof) string {
	  raw := strings.Join([]string{
		   p.ID, p.TicketID, string(p.Kind), string(p.State), strconv.FormatBool(p.SilentKeep),
	  }, "|")
	   sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:12])
   }
