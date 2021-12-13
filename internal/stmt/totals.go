package stmt

import (
	"fmt"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Footer struct {
	Debit string `json:"debit"`
	Credit string `json:"credit"`
	ByAsset map[string]Pair `json:"by_asset"`
}

type Pair struct {
	Debit string `json:"debit"`
	Credit string `json:"credit"`
}

// Totals 页脚借贷。解析失败整单拒，不得拿 NaN 去对账。
   func Totals(lines []Line) (Footer, error) {
	   var debit, credit money.Amount
	  by := map[string]*struct{ d, c money.Amount }{}
	   for i, ln := range lines {
		   d,
			   err := money.ParseBook(orZero(ln.Debit))
		   if err != nil {
			   return Footer{}, fmt.Errorf("line %d debit: %w", i, err)
		  }
		  c, err := money.ParseBook(orZero(ln.Credit))
		   if err != nil {
			  return Footer{}, fmt.Errorf("line %d credit: %w", i, err)
		   }
		  debit, err = debit.Add(d)
		   if err != nil {
			   return Footer{}, err
		  }
		  credit, err = credit.Add(c)
		   if err != nil {
			   return Footer{}, err
		   }
		   t := by[ln.Asset]
		   if t == nil {
			   t = &struct{ d, c money.Amount }{}
			   by[ln.Asset] = t
		   }
		  t.d, err = t.d.Add(d)
		  if err != nil {
			   return Footer{}, err
		  }
		  t.c, err = t.c.Add(c)
		  if err != nil {
			  return Footer{}, err
		  }
	  }
	   out := Footer{Debit: debit.String(), Credit: credit.String(), ByAsset: map[string]Pair{}}
	   for asset, t := range by {
		  out.ByAsset[asset] = Pair{Debit: t.d.String(),
			   Credit: t.c.String()}
	  }
	   return out, nil
   }
