package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

type FundPrefRow struct {
	UserID string
	Body []byte
	UpdatedAt time.Time
	Version   int64
}

type FundPrefAudit struct {
	ID     string
	UserID string
	Actor  string
	Action string
	Before string
	After  string
	At     time.Time
}

type FundPref struct{ db *persist.SQLDB }

  func NewFundPref(db *persist.SQLDB) *FundPref { return &FundPref{db: db} }

   func (r *FundPref) Put(ctx context.Context, row FundPrefRow) error {
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	   row.UserID = strings.TrimSpace(row.UserID)
	  if row.UserID == "" {
		  return ErrConflict
	   }
	  if row.UpdatedAt.IsZero() {
		  row.UpdatedAt = time.Now().UTC()
	   }
	   q := `INSERT INTO aa_fund_pref (user_id, body, version, updated_at) VALUES (?,?,?,?)
ON DUPLICATE KEY UPDATE body=VALUES(body), version=VALUES(version), updated_at=VALUES(updated_at)`
	   if r.db.Dialect == persist.Postgres {
		  q = `INSERT INTO aa_fund_pref (user_id, body, version, updated_at) VALUES (?,?,?,?) 
ON CONFLICT (user_id) DO UPDATE SET body=EXCLUDED.body, version=EXCLUDED.version, updated_at=EXCLUDED.updated_at`
	   }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, q, row.UserID, row.Body, row.Version, row.UpdatedAt)
	   return err
  }

   func (r *FundPref) Get(ctx context.Context, userID string) (FundPrefRow, bool, error) {
	   if !sqlOn(r.db) {
		   return FundPrefRow{}, false, ErrUnavailable
	  }
	   var row FundPrefRow
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT user_id, body, version, updated_at FROM aa_fund_pref WHERE user_id=?`,
		   strings.TrimSpace(userID),
	  ).Scan(&row.UserID, &row.Body, &row.Version, &row.UpdatedAt)
	   if errors.Is(err, sql.ErrNoRows) {
		   return FundPrefRow{}, false, nil
	   }
	  if err != nil {
		  return FundPrefRow{}, false, err
	  }
	  return row, true, nil
  }

  func (r *FundPref) PutAudit(ctx context.Context, rec FundPrefAudit) error {
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	   }
	  if strings.TrimSpace(rec.ID) == "" || strings.TrimSpace(rec.UserID) == "" {
		   return ErrConflict
	  }
	  if rec.At.IsZero() {
		  rec.At = time.Now().UTC()
	  }
	   _, err := r.db.Conn(ctx).ExecContext(ctx,
		   `INSERT INTO aa_fund_pref_audit (id, user_id, actor, action, before_body, after_body, at) VALUES (?,?,?,?,?,?,?)`,
		  rec.ID, rec.UserID, rec.Actor, rec.Action, rec.Before, rec.After, rec.At)
	   return err
  }

  func (r *FundPref) ListAudit(ctx context.Context, userID string, limit int) ([]FundPrefAudit, error) {
	  if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	  }
	   if limit <= 0 {
		   limit = 50
	   }
	   q := `SELECT id, user_id, actor, action, before_body, after_body, at FROM aa_fund_pref_audit`
	   args := []any{}
	   if strings.TrimSpace(userID) != "" {
		  q += ` WHERE user_id=?`
		   args = append(args,
			  strings.TrimSpace(userID))
	  }
	   q += ` ORDER BY at DESC LIMIT ?`
	  args = append(args, limit)
	   rows, err := r.db.Conn(ctx).QueryContext(ctx, q, args...)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   out := make([]FundPrefAudit, 0)
	   for rows.Next() {
		   var rec FundPrefAudit
		  if err := rows.Scan(&rec.ID, &rec.UserID, &rec.Actor, &rec.Action, &rec.Before, &rec.After, &rec.At); err != nil {
			   return nil, err
		   }
		   out = append(out, rec)
	  }
	  return out, rows.Err()
   }
