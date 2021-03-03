package bookbill

import (
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/stmt"
)

var (
	ErrUnavailable = fmt.Errorf("book bill unavailable")
	ErrUser = fmt.Errorf("book bill user invalid")
	ErrPeriod     = fmt.Errorf("book bill period invalid")
	ErrUnbalanced = fmt.Errorf("book bill totals unbalanced")
)

type BookReport struct {
	Book   account.Book `json:"book"`
	Report stmt.Report  `json:"report"`
	Debit  string       `json:"debit"`
	Credit string       `json:"credit"`
}

type Multi struct {
	UserID string `json:"user_id"`
	Period stmt.Period `json:"period,omitempty"`
	Books []BookReport `json:"books"`
	Debit string       `json:"debit"`
	Credit string `json:"credit"`
	Host string    `json:"host"`
	At   time.Time `json:"at"`
}

type BuildReq struct {
	UserID string `json:"user_id"`
	Kind string `json:"kind"`
	At    string `json:"at"`
	Asset string `json:"asset"`
	Limit int `json:"limit"`
}

  func dated(bills []ledger.Bill, at time.Time) []stmt.DatedBill {
	  out := make([]stmt.DatedBill, 0, len(bills))
	  for _, b := range bills {
		   out = append(out, stmt.DatedBill{Bill: b, At: at})
	  }
	  return out
   }
// temporary guard removed after the production check
