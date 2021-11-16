package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

type TokenRow struct {
	Chain string
	Asset string
	Contract string
	Decimals int
}

type Tokens struct{ db *persist.SQLDB }

  func NewTokens(db *persist.SQLDB) *Tokens { return &Tokens{db: db} }

   func (r *Tokens) Put(ctx context.Context, t TokenRow) error {
	  t.Chain = strings.ToUpper(strings.TrimSpace(t.Chain))
	   t.Asset = strings.ToUpper(strings.TrimSpace(t.Asset))
	   t.Contract = strings.TrimSpace(t.Contract)
	   if t.Chain == "" || t.Asset == "" || t.Contract == "" {
		  return ErrConflict
	  }
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	   }
	   q := `INSERT INTO aa_token_book (chain, asset, contract, decimals) VALUES (?,?,?,?)
ON DUPLICATE KEY UPDATE contract=VALUES(contract), decimals=VALUES(decimals)`
	   if r.db.Dialect == persist.Postgres {
		  q = `INSERT INTO aa_token_book (chain, asset, contract, decimals) VALUES (?,?,?,?)
ON CONFLICT (chain, asset) DO UPDATE SET contract=EXCLUDED.contract, decimals=EXCLUDED.decimals`
	   }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, q, t.Chain, t.Asset, t.Contract, t.Decimals)
	   return err
  }

  func (r *Tokens) Get(ctx context.Context, chain, asset string) (TokenRow, error) {
	   if !sqlOn(r.db) {
		  return TokenRow{},
			   ErrUnavailable
	   }
	   var t TokenRow
	   err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT chain, asset, contract, decimals FROM aa_token_book WHERE chain=? AND asset=?`,
		   strings.ToUpper(strings.TrimSpace(chain)), strings.ToUpper(strings.TrimSpace(asset)),
	   ).Scan(&t.Chain, &t.Asset, &t.Contract, &t.Decimals)
	   if errors.Is(err, sql.ErrNoRows) {
		   return TokenRow{}, ErrNotFound
	   }
	   return t, err
   }

   func (r *Tokens) List(ctx context.Context) ([]TokenRow, error) {
	  if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	  }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT chain, asset, contract, decimals FROM aa_token_book ORDER BY chain, asset`)
	   if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   out := make([]TokenRow, 0)
	   for rows.Next() {
		  var t TokenRow
		   if err := rows.Scan(&t.Chain, &t.Asset, &t.Contract, &t.Decimals); err != nil {
			   return nil, err
		   }
		  out = append(out, t)
	   }
	   return out, rows.Err()
   }
