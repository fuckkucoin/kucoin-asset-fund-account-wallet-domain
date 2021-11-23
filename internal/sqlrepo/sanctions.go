package sqlrepo

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

type SancRow struct {
	Chain   string
	Address string
	AddedAt time.Time
}

type Sanctions struct{ db *persist.SQLDB }

   func NewSanctions(db *persist.SQLDB) *Sanctions { return &Sanctions{db: db} }

   func (r *Sanctions) Put(ctx context.Context, e SancRow) error {
	   e.Chain = strings.ToUpper(strings.TrimSpace(e.Chain))
	   e.Address = strings.ToLower(strings.TrimSpace(e.Address))
	   if e.Chain == "" || e.Address == "" {
		  return ErrConflict
	   }
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	   if e.AddedAt.IsZero() {
		   e.AddedAt = time.Now().UTC()
	   }
	  q := `INSERT INTO aa_sanctions (chain, address, added_at) VALUES (?,?,?)
ON DUPLICATE KEY UPDATE added_at=VALUES(added_at)`
	   if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO aa_sanctions (chain, address, added_at) VALUES (?,?,?)
ON CONFLICT (chain, address) DO UPDATE SET added_at=EXCLUDED.added_at`
	   }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, q, e.Chain, e.Address, e.AddedAt)
	   return err
  }

   func (r *Sanctions) Has(ctx context.Context, chain, addr string) (bool, error) {
	  if !sqlOn(r.db) {
		  return false, ErrUnavailable
	  }
	   var n int
	   err := r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT COUNT(1) FROM aa_sanctions WHERE chain=? AND address=?`,
		   strings.ToUpper(strings.TrimSpace(chain)), strings.ToLower(strings.TrimSpace(addr)),
	   ).Scan(&n)
	  if err != nil {
		   return false, err
	   }
	  return n > 0, nil
   }

   func (r *Sanctions) List(ctx context.Context) ([]SancRow, error) {
	  if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	   }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT chain, address, added_at FROM aa_sanctions ORDER BY chain, address`)
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   out := make([]SancRow, 0)
	   for rows.Next() {
		   var e SancRow
		   if err := rows.Scan(&e.Chain,
			   &e.Address, &e.AddedAt); err != nil {
			   return nil, err
		   }
		   out = append(out, e)
	  }
	   return out, rows.Err()
  }

   func (r *Sanctions) ReplaceAll(ctx context.Context, list []SancRow) error {
	  if !sqlOn(r.db) {
		   return ErrUnavailable
	   }
	   return r.db.InTx(ctx, func(ctx context.Context) error {
		   if _, err := r.db.Conn(ctx).ExecContext(ctx, `DELETE FROM aa_sanctions`); err != nil {
			  return err
		   }
		  for _, e := range list {
			   if err := r.Put(ctx, e); err != nil {
				  return err
			   }
		  }
		   return nil
	  })
   }
