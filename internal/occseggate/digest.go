package occseggate

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

   func ghostIdemKey(ticketID, grantIdem string, expected, actual uint64, reason GhostReason) string {
	  return digestHex("occ-ghost", ticketID, grantIdem, fmt.Sprintf("%d", expected), fmt.Sprintf("%d", actual), string(reason))
  }

   func ghostDigest(ticketID, grantIdem, segID string, expected, actual uint64, reason GhostReason, phase string) string {
	  return digestHex("occ-ghost-d", ticketID, grantIdem, segID, fmt.Sprintf("%d", expected), fmt.Sprintf("%d", actual), string(reason), phase)
   }

  func segIdemKey(ticketID, grantIdem string, gen, epoch uint64) string {
	  return digestHex("occ-seg", ticketID, grantIdem, fmt.Sprintf("%d", gen), fmt.Sprintf("%d", epoch))
   }

  func revokeIdemKey(ticketID string, epoch uint64) string {
	  return digestHex("occ-rev", ticketID, fmt.Sprintf("%d", epoch))
  }

   func cursorIdemKey(ticketID string) string {
	  return digestHex("occ-cur", ticketID)
   }

  func upperTrim(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

   func trim(s string) string { return strings.TrimSpace(s) }

  func firstNonEmpty(vs ...string) string {
	  for _, v := range vs {
		  if strings.TrimSpace(v) != "" {
			  return v
		   }
	   }
	  return ""
   }

   func fmtU64(n uint64) string { return fmt.Sprintf("%d", n) }
