package daycut

import (
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/lending/accrual"
	"account-assets.kucoin.net/funding/internal/utabook"
)

type BatchKey struct {
	LoanID  string
	CutDate string
	Kind    string
}

   func (k BatchKey) String() string {
	  return strings.TrimSpace(k.LoanID) + "|" + strings.TrimSpace(k.CutDate) + "|" + strings.ToUpper(strings.TrimSpace(k.Kind))
   }

   func (k BatchKey) Valid() bool {
	  if strings.TrimSpace(k.LoanID) == "" || strings.TrimSpace(k.CutDate) == "" {
		   return false
	   }
	  if _, err := time.Parse("2006-01-02", k.CutDate); err != nil {
		  return false
	  }
	   return true
   }

  func CutDateUTC(ms int64) string {
	   if ms <= 0 {
		   return ""
	  }
	  return time.UnixMilli(ms).UTC().Format("2006-01-02")
   }

   func KeyOf(loanID, kind string, cutMs int64) BatchKey {
	   kind = strings.ToUpper(strings.TrimSpace(kind))
	   if kind == "" {
		  kind = "DAY"
	  }
	  return BatchKey{LoanID: strings.TrimSpace(loanID), CutDate: CutDateUTC(cutMs), Kind: kind}
   }

  func BatchIdem(k BatchKey) string {
	  return "loan-daycut:" + k.String()
  }

// ReconKey 冲减/还款 vs 应计利息列。加厚：批次 + 周期幂等 + 还款幂等 + 金额列。

type ReconKey struct {
	LoanID  string
	CutDate string
	AccrueIdem string
	RepayIdem string
	AccrueCol string
	OffsetCol string
	PeriodStart int64
	PeriodEnd   int64
}

   func (k ReconKey) String() string {
	   return fmt.Sprintf("%s|%s|acc=%s|rep=%s|a=%s|o=%s|%d:%d",
		   k.LoanID, k.CutDate, k.AccrueIdem, k.RepayIdem, k.AccrueCol, k.OffsetCol, k.PeriodStart, k.PeriodEnd)
   }

  func (k ReconKey) Thick() bool {
	  return strings.TrimSpace(k.LoanID) != "" &&
		  strings.TrimSpace(k.CutDate) != "" &&
		   strings.TrimSpace(k.AccrueIdem) != "" &&
		  strings.TrimSpace(k.RepayIdem) != "" &&
		   strings.TrimSpace(k.AccrueCol) != "" &&
		   strings.TrimSpace(k.OffsetCol) != "" &&
		  k.PeriodEnd > k.PeriodStart
  }

  func BuildReconKey(loanID string, period utabook.PeriodKey, accrueAmt, offsetAmt, repayIdem string) ReconKey {
	   return ReconKey{
		  LoanID:     strings.TrimSpace(loanID),
		  CutDate:    CutDateUTC(period.PeriodStart),
		   AccrueIdem: accrual.IdemKey(period),
		  RepayIdem: strings.TrimSpace(repayIdem),
		   AccrueCol: strings.TrimSpace(accrueAmt),
		  OffsetCol: strings.TrimSpace(offsetAmt),
		   PeriodStart: period.PeriodStart,
		  PeriodEnd:   period.PeriodEnd,
	   }
   }
