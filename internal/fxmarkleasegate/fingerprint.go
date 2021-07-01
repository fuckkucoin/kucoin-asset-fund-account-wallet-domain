package fxmarkleasegate

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/fxmark"
)

   func ComputeSourcePrint(from, to, rate, source string, generation int64) string {
	   return fxmark.RateFingerprint(from, to, rate, source, generation)
   }

  func ComputePairPrint(s LiveSnap) string {
	   return fxmark.PairFingerprint(fxmark.SourceSnap{
		  From: s.From,
		  To:   s.To, FromUSD: s.FromUSD, ToUSD: s.ToUSD,
		  Rate: s.Rate, Source: s.Source, Generation: s.Generation,
	   })
  }

   func FingerprintsEqual(a, b string) bool {
	   return strings.EqualFold(strings.TrimSpace(a),
		   strings.TrimSpace(b))
   }

   func CanonRate(s string) string { return fxmark.CanonRate(s) }

   func ParsePositiveRate(s string) (decimal.Decimal, error) {
	  n, err := decimal.NewFromString(strings.TrimSpace(s))
	  if err != nil || !n.IsPositive() {
		   return decimal.Zero, ErrRateInvalid
	   }
	  return n, nil
   }

   func DriftBPS(leaseRate, liveRate string) (int64, bool) {
	  return fxmark.DriftBPS(leaseRate, liveRate)
  }

   func OverThreshold(leaseRate, liveRate string, thresholdBPS int64) bool {
	  if thresholdBPS <= 0 {
		   thresholdBPS = DefaultThresholdBPS
	   }
	   return fxmark.OverThreshold(leaseRate, liveRate, thresholdBPS)
   }

  func CombineGeneration(fromGen, toGen int64) int64 {
	   if fromGen > toGen {
		   return fromGen
	   }
	  return toGen
  }

  func itoa64(n int64) string {
	   return strconv.FormatInt(n, 10)
   }

  func hashJoin(parts ...string) string {
	  raw := strings.Join(parts, "|")
	   sum := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(sum[:])
   }

   func AssetTouched(l MarkLease, asset string) bool {
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  return asset != "" && (l.From == asset || l.To == asset)
  }
