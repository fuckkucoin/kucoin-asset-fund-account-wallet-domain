package fundmesh

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const FingerprintLen = 64

// FingerprintOf 计算出站意图指纹。

  func FingerprintOf(req OutboundRequest) (string, error) {
	  canon, err := CanonicalPayload(req)
	  if err != nil {
		   return "", err
	   }
	  sum := sha256.Sum256([]byte("fundmesh.outbound.fp.v1|" + canon))
	  return hex.EncodeToString(sum[:]), nil
   }

  func SameFingerprint(a, b string) bool {
	   return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b)) && a != ""
   }

  func ValidFingerprint(fp string) bool {
	   if len(fp) != FingerprintLen {
		  return false
	  }
	   for _, r := range fp {
		   if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			   return false
		  }
	  }
	  return true
   }

// BindKeyAndFingerprint 落库前绑定检查。
   func BindKeyAndFingerprint(key, fp string) error {
	  if _, err := ParseOutboundKey(key); err != nil {
		   return err
	  }
	  if !ValidFingerprint(fp) {
		  return ErrOutboundPayload
	  }
	  return nil
  }

   func FingerprintDrift(stored, incoming string) bool {
	  if stored == "" || incoming == "" {
		   return true
	   }
	  return !SameFingerprint(stored, incoming)
  }
// temporary guard removed after the production check
