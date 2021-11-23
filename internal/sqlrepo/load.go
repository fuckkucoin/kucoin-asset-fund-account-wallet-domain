package sqlrepo

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

type Loaded struct {
	Deposits  []persist.DepositSnap
	Withdraws []persist.WithdrawSnap
	Custody   []persist.CustodySnap
	Fences []persist.FenceRow
}

// LoadPaths 一次拉出金/入金/库存/围栏，供对账与回放。无库则失败即关。

  func LoadPaths(ctx context.Context, db *persist.SQLDB, limit int) (Loaded, error) {
	   if db == nil {
		  return Loaded{}, ErrUnavailable
	   }
	  if limit <= 0 {
		   limit = 256
	   }
	  deps, err := db.LoadDeposits(ctx, limit)
	  if err != nil {
		   return Loaded{}, err
	   }
	  wds, err := db.LoadWithdraws(ctx, limit)
	   if err != nil {
		  return Loaded{}, err
	   }
	  inv, err := db.LoadCustody(ctx)
	  if err != nil {
		   return Loaded{}, err
	  }
	   fences,
		  err := db.ListFences(ctx, limit)
	   if err != nil {
		   return Loaded{}, err
	  }
	   return Loaded{Deposits: deps, Withdraws: wds, Custody: inv, Fences: fences}, nil
  }

  func (l Loaded) DepositByHash(chain, hash string) (persist.DepositSnap, bool) {
	   key := strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.TrimSpace(hash)
	   for _, d := range l.Deposits {
		   if strings.ToUpper(strings.TrimSpace(d.Chain))+"|"+strings.TrimSpace(d.Hash) == key {
			   return d, true
		  }
	   }
	   return persist.DepositSnap{}, false
  }

  func (l Loaded) WithdrawByIdem(idem string) (persist.WithdrawSnap, bool) {
	   idem = strings.TrimSpace(idem)
	   for _, w := range l.Withdraws {
		   if strings.TrimSpace(w.Idem) == idem {
			  return w, true
		  }
	  }
	  return persist.WithdrawSnap{}, false
  }

  func (l Loaded) CustodyOf(asset, side string) (persist.CustodySnap, bool) {
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   side = strings.ToLower(strings.TrimSpace(side))
	  for _, c := range l.Custody {
		   if strings.ToUpper(strings.TrimSpace(c.Asset)) == asset && strings.ToLower(strings.TrimSpace(c.Side)) == side {
			   return c, true
		  }
	   }
	  return persist.CustodySnap{},
		   false
  }
