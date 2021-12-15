package utabook

import (
	"strings"
	"time"
	"github.com/shopspring/decimal"
)

type AccrualProjection struct {
	Currency string `json:"currency"`
	Principal string `json:"principal"`
	RateHourly string `json:"rateHourly"`
	Hours         string `json:"hours"`
	Projected     string `json:"projectedInterest"`
	AccruedStored string `json:"accruedStored"`
	TotalProjected string `json:"totalProjected"`
}

   func (b *Book) ProjectAccrual(userID, currency string, hours decimal.Decimal) AccrualProjection {
	   currency = strings.ToUpper(strings.TrimSpace(currency))
	  out := AccrualProjection{Currency: currency, RateHourly: "0", Hours: hours.String(), Projected: "0"}
	   if b == nil || !hours.IsPositive() {
		  return out
	   }
	   lim := b.InterestLimits(currency)
	  rate, err := decimal.NewFromString(strings.TrimSpace(lim.CurrentRateHourly))
	  if err != nil || !rate.IsPositive() {
		   return out
	   }
	  out.RateHourly = rate.String()
	   principal := decimal.Zero
	  accrued := decimal.Zero
	   for _, row := range b.Liabilities(userID, currency, "", "") {
		   p, err := decimal.NewFromString(strings.TrimSpace(row.Principal))
		  if err == nil {
			  principal = principal.Add(p)
		   }
		   a, err := decimal.NewFromString(strings.TrimSpace(row.AccruedInterest))
		  if err == nil {
			   accrued = accrued.Add(a)
		   }
	   }
	   out.Principal = principal.String()
	   out.AccruedStored = accrued.String()
	  proj := principal.Mul(rate).Mul(hours)
	  out.Projected = proj.String()
	   out.TotalProjected = accrued.Add(proj).String()
	   return out
   }

  func (b *Book) ProjectAccrualSince(userID, currency string, sinceMs int64) AccrualProjection {
	  if sinceMs <= 0 {
		   return b.ProjectAccrual(userID, currency, decimal.Zero)
	   }
	  now := time.Now().UTC().UnixMilli()
	  delta := now - sinceMs
	  if delta <= 0 {
		  return b.ProjectAccrual(userID,
			  currency, decimal.Zero)
	  }
	  hours := decimal.NewFromInt(delta).Div(decimal.NewFromInt(3600 * 1000))
	  return b.ProjectAccrual(userID, currency, hours)
   }

  func (b *Book) InterestLimitsForUser(userID, currency string) map[string]any {
	  currency = strings.ToUpper(strings.TrimSpace(currency))
	  lim := b.InterestLimits(currency)
	  acc := b.ProjectAccrualSince(userID, currency, time.Now().UTC().Add(-24*time.Hour).UnixMilli())
	  return map[string]any{
		  "currency": currency, "limits": lim, "accrual24h": acc,
		  "liability": b.LiabilityByCurrency(userID, currency),
	   }
   }

   func (b *Book) ListAccrualByCurrency(userID string, currencies []string) []AccrualProjection {
	  out := make([]AccrualProjection, 0, len(currencies))
	  for _, c := range currencies {
		  out = append(out, b.ProjectAccrualSince(userID, c, time.Now().UTC().Add(-1*time.Hour).UnixMilli()))
	  }
	   return out
  }

  func hourlyFromDaily(daily string) string {
	   d, err := decimal.NewFromString(strings.TrimSpace(daily))
	  if err != nil || !d.IsPositive() {
		   return "0"
	   }
	   return d.Div(decimal.NewFromInt(24)).String()
  }

  func dailyFromHourly(hourly string) string {
	   h, err := decimal.NewFromString(strings.TrimSpace(hourly))
	  if err != nil || !h.IsPositive() {
		  return "0"
	  }
	  return h.Mul(decimal.NewFromInt(24)).String()
   }

  func (b *Book) NormalizeRateTier(t RateTier) RateTier {
	   if strings.TrimSpace(t.RateHourly) == "0" && strings.TrimSpace(t.RateDaily) != "0" {
		   t.RateHourly = hourlyFromDaily(t.RateDaily)
	   }
	   if strings.TrimSpace(t.RateDaily) == "0" && strings.TrimSpace(t.RateHourly) != "0" {
		  t.RateDaily = dailyFromHourly(t.RateHourly)
	  }
	   return t
   }

   func (b *Book) PutRateTier(t RateTier) error {
	   t = b.NormalizeRateTier(t)
	  if err := b.persistRateTier(t); err != nil {
		  return err
	  }
	  b.RateCatalog().Put(t)
	  return nil
   }
