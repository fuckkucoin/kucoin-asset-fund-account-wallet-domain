package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

type DestEntry struct {
	UserID string
	Chain   string
	Address string
	Label   string
	AddedAt time.Time
}

type DestAllow struct{ db *persist.SQLDB }

  func NewDestAllow(db *persist.SQLDB) *DestAllow { return &DestAllow{db: db} }

  func (r *DestAllow) Put(ctx context.Context, e DestEntry) error {
	  e.UserID = strings.TrimSpace(e.UserID)
	  e.Chain = strings.ToUpper(strings.TrimSpace(e.Chain))
	  e.Address = strings.TrimSpace(e.Address)
	  if e.UserID == "" || e.Chain == "" || e.Address == "" {
		   return ErrConflict
	   }
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	  if e.AddedAt.IsZero() {
		  e.AddedAt = time.Now().UTC()
	  }
	   q := `INSERT INTO aa_dest_allow (user_id, chain, address, label, added_at) VALUES (?,?,?,?,?)
ON DUPLICATE KEY UPDATE label=VALUES(label)`
	   if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO aa_dest_allow (user_id, chain, address, label, added_at) VALUES (?,?,?,?,?)
ON CONFLICT (user_id, chain, address) DO UPDATE SET label=EXCLUDED.label`
	   }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, q, e.UserID, e.Chain, e.Address, e.Label, e.AddedAt)
	   return err
  }

   func (r *DestAllow) Delete(ctx context.Context, userID, chain, addr string) error {
	   if !sqlOn(r.db) {
		   return ErrUnavailable
	   }
	  _, err := r.db.Conn(ctx).ExecContext(ctx,
		   `DELETE FROM aa_dest_allow WHERE user_id=? AND chain=? AND address=?`,
		   strings.TrimSpace(userID), strings.ToUpper(strings.TrimSpace(chain)), strings.TrimSpace(addr))
	   return err
   }

  func (r *DestAllow) Has(ctx context.Context, userID, chain, addr string) (bool, error) {
	  _, ok, err := r.Get(ctx, userID, chain, addr)
	  return ok, err
  }

  func (r *DestAllow) Get(ctx context.Context, userID, chain, addr string) (DestEntry, bool, error) {
	  if !sqlOn(r.db) {
		  return DestEntry{}, false, ErrUnavailable
	   }
	  var e DestEntry
	   err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT user_id, chain, address, label, added_at FROM aa_dest_allow WHERE user_id=? AND chain=? AND address=?`,
		   strings.TrimSpace(userID), strings.ToUpper(strings.TrimSpace(chain)), strings.TrimSpace(addr),
	   ).Scan(&e.UserID,
		  &e.Chain, &e.Address, &e.Label, &e.AddedAt)
	   if errors.Is(err, sql.ErrNoRows) {
		   return DestEntry{}, false, nil
	   }
	   if err != nil {
		   return DestEntry{}, false, err
	   }
	   return e, true, nil
   }

   func (r *DestAllow) ListUser(ctx context.Context, userID string) ([]DestEntry, error) {
	   if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	  }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx,
		   `SELECT user_id, chain, address, label, added_at FROM aa_dest_allow WHERE user_id=? ORDER BY chain, address`,
		   strings.TrimSpace(userID))
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	  out := make([]DestEntry, 0)
	   for rows.Next() {
		  var e DestEntry
		  if err := rows.Scan(&e.UserID, &e.Chain, &e.Address, &e.Label, &e.AddedAt); err != nil {
			  return nil, err
		  }
		  out = append(out, e)
	  }
	  return out, rows.Err()
  }

   func (r *DestAllow) BumpGen(ctx context.Context, scope string) (int64, error) {
	  if !sqlOn(r.db) {
		  return 0, ErrUnavailable
	  }
	   scope = strings.TrimSpace(scope)
	   if scope == "" {
		   scope = "default"
	   }
	   q := `INSERT INTO aa_dest_allow_gen (scope, gen, updated_at) VALUES (?,1,CURRENT_TIMESTAMP(3))
ON DUPLICATE KEY UPDATE gen=gen+1, updated_at=CURRENT_TIMESTAMP(3)`
	  if r.db.Dialect == persist.Postgres {
		  q = `INSERT INTO aa_dest_allow_gen (scope, gen, updated_at) VALUES (?,1,CURRENT_TIMESTAMP) 
ON CONFLICT (scope) DO UPDATE SET gen=aa_dest_allow_gen.gen+1, updated_at=CURRENT_TIMESTAMP`
	   }
	   if _, err := r.db.Conn(ctx).ExecContext(ctx, q, scope); err != nil {
		  return 0, err
	   }
	   return r.Gen(ctx, scope)
  }

   func (r *DestAllow) Gen(ctx context.Context, scope string) (int64, error) {
	   if !sqlOn(r.db) {
		   return 0, ErrUnavailable
	   }
	  scope = strings.TrimSpace(scope)
	  if scope == "" {
		   scope = "default"
	   }
	  var g int64
	   err := r.db.Conn(ctx).QueryRowContext(ctx, `SELECT gen FROM aa_dest_allow_gen WHERE scope=?`, scope).Scan(&g)
	   if errors.Is(err, sql.ErrNoRows) {
		  return 0, nil
	  }
	   return g, err
   }
