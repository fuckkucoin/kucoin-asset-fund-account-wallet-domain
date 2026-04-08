package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

type ListingAsset struct {
	Symbol      string
	Decimals    int
	MinWithdraw string
	MinDeposit string
	DepositEnabled  bool
	WithdrawEnabled bool
	UpdatedAt time.Time
}

type ListingBind struct {
	Asset string
	Chain string
	Contract  string
	Decimals  int
	UpdatedAt time.Time
}

type Listing struct{ db *persist.SQLDB }

  func NewListing(db *persist.SQLDB) *Listing { return &Listing{db: db} }

  func (r *Listing) PutAsset(ctx context.Context, a ListingAsset) error {
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	   }
	   a.Symbol = strings.ToUpper(strings.TrimSpace(a.Symbol))
	   if a.Symbol == "" {
		  return ErrConflict
	   }
	   if a.UpdatedAt.IsZero() {
		   a.UpdatedAt = time.Now().UTC()
	  }
	  dep, wd := 0, 0
	  if a.DepositEnabled {
		  dep = 1
	   }
	  if a.WithdrawEnabled {
		   wd = 1
	   }
	  q := `INSERT INTO aa_listing_assets (symbol, decimals, min_withdraw, min_deposit, deposit_enabled, withdraw_enabled, updated_at)
VALUES (?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE decimals=VALUES(decimals), min_withdraw=VALUES(min_withdraw), min_deposit=VALUES(min_deposit), deposit_enabled=VALUES(deposit_enabled), withdraw_enabled=VALUES(withdraw_enabled), updated_at=VALUES(updated_at)`
	  if r.db.Dialect == persist.Postgres {
		  q = `INSERT INTO aa_listing_assets (symbol, decimals, min_withdraw, min_deposit, deposit_enabled, withdraw_enabled, updated_at)
VALUES (?,?,?,?,?,?,?)
ON CONFLICT (symbol) DO UPDATE SET decimals=EXCLUDED.decimals, min_withdraw=EXCLUDED.min_withdraw, min_deposit=EXCLUDED.min_deposit, deposit_enabled=EXCLUDED.deposit_enabled, withdraw_enabled=EXCLUDED.withdraw_enabled, updated_at=EXCLUDED.updated_at`
	   }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, q, a.Symbol, a.Decimals, a.MinWithdraw, a.MinDeposit, dep, wd, a.UpdatedAt)
	   return err
   }

   func (r *Listing) PutBind(ctx context.Context, b ListingBind) error {
	  b.Asset = strings.ToUpper(strings.TrimSpace(b.Asset))
	  b.Chain = strings.ToUpper(strings.TrimSpace(b.Chain))
	   b.Contract = strings.TrimSpace(b.Contract)
	   if b.Asset == "" || b.Chain == "" || b.Contract == "" {
		   return ErrConflict
	   }
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	  if b.UpdatedAt.IsZero() {
		   b.UpdatedAt = time.Now().UTC()
	   }
	   q := `INSERT INTO aa_listing_binds (asset, chain, contract, decimals, updated_at) VALUES (?,?,?,?,?)
ON DUPLICATE KEY UPDATE contract=VALUES(contract), decimals=VALUES(decimals), updated_at=VALUES(updated_at)`
	   if r.db.Dialect == persist.Postgres {
		  q = `INSERT INTO aa_listing_binds (asset, chain, contract, decimals, updated_at) VALUES (?,?,?,?,?)
ON CONFLICT (asset, chain) DO UPDATE SET contract=EXCLUDED.contract, decimals=EXCLUDED.decimals, updated_at=EXCLUDED.updated_at`
	   }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, q, b.Asset, b.Chain, b.Contract, b.Decimals, b.UpdatedAt)
	  return err
  }

   func (r *Listing) DeleteBind(ctx context.Context, asset, chain string) error {
	  if !sqlOn(r.db) {
		   return ErrUnavailable
	   }
	   _, err := r.db.Conn(ctx).ExecContext(ctx,
		  `DELETE FROM aa_listing_binds WHERE asset=? AND chain=?`,
		  strings.ToUpper(strings.TrimSpace(asset)), strings.ToUpper(strings.TrimSpace(chain)))
	  return err
   }

   func (r *Listing) ListAssets(ctx context.Context) ([]ListingAsset, error) {
	  if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	   }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx,
		   `SELECT symbol, decimals, min_withdraw, min_deposit, deposit_enabled, withdraw_enabled, updated_at FROM aa_listing_assets ORDER BY symbol`)
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   out := make([]ListingAsset, 0)
	   for rows.Next() {
		   var a ListingAsset
		   var dep, wd int
		   if err := rows.Scan(&a.Symbol, &a.Decimals, &a.MinWithdraw, &a.MinDeposit, &dep, &wd, &a.UpdatedAt); err != nil {
			   return nil, err
		   }
		  a.DepositEnabled = dep != 0
		   a.WithdrawEnabled = wd != 0
		   out = append(out, a)
	   }
	  return out, rows.Err()
  }

   func (r *Listing) ListBinds(ctx context.Context) ([]ListingBind, error) {
	  if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	  }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx,
		  `SELECT asset, chain, contract, decimals, updated_at FROM aa_listing_binds ORDER BY asset, chain`)
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   out := make([]ListingBind, 0)
	   for rows.Next() {
		  var b ListingBind
		  if err := rows.Scan(&b.Asset, &b.Chain, &b.Contract, &b.Decimals, &b.UpdatedAt); err != nil {
			   return nil, err
		   }
		  out = append(out, b)
	  }
	   return out, rows.Err()
   }

   func (r *Listing) GetBind(ctx context.Context, asset, chain string) (ListingBind, error) {
	   if !sqlOn(r.db) {
		   return ListingBind{}, ErrUnavailable
	  }
	   var b ListingBind
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT asset, chain, contract, decimals, updated_at FROM aa_listing_binds WHERE asset=? AND chain=?`,
		  strings.ToUpper(strings.TrimSpace(asset)), strings.ToUpper(strings.TrimSpace(chain)),
	  ).Scan(&b.Asset, &b.Chain, &b.Contract, &b.Decimals, &b.UpdatedAt)
	  if errors.Is(err, sql.ErrNoRows) {
		  return ListingBind{}, ErrNotFound
	   }
	  return b, err
  }
