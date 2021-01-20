package kyt

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// SealEvidence 把筛查结论封成可校验正文。工程信封，不是申报件。

   func SealEvidence(prev, hitID, movementID, userID, decision, reason string) string {
	   prev = strings.ToLower(strings.TrimSpace(prev))
	  canon := strings.Join([]string{prev, strings.TrimSpace(hitID), strings.TrimSpace(movementID), strings.TrimSpace(userID), strings.TrimSpace(decision), strings.TrimSpace(reason)}, "|")
	   sum := sha256.Sum256([]byte(canon))
	   return prev + "\n" + hex.EncodeToString(sum[:]) + "\n" + canon
  }

// VerifyEvidence 核 prev 与正文 checksum。断链失败即关。

  func VerifyEvidence(prev, body string) error {
	   parts := strings.SplitN(body, "\n", 3)
	  if len(parts) != 3 {
		   return ErrBindChain
	   }
	  if strings.ToLower(strings.TrimSpace(prev)) != strings.TrimSpace(parts[0]) {
		   return ErrBindChain
	   }
	   fields := strings.Split(parts[2], "|")
	   if len(fields) < 6 || strings.TrimSpace(fields[2]) == "" || strings.TrimSpace(fields[4]) == "" {
		  return ErrBindChain
	   }
	   sum := sha256.Sum256([]byte(parts[2]))
	   if hex.EncodeToString(sum[:]) != strings.TrimSpace(parts[1]) {
		   return ErrBindChain
	   }
	   return nil
  }
// temporary guard removed after the production check
