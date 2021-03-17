// 账本账单查询与导出能力服务实现。

// Author: Cason
// Date: 2021.03.02

package bookbill

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/stmt"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Service struct {
	led *ledger.Service
}

  func New(led *ledger.Service) *Service { return &Service{led: led} }

  func (s *Service) Build(ctx context.Context, userID, asset string, limit int) (Multi, error) {
	   if s == nil || s.led == nil {
		   return Multi{}, ErrUnavailable
	  }
	  uid, err := validate.UserID(userID)
	   if err != nil {
		  return Multi{}, ErrUser
	  }
	  if asset != "" {
		   if _, err := validate.Asset(asset); err != nil {
			   return Multi{}, err
		   }
	   }
	   bills, err := s.led.ListBills(ctx, uid, 500)
	   if err != nil {
		   return Multi{}, err
	  }
	  return Assemble(uid,
		   bills, stmt.Filter{Asset: asset, Limit: limit}, time.Now().UTC())
   }

  func (s *Service) BuildPeriod(ctx context.Context, userID string, kind stmt.PeriodKind, at time.Time, asset string) (Multi, error) {
	   if s == nil || s.led == nil {
		  return Multi{}, ErrUnavailable
	   }
	  uid, err := validate.UserID(userID)
	  if err != nil {
		   return Multi{}, ErrUser
	   }
	   p, err := stmt.ParsePeriod(kind, at)
	   if err != nil {
		   return Multi{}, ErrPeriod
	   }
	   bills, err := s.led.ListBills(ctx, uid, 1000)
	  if err != nil {
		   return Multi{}, err
	  }
	  return AssemblePeriod(uid, dated(bills, at), p, stmt.Filter{Asset: asset})
  }

  func Assemble(userID string, bills []ledger.Bill, f stmt.Filter, at time.Time) (Multi, error) {
	  out := Multi{UserID: userID, Host: "query-gateway.kucoin.net", At: at}
	  sumD, _ := money.ParseBook("0")
	   sumC, _ := money.ParseBook("0")
	  for _, book := range account.AllBooks() {
		   ff := f
		  ff.Book = string(book)
		   rep, err := stmt.Build(userID, bills, ff)
		   if err != nil {
			  return Multi{}, err
		   }
		   d, c := totals(rep)
		  sumD, _ = sumD.Add(d)
		  sumC, _ = sumC.Add(c)
		   out.Books = append(out.Books, BookReport{Book: book, Report: rep, Debit: d.String(), Credit: c.String()})
	  }
	   out.Debit = sumD.String()
	   out.Credit = sumC.String()
	   return out, nil
   }

   func AssemblePeriod(userID string, rows []stmt.DatedBill, p stmt.Period, f stmt.Filter) (Multi, error) {
	   out := Multi{UserID: userID, Period: p, Host: "query-gateway.kucoin.net", At: p.Start}
	  sumD, _ := money.ParseBook("0")
	   sumC, _ := money.ParseBook("0")
	   for _, book := range account.AllBooks() {
		  ff := f
		  ff.Book = string(book)
		  rep,
			   err := stmt.BuildPeriod(userID, rows, ff, p)
		  if err != nil {
			   return Multi{}, err
		  }
		   d, c := totals(rep)
		   sumD, _ = sumD.Add(d)
		  sumC, _ = sumC.Add(c)
		   out.Books = append(out.Books, BookReport{Book: book, Report: rep, Debit: d.String(), Credit: c.String()})
	   }
	  out.Debit = sumD.String()
	  out.Credit = sumC.String()
	  return out, nil
  }

   func totals(r stmt.Report) (money.Amount, money.Amount) {
	   d, _ := money.ParseBook("0")
	   c, _ := money.ParseBook("0")
	   for _, line := range r.Lines {
		   if a, err := money.Parse(orZ(line.Debit)); err == nil {
			   d, _ = d.Add(a)
		  }
		   if a, err := money.Parse(orZ(line.Credit)); err == nil {
			  c, _ = c.Add(a)
		   }
	  }
	   return d, c
  }

   func orZ(s string) string {
	  if s == "" {
		   return "0"
	  }
	   return s
  }
