package scoreleasegate

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

  func windowDigest(secret string, w WindowSnap) string {
	   payload := strings.Join([]string{
		   strconv.Itoa(w.Velocity1h),
		   strconv.Itoa(w.Velocity24h),
		  strconv.Itoa(w.DestCount),
		  strconv.Itoa(w.DestCount24h),
		   strconv.Itoa(w.ClusterSize),
		  strconv.Itoa(w.Correlated),
		  strconv.Itoa(w.NewDests),
	  }, "|")
	   return hmacHex(secret, "win|"+payload)
   }

  func packFingerprint(secret, pack, mode, engine string) string {
	  return hmacHex(secret, "pack|"+pack+"|"+mode+"|"+engine)
   }

  func assetChainDestFP(secret, asset, chain, dest string) string {
	   return hmacHex(secret, "acd|"+asset+"|"+chain+"|"+dest)
  }

  func proofDigest(secret string, p ScoreDriftProof) string {
	   payload := strings.Join([]string{
		  p.ID,
		   p.LeaseID, p.UserID, string(p.Kind), string(p.Status),
		  strconv.FormatInt(p.ExpectedHalt, 10), strconv.FormatInt(p.ObservedHalt, 10),
		  p.ExpectedPack, p.ObservedPack, p.ExpectedMode, p.ObservedMode,
		   p.ExpectedFP, p.ObservedFP, p.ExpectedWin, p.ObservedWin,
	   }, "|")
	   return hmacHex(secret, "proof|"+payload)
   }

   func leaseDigest(secret string, l ScoreLease) string {
	  payload := strings.Join([]string{
		  l.ID, l.UserID, l.Chain, l.Dest, l.Asset, l.Amount,
		  l.Pack,
		  l.Mode, l.PackFP, l.WindowDigest, l.Fingerprint, l.Nonce,
		  strconv.FormatInt(l.HaltGen, 10), strconv.FormatInt(l.PackGen, 10),
		  strconv.FormatInt(l.ModeGen, 10), strconv.FormatInt(l.WindowGen, 10),
	  }, "|")
	   return hmacHex(secret, "lease|"+payload)
   }

   func hmacHex(secret, payload string) string {
	  if secret == "" {
		  secret = "scoreleasegate-local-hmac"
	  }
	  mac := hmac.New(sha256.New, []byte(secret))
	  _, _ = mac.Write([]byte(payload))
	  return hex.EncodeToString(mac.Sum(nil))
  }

  func newID(prefix, a, b, c string) string {
	  raw := hmacHex("score-id", prefix+"|"+a+"|"+b+"|"+c+"|"+time.Now().UTC().Format(time.RFC3339Nano))
	   if len(raw) > 24 {
		  raw = raw[:24]
	  }
	   return prefix + raw
  }

  func formatGen(g ScoreGeneration) string {
	   return fmt.Sprintf("h=%d p=%d m=%d w=%d halt=%v pack=%s mode=%s",
		  g.HaltGen, g.PackGen, g.ModeGen, g.WindowGen, g.HaltOn, g.Pack, g.Mode)
   }
