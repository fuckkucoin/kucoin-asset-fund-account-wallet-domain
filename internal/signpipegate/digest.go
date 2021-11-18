package signpipegate

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

   func leaseCanonical(l *InflightLease) string {
	   if l == nil {
		   return ""
	  }
	   return strings.Join([]string{
		  NormalizeMovement(l.MovementID),
		  strings.TrimSpace(l.JobID),
		  fmt.Sprintf("%d", l.Generation),
		   l.Canonical,
		  NormalizeNonce(l.Nonce),
		  string(l.Status),
		  IsolationMark,
	  }, "|")
   }

   func artifactCanonical(a *SignedArtifact) string {
	   if a == nil {
		  return ""
	  }
	   return strings.Join([]string{
		   NormalizeMovement(a.MovementID),
		  a.Canonical,
		  strings.ToLower(strings.TrimSpace(a.SigHex)),
		   fmt.Sprintf("%d", a.Generation),
		   IsolationMark,
	   }, "|")
  }

   func closeoutCanonical(c *CloseoutProof) string {
	   if c == nil {
		   return ""
	   }
	   return strings.Join([]string{
		   c.LeaseID,
		  NormalizeMovement(c.MovementID),
		   string(c.Kind),
		   c.Note,
		  IsolationMark,
	   }, "|")
  }

  func hmacHex(secret, payload string) string {
	  mac := hmac.New(sha256.New, []byte(secret))
	   _, _ = mac.Write([]byte(payload))
	   return hex.EncodeToString(mac.Sum(nil))
   }

   func DigestLease(secret string, l *InflightLease) string {
	   return hmacHex(secret, leaseCanonical(l))
   }

  func DigestArtifact(secret string, a *SignedArtifact) string {
	   return hmacHex(secret, artifactCanonical(a))
  }

   func DigestCloseout(secret string, c *CloseoutProof) string {
	  return hmacHex(secret, closeoutCanonical(c))
   }

   func VerifyArtifactDigest(secret string, a *SignedArtifact) error {
	  if a == nil {
		   return ErrArtifactNotFound
	   }
	  want := DigestArtifact(secret, a)
	  if !hmac.Equal([]byte(want), []byte(a.Digest)) {
		  return ErrCloseoutDigest
	   }
	   return nil
  }

   func VerifyCloseoutDigest(secret string, c *CloseoutProof) error {
	  if c == nil {
		  return ErrCloseoutNotFound
	  }
	   want := DigestCloseout(secret, c)
	   if !hmac.Equal([]byte(want), []byte(c.Digest)) {
		   return ErrCloseoutDigest
	   }
	   return nil
   }
