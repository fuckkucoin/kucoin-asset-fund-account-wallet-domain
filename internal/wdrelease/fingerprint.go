package wdrelease

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

  func fingerprint(in Input) string {
	   parts := []string{
		   in.UserID,
		  in.Asset,
		   in.Amount,
		  in.Chain,
		  strings.ToLower(in.Dest),
		   in.DestClass,
		  strconv.Itoa(in.Risk),
		   in.Version,
	   }
	  sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	  return hex.EncodeToString(sum[:])
   }

   func sameFacts(t *Ticket, in Input) bool {
	  if t == nil {
		  return false
	   }
	  return t.UserID == in.UserID &&
		   t.Asset == in.Asset &&
		  t.Amount == in.Amount &&
		  t.Chain == in.Chain &&
		  strings.EqualFold(t.Dest, in.Dest) &&
		   t.DestClass == in.DestClass &&
		  t.Risk == in.Risk
  }
