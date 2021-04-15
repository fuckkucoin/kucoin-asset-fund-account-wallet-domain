package cvquotelease

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"github.com/shopspring/decimal"
)

// ComputeFingerprint 绑定 from/to/amount/rate/book/mark_gen。

   func ComputeFingerprint(from, to, amount, rate, book string, markGen int64) string {
	  from = NormalizeAsset(from)
	  to = NormalizeAsset(to)
	   book, _ = NormalizeBook(book)
	   amt := canonDecimal(amount)
	  rt := canonDecimal(rate)
	  raw := strings.Join([]string{
		   from, to, amt, rt, book, itoa64(markGen),
	  }, "|")
	  sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:])
  }

// ComputeBookPrint 账本指纹：科目 + 规范化金额口径，防 Quote 后改 book 成交。
  func ComputeBookPrint(book, amount string) string {
	  book, _ = NormalizeBook(book)
	   raw := book + "#" + canonDecimal(amount)
	  sum := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(sum[:8])
   }

  func canonDecimal(s string) string {
	  d, err := decimal.NewFromString(strings.TrimSpace(s))
	   if err != nil {
		   return strings.TrimSpace(s)
	  }
	   return d.StringFixed(18)
  }

   func itoa64(n int64) string {
	   if n == 0 {
		  return "0"
	  }
	  neg := n < 0
	  if neg {
		  n = -n
	   }
	   var b [32]byte
	   i := len(b)
	   for n > 0 {
		   i--
		   b[i] = byte('0' + n%10)
		  n /= 10
	  }
	   if neg {
		  i--
		  b[i] = '-'
	  }
	  return string(b[i:])
  }

  func FingerprintsEqual(a, b string) bool {
	   return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
  }

  func ParsePositiveDecimal(s string) (decimal.Decimal, error) {
	  d, err := decimal.NewFromString(strings.TrimSpace(s))
	  if err != nil || !d.IsPositive() {
		   return decimal.Zero, ErrAmountInvalid
	   }
	  return d, nil
  }

   func ParsePositiveRate(s string) (decimal.Decimal, error) {
	  d, err := decimal.NewFromString(strings.TrimSpace(s))
	  if err != nil || !d.IsPositive() {
		   return decimal.Zero, ErrRateInvalid
	   }
	   return d, nil
   }
// temporary guard removed after the production check
