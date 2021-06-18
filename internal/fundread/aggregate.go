package fundread

import (
	"sort"
	"account-assets.kucoin.net/funding/internal/fundmat"
	"github.com/shopspring/decimal"
)

// Rollup 按资产汇总可用、冻结、在途与占用。

   func Rollup(rows []fundmat.MatRow) ([]AssetTotal, error) {
	  type acc struct {
		   av, fr, inf, oc decimal.Decimal
	   }
	  by := map[string]*acc{}
	   order := make([]string, 0)
	  for _, row := range rows {
		  a := by[row.Asset]
		  if a == nil {
			   a = &acc{}
			  by[row.Asset] = a
			  order = append(order, row.Asset)
		   }
		  var err error
		  if a.av, err = addAmt(a.av, row.Available); err != nil {
			   return nil, err
		   }
		   if a.fr, err = addAmt(a.fr, row.Frozen); err != nil {
			   return nil, err
		   }
		  if a.inf, err = addAmt(a.inf, row.InFlight); err != nil {
			  return nil, err
		  }
		   if a.oc, err = addAmt(a.oc, row.Occupancy); err != nil {
			  return nil, err
		   }
	   }
	  sort.Strings(order)
	   out := make([]AssetTotal, 0, len(order))
	   for _, asset := range order {
		   a := by[asset]
		  out = append(out, AssetTotal{
			  Asset:     asset,
			  Available: a.av.String(), Frozen: a.fr.String(),
			  InFlight: a.inf.String(), Occupancy: a.oc.String(),
		   })
	  }
	  return out, nil
  }

   func addAmt(cur decimal.Decimal, s string) (decimal.Decimal, error) {
	   c, err := fundmat.Canon(s)
	   if err != nil {
		  return decimal.Zero, err
	  }
	   d, err := decimal.NewFromString(c)
	   if err != nil {
		  return decimal.Zero, fundmat.ErrBadEvent
	   }
	  return cur.Add(d), nil
  }
