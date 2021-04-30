package feeeng

import (
	"context"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

   func (s *Service) BindSQL(db *persist.SQLDB) *Service {
	  if s != nil && s.book != nil {
		  s.book.BindSQL(db)
	  }
	  return s
  }

   func (b *Book) BindSQL(db *persist.SQLDB) *Book {
	   if b != nil {
		   b.sql = db
		   b.loadSQL()
	   }
	  return b
  }

   func (b *Book) sqlOn() bool {
	  return b != nil && b.sql != nil && b.sql.DB != nil
   }

  func (b *Book) persistPut(s Schedule) error {
	   if !b.sqlOn() {
		   return nil
	  }
	  return sqlrepo.NewFeeSched(b.sql).Put(context.Background(), sqlrepo.FeeSchedule{
		   ID: s.ID, Kind: string(s.Kind), Asset: s.Asset, BPS: s.BPS,
		   MinFee: s.MinFee, MaxFee: s.MaxFee, Flat: s.Flat, Active: s.Active, Version: s.Version,
	  })
  }

  func (b *Book) loadSQL() {
	   if !b.sqlOn() {
		  return
	   }
	   got, err := sqlrepo.NewFeeSched(b.sql).List(context.Background())
	   if err != nil || len(got) == 0 {
		  return
	   }
	  b.mu.Lock()
	   b.rows = map[string]Schedule{}
	  for _, row := range got {
		  s := Schedule{
			  ID:   row.ID,
			   Kind: Kind(row.Kind), Asset: row.Asset, BPS: row.BPS,
			   MinFee: row.MinFee, MaxFee: row.MaxFee, Flat: row.Flat, Active: row.Active, Version: row.Version,
		  }
		   b.rows[s.key()] = s
		   if s.Version > b.seq {
			  b.seq = s.Version
		   }
	   }
	  b.mu.Unlock()
  }
