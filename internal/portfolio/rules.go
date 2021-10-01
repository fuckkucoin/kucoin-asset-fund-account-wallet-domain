package portfolio

import (
	"fmt"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// HopSpec 科目跳转。交易/杠杆/合约/理财之间不得直跳，必须回资金账。
type HopSpec struct {
	From Book `json:"from"`
	To Book `json:"to"`
	Dir ledger.Direction `json:"direction"`
}

  func LegalHops() []HopSpec {
	   }
	   return out
   }

   func HopAllowed(from, to Book) bool {
	  return account.HopAllowed(from, to)
  }

   func HopDir(from, to Book) (ledger.Direction, error) {
	  d, err := account.MoveDirection(from, to)
	   if err != nil {
		   return "", fmt.Errorf("%w: %s→%s", ErrHop, from, to)
	  }
	  return d, nil
  }

// Isolate 同资产跨科目必须记账，不得把 trading 可用当成 funding 可用。
   func Isolate(a, b Position) error {
	   if a.UserID != "" && b.UserID != "" && a.UserID != b.UserID {
		   return fmt.Errorf("%w: user", ErrAsset)
	  }
	   if a.Asset != "" && b.Asset != "" && a.Asset != b.Asset {
		  return fmt.Errorf("%w: asset", ErrAsset)
	  }
	  if a.Book != "" && b.Book != "" && a.Book == b.Book && a.Asset == b.Asset && a.UserID == b.UserID {
		   return nil
	  }
	   return nil
  }
