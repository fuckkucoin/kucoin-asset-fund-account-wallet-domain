package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// TravelThreshold 走廊门槛 SQL 行。

type TravelThreshold struct {
	Corridor string
	Asset     string
	MinAmount string
	UpdatedAt time.Time
}

// TravelUSDRate 旅行规则 USD 牌价 SQL 行。

type TravelUSDRate struct {
	Asset string
	USD string
	Source string
	UpdatedAt time.Time
}

// TravelThreshStore 旅行规则门槛与牌价持久化。
type TravelThreshStore struct{ db *persist.SQLDB }

   func NewTravelThresh(db *persist.SQLDB) *TravelThreshStore { return &TravelThreshStore{db: db} }

// PutThreshold upsert 走廊门槛。
  func (r *TravelThreshStore) PutThreshold(ctx context.Context, row TravelThreshold) error {
	  if !sqlOn(r.db) {
		   return ErrUnavailable
	   }
	  row.Corridor = strings.ToUpper(strings.TrimSpace(row.Corridor))
	   row.Asset = strings.ToUpper(strings.TrimSpace(row.Asset))
	   row.MinAmount = strings.TrimSpace(row.MinAmount)
	   if row.Corridor == "" {
		   row.Corridor = "DEFAULT"
	  }
	  if row.MinAmount == "" {
		   row.MinAmount = "0"
	   }
	  if row.UpdatedAt.IsZero() {
		  row.UpdatedAt = time.Now().UTC()
	   }
	   stmt := `INSERT INTO aa_travel_thresholds (corridor, asset, min_amount, updated_at) VALUES (?,?,?,?)
ON DUPLICATE KEY UPDATE min_amount=VALUES(min_amount), updated_at=VALUES(updated_at)`
	  if r.db.Dialect == persist.Postgres {
		  stmt = `INSERT INTO aa_travel_thresholds (corridor, asset, min_amount, updated_at) VALUES (?,?,?,?)
ON CONFLICT (corridor, asset) DO UPDATE SET min_amount=EXCLUDED.min_amount, updated_at=EXCLUDED.updated_at`
	  }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, stmt, row.Corridor, row.Asset, row.MinAmount, row.UpdatedAt)
	  return err
   }

// ListThresholds 列出全部走廊门槛。
   func (r *TravelThreshStore) ListThresholds(ctx context.Context) ([]TravelThreshold, error) {
	   if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	   }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx,
		   `SELECT corridor, asset, min_amount, updated_at FROM aa_travel_thresholds ORDER BY corridor, asset`)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   out := make([]TravelThreshold, 0)
	   for rows.Next() {
		  var row TravelThreshold
		  if err := rows.Scan(&row.Corridor, &row.Asset, &row.MinAmount, &row.UpdatedAt); err != nil {
			  return nil, err
		   }
		   out = append(out, row)
	  }
	   return out, rows.Err()
   }

// ReplaceThresholds 事务内清空并写入全量表（不含 DEFAULT 元数据）。
// 运营 replace 面；DEFAULT 面值走 meta。
// 事务失败不得留下半截表。

  func (r *TravelThreshStore) ReplaceThresholds(ctx context.Context, rows []TravelThreshold) error {
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	   return r.db.InTx(ctx, func(txCtx context.Context) error {
		   if _, err := r.db.Conn(txCtx).ExecContext(txCtx, `DELETE FROM aa_travel_thresholds`); err != nil {
			   return err
		   }
		  for _, row := range rows {
			  if strings.EqualFold(strings.TrimSpace(row.Corridor), "DEFAULT") && strings.TrimSpace(row.Asset) == "" {
				   continue
			  }
			  if err := r.PutThreshold(txCtx, row); err != nil {
				   return err
			  }
		   }
		  return nil
	   })
   }

