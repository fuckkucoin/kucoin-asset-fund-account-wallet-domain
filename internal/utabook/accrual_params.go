package utabook

import (
	"strings"
	"github.com/shopspring/decimal"
)

// AccrualParams 借据上的计息参数。HourlyRate 来自 RateTier，不在 poster 内写死。

type AccrualParams struct {
	Currency string `json:"currency"`
	HourlyRate string `json:"hourlyRate"`
	DailyRate  string `json:"dailyRate"`
	PeriodKind string `json:"periodKind"` // HOUR | DAY
	InterestFree string `json:"interestFreeLimit"`
	LoanID string `json:"loanId"`
	MarginMode string `json:"marginMode"`
	Symbol     string `json:"symbol"`
}

   func (p AccrualParams) Normalize() AccrualParams {
	  p.Currency = strings.ToUpper(strings.TrimSpace(p.Currency))
	  p.PeriodKind = strings.ToUpper(strings.TrimSpace(p.PeriodKind))
	  if p.PeriodKind != "DAY" {
		   p.PeriodKind = "HOUR"
	   }
	  p.HourlyRate = strings.TrimSpace(p.HourlyRate)
	  p.DailyRate = strings.TrimSpace(p.DailyRate)
	   p.InterestFree = strings.TrimSpace(p.InterestFree)
	  p.LoanID = strings.TrimSpace(p.LoanID)
	  p.MarginMode = strings.ToUpper(strings.TrimSpace(p.MarginMode))
	   p.Symbol = strings.ToUpper(strings.TrimSpace(p.Symbol))
	   if p.HourlyRate == "" {
		  p.HourlyRate = "0"
	  }
	   if p.DailyRate == "" {
		   p.DailyRate = "0"
	  }
	  if p.InterestFree == "" {
		   p.InterestFree = "0"
	  }
	   if p.MarginMode == "" {
		  p.MarginMode = "CROSS"
	  }
	  return p
   }

   func (p AccrualParams) RateForPeriod() (decimal.Decimal, bool) {
	   p = p.Normalize()
	  raw := p.HourlyRate
	  if p.PeriodKind == "DAY" {
		  if strings.TrimSpace(p.DailyRate) != "" && p.DailyRate != "0" {
			  raw = p.DailyRate
		   } else {
			  h, err := decimal.NewFromString(p.HourlyRate)
			  if err != nil {
				  return decimal.Zero, false
			  }
			   return h.Mul(decimal.NewFromInt(24)), h.IsPositive()
		   }
	   }
	   r, err := decimal.NewFromString(raw)
	  if err != nil || !r.IsPositive() {
		  return decimal.Zero, false
	  }
	   return r, true
  }

  func (p AccrualParams) InterestFreeDec() decimal.Decimal {
	   v,
		   err := decimal.NewFromString(strings.TrimSpace(p.InterestFree))
	   if err != nil || v.IsNegative() {
		   return decimal.Zero
	   }
	  return v
   }

// ParamsFromCatalog 从利率目录组装参数。目录无正利率则返回零参。

  func (b *Book) ParamsFromCatalog(currency, periodKind, loanID, marginMode, symbol string) AccrualParams {
	  p := AccrualParams{
		   Currency: currency, PeriodKind: periodKind, LoanID: loanID,
		  MarginMode: marginMode, Symbol: symbol, HourlyRate: "0", DailyRate: "0", InterestFree: "0",
	  }.Normalize()
	  if b == nil {
		   return p
	  }
	   lim := b.InterestLimits(p.Currency)
	   p.HourlyRate = strings.TrimSpace(lim.CurrentRateHourly)
	  p.DailyRate = strings.TrimSpace(lim.CurrentRateDaily)
	  p.InterestFree = strings.TrimSpace(lim.InterestFreeBorrowLimit)
	   if p.HourlyRate == "" {
		   p.HourlyRate = "0"
	  }
	  if p.DailyRate == "" {
		   p.DailyRate = "0"
	   }
	   return p.Normalize()
   }

// BindParamsToLiability 把目录利率写到借据字段（不写常量）。
  func (b *Book) BindParamsToLiability(userID string, row LiabilityRow, periodKind string) (LiabilityRow, AccrualParams) {
	  p := b.ParamsFromCatalog(row.Currency, periodKind, "", row.MarginMode, row.Symbol)
	   p.LoanID = LoanNoteID(userID, row.Currency, row.MarginMode, row.Symbol)
	  return row, p
  }

  func LoanNoteID(userID, currency, marginMode, symbol string) string {
	  return strings.TrimSpace(userID) + "|" + strings.ToUpper(strings.TrimSpace(currency)) + "|" +
		   strings.ToUpper(strings.TrimSpace(marginMode)) + "|" + strings.ToUpper(strings.TrimSpace(symbol))
   }
