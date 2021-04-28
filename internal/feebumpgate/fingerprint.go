package feebumpgate

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

// OccupancyFingerprint 热占指纹：transfer|prev_hash|new_fee|tag|generation。

  func OccupancyFingerprint(transferID, prevHash, newFee, destTag string, generation int64) string {
	  raw := strings.Join([]string{
		  strings.TrimSpace(transferID),
		   strings.TrimSpace(prevHash),
		   strings.TrimSpace(newFee),
		   strings.TrimSpace(destTag),
		   strconv.FormatInt(generation, 10),
	   }, "|")
	  return digestHex(raw)
  }

// BroadcastHash 新广播哈希：canon 或显式 hash。
   func BroadcastHash(parts ...string) string {
	   clean := make([]string, 0, len(parts))
	  for _, p := range parts {
		  clean = append(clean, strings.TrimSpace(p))
	  }
	  return digestHex(strings.Join(clean, "|"))
   }

// CanonDigest 规范串摘要。

   func CanonDigest(canon string) string {
	   return digestHex(strings.TrimSpace(canon))
  }

// SigDigest 签章摘要。
  func SigDigest(sig []byte) string {
	  if len(sig) == 0 {
		   return digestHex("")
	   }
	   sum := sha256.Sum256(sig)
	   return hex.EncodeToString(sum[:])
   }

// BuildCanon 与 custody CanonicalFeeBump 字段序一致：id|chain|to|amount|newFee|tag。

   func BuildCanon(transferID, chain, to, amount, newFee, destTag string) string {
	   return strings.Join([]string{
		   strings.TrimSpace(transferID),
		  strings.TrimSpace(chain),
		   strings.TrimSpace(to),
		   strings.TrimSpace(amount),
		  strings.TrimSpace(newFee),
		   strings.TrimSpace(destTag),
	   }, "|")
   }

   func digestHex(s string) string {
	  sum := sha256.Sum256([]byte(s))
	   return hex.EncodeToString(sum[:])
  }

// SameHash 恒定时比较。

  func SameHash(a, b string) bool {
	  return strings.TrimSpace(a) != "" && strings.TrimSpace(a) == strings.TrimSpace(b)
  }

// EmptyHash 空哈希。

  func EmptyHash(h string) bool {
	  return strings.TrimSpace(h) == ""
  }

// FeeUnits 手续费整数比较（最小单位字符串）。
  func FeeUnits(s string) (int64, bool) {
	  s = strings.TrimSpace(s)
	   if s == "" {
		   return 0, false
	  }
	   neg := false
	  if strings.HasPrefix(s, "-") {
		  return 0, false
	   }
	  var n int64
	  dot := false
	   frac := 0
	   for _, c := range s {
		   if c == '.' {
			   if dot {
				   return 0, false
			  }
			  dot = true
			   continue
		  }
		  if c < '0' || c > '9' {
			   return 0, false
		  }
		   n = n*10 + int64(c-'0')
		   if dot {
			   frac++
		   }
		   _ = neg
	  }
	   for frac < 8 {
		  n *= 10
		  frac++
	   }
	   for frac > 8 {
		   n /= 10
		   frac--
	   }
	   return n, true
   }

// FeeRaised new > prev。首代 prev 可空视为 0。
  func FeeRaised(prev, next string) bool {
	  pn, pok := FeeUnits(prev)
	  if !pok {
		  pn = 0
	   }
	  nn, nok := FeeUnits(next)
	   if !nok {
		   return false
	   }
	   return nn > pn
  }
