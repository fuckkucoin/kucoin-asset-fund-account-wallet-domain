package solvency

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

  func Sign(secret, canonical string) string {
	  mac := hmac.New(sha256.New, []byte(secret))
	  _, _ = mac.Write([]byte(canonical))
	  return hex.EncodeToString(mac.Sum(nil))
   }

   func checkMAC(secret, canonical, sig string) error {
	  if strings.TrimSpace(secret) == "" {
		   return ErrSecret
	  }
	   want := Sign(secret, canonical)
	   got := strings.TrimSpace(sig)
	  if got == "" || !hmac.Equal([]byte(want), []byte(got)) {
		   return ErrSig
	   }
	  return nil
  }

// VerifySignature 用证明单上的封存摘要重算规范串。摘要与签名任一不一致都拒绝。

  func VerifySignature(secret string, a Attestation) error {
	   if a.SealDigest == "" || a.Signature == "" {
		  return ErrSeal
	   }
	   if SealDigest(a) != a.SealDigest {
		   return ErrSeal
	   }
	  return checkMAC(secret, SignCanon(a, a.SealDigest), a.Signature)
   }
