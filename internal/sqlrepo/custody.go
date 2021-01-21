package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Custody 热/冷库存快照 aa_custody_inv。side 只认 hot/cold，金额过 ParseBook（允许 0）。

type Custody struct {
	db *persist.SQLDB
}

  func NewCustody(db *persist.SQLDB) *Custody { return &Custody{db: db} }

type InvRow struct {
	Asset     string
	Side      string
	Available string
	Frozen    string
	Fence     persist.Fence
	TakenAt time.Time
}

  func (r *Custody) Put(ctx context.Context, asset, side, available, frozen string) (InvRow, error) {
	  if r == nil || r.db == nil {
		  return InvRow{}, ErrUnavailable
	   }
	   if rejectSci(available) {
		  return InvRow{}, ErrAmount
	  }
	   if _, err := money.ParseBook(available); err != nil {
		   return InvRow{}, ErrAmount
	  }
	  if frozen == "" {
		   frozen = "0"
	  }
	  if rejectSci(frozen) {
		   return InvRow{}, ErrAmount
	  }
	  if _, err := money.ParseBook(frozen); err != nil {
		  return InvRow{}, ErrAmount
	   }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  side = strings.ToLower(strings.TrimSpace(side))
	  if asset == "" || (side != "hot" && side != "cold") {
		  return InvRow{}, ErrConflict
	  }
	   var out InvRow
	  err := r.db.InTx(ctx, func(ctx context.Context) error {
		  id := asset + "|" + side
		  f, err := r.db.BumpFence(ctx, "inv|"+id, persist.NewToken())
		  if err != nil {
			  return err
		  }
		   upsert := `INSERT INTO aa_custody_inv (asset, side, available, frozen, fence_epoch, fence_token, taken_at) VALUES (?,?,?,?,?,?,CURRENT_TIMESTAMP(3)) 
ON DUPLICATE KEY UPDATE available=VALUES(available), frozen=VALUES(frozen), fence_epoch=VALUES(fence_epoch), fence_token=VALUES(fence_token), taken_at=CURRENT_TIMESTAMP(3)`
		  if r.db.Dialect == persist.Postgres {
			  upsert = `INSERT INTO aa_custody_inv (asset, side, available, frozen, fence_epoch, fence_token, taken_at) VALUES (?,?,?,?,?,?,CURRENT_TIMESTAMP)
ON CONFLICT (asset, side) DO UPDATE SET available=EXCLUDED.available, frozen=EXCLUDED.frozen, fence_epoch=EXCLUDED.fence_epoch, fence_token=EXCLUDED.fence_token, taken_at=CURRENT_TIMESTAMP`
		  }
		   _,
			  err = r.db.Conn(ctx).ExecContext(ctx, upsert, asset, side, available, frozen, f.Epoch, f.Token)
		   if err != nil {
			   return err
		  }
		  got, err := r.get(ctx, asset, side)
		   if err != nil {
			  return err
		   }
		  out = got
		  return nil
	  })
	   return out, err
  }

   func (r *Custody) Get(ctx context.Context, asset, side string) (InvRow, error) {
	  if r == nil || r.db == nil {
		   return InvRow{}, ErrUnavailable
	  }
	  return r.get(ctx,
		  strings.ToUpper(strings.TrimSpace(asset)), strings.ToLower(strings.TrimSpace(side)))
  }

   func (r *Custody) get(ctx context.Context, asset, side string) (InvRow, error) {
	   var row InvRow
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT asset, side, available, frozen, fence_epoch, fence_token, taken_at FROM aa_custody_inv WHERE asset=? AND side=?`,
		  asset, side,
	   ).Scan(&row.Asset,
		   &row.Side, &row.Available, &row.Frozen, &row.Fence.Epoch, &row.Fence.Token, &row.TakenAt)
	  if errors.Is(err, sql.ErrNoRows) {
		   return InvRow{}, ErrNotFound
	   }
	   return row, err
  }
