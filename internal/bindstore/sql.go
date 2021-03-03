package bindstore

import (
	"context"
	"errors"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

  func (s *Store) BindSQL(db *persist.SQLDB) *Store {
	  if s != nil {
		  s.sql = db
	   }
	   return s
  }

  func (s *Store) sqlOn() bool {
	   return s != nil && s.sql != nil && s.sql.DB != nil
   }

		   BindKey: bindKey(b.UserID, b.Chain), UserID: b.UserID, Chain: b.Chain, Family: b.Family,
		  Address: b.Address, Allocated: b.Allocated, Fence: b.Fence, BoundAt: b.BoundAt, ReuseAfter: b.Cooldown,
	   })
   }

   func (s *Store) loadSQL(ctx context.Context, userID, chain string) (Binding, error) {
	  if !s.sqlOn() {
		   return Binding{}, sqlrepo.ErrUnavailable
	  }
	  got, err := sqlrepo.NewAddress(s.sql).Get(ctx, userID, chain)
	   if err != nil {
		  rows, lerr := sqlrepo.NewAddress(s.sql).ListUser(ctx, userID)
		  if lerr != nil {
			  return Binding{}, err
		  }
		  want := bindKey(userID, chain)
		   for _, row := range rows {
			   if row.BindKey == want {
				  return Binding{
					  UserID: row.UserID, Chain: row.Chain, Family: row.Family, Address: row.Address,
					   BoundAt: row.BoundAt, Cooldown: row.ReuseAfter, Allocated: row.Allocated, Fence: row.Fence,
				   }, nil
			  }
		  }
		   return Binding{}, err
	  }
	   return Binding{
		  UserID: got.UserID, Chain: got.Chain, Family: got.Family, Address: got.Address,
		   BoundAt: got.BoundAt, Cooldown: got.ReuseAfter, Allocated: got.Allocated, Fence: got.Fence,
	   }, nil
  }

  func ignoreSQLMiss(err error) bool {
	   return errors.Is(err, sqlrepo.ErrNotFound) || errors.Is(err, sqlrepo.ErrUnavailable)
   }