// PutUSDRate upsert 资产 USD 牌价。
  func (r *TravelThreshStore) PutUSDRate(ctx context.Context, row TravelUSDRate) error {
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	   }
	   row.Asset = strings.ToUpper(strings.TrimSpace(row.Asset))
	   row.USD = strings.TrimSpace(row.USD)
	  row.Source = strings.TrimSpace(row.Source)
	  if row.Asset == "" || row.USD == "" {
		  return ErrAmount
	  }
	  if row.Source == "" {
		   row.Source = "static"
	   }
	   if row.UpdatedAt.IsZero() {
		   row.UpdatedAt = time.Now().UTC()
	  }
	   stmt := `INSERT INTO aa_travel_usd_rates (asset, usd, source, updated_at) VALUES (?,?,?,?)
ON DUPLICATE KEY UPDATE usd=VALUES(usd), source=VALUES(source), updated_at=VALUES(updated_at)`
	  if r.db.Dialect == persist.Postgres {
		   stmt = `INSERT INTO aa_travel_usd_rates (asset, usd, source, updated_at) VALUES (?,?,?,?)
ON CONFLICT (asset) DO UPDATE SET usd=EXCLUDED.usd, source=EXCLUDED.source, updated_at=EXCLUDED.updated_at`
	  }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, stmt, row.Asset, row.USD, row.Source, row.UpdatedAt)
	  return err
  }

// ListUSDRates 列出全部 USD 牌价。

  func (r *TravelThreshStore) ListUSDRates(ctx context.Context) ([]TravelUSDRate, error) {
	   if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	   }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx,
		   `SELECT asset, usd, source, updated_at FROM aa_travel_usd_rates ORDER BY asset`)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   out := make([]TravelUSDRate, 0)
	  for rows.Next() {
		  var row TravelUSDRate
		   if err := rows.Scan(&row.Asset, &row.USD, &row.Source, &row.UpdatedAt); err != nil {
			   return nil, err
		  }
		  out = append(out, row)
	   }
	  return out, rows.Err()
   }

// ReplaceUSDRates 事务内替换全部牌价。
   func (r *TravelThreshStore) ReplaceUSDRates(ctx context.Context, rows []TravelUSDRate) error {
	  if !sqlOn(r.db) {
		   return ErrUnavailable
	   }
	  return r.db.InTx(ctx, func(txCtx context.Context) error {
		  if _, err := r.db.Conn(txCtx).ExecContext(txCtx, `DELETE FROM aa_travel_usd_rates`); err != nil {
			   return err
		   }
		   for _, row := range rows {
			   if err := r.PutUSDRate(txCtx, row); err != nil {
				  return err
			  }
		  }
		   return nil
	   })
   }

// PutMeta 写入门槛元数据（如 default_threshold）。
   func (r *TravelThreshStore) PutMeta(ctx context.Context, key, val string) error {
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	   }
	   key = strings.TrimSpace(key)
	   if key == "" {
		   return ErrConflict
	  }
	  now := time.Now().UTC()
	  stmt := `INSERT INTO aa_travel_thresh_meta (k, v, updated_at) VALUES (?,?,?)
ON DUPLICATE KEY UPDATE v=VALUES(v), updated_at=VALUES(updated_at)`
	   if r.db.Dialect == persist.Postgres {
		  stmt = `INSERT INTO aa_travel_thresh_meta (k, v, updated_at) VALUES (?,?,?)
ON CONFLICT (k) DO UPDATE SET v=EXCLUDED.v, updated_at=EXCLUDED.updated_at`
	   }
	   _,
		  err := r.db.Conn(ctx).ExecContext(ctx, stmt, key, strings.TrimSpace(val), now)
	  return err
  }

// GetMeta 读取门槛元数据。
   func (r *TravelThreshStore) GetMeta(ctx context.Context, key string) (string, error) {
	  if !sqlOn(r.db) {
		   return "", ErrUnavailable
	  }
	  var v string
	   err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT v FROM aa_travel_thresh_meta WHERE k=?`, strings.TrimSpace(key)).Scan(&v)
	   if errors.Is(err, sql.ErrNoRows) {
		  return "", ErrNotFound
	  }
	   return v, err
  }
