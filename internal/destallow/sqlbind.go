package destallow

import (
	"context"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/authn"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

const ProdCooldown = 24 * time.Hour

  func BindProd(b *Book, db *persist.SQLDB, kind, profile string) *Book {
	  if b == nil {
		  return b
	   }
	   b.BindSQL(db)
	  if authn.ProductionProfile(kind, profile) {
		  b.SetCooldown(ProdCooldown)
	   }
	   return b
  }

  func (b *Book) BindSQL(db *persist.SQLDB) *Book {
	  if b != nil {
		  b.sql = db
	   }
	   return b
  }

   func (b *Book) sqlOn() bool {
	  return b != nil && b.sql != nil && b.sql.DB != nil
   }

  func (b *Book) persistAdd(e Entry) error {
	   if !b.sqlOn() {
		   return nil
	   }
	   err := sqlrepo.NewDestAllow(b.sql).Put(context.Background(), sqlrepo.DestEntry{
		  UserID: e.UserID, Chain: e.Chain, Address: e.Address, Label: e.Label, AddedAt: e.AddedAt,
	  })
	   if err != nil {
		  return fmt.Errorf("%w: %v", ErrStore, err)
	  }
	   return nil
  }

  func (v *Versioned) BindSQL(db *persist.SQLDB) *Versioned {
	   if v != nil {
		  if v.Book == nil {
			   v.Book = New()
		   }
		  v.Book.BindSQL(db)
	   }
	   return v
   }

  func (v *Versioned) persistGen() (int64, error) {
	  if v == nil || v.Book == nil || !v.Book.sqlOn() {
		  return 0, nil
	   }
	   return sqlrepo.NewDestAllow(v.Book.sql).BumpGen(context.Background(), "default")
  }

   func (v *Versioned) loadGen() int64 {
	  if v == nil || v.Book == nil || !v.Book.sqlOn() {
		   if v == nil {
			   return 0
		  }
		  v.mu.Lock()
		  defer v.mu.Unlock()
		   return v.gen
	   }
	  g, err := sqlrepo.NewDestAllow(v.Book.sql).Gen(context.Background(), "default")
	  if err != nil {
		  return 0
	   }
	   return g
  }
