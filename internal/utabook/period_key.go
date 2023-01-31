package utabook

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type PeriodKey struct {
	LoanID      string
	PeriodStart int64
	PeriodEnd   int64
	Kind        string
}

   func (k PeriodKey) String() string {
	  return k.LoanID + "|" + strconv.FormatInt(k.PeriodStart, 10) + "|" + strconv.FormatInt(k.PeriodEnd, 10)
   }

  func (k PeriodKey) Valid() bool {
	  return strings.TrimSpace(k.LoanID) != "" && k.PeriodEnd > k.PeriodStart && k.PeriodStart > 0
   }

   func ParsePeriodKey(s string) (PeriodKey, bool) {
	  parts := strings.Split(strings.TrimSpace(s), "|")
	  if len(parts) < 3 {
		   return PeriodKey{}, false
	   }
	   end, err2 := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	  start, err1 := strconv.ParseInt(parts[len(parts)-2], 10, 64)
	   if err1 != nil || err2 != nil {
		   return PeriodKey{}, false
	  }
	  loan := strings.Join(parts[:len(parts)-2], "|")
	  k := PeriodKey{LoanID: loan, PeriodStart: start, PeriodEnd: end}
	  return k, k.Valid()
   }

  func AlignHourUTC(ms int64) int64 {
	   if ms <= 0 {
		  return 0
	  }
	   t := time.UnixMilli(ms).UTC()
	   return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, time.UTC).UnixMilli()
   }

   func AlignDayUTC(ms int64) int64 {
	   if ms <= 0 {
		  return 0
	  }
	  t := time.UnixMilli(ms).UTC()
	   return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).UnixMilli()
  }

  func PeriodSpanMs(kind string) int64 {
	   if strings.EqualFold(strings.TrimSpace(kind), "DAY") {
		  return 24 * 3600 * 1000
	   }
	   return 3600 * 1000
   }

   func PeriodOf(loanID, kind string, nowMs int64) PeriodKey {
	  kind = strings.ToUpper(strings.TrimSpace(kind))
	  var start int64
	   if kind == "DAY" {
		   start = AlignDayUTC(nowMs)
	   } else {
		  kind = "HOUR"
		   start = AlignHourUTC(nowMs)
	   }
	  return PeriodKey{LoanID: strings.TrimSpace(loanID), PeriodStart: start, PeriodEnd: start + PeriodSpanMs(kind), Kind: kind}
   }

   func NextPeriod(k PeriodKey) PeriodKey {
	  span := k.PeriodEnd - k.PeriodStart
	   if span <= 0 {
		   span = PeriodSpanMs(k.Kind)
	   }
	  return PeriodKey{LoanID: k.LoanID, PeriodStart: k.PeriodEnd, PeriodEnd: k.PeriodEnd + span, Kind: k.Kind}
  }

   func PrevPeriod(k PeriodKey) PeriodKey {
	  span := k.PeriodEnd - k.PeriodStart
	   if span <= 0 {
		   span = PeriodSpanMs(k.Kind)
	   }
	   return PeriodKey{LoanID: k.LoanID, PeriodStart: k.PeriodStart - span, PeriodEnd: k.PeriodStart, Kind: k.Kind}
  }

  func SequenceFrom(loanID, kind string, startMs int64, n int) []PeriodKey {
	   if n <= 0 {
		  return nil
	  }
	   cur := PeriodOf(loanID, kind, startMs)
	   out := make([]PeriodKey, 0, n)
	  for i := 0; i < n; i++ {
		  out = append(out, cur)
		   cur = NextPeriod(cur)
	  }
	  return out
  }

   func PeriodLabel(k PeriodKey) string {
	   if !k.Valid() {
		  return ""
	   }
	   return fmt.Sprintf("%s:%d-%d", k.Kind, k.PeriodStart, k.PeriodEnd)
   }
