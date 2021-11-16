package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

type RiskCap struct {
	UserID string
	Asset string
	Max   string
}

type RiskItem struct {
	ID     string
	UserID string
	Reason string
	Asset  string
	Amount string
	Dest   string
	Status string
	Actor    string
	Fence    persist.Fence
	OpenedAt time.Time
}

type RiskPack struct{ db *persist.SQLDB }

   func NewRiskPack(db *persist.SQLDB) *RiskPack { return &RiskPack{db: db} }

   func (r *RiskPack) PutCap(ctx context.Context, c RiskCap) error {
	   c.UserID = strings.TrimSpace(c.UserID)
	   c.Asset = strings.ToUpper(strings.TrimSpace(c.Asset))
	  c.Max = strings.TrimSpace(c.Max)
	  if c.Asset == "" || c.Max == "" {
		   return ErrConflict
	   }
	  if !sqlOn(r.db) {
		   return ErrUnavailable
	   }
	  q := `INSERT INTO aa_risk_caps (user_id, asset, max_amt) VALUES (?,?,?)
ON DUPLICATE KEY UPDATE max_amt=VALUES(max_amt)`
	  if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO aa_risk_caps (user_id, asset, max_amt) VALUES (?,?,?)
ON CONFLICT (user_id, asset) DO UPDATE SET max_amt=EXCLUDED.max_amt`
	   }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, q, c.UserID, c.Asset, c.Max)
	   return err
   }

  func (r *RiskPack) GetCap(ctx context.Context, userID, asset string) (RiskCap, error) {
	   if !sqlOn(r.db) {
		  return RiskCap{}, ErrUnavailable
	   }
	   var c RiskCap
	   err := r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT user_id, asset, max_amt FROM aa_risk_caps WHERE user_id=? AND asset=?`,
		   strings.TrimSpace(userID),
		  strings.ToUpper(strings.TrimSpace(asset)),
	   ).Scan(&c.UserID, &c.Asset, &c.Max)
	   if errors.Is(err, sql.ErrNoRows) {
		   return RiskCap{}, ErrNotFound
	  }
	   return c, err
   }

   func (r *RiskPack) ListCaps(ctx context.Context) ([]RiskCap, error) {
	   if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	  }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT user_id, asset, max_amt FROM aa_risk_caps ORDER BY user_id, asset`)
	   if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	  out := make([]RiskCap, 0)
	  for rows.Next() {
		  var c RiskCap
		  if err := rows.Scan(&c.UserID, &c.Asset, &c.Max); err != nil {
			  return nil, err
		   }
		   out = append(out, c)
	  }
	  return out, rows.Err()
  }

   func (r *RiskPack) PutItem(ctx context.Context, it RiskItem) error {
	   it.ID = strings.TrimSpace(it.ID)
	  it.UserID = strings.TrimSpace(it.UserID)
	   if it.ID == "" || it.UserID == "" {
		   return ErrConflict
	  }
	   if !sqlOn(r.db) {
		   return ErrUnavailable
	  }
	   if it.OpenedAt.IsZero() {
		  it.OpenedAt = time.Now().UTC()
	  }
	  q := `INSERT INTO aa_risk_items (id, user_id, reason, asset, amount, dest, status, actor, fence_epoch, fence_token, opened_at) 
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE reason=VALUES(reason), asset=VALUES(asset), amount=VALUES(amount), dest=VALUES(dest), status=VALUES(status), actor=VALUES(actor), fence_epoch=VALUES(fence_epoch), fence_token=VALUES(fence_token)`
	   if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO aa_risk_items (id, user_id, reason, asset, amount, dest, status, actor, fence_epoch, fence_token, opened_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET reason=EXCLUDED.reason, asset=EXCLUDED.asset, amount=EXCLUDED.amount, dest=EXCLUDED.dest, status=EXCLUDED.status, actor=EXCLUDED.actor, fence_epoch=EXCLUDED.fence_epoch, fence_token=EXCLUDED.fence_token`
	  }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, q, it.ID, it.UserID, it.Reason, it.Asset, it.Amount, it.Dest, it.Status, it.Actor, it.Fence.Epoch, it.Fence.Token, it.OpenedAt)
	  return err
   }

   func (r *RiskPack) GetItem(ctx context.Context, id string) (RiskItem, error) {
	   if !sqlOn(r.db) {
		  return RiskItem{}, ErrUnavailable
	   }
	  var it RiskItem
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT id, user_id, reason, asset, amount, dest, status, actor, fence_epoch, fence_token, opened_at FROM aa_risk_items WHERE id=?`,
		  strings.TrimSpace(id),
	   ).Scan(&it.ID, &it.UserID, &it.Reason, &it.Asset, &it.Amount, &it.Dest, &it.Status, &it.Actor, &it.Fence.Epoch, &it.Fence.Token, &it.OpenedAt)
	  if errors.Is(err, sql.ErrNoRows) {
		  return RiskItem{}, ErrNotFound
	  }
	   return it, err
  }

   func (r *RiskPack) ListItems(ctx context.Context) ([]RiskItem, error) {
	  if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	   }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT id, user_id, reason, asset, amount, dest, status, actor, fence_epoch, fence_token, opened_at FROM aa_risk_items ORDER BY opened_at`)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	  out := make([]RiskItem, 0)
	  for rows.Next() {
		  var it RiskItem
		  if err := rows.Scan(&it.ID, &it.UserID, &it.Reason, &it.Asset, &it.Amount, &it.Dest, &it.Status, &it.Actor, &it.Fence.Epoch, &it.Fence.Token, &it.OpenedAt); err != nil {
			   return nil, err
		   }
		   out = append(out, it)
	   }
	   return out, rows.Err()
   }

   func (r *RiskPack) ListPending(ctx context.Context, limit int) ([]RiskItem, error) {
	  if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	  }
	   if limit <= 0 {
		  limit = 50
	  }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx,
		  `SELECT id, user_id, reason, asset, amount, dest, status, actor, fence_epoch, fence_token, opened_at FROM aa_risk_items WHERE status IN ('queued','assigned') ORDER BY opened_at LIMIT ?`,
		  limit)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   out := make([]RiskItem, 0)
	  for rows.Next() {
		   var it RiskItem
		   if err := rows.Scan(&it.ID, &it.UserID, &it.Reason, &it.Asset, &it.Amount, &it.Dest, &it.Status, &it.Actor, &it.Fence.Epoch, &it.Fence.Token, &it.OpenedAt); err != nil {
			  return nil, err
		  }
		   out = append(out, it)
	  }
	   return out, rows.Err()
  }
