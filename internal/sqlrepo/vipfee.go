package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

type VIPSchedule struct {
	ID string
	Tier string
	Kind   string
	Asset  string
	BPS    int
	Active bool
	Version int
}

type VIPPin struct {
	UserID string
	Tier  string
	Actor string
	Note  string
	At    time.Time
}

type VIPFill struct {
	ID       string
	UserID   string
	Asset    string
	Notional string
	Day      string
	At       time.Time
}

type VIPFee struct{ db *persist.SQLDB }

   func NewVIPFee(db *persist.SQLDB) *VIPFee { return &VIPFee{db: db} }

  func (r *VIPFee) PutSchedule(ctx context.Context, s VIPSchedule) error {
	   s.Tier = strings.TrimSpace(s.Tier)
	  s.Kind = strings.ToLower(strings.TrimSpace(s.Kind))
	  s.Asset = strings.ToUpper(strings.TrimSpace(s.Asset))
	   if s.Asset == "" {
		  s.Asset = "*"
	   }
	  if s.ID == "" || s.Tier == "" || s.Kind == "" {
		   return ErrConflict
	   }
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	   act := 0
	   if s.Active {
		  act = 1
	   }
	  q := `INSERT INTO aa_vip_schedules (id, tier, kind, asset, bps, active, version) VALUES (?,?,?,?,?,?,?) 
ON DUPLICATE KEY UPDATE id=VALUES(id), bps=VALUES(bps), active=VALUES(active), version=VALUES(version)`
	  if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO aa_vip_schedules (id, tier, kind, asset, bps, active, version) VALUES (?,?,?,?,?,?,?)
ON CONFLICT (tier, kind, asset) DO UPDATE SET id=EXCLUDED.id, bps=EXCLUDED.bps, active=EXCLUDED.active, version=EXCLUDED.version`
	   }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, q, s.ID, s.Tier, s.Kind, s.Asset, s.BPS, act, s.Version)
	  return err
  }

  func (r *VIPFee) ListSchedules(ctx context.Context) ([]VIPSchedule, error) {
	   if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	   }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT id, tier, kind, asset, bps, active, version FROM aa_vip_schedules ORDER BY tier, kind, asset`)
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   out := make([]VIPSchedule, 0)
	  for rows.Next() {
		   var s VIPSchedule
		   var act int
		  if err := rows.Scan(&s.ID, &s.Tier, &s.Kind, &s.Asset, &s.BPS, &act, &s.Version); err != nil {
			   return nil, err
		  }
		  s.Active = act != 0
		   out = append(out, s)
	   }
	   return out, rows.Err()
   }

  func (r *VIPFee) GetSchedule(ctx context.Context, tier, kind, asset string) (VIPSchedule, error) {
	   if !sqlOn(r.db) {
		  return VIPSchedule{}, ErrUnavailable
	  }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   var s VIPSchedule
	   var act int
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT id, tier, kind, asset, bps, active, version FROM aa_vip_schedules WHERE tier=? AND kind=? AND asset=?`,
		   strings.TrimSpace(tier), strings.ToLower(strings.TrimSpace(kind)), asset,
	  ).Scan(&s.ID,
		   &s.Tier, &s.Kind, &s.Asset, &s.BPS, &act, &s.Version)
	  if errors.Is(err, sql.ErrNoRows) {
		  return VIPSchedule{}, ErrNotFound
	  }
	  if err != nil {
		  return VIPSchedule{}, err
	   }
	  s.Active = act != 0
	  return s, nil
   }

  func (r *VIPFee) PutPin(ctx context.Context, p VIPPin) error {
	  p.UserID = strings.TrimSpace(p.UserID)
	   p.Tier = strings.TrimSpace(p.Tier)
	  if p.UserID == "" || p.Tier == "" {
		  return ErrConflict
	   }
	  if !sqlOn(r.db) {
		   return ErrUnavailable
	  }
	  if p.At.IsZero() {
		   p.At = time.Now().UTC()
	   }
	  q := `INSERT INTO aa_vip_pins (user_id, tier, actor, note, at) VALUES (?,?,?,?,?) 
ON DUPLICATE KEY UPDATE tier=VALUES(tier), actor=VALUES(actor), note=VALUES(note), at=VALUES(at)`
	  if r.db.Dialect == persist.Postgres {
		  q = `INSERT INTO aa_vip_pins (user_id, tier, actor, note, at) VALUES (?,?,?,?,?)
ON CONFLICT (user_id) DO UPDATE SET tier=EXCLUDED.tier, actor=EXCLUDED.actor, note=EXCLUDED.note, at=EXCLUDED.at`
	  }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, q, p.UserID, p.Tier, p.Actor, p.Note, p.At)
	  return err
  }

  func (r *VIPFee) DeletePin(ctx context.Context, userID string) error {
	   if !sqlOn(r.db) {
		   return ErrUnavailable
	  }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, `DELETE FROM aa_vip_pins WHERE user_id=?`, strings.TrimSpace(userID))
	   return err
   }

   func (r *VIPFee) GetPin(ctx context.Context, userID string) (VIPPin, error) {
	   if !sqlOn(r.db) {
		   return VIPPin{}, ErrUnavailable
	   }
	   var p VIPPin
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT user_id, tier, actor, note, at FROM aa_vip_pins WHERE user_id=?`,
		  strings.TrimSpace(userID),
	  ).Scan(&p.UserID, &p.Tier, &p.Actor, &p.Note, &p.At)
	  if errors.Is(err, sql.ErrNoRows) {
		  return VIPPin{}, ErrNotFound
	  }
	  return p, err
  }

  func (r *VIPFee) ListPins(ctx context.Context) ([]VIPPin, error) {
	  if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	   }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT user_id, tier, actor, note, at FROM aa_vip_pins ORDER BY user_id`)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   out := make([]VIPPin, 0)
	   for rows.Next() {
		   var p VIPPin
		   if err := rows.Scan(&p.UserID, &p.Tier, &p.Actor, &p.Note, &p.At); err != nil {
			   return nil, err
		   }
		  out = append(out, p)
	  }
	  return out, rows.Err()
   }

   func (r *VIPFee) PutFill(ctx context.Context, f VIPFill) error {
	  f.ID = strings.TrimSpace(f.ID)
	  f.UserID = strings.TrimSpace(f.UserID)
	  f.Asset = strings.ToUpper(strings.TrimSpace(f.Asset))
	   if f.ID == "" || f.UserID == "" || f.Asset == "" {
		   return ErrConflict
	  }
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	  if f.At.IsZero() {
		  f.At = time.Now().UTC()
	   }
	  q := `INSERT INTO aa_vip_fills (id, user_id, asset, notional, day, at) VALUES (?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE notional=VALUES(notional), day=VALUES(day), at=VALUES(at)`
	   if r.db.Dialect == persist.Postgres {
		  q = `INSERT INTO aa_vip_fills (id, user_id, asset, notional, day, at) VALUES (?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET notional=EXCLUDED.notional, day=EXCLUDED.day, at=EXCLUDED.at`
	  }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, q, f.ID, f.UserID, f.Asset, f.Notional, f.Day, f.At)
	  return err
   }

  func (r *VIPFee) ListFills(ctx context.Context) ([]VIPFill, error) {
	   if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	  }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT id, user_id, asset, notional, day, at FROM aa_vip_fills ORDER BY at`)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   out := make([]VIPFill, 0)
	   for rows.Next() {
		   var f VIPFill
		   if err := rows.Scan(&f.ID, &f.UserID, &f.Asset, &f.Notional, &f.Day, &f.At); err != nil {
			   return nil, err
		   }
		  out = append(out, f)
	   }
	  return out, rows.Err()
   }
