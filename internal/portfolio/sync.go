package portfolio

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Drift struct {
	Book Book `json:"book"`
	Asset string `json:"asset"`
	Left  string `json:"shadow"`
	Right string `json:"ledger"`
	Note string `json:"note"`
}

// DiffLedger 影子子账对账本科目。对不上不得自行抹平。
   func DiffLedger(shadow Snap, views []ledger.BookView) []Drift {
	  by := map[string]ledger.BookView{}
	   for _, v := range views {
		  b, err := ParseBook(v.Subject)
		  if err != nil {
			   b, err = ParseBook(v.Book)
			  if err != nil {
				   continue
			   }
		   }
		  by[string(b)+"|"+strings.ToUpper(v.Asset)] = v
	  }
	  out := make([]Drift, 0)
	  for _, p := range shadow.Positions {
		  v, ok := by[string(p.Book)+"|"+p.Asset]
		   if !ok {
			   if p.Available != "0" || p.Frozen != "0" {
				   out = append(out, Drift{Book: p.Book, Asset: p.Asset, Left: p.Available, Right: "0", Note: "missing on ledger"})
			  }
			  continue
		  }
		   if err := sameBook(p.Available, v.Available); err != nil {
			  out = append(out, Drift{Book: p.Book, Asset: p.Asset, Left: p.Available, Right: v.Available, Note: "available"})
		  }
		  if err := sameBook(p.Frozen, v.Frozen); err != nil {
			   out = append(out, Drift{Book: p.Book, Asset: p.Asset, Left: p.Frozen, Right: v.Frozen, Note: "frozen"})
		   }
	  }
	  return out
  }

  func sameBook(a, b string) error {
	   l, err := money.ParseBook(orZ(a))
	   if err != nil {
		  return err
	  }
	  r, err := money.ParseBook(orZ(b))
	   if err != nil {
		   return err
	  }
	  if err := money.Conserve(l, r); err != nil {
		  return fmt.Errorf("%w: %v", ErrConserve, err)
	  }
	   return nil
   }
