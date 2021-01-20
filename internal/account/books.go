// @FileName: books.go
// @Summary: 账户科目，funding / trading / margin / isolated / futures / earn。
// @Author: Cason
// @Created: 2021.01.03

package account

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/shopspring/decimal"
)

type Book string

const (
	BookFunding  Book = "funding"
	BookTrading  Book = "trading"
	BookMargin   Book = "margin"
	BookIsolated Book = "isolated"
	BookFutures Book = "futures"
	BookEarn Book = "earn"
)

  func ParseBook(s string) (Book, error) {
	   switch Book(strings.ToLower(strings.TrimSpace(s))) {
	  case BookFunding, "spot", "wallet", "main":
		  return BookFunding, nil
	   case BookTrading:
		   return BookTrading, nil
	   case BookMargin:
		  return BookMargin, nil
	  case BookIsolated, "isolated_margin":
		   return BookIsolated, nil
	   case BookFutures:
		   return BookFutures, nil
	  case BookEarn:
		   return BookEarn, nil
	   default:
		   return "", fmt.Errorf("unknown book %q", s)
	   }
   }

   func (b Book) AccountType() ledger.AccountType {
	   switch b {
	  case BookTrading:
		  return ledger.AccountTrading
	  case BookMargin:
		  return ledger.AccountMargin
	  case BookIsolated:
		  return ledger.AccountIsolated
	  case BookFutures:
		  return ledger.AccountFutures
	   case BookEarn:
		   return ledger.AccountEarn
	   default:
		  return ledger.AccountSpot
	   }
   }

  func AllBooks() []Book {
	  return []Book{BookFunding, BookTrading, BookMargin, BookIsolated, BookFutures, BookEarn}
   }

  func addBookAmt(a, b string) string {
	  x, e1 := decimal.NewFromString(a)
	  y, e2 := decimal.NewFromString(b)
	  if e1 != nil || e2 != nil {
		   return a
	  }
	  return x.Add(y).String()
   }

// MoveDirection 只允许资金账与交易/杠杆/合约/理财之间互转。
   func MoveDirection(from, to Book) (ledger.Direction, error) {
	   switch {
	  case from == BookFunding && to == BookTrading:
		  return ledger.FundingToTrading,
			   nil
	  case from == BookTrading && to == BookFunding:
		  return ledger.TradingToFunding, nil
	  case from == BookFunding && to == BookMargin:
		   return ledger.FundingToMargin, nil
	   case from == BookMargin && to == BookFunding:
		   return ledger.MarginToFunding, nil
	  case from == BookFunding && to == BookIsolated:
		  return ledger.FundingToIsolated, nil
	  case from == BookIsolated && to == BookFunding:
		   return ledger.IsolatedToFunding, nil
	  case from == BookFunding && to == BookFutures:
		   return ledger.FundingToFutures, nil
	   case from == BookFutures && to == BookFunding:
		  return ledger.FuturesToFunding, nil
	   case from == BookFunding && to == BookEarn:
		  return ledger.EarnSubscribe, nil
	  case from == BookEarn && to == BookFunding:
		   return ledger.EarnRedeem, nil
	   default:
		  return "", fmt.Errorf("book move %s → %s is not a funding hop", from, to)
	  }
  }
