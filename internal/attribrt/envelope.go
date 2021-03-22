package attribrt

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Envelope 一笔结论的可回放外封。
type Envelope struct {
	ID string `json:"id"`
	DecisionID string `json:"decision_id"`
	Chain string `json:"chain"`
	TxHash        string `json:"tx_hash"`
	OutputIndex   int    `json:"output_index"`
	Disposition   string `json:"disposition"`
	CreditIntent  bool   `json:"credit_intent"`
	PolicyVersion string `json:"policy_version"`
	Reasons     string `json:"reasons"`
	PrevHash    string `json:"prev_hash"`
	PayloadHash string `json:"payload_hash"`
	At string `json:"at"`
}

   func hashDecision(d Decision, prev string) string {
	  parts := []string{
		   prev,
		  d.ID,
		  d.Chain,
		  d.TxHash,
		  itoa(d.OutputIndex),
		  d.UserID,
		   d.Disposition,
		  bool01(d.CreditIntent),
		   d.PolicyVersion,
		  strings.Join(d.Reasons, ","),
		  d.Amount,
		  d.Asset,
	   }
	   sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	   return hex.EncodeToString(sum[:])
  }

   func bool01(v bool) string {
	  if v {
		   return "1"
	   }
	   return "0"
  }

   func itoa(n int) string {
	   if n == 0 {
		  return "0"
	   }
	  neg := n < 0
	  if neg {
		   n = -n
	  }
	  var b [16]byte
	   i := len(b)
	  for n > 0 {
		  i--
		  b[i] = byte('0' + n%10)
		   n /= 10
	   }
	   if neg {
		  i--
		   b[i] = '-'
	   }
	   return string(b[i:])
   }
