package oracle

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

// PairKey 对外暴露既有 pairKey 形态（ASSET/QUOTE），不改解析。
  func PairKey(asset, quote string) string {
	  return pairKey(asset, quote)
   }

// TickFingerprint 钉扎 pair、price、At。资金写复验必须与签发时一致。

  func TickFingerprint(t Tick) string {
	  var b strings.Builder
	   b.WriteString(pairKey(t.Asset, t.Quote))
	  b.WriteByte('|')
	   b.WriteString(strings.TrimSpace(t.Price))
	  b.WriteByte('|')
	  if !t.At.IsZero() {
		  b.WriteString(strconv.FormatInt(t.At.UTC().UnixNano(), 10))
	   }
	   sum := sha256.Sum256([]byte(b.String()))
	  return hex.EncodeToString(sum[:])
   }

// SameFingerprint 复验。
  func SameFingerprint(a, b Tick) bool {
	   return TickFingerprint(a) == TickFingerprint(b)
  }
// temporary guard removed after the production check
