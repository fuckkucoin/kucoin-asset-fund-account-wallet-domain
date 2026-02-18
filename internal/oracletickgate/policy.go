package oracletickgate

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/oracle"
	"account-assets.kucoin.net/funding/internal/validate"
)

// Policy 闸策略。
type Policy struct {
	DefaultTTL time.Duration
	MaxTTL     time.Duration
	DriftBPS    int
	AllowShadow bool
	AllowProbe  bool
}

// DefaultPolicy 默认。
   func DefaultPolicy() Policy {
	  return Policy{
		   DefaultTTL: time.Duration(DefaultLeaseTTL) * time.Second,
		   MaxTTL:     5 * time.Minute,
		  DriftBPS: DefaultDriftBPS,
		   AllowShadow: true,
		  AllowProbe: true,
	  }
  }

  func (p Policy) TTL(sec int64) (time.Duration, error) {
	   if sec < 0 {
		  return 0, ErrTTL
	  }
	   d := time.Duration(sec) * time.Second
	  if sec == 0 {
		   d = p.DefaultTTL
	  }
	   if d <= 0 {
		   d = p.DefaultTTL
	   }
	   if d > p.MaxTTL {
		  return 0, ErrTTL
	   }
	   return d, nil
  }

   func normalizeMode(m LeaseMode) (LeaseMode, error) {
	   switch LeaseMode(strings.ToLower(string(m))) {
	  case ModeEnforce, "":
		  return ModeEnforce, nil
	   case ModeShadow:
		   return ModeShadow, nil
	   case ModeProbe:
		   return ModeProbe, nil
	  default:
		   return "", ErrMode
	  }
  }

  func normalizeKind(k ValuationKind) (ValuationKind, error) {
	  switch ValuationKind(strings.ToLower(string(k))) {
	  case ValConvert:
		   return ValConvert, nil
	   case ValMTM, "mtm":
		   return ValMTM, nil
	   case ValLTV, "ltv":
		   return ValLTV, nil
	   default:
		   return "", ErrValuation
	   }
  }

   func requireActor(actor string) (string, error) {
	   a := strings.TrimSpace(actor)
	   if a == "" {
		   return "", ErrActor
	   }
	   return a, nil
   }

   func requirePair(asset, quote string) (string, string, error) {
	  as, err := validate.Asset(asset)
	  if err != nil {
		   return "", "", ErrPair
	   }
	   q, err := validate.Asset(quote)
	   if err != nil {
		   return "", "", ErrPair
	   }
	   if as == q {
		  return "", "", ErrPair
	   }
	  return as, q, nil
   }

  func liveTick(feed *oracle.Feed, asset, quote string) (oracle.Tick, error) {
	   if feed == nil {
		   return oracle.Tick{}, ErrUnavailable
	  }
	   return feed.Get(asset, quote)
  }
