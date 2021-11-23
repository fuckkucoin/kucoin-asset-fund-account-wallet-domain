package sqlrepo

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// SubKeyAuditRow 审计落盘行。
type SubKeyAuditRow struct {
	ID string
	Owner   string
	SubName string
	APIKey  string
	Action string
	Detail    string
	ClientIP  string
	ActorIP   string
	CreatedAt int64
}

// SubKeyQuotaRow 配额行。
type SubKeyQuotaRow struct {
	Owner     string
	SubName   string
	MaxKeys   int
	UpdatedAt int64
}

   func (r *FundingSubjects) PutSubKeyAudit(ctx context.Context, row SubKeyAuditRow) error {
	  if !sqlOn(r.db) {
		   return ErrUnavailable
	  }
	   row.ID = strings.TrimSpace(row.ID)
	  if row.ID == "" {
		  return ErrConflict
	  }
	  actor := strings.TrimSpace(row.ActorIP)
	   if actor == "" {
		  actor = strings.TrimSpace(row.ClientIP)
	   }
	  q := `INSERT INTO aa_sub_api_key_audit (id, owner, sub_name, api_key, action, detail, client_ip, actor_ip, created_at) 
VALUES (?,?,?,?,?,?,?,?,?)`
	  _, err := r.db.Conn(ctx).ExecContext(ctx, q, row.ID, row.Owner, row.SubName, row.APIKey, row.Action, row.Detail, actor, actor, row.CreatedAt)
	   return err
   }

  func (r *FundingSubjects) ListSubKeyAudit(ctx context.Context, owner string, limit int) ([]SubKeyAuditRow, error) {
	  if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	   }
	  if limit <= 0 || limit > 500 {
		  limit = 200
	  }
	   owner = strings.TrimSpace(owner)
	  var rows *sql.Rows
	  var err error
	  if owner != "" {
		   rows, err = r.db.Conn(ctx).QueryContext(ctx,
			  `SELECT id, owner, sub_name, api_key, action, detail, client_ip, COALESCE(actor_ip,''), created_at FROM aa_sub_api_key_audit WHERE owner=? ORDER BY created_at DESC LIMIT ?`,
			   owner, limit)
	  } else {
		   rows, err = r.db.Conn(ctx).QueryContext(ctx,
			   `SELECT id, owner, sub_name, api_key, action, detail, client_ip, COALESCE(actor_ip,''), created_at FROM aa_sub_api_key_audit ORDER BY created_at DESC LIMIT ?`,
			  limit)
	   }
	  if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   out := make([]SubKeyAuditRow, 0)
	   for rows.Next() {
		   var row SubKeyAuditRow
		   if err := rows.Scan(&row.ID, &row.Owner, &row.SubName, &row.APIKey, &row.Action, &row.Detail, &row.ClientIP, &row.ActorIP, &row.CreatedAt); err != nil {
			  return nil, err
		   }
		   out = append(out, row)
	  }
	   return out, rows.Err()
  }

// ListSubKeyAuditFiltered 按母户过滤。owner 为空拒绝，避免跨户扫审计。
   func (r *FundingSubjects) ListSubKeyAuditFiltered(ctx context.Context,
	   owner, subName, apiKey, action string, limit int, since, until int64) ([]SubKeyAuditRow, error) {
	   if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	  }
	  owner = strings.TrimSpace(owner)
	   if owner == "" {
		  return nil, ErrConflict
	   }
	  subName = strings.TrimSpace(subName)
	  apiKey = strings.TrimSpace(apiKey)
	  action = strings.TrimSpace(action)
	   if limit <= 0 || limit > 200 {
		   limit = 200
	   }
	   q := `SELECT id, owner, sub_name, api_key, action, detail, client_ip, COALESCE(actor_ip,''), created_at FROM aa_sub_api_key_audit WHERE owner=?`
	   args := []any{owner}
	  if subName != "" {
		   q += ` AND sub_name=?`
		   args = append(args, subName)
	   }
	  if apiKey != "" {
		  q += ` AND api_key=?`
		   args = append(args, apiKey)
	  }
	   if action != "" {
		  q += ` AND action=?`
		  args = append(args, action)
	  }
	  if since > 0 {
		   q += ` AND created_at>=?`
		  args = append(args, since)
	   }
	   if until > 0 {
		  q += ` AND created_at<=?`
		  args = append(args, until)
	  }
	   q += ` ORDER BY created_at DESC LIMIT ?`
	   args = append(args, limit)
	  rows,
		  err := r.db.Conn(ctx).QueryContext(ctx, q, args...)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  out := make([]SubKeyAuditRow, 0)
	   for rows.Next() {
		   var row SubKeyAuditRow
		   if err := rows.Scan(&row.ID, &row.Owner, &row.SubName, &row.APIKey, &row.Action, &row.Detail, &row.ClientIP, &row.ActorIP, &row.CreatedAt); err != nil {
			  return nil, err
		  }
		   out = append(out, row)
	  }
	   return out, rows.Err()
   }

// SweepSubKeyAudit 删除某母户早于 beforeMs 的审计行。缺 owner 或时间则拒绝。
  func (r *FundingSubjects) SweepSubKeyAudit(ctx context.Context, owner string, beforeMs int64) (int, error) {
	  if !sqlOn(r.db) {
		   return 0, ErrUnavailable
	   }
	  owner = strings.TrimSpace(owner)
	   if owner == "" || beforeMs <= 0 {
		   return 0, ErrConflict
	   }
	  res, err := r.db.Conn(ctx).ExecContext(ctx, `DELETE FROM aa_sub_api_key_audit WHERE owner=? AND created_at<?`, owner, beforeMs)
	   if err != nil {
		   return 0, err
	  }
	  n, _ := res.RowsAffected()
	   return int(n), nil
   }

  func (r *FundingSubjects) PutSubKeyQuota(ctx context.Context, row SubKeyQuotaRow) error {
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	  row.Owner = strings.TrimSpace(row.Owner)
	  if row.Owner == "" || row.MaxKeys <= 0 {
		  return ErrConflict
	  }
	  if row.UpdatedAt == 0 {
		  row.UpdatedAt = time.Now().UTC().UnixMilli()
	  }
	   q := `INSERT INTO aa_sub_key_quota (owner, sub_name, max_keys, updated_at) VALUES (?,?,?,?)
ON DUPLICATE KEY UPDATE max_keys=VALUES(max_keys), updated_at=VALUES(updated_at)`
	  if r.db.Dialect == persist.Postgres {
		  q = `INSERT INTO aa_sub_key_quota (owner, sub_name, max_keys, updated_at) VALUES (?,?,?,?)
ON CONFLICT (owner, sub_name) DO UPDATE SET max_keys=EXCLUDED.max_keys, updated_at=EXCLUDED.updated_at`
	   }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, q, row.Owner, row.SubName, row.MaxKeys, row.UpdatedAt)
	  return err
  }

   func (r *FundingSubjects) ListSubKeyQuota(ctx context.Context) ([]SubKeyQuotaRow, error) {
	  if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	  }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT owner, sub_name, max_keys, updated_at FROM aa_sub_key_quota`)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   out := make([]SubKeyQuotaRow, 0)
	  for rows.Next() {
		  var row SubKeyQuotaRow
		  if err := rows.Scan(&row.Owner, &row.SubName, &row.MaxKeys, &row.UpdatedAt); err != nil {
			  return nil, err
		   }
		   out = append(out, row)
	  }
	  return out, rows.Err()
  }
