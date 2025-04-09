package exjob

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// SQLStore 导出任务元数据仓储。

type SQLStore struct {
	db *sql.DB
}

// NewSQLStore 绑定 *sql.DB。

   func NewSQLStore(db *sql.DB) *SQLStore {
	   if db == nil {
		   return nil
	   }
	  return &SQLStore{db: db}
  }

// BindSQL 挂 persist.SQLDB 的底层连接。

  func (b *Book) BindSQL(db *persist.SQLDB) *Book {
	  if b == nil || db == nil || db.DB == nil {
		   return b
	   }
	   return b.BindSQLStore(NewSQLStore(db.DB))
   }

// BindSQLStore 直接挂已构造的元数据仓储。

   func (b *Book) BindSQLStore(s *SQLStore) *Book {
	   if b != nil {
		  b.sql = s
	   }
	   return b
   }

// UpsertMeta 写入或更新任务元数据（不含 body）。
  func (s *SQLStore) UpsertMeta(ctx context.Context, j Job) error {
	  if s == nil || s.db == nil {
		   return nil
	   }
	  _, err := s.db.ExecContext(ctx, ` 
INSERT INTO aa_export_job
  (id, user_id, kind, format, status, start_at, end_at, checksum, body_bytes, error_msg, created_at, updated_at, expired_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  status=VALUES(status), checksum=VALUES(checksum), body_bytes=VALUES(body_bytes),
  error_msg=VALUES(error_msg), updated_at=VALUES(updated_at), expired_at=VALUES(expired_at)`,
		   j.ID, j.UserID, j.Kind, j.Format, j.Status,
		  nullTime(j.StartAt),
		  nullTime(j.EndAt),
		  j.Checksum, len(j.Body), j.Err,
		   j.CreatedAt.UTC(), time.Now().UTC(),
		  nullTime(j.ExpiredAt),
	   )
	  return err
   }

// MarkExpired 批量将超时 queued/running 标 expired。
   func (s *SQLStore) MarkExpired(ctx context.Context, before time.Time) (int64, error) {
	   if s == nil || s.db == nil {
		  return 0, nil
	   }
	  res, err := s.db.ExecContext(ctx, `
UPDATE aa_export_job
SET status=?, updated_at=?, expired_at=?
WHERE status IN (?, ?) AND created_at < ? AND expired_at IS NULL`,
		   StatusExpired, time.Now().UTC(), time.Now().UTC(),
		   StatusQueued, StatusRunning, before.UTC(),
	  )
	  if err != nil {
		  return 0, err
	   }
	  return res.RowsAffected()
  }

// CountToday 统计用户当日已创建任务数（含 expired，不含 failed 重试占用策略由上层定）。
   func (s *SQLStore) CountToday(ctx context.Context, userID string, dayStart time.Time) (int, error) {
	   if s == nil || s.db == nil {
		   return 0, nil
	   }
	   userID = strings.TrimSpace(userID)
	  var n int
	  err := s.db.QueryRowContext(ctx, `
SELECT COUNT(1) FROM aa_export_job WHERE user_id=? AND created_at>=?`,
		   userID, dayStart.UTC(),
	   ).Scan(&n)
	  return n, err
   }

   func nullTime(t time.Time) any {
	   if t.IsZero() {
		   return nil
	   }
	   return t.UTC()
  }
