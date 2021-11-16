package sqlrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

type CfgPack struct {
	Name string
	Version int
	MaxSingle string
	DailyWithdraw string
	HourlyVel     int
	DailyVel      int
	AssetMax      map[string]string
	Killed bool
}

type CfgFlag struct {
	Name string
	PinVersion int
	Killed     bool
}

type CfgStore struct{ db *persist.SQLDB }

   func NewCfgStore(db *persist.SQLDB) *CfgStore { return &CfgStore{db: db} }

  func encodeAssetMax(m map[string]string) (string, error) {
	  if len(m) == 0 {
		   return "", nil
	   }
	   b, err := json.Marshal(m)
	   if err != nil {
		  return "", err
	   }
	  return string(b), nil
  }

  func decodeAssetMax(s string) map[string]string {
	  s = strings.TrimSpace(s)
	  if s == "" {
		   return nil
	  }
	  out := map[string]string{}
	   if err := json.Unmarshal([]byte(s), &out); err != nil {
		   return nil
	  }
	  return out
  }

   func (r *CfgStore) PutPack(ctx context.Context, p CfgPack) error {
	  p.Name = strings.TrimSpace(p.Name)
	   if p.Name == "" || p.Version <= 0 {
		  return ErrConflict
	   }
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	  body,
		   err := encodeAssetMax(p.AssetMax)
	  if err != nil {
		   return ErrConflict
	   }
	  killed := 0
	   if p.Killed {
		   killed = 1
	  }
	   q := `INSERT INTO aa_cfg_packs (name, version, max_single, daily_withdraw, hourly_vel, daily_vel, asset_max, killed) VALUES (?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE max_single=VALUES(max_single), daily_withdraw=VALUES(daily_withdraw), hourly_vel=VALUES(hourly_vel), daily_vel=VALUES(daily_vel), asset_max=VALUES(asset_max), killed=VALUES(killed)`
	  if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO aa_cfg_packs (name, version, max_single, daily_withdraw, hourly_vel, daily_vel, asset_max, killed) VALUES (?,?,?,?,?,?,?,?)
ON CONFLICT (name, version) DO UPDATE SET max_single=EXCLUDED.max_single, daily_withdraw=EXCLUDED.daily_withdraw, hourly_vel=EXCLUDED.hourly_vel, daily_vel=EXCLUDED.daily_vel, asset_max=EXCLUDED.asset_max, killed=EXCLUDED.killed`
	  }
	  _,
		   err = r.db.Conn(ctx).ExecContext(ctx, q, p.Name, p.Version, p.MaxSingle, p.DailyWithdraw, p.HourlyVel, p.DailyVel, body, killed)
	  return err
   }

   func (r *CfgStore) ListPacks(ctx context.Context) ([]CfgPack, error) {
	  if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	  }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT name, version, max_single, daily_withdraw, hourly_vel, daily_vel, asset_max, killed FROM aa_cfg_packs ORDER BY name, version`)
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   out := make([]CfgPack, 0)
	  for rows.Next() {
		  var p CfgPack
		  var body string
		   var killed int
		   if err := rows.Scan(&p.Name, &p.Version, &p.MaxSingle, &p.DailyWithdraw, &p.HourlyVel, &p.DailyVel, &body, &killed); err != nil {
			   return nil, err
		  }
		   p.AssetMax = decodeAssetMax(body)
		   p.Killed = killed != 0
		  out = append(out, p)
	  }
	  return out, rows.Err()
  }

   func (r *CfgStore) History(ctx context.Context, name string) ([]CfgPack, error) {
	  if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	   }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx,
		   `SELECT name, version, max_single, daily_withdraw, hourly_vel, daily_vel, asset_max, killed FROM aa_cfg_packs WHERE name=? ORDER BY version`,
		  strings.TrimSpace(name))
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  out := make([]CfgPack, 0)
	   for rows.Next() {
		   var p CfgPack
		   var body string
		  var killed int
		  if err := rows.Scan(&p.Name, &p.Version, &p.MaxSingle, &p.DailyWithdraw, &p.HourlyVel, &p.DailyVel, &body, &killed); err != nil {
			  return nil, err
		  }
		   p.AssetMax = decodeAssetMax(body)
		  p.Killed = killed != 0
		  out = append(out, p)
	   }
	  return out, rows.Err()
  }

  func (r *CfgStore) PutFlag(ctx context.Context, f CfgFlag) error {
	   f.Name = strings.TrimSpace(f.Name)
	   if f.Name == "" {
		   return ErrConflict
	   }
	   if !sqlOn(r.db) {
		   return ErrUnavailable
	   }
	   killed := 0
	   if f.Killed {
		   killed = 1
	   }
	  q := `INSERT INTO aa_cfg_flags (name, pin_version, killed) VALUES (?,?,?)
ON DUPLICATE KEY UPDATE pin_version=VALUES(pin_version), killed=VALUES(killed)`
	  if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO aa_cfg_flags (name, pin_version, killed) VALUES (?,?,?)
ON CONFLICT (name) DO UPDATE SET pin_version=EXCLUDED.pin_version, killed=EXCLUDED.killed`
	  }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, q, f.Name, f.PinVersion, killed)
	   return err
   }

  func (r *CfgStore) GetFlag(ctx context.Context, name string) (CfgFlag, error) {
	  if !sqlOn(r.db) {
		  return CfgFlag{}, ErrUnavailable
	  }
	   var f CfgFlag
	   var killed int
	   err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT name, pin_version, killed FROM aa_cfg_flags WHERE name=?`,
		   strings.TrimSpace(name),
	   ).Scan(&f.Name, &f.PinVersion, &killed)
	   if errors.Is(err, sql.ErrNoRows) {
		  return CfgFlag{}, ErrNotFound
	   }
	  if err != nil {
		   return CfgFlag{}, err
	  }
	   f.Killed = killed != 0
	  return f, nil
  }

  func (r *CfgStore) ListFlags(ctx context.Context) ([]CfgFlag, error) {
	   if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	   }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT name, pin_version, killed FROM aa_cfg_flags ORDER BY name`)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   out := make([]CfgFlag, 0)
	   for rows.Next() {
		   var f CfgFlag
		   var killed int
		  if err := rows.Scan(&f.Name, &f.PinVersion, &killed); err != nil {
			  return nil, err
		  }
		   f.Killed = killed != 0
		  out = append(out, f)
	  }
	   return out, rows.Err()
  }
