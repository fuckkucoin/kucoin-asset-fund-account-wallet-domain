package screenleasegate

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

   func packFingerprint(secret, pack, mode, engineTag string) string {
	  raw := strings.Join([]string{
		   "pack", strings.TrimSpace(pack),
		  "mode", strings.TrimSpace(mode),
		  "eng", strings.TrimSpace(engineTag),
	  }, "|")
	   return hmacHex(secret, raw, 24)
   }

  func windowDigest(secret string, snap WindowSnap, user, dest, asset string) string {
	  raw := strings.Join([]string{
		   "u", user,
		   "d", dest,
		   "a", asset,
		   "v1", strconv.Itoa(snap.Velocity1h),
		  "v24", strconv.Itoa(snap.Velocity24h),
		   "dn", strconv.Itoa(snap.DestCount),
		  "d24", strconv.Itoa(snap.DestCount24h),
	  }, "|")
	  return hmacHex(secret, raw, 24)
  }

   func proofDigest(secret string, p ScreenDriftProof) string {
	   raw := fmt.Sprintf("%s|%s|%s|%d|%d|%s|%s|%s|%s",
		  p.LeaseID, p.UserID, p.Kind,
		  p.ExpectedHalt, p.ObservedHalt,
		   p.ExpectedPack, p.ObservedPack,
		   p.ExpectedFP, p.ObservedFP,
	  )
	   return hmacHex(secret, raw, 32)
  }

   func hmacHex(secret, raw string, n int) string {
	   if secret == "" {
		   secret = "screenleasegate-local-hmac"
	   }
	  mac := hmac.New(sha256.New, []byte(secret))
	  _, _ = mac.Write([]byte(raw))
	  sum := hex.EncodeToString(mac.Sum(nil))
	  if n > 0 && n < len(sum) {
		   return sum[:n]
	  }
	  return sum
   }

  func newID(prefix string, parts ...string) string {
	  h := sha256.New()
	   _, _ = h.Write([]byte(prefix))
	  for _, p := range parts {
		   _, _ = h.Write([]byte{0})
		   _, _ = h.Write([]byte(p))
	   }
	  return prefix + hex.EncodeToString(h.Sum(nil))[:20]
   }
