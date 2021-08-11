package hotreserve

import (
	"time"
	"github.com/shopspring/decimal"
)

type Policy struct {
	Window   time.Duration
	Lookback time.Duration
	Haircut string
	Stress  string
	Buffer string
	ReserveTTL time.Duration
	ForecastMaxAge time.Duration
	TickEvery      time.Duration
}

   func DefaultPolicy() Policy {
	  return Policy{
		  Window: 15 * time.Minute,
		  Lookback: 60 * time.Minute,
		   Haircut:  "0.35",
		  Stress:         "0.25",
		  Buffer:         "0",
		   ReserveTTL:     30 * time.Minute,
		   ForecastMaxAge: 5 * time.Minute,
		  TickEvery:      30 * time.Second,
	  }
  }

// validate 拒绝把折价、压力或窗口配成无法复算的值。
// 折价落在 [0,1)，压力与缓冲非负，窗口与回看都不短于 1 秒。

  func (p Policy) validate() error {
	   if p.Window < time.Second || p.Lookback < time.Second {
		   return ErrAmount
	  }
	   if p.ReserveTTL <= 0 || p.ForecastMaxAge <= 0 || p.TickEvery <= 0 {
		  return ErrAmount
	  }
	   h, err := parseNonNeg(p.Haircut)
	   if err != nil {
		   return err
	   }
	  if h.GreaterThanOrEqual(decimal.NewFromInt(1)) {
		  return ErrAmount
	   }
	  if _, err := parseNonNeg(p.Stress); err != nil {
		   return err
	   }
	  if _, err := parseNonNeg(p.Buffer); err != nil {
		  return err
	   }
	  return nil
  }
