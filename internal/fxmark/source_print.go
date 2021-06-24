package fxmark

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"github.com/shopspring/decimal"
)

const (
	SourceKV   = "kv"
	SourceHTTP = "http"
	SourceLive = "live"
	SourceStatic = "static"
	SourceStable = "stable"
)

// SourceSnap 一次 Rate/USD 读取的可复核快照。

type SourceSnap struct {
	From string
	To      string
	FromUSD string
	ToUSD   string
	Rate   string
	Source string
	Generation int64
}

   func CanonRate(s string) string {
	  s = strings.TrimSpace(s)
	  if s == "" {
		   return ""
	  }
	  n, err := decimal.NewFromString(s)
	   if err != nil || !n.IsPositive() {
		   return s
	  }
	  return n.StringFixed(18)
  }

   func RateFingerprint(from, to, rate, source string, generation int64) string {
	   raw := strings.Join([]string{
		  strings.ToUpper(strings.TrimSpace(from)),
		   strings.ToUpper(strings.TrimSpace(to)),
		   CanonRate(rate),
		  strings.ToLower(strings.TrimSpace(source)),
		   itoa64(generation),
	  }, "|")
	  sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:])
  }

   func PairFingerprint(s SourceSnap) string {
	   raw := strings.Join([]string{
		  strings.ToUpper(strings.TrimSpace(s.From)),
		  strings.ToUpper(strings.TrimSpace(s.To)),
		  CanonRate(s.FromUSD),
		   CanonRate(s.ToUSD),
		  CanonRate(s.Rate),
		   strings.ToLower(strings.TrimSpace(s.Source)),
		  itoa64(s.Generation),
	  }, "|")
	   sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:])
  }

  func DriftBPS(leaseRate, liveRate string) (int64, bool) {
	   a, err1 := decimal.NewFromString(strings.TrimSpace(leaseRate))
	  b, err2 := decimal.NewFromString(strings.TrimSpace(liveRate))
	  if err1 != nil || err2 != nil || !a.IsPositive() || !b.IsPositive() {
		  return 0, false
	   }
	   diff := b.Sub(a).Abs()
	   bps := diff.Mul(decimal.NewFromInt(10000)).Div(a)
	   return bps.IntPart(), true
  }

  func OverThreshold(leaseRate, liveRate string, thresholdBPS int64) bool {
	  if thresholdBPS <= 0 {
		   thresholdBPS = 10
	  }
	  bps, ok := DriftBPS(leaseRate, liveRate)
	  if !ok {
		   return true
	  }
	  return bps > thresholdBPS
   }

  func itoa64(n int64) string {
	   if n == 0 {
		  return "0"
	   }
	  neg := n < 0
	   if neg {
		  n = -n
	  }
	   var buf [20]byte
	   i := len(buf)
	   for n > 0 {
		  i--
		  buf[i] = byte('0' + n%10)
		   n /= 10
	   }
	  if neg {
		   i--
		   buf[i] = '-'
	  }
	  return string(buf[i:])
   }
