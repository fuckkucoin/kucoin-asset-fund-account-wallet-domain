package por

import (
	"fmt"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Drift struct {
	Asset string `json:"asset"`
	Left  string `json:"left"`
	Right string `json:"right"`
}

// Compare 两棵树的根和按资产合计。根不同不得当同一快照。
   func Compare(a, b Snapshot) ([]Drift, error) {
	  if a.Root == "" || b.Root == "" {
		   return nil, ErrEmpty
	  }
	  if a.Root == b.Root {
		  return nil, nil
	   }
	  left := map[string]string{}
	  for _, t := range a.Totals {
		  left[t.Asset] = t.Amount
	   }
	   right := map[string]string{}
	   for _, t := range b.Totals {
		   right[t.Asset] = t.Amount
	   }
	  seen := map[string]struct{}{}
	  out := make([]Drift, 0)
	  for asset, lv := range left {
		  seen[asset] = struct{}{}
		   rv := right[asset]
		   if err := sameAmt(lv, rv); err != nil {
			  out = append(out,
				  Drift{Asset: asset, Left: lv, Right: orZ(rv)})
		  }
	   }
	   for asset, rv := range right {
		   if _, ok := seen[asset]; ok {
			   continue
		   }
		  out = append(out, Drift{Asset: asset, Left: "0", Right: rv})
	  }
	   return out, fmt.Errorf("%w", ErrRoot)
   }

  func sameAmt(a, b string) error {
	  l, err := money.ParseBook(orZ(a))
	   if err != nil {
		   return err
	  }
	  r, err := money.ParseBook(orZ(b))
	  if err != nil {
		  return err
	   }
	   return money.Conserve(l, r)
  }
