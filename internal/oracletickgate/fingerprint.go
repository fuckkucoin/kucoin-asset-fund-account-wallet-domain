package oracletickgate

import (
	"strings"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/oracle"
	"account-assets.kucoin.net/funding/pkg/money"
)

// FingerprintOf 委托 oracle.TickFingerprint，避免改 pairKey 形态。
  func FingerprintOf(t oracle.Tick) string {
	  return oracle.TickFingerprint(t)
  }

// PriceDriftBPS 签发价与活价的绝对基点差。缺价返回 -1。
   func PriceDriftBPS(issued, live string) int {
	  a, err := money.Parse(strings.TrimSpace(issued))
	  if err != nil || !a.IsPositive() {
		   return -1
	   }
	   b, err := money.Parse(strings.TrimSpace(live))
	   if err != nil || !b.IsPositive() {
		  return -1
	   }
	  diff := a.Dec().Sub(b.Dec()).Abs()
	  bps := diff.Div(a.Dec()).Mul(decimal.NewFromInt(10000))
	   if thresholdBPS <= 0 {
		   thresholdBPS = DefaultDriftBPS
	   }
	  n := PriceDriftBPS(issued, live)
	   if n < 0 {
		   return true
	   }
	   return n > thresholdBPS
   }

// RecheckLive 活 Tick 复验租约指纹与世代。

type RecheckResult struct {
	OK     bool
	Reason error
	ProofKind ProofKind
	Live      oracle.Tick
	LiveGen int64
	LiveFP  string
}

  func RecheckLive(lease TickLease, live oracle.Tick, liveErr error, liveGen int64, now time.Time, maxAge time.Duration, driftBPS int) RecheckResult {
	  if liveErr == oracle.ErrMissing || liveErr == oracle.ErrUnavailable {
		   return RecheckResult{Reason: ErrTickMissing, ProofKind: ProofTickMissing, LiveGen: liveGen}
	   }
	   if liveErr == oracle.ErrStale {
		  return RecheckResult{Reason: ErrTickStale, ProofKind: ProofTickStale, Live: live, LiveGen: liveGen, LiveFP: FingerprintOf(live)}
	   }
	  if liveErr != nil {
		  return RecheckResult{Reason: liveErr,
			  ProofKind: ProofTickMissing, LiveGen: liveGen}
	  }
	   if !lease.Alive(now) {
		  return RecheckResult{Reason: ErrLeaseExpired, ProofKind: ProofLeaseExpired, Live: live, LiveGen: liveGen, LiveFP: FingerprintOf(live)}
	   }
	   if maxAge > 0 && now.Sub(live.At) > maxAge {
		  return RecheckResult{Reason: ErrTickStale, ProofKind: ProofTickStale, Live: live, LiveGen: liveGen, LiveFP: FingerprintOf(live)}
	  }
	   fp := FingerprintOf(live)
	   if liveGen != 0 && lease.Generation != liveGen {
		   return RecheckResult{Reason: ErrGeneration, ProofKind: ProofGeneration, Live: live, LiveGen: liveGen, LiveFP: fp}
	   }
	   if fp != lease.Fingerprint {
		  return RecheckResult{Reason: ErrFingerprint, ProofKind: ProofTickFlip, Live: live, LiveGen: liveGen, LiveFP: fp}
	  }
	  if DriftExceeded(lease.Price, live.Price, driftBPS) {
		   return RecheckResult{Reason: ErrPriceDrift, ProofKind: ProofPriceDrift, Live: live, LiveGen: liveGen, LiveFP: fp}
	   }
	  return RecheckResult{OK: true, Live: live, LiveGen: liveGen, LiveFP: fp}
  }
