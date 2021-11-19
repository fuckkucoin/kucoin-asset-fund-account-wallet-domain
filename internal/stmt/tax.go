package stmt

import (
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
)

type TaxLine struct {
	Asset   string `json:"asset"`
	Inflows string `json:"inflows"`
	Outflows string `json:"outflows"`
	Net      string `json:"net"`
	Count    int    `json:"count"`
}

type TaxReport struct {
	UserID string `json:"user_id"`
	Year  int       `json:"year"`
	Lines []TaxLine `json:"lines"`
	Note  string    `json:"note"`
}

// BuildTaxYear 按日历年滚流入/流出。净额允许为负（流出多于流入），用字符串符号表示。
// 这是流水汇总，不是报税件，不得当核验申报。

   func BuildTaxYear(userID string, rows []DatedBill, year int) (TaxReport, error) {
	  if year < 2000 || year > 2100 {
		  return TaxReport{}, fmt.Errorf("invalid year")
	   }
	  start := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	   end := start.AddDate(1, 0, 0)
	   type acc struct {
		   in, out money.Amount
		  n       int
	   }
	  by := map[string]*acc{}
	   for _, r := range rows {
		   if r.At.Before(start) || !r.At.Before(end) {
			   continue
		   }
		   d, err := money.ParseBook(orZero(r.Debit))
		  if err != nil {
			  return TaxReport{}, err
		  }
		  c, err := money.ParseBook(orZero(r.Credit))
		  if err != nil {
			  return TaxReport{}, err
		   }
		   a := by[r.Asset]
		   if a == nil {
			   a = &acc{}
			  by[r.Asset] = a
		   }
		  if c.IsPositive() {
			  a.in, err = a.in.Add(c)
			  if err != nil {
				   return TaxReport{}, err
			   }
		  }
		  if d.IsPositive() {
			   a.out, err = a.out.Add(d)
			   if err != nil {
				   return TaxReport{}, err
			   }
		   }
		  a.n++
	   }
	  out := TaxReport{
		  UserID: userID, Year: year, Note: "journal roll only; not a tax filing",
	   }
	   for asset, a := range by {
		  net := signedNet(a.in, a.out)
		  out.Lines = append(out.Lines, TaxLine{Asset: asset, Inflows: a.in.String(), Outflows: a.out.String(), Net: net, Count: a.n})
	   }
	   return out, nil
  }

   func signedNet(in, out money.Amount) string {
	  if in.Cmp(out) >= 0 {
		   n, err := in.Sub(out)
		   if err != nil {
			   return "?"
		   }
		  return n.String()
	   }
	   n, err := out.Sub(in)
	  if err != nil {
		   return "?"
	   }
	  return "-" + n.String()
   }

  func BillsWithTime(bills []ledger.Bill, at time.Time) []DatedBill {
	   out := make([]DatedBill, 0, len(bills))
	   for _, b := range bills {
		   out = append(out, DatedBill{Bill: b, At: at})
	   }
	   return out
   }

  func AssetsOf(rep TaxReport) []string {
	   out := make([]string, 0, len(rep.Lines))
	   for _, ln := range rep.Lines {
		  out = append(out,
			  strings.ToUpper(ln.Asset))
	   }
	   return out
   }
