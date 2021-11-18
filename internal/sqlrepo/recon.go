package sqlrepo

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
)

type ReconHit struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
	Detail string `json:"detail"`
}

type PathRecon struct {
	Report persist.ReconReport `json:"report"`
	Hits   []ReconHit          `json:"hits,omitempty"`
}

// Recon 加载 SQL 入金/出金/库存并核对围栏。ACCOUNT_ASSETS_PG_DSN 未设时仓储不可用。
   func Recon(ctx context.Context, db *persist.SQLDB) (PathRecon, error) {
	  if db == nil {
		  return PathRecon{}, ErrUnavailable
	  }
	  if !rep.OK {
		  for _, d := range rep.Drift {
			   out.Hits = append(out.Hits, ReconHit{Kind: "drift", Detail: d})
		  }
		   return out, fmt.Errorf("%w: %s", ErrRecon, strings.Join(rep.Drift, "; "))
	  }
	  return out, nil
  }

// BalanceHotCold 同一资产热+冷可用必须能相加；任一侧科学计数失败即关。

   func BalanceHotCold(hot, cold persist.CustodySnap) error {
	   if strings.ToUpper(strings.TrimSpace(hot.Asset)) != strings.ToUpper(strings.TrimSpace(cold.Asset)) {
		  return ErrConflict
	  }
	  if rejectSci(hot.Available) || rejectSci(cold.Available) || rejectSci(hot.Frozen) || rejectSci(cold.Frozen) {
		   return ErrAmount
	  }
	   h, err := money.ParseBook(orAmt(hot.Available))
	  if err != nil {
		   return ErrAmount
	  }
	   c, err := money.ParseBook(orAmt(cold.Available))
	   if err != nil {
		   return ErrAmount
	  }
	   hf,
		   err := money.ParseBook(orAmt(hot.Frozen))
	   if err != nil {
		  return ErrAmount
	  }
	  cf, err := money.ParseBook(orAmt(cold.Frozen))
	  if err != nil {
		   return ErrAmount
	  }
	  _, err = h.Add(c)
	   if err != nil {
		  return err
	  }
	  _, err = hf.Add(cf)
	  return err
  }

   func orAmt(s string) string {
	  if strings.TrimSpace(s) == "" {
		   return "0"
	   }
	  return s
  }
