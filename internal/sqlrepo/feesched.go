package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

type FeeSchedule struct {
	ID    string
	Kind  string
	Asset string
	BPS int
	MinFee string
	MaxFee string
	Flat   string
	Active bool
	Version int
}

type FeeSched struct{ db *persist.SQLDB }

   func NewFeeSched(db *persist.SQLDB) *FeeSched { return &FeeSched{db: db} }

  func (r *FeeSched) Put(ctx context.Context, s FeeSchedule) error {
	  s.Kind = strings.ToLower(strings.TrimSpace(s.Kind))
	  s.Asset = strings.ToUpper(strings.TrimSpace(s.Asset))
	   if s.Asset == "" {
		   s.Asset = "*"
	   }
	  if s.ID == "" || s.Kind == "" {
		   return ErrConflict
	  }
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	   }
	  act := 0
	   if s.Active {
		  act = 1
	   }
	  q := `INSERT INTO aa_fee_schedules (id, kind, asset, bps, min_fee, max_fee, flat, active, version) VALUES (?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE id=VALUES(id), bps=VALUES(bps), min_fee=VALUES(min_fee), max_fee=VALUES(max_fee), flat=VALUES(flat), active=VALUES(active), version=VALUES(version)`
	   if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO aa_fee_schedules (id, kind, asset, bps, min_fee, max_fee, flat, active, version) VALUES (?,?,?,?,?,?,?,?,?) 
ON CONFLICT (kind, asset) DO UPDATE SET id=EXCLUDED.id, bps=EXCLUDED.bps, min_fee=EXCLUDED.min_fee, max_fee=EXCLUDED.max_fee, flat=EXCLUDED.flat, active=EXCLUDED.active, version=EXCLUDED.version`
	  }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, q, s.ID, s.Kind, s.Asset, s.BPS, s.MinFee, s.MaxFee, s.Flat, act, s.Version)
	   return err
   }

   func (r *FeeSched) Get(ctx context.Context, kind, asset string) (FeeSchedule, error) {
	   if !sqlOn(r.db) {
		   return FeeSchedule{}, ErrUnavailable
	   }
	  var s FeeSchedule
	   var act int
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT id, kind, asset, bps, min_fee, max_fee, flat, active, version FROM aa_fee_schedules WHERE kind=? AND asset=?`,
		   strings.ToLower(strings.TrimSpace(kind)), strings.ToUpper(strings.TrimSpace(asset)),
	  ).Scan(&s.ID, &s.Kind, &s.Asset, &s.BPS, &s.MinFee, &s.MaxFee, &s.Flat, &act, &s.Version)
	   if errors.Is(err, sql.ErrNoRows) {
		  return FeeSchedule{}, ErrNotFound
	  }
	   if err != nil {
		   return FeeSchedule{}, err
	   }
	  s.Active = act != 0
	   return s, nil
   }

  func (r *FeeSched) List(ctx context.Context) ([]FeeSchedule, error) {
	   if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	   }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT id, kind, asset, bps, min_fee, max_fee, flat, active, version FROM aa_fee_schedules ORDER BY kind, asset`)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   out := make([]FeeSchedule, 0)
	   for rows.Next() {
		  var s FeeSchedule
		  var act int
		   if err := rows.Scan(&s.ID, &s.Kind, &s.Asset, &s.BPS, &s.MinFee, &s.MaxFee, &s.Flat, &act, &s.Version); err != nil {
			   return nil, err
		  }
		   s.Active = act != 0
		   out = append(out, s)
	  }
	  return out, rows.Err()
   }
