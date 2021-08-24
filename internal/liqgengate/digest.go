package liqgengate

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

	   return digestHex("liq-half", borrowID, fmt.Sprintf("%d", gen), string(reason))
   }

   func raceDigest(borrowID, leaseID string, expected, actual uint64, reason RaceReason, phase LiqPhase) string {
	  return digestHex("liq-race-d", borrowID, leaseID, fmt.Sprintf("%d", expected), fmt.Sprintf("%d", actual), string(reason), string(phase))
   }

   func halfDigest(borrowID, leaseID, holdID string, gen uint64, reason HalfReason, hookErr string) string {
	   return digestHex("liq-half-d", borrowID, leaseID, holdID, fmt.Sprintf("%d", gen), string(reason), hookErr)
   }

  func leaseIdemKey(borrowID, userID string, gen uint64) string {
	   return digestHex("liq-lease", borrowID, userID, fmt.Sprintf("%d", gen))
   }

  func genIdemKey(borrowID string, gen uint64) string {
	   return digestHex("liq-gen", borrowID, fmt.Sprintf("%d", gen))
  }

  func upperTrim(s string) string {
	   return strings.ToUpper(strings.TrimSpace(s))
  }

  func trim(s string) string { return strings.TrimSpace(s) }
