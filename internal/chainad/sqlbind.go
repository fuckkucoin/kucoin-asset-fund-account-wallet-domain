package chainad

import (
	"context"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

   func (b *TokenBook) BindSQL(db *persist.SQLDB) *TokenBook {
	   if b != nil {
		   b.sql = db
		  b.loadAll()
	  }
	  return b
  }

  func (b *TokenBook) sqlOn() bool {
	   return b != nil && b.sql != nil && b.sql.DB != nil
  }

  func (b *TokenBook) persistToken(t Token) error {
	  if !b.sqlOn() {
		   return nil
	   }
	   return sqlrepo.NewTokens(b.sql).Put(context.Background(), sqlrepo.TokenRow{
		   Chain: t.Chain, Asset: t.Asset, Contract: t.Contract, Decimals: t.Decimals,
	  })
  }

   func (b *TokenBook) loadToken(chain, asset string) (Token, bool) {
	  if !b.sqlOn() {
		  return Token{}, false
	   }
	   got, err := sqlrepo.NewTokens(b.sql).Get(context.Background(), chain, asset)
	   if err != nil {
		  return Token{}, false
	   }
	  return Token{Chain: got.Chain, Asset: got.Asset, Contract: got.Contract, Decimals: got.Decimals}, true
   }

  func (b *TokenBook) listTokens() ([]Token, bool) {
	  if !b.sqlOn() {
		   return nil, false
	  }
	  rows, err := sqlrepo.NewTokens(b.sql).List(context.Background())
	  if err != nil {
		  return nil, false
	  }
	   out := make([]Token, 0, len(rows))
	   for _, t := range rows {
		  out = append(out, Token{Chain: t.Chain, Asset: t.Asset, Contract: t.Contract, Decimals: t.Decimals})
	  }
	   return out, true
   }

  func (b *TokenBook) loadAll() {
	  got, ok := b.listTokens()
	   if !ok {
		  return
	  }
	   b.mu.Lock()
	   for _, t := range got {
		  b.rows[tokenKey(t.Chain,
			  t.Asset)] = t
	   }
	  b.mu.Unlock()
   }
