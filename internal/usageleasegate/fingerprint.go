package usageleasegate

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/usage"
	"account-assets.kucoin.net/funding/pkg/money"
)

// ComputeWindowPrint 绑定 kind/user/asset/dest/today/velocity/dest_count。

  func ComputeWindowPrint(kind usage.Kind, user, asset, dest, today string, vel1h, vel24, destN int) string {
	  raw := strings.Join([]string{
		   string(kind),
		   strings.TrimSpace(user),
		  strings.ToUpper(strings.TrimSpace(asset)),
		   strings.TrimSpace(dest),
		  canonDecimal(today),
		  itoa(vel1h),
		   itoa(vel24),
		   itoa(destN),
	  }, "|")
	  sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:])
  }

// ComputeCapPrint 限额指纹：日累计上限 + 速度上限。
   func ComputeCapPrint(dailyCap string, velocityCap int) string {
	   raw := canonDecimal(dailyCap) + "#" + itoa(velocityCap)
	  sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:])
   }

   func ComputeViewPrint(v WindowView) string {
	   return ComputeWindowPrint(v.Kind, v.UserID, v.Asset, v.Dest, v.Today, v.Velocity1h, v.Velocity24, v.DestN)
   }

  func FingerprintsEqual(a, b string) bool {
	  return strings.EqualFold(strings.TrimSpace(a),
		  strings.TrimSpace(b))
  }

   func canonDecimal(s string) string {
	  s = strings.TrimSpace(s)
	  if s == "" {
		  s = "0"
	  }
	   d, err := decimal.NewFromString(s)
	   if err != nil {
		  return s
	  }
	   return d.StringFixed(18)
   }

  func itoa(n int) string {
	   return strconv.FormatInt(int64(n), 10)
  }

  func ParsePositiveAmount(s string) (money.Amount, error) {
	  amt, err := money.Parse(strings.TrimSpace(s))
	  if err != nil || !amt.IsPositive() {
		  return money.Zero(),
			  ErrAmountInvalid
	   }
	  return amt, nil
  }

   func ParsePositiveCap(s string) (money.Amount, error) {
	  amt, err := money.Parse(strings.TrimSpace(s))
	   if err != nil || !amt.IsPositive() {
		  return money.Zero(),
			  ErrCapInvalid
	   }
	  return amt, nil
   }

   func MustAdd(a, b money.Amount) money.Amount {
	   sum, err := a.Add(b)
	   if err != nil {
		  return a
	  }
	  return sum
   }

   func WouldExceed(today, reserved, attempt, cap money.Amount) bool {
	  sum := MustAdd(today, reserved)
	  sum = MustAdd(sum, attempt)
	  return sum.Cmp(cap) > 0
   }
