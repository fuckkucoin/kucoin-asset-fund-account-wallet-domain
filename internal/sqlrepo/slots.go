package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

type AddrSlot struct {
	Address string
	Chain  string
	Family string
	UserID string
	Status string
	Reserved  time.Time
	BoundAt   time.Time
	CoolUntil time.Time
}

type Slots struct{ db *persist.SQLDB }

  func NewSlots(db *persist.SQLDB) *Slots { return &Slots{db: db} }

  func (r *Slots) Put(ctx context.Context, s AddrSlot) error {
	  s.Address = strings.TrimSpace(s.Address)
	  s.Chain = strings.ToUpper(strings.TrimSpace(s.Chain))
	  s.Family = strings.ToUpper(strings.TrimSpace(s.Family))
	   s.Status = strings.ToLower(strings.TrimSpace(s.Status))
	  if s.Address == "" || s.Chain == "" || s.Status == "" {
		   return ErrConflict
	   }
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	  q := `INSERT INTO aa_addr_slots (address, chain, family, user_id, status, reserved_at, bound_at, cool_until)
VALUES (?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE chain=VALUES(chain), family=VALUES(family), user_id=VALUES(user_id), status=VALUES(status), reserved_at=VALUES(reserved_at), bound_at=VALUES(bound_at), cool_until=VALUES(cool_until)`
	  if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO aa_addr_slots (address, chain, family, user_id, status, reserved_at, bound_at, cool_until)
VALUES (?,?,?,?,?,?,?,?)
ON CONFLICT (address) DO UPDATE SET chain=EXCLUDED.chain, family=EXCLUDED.family, user_id=EXCLUDED.user_id, status=EXCLUDED.status, reserved_at=EXCLUDED.reserved_at, bound_at=EXCLUDED.bound_at, cool_until=EXCLUDED.cool_until`
	   }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, q, s.Address, s.Chain, s.Family, s.UserID, s.Status, nullTime(s.Reserved), nullTime(s.BoundAt), nullTime(s.CoolUntil))
	  return err
  }

  func (r *Slots) Get(ctx context.Context, address string) (AddrSlot, error) {
	   if !sqlOn(r.db) {
		  return AddrSlot{}, ErrUnavailable
	   }
	   var s AddrSlot
	  var reserved, bound, cool sql.NullTime
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT address, chain, family, user_id, status, reserved_at, bound_at, cool_until FROM aa_addr_slots WHERE address=?`,
		   strings.TrimSpace(address),
	   ).Scan(&s.Address, &s.Chain, &s.Family, &s.UserID, &s.Status, &reserved, &bound, &cool)
	   if errors.Is(err, sql.ErrNoRows) {
		  return AddrSlot{}, ErrNotFound
	  }
	   if err != nil {
		   return AddrSlot{}, err
	  }
	  if reserved.Valid {
		  s.Reserved = reserved.Time
	  }
	   if bound.Valid {
		   s.BoundAt = bound.Time
	   }
	  if cool.Valid {
		  s.CoolUntil = cool.Time
	  }
	   return s, nil
   }

  func (r *Slots) ListChain(ctx context.Context, chain string) ([]AddrSlot, error) {
	   if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	  }
	   q := `SELECT address, chain, family, user_id, status, reserved_at, bound_at, cool_until FROM aa_addr_slots`
	  args := []any{}
	   if strings.TrimSpace(chain) != "" {
		   q += ` WHERE chain=?`
		  args = append(args, strings.ToUpper(strings.TrimSpace(chain)))
	   }
	   rows,
		  err := r.db.Conn(ctx).QueryContext(ctx, q, args...)
	   if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	   out := make([]AddrSlot, 0)
	   for rows.Next() {
		   var s AddrSlot
		   var reserved, bound, cool sql.NullTime
		  if err := rows.Scan(&s.Address, &s.Chain, &s.Family, &s.UserID, &s.Status, &reserved, &bound, &cool); err != nil {
			   return nil, err
		  }
		   if reserved.Valid {
			  s.Reserved = reserved.Time
		   }
		  if bound.Valid {
			   s.BoundAt = bound.Time
		  }
		   if cool.Valid {
			   s.CoolUntil = cool.Time
		  }
		   out = append(out, s)
	  }
	   return out, rows.Err()
   }

  func nullTime(t time.Time) any {
	   if t.IsZero() {
		  return nil
	  }
	  return t
  }
