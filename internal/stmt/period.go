package stmt

import (
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
)

type PeriodKind string

const (
	PeriodDay   PeriodKind = "day"
	PeriodMonth PeriodKind = "month"
)

type Period struct {
	Kind PeriodKind
	Start time.Time
	End time.Time
}

// ParsePeriod 按 UTC 切日或月。end 是开区间上沿，避免跨日重复记账。
  func ParsePeriod(kind PeriodKind, at time.Time) (Period, error) {
	  if at.IsZero() {
		   at = time.Now().UTC()
	  }
	  at = at.UTC()
	  switch kind {
	  case PeriodDay, "":
		  start := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
		  return Period{Kind: PeriodDay,
			  Start: start, End: start.Add(24 * time.Hour)}, nil
	   case PeriodMonth:
		  start := time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, time.UTC)
		  return Period{Kind: PeriodMonth, Start: start, End: start.AddDate(0, 1, 0)}, nil
	   default:
		   return Period{}, fmt.Errorf("unknown period %q", kind)
	   }
  }

type DatedBill struct {
	ledger.Bill
	At time.Time
}

// BuildPeriod 只滚落在 [start, end) 的流水。running 仍按该窗口前缀，不回看窗外。

  func BuildPeriod(userID string, rows []DatedBill, f Filter, p Period) (Report, error) {
	   bills := make([]ledger.Bill, 0, len(rows))
	  for _, r := range rows {
		   if r.At.IsZero() {
			   continue
		  }
		  if r.At.Before(p.Start) || !r.At.Before(p.End) {
			  continue
		  }
		  bills = append(bills, r.Bill)
	   }
	  return Build(userID, bills, f)
  }

   func ParsePeriodKind(s string) (PeriodKind, error) {
	  switch PeriodKind(strings.ToLower(strings.TrimSpace(s))) {
	   case PeriodDay, "":
		  return PeriodDay, nil
	   case PeriodMonth:
		  return PeriodMonth, nil
	  default:
		   return "", fmt.Errorf("unknown period %q", s)
	   }
  }
