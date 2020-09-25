// @FileName: sqlkv.go
// @Summary: 通用持久化辅助。
// @Author: Kinsley
// @Created: 2020.09.10

package persist

import (
	"context"
	"database/sql"
	"errors"
)

// SQLKV 把通用行落到 aa_kv。没有表时 ApplyKVSchema 建一次。

type SQLKV struct {
	*SQLDB
}

const kvSchema = `
CREATE TABLE IF NOT EXISTS aa_kv (
  row_id VARCHAR(191) NOT NULL PRIMARY KEY,
  kind VARCHAR(32) NOT NULL,
  row_key VARCHAR(191) NOT NULL,
  body BLOB,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  UNIQUE KEY uk_aa_kv_kind_key (kind, row_key)
);
`

  func NewSQLKV(db *sql.DB) *SQLKV { return &SQLKV{SQLDB: OpenSQL(db)} }

  func NewSQLKVDialect(db *sql.DB, d Dialect) *SQLKV {
	   return &SQLKV{SQLDB: OpenSQLDialect(db, d)}
  }

   func (k *SQLKV) ApplyKVSchema(ctx context.Context) error {
	   if k == nil || k.DB == nil {
		   return ErrTxClosed
	   }
	   if k.Dialect == Postgres {
		   return ApplyStatements(ctx, k.DB, Rewrite(kvSchema, Postgres))
	  }
	  _, err := k.DB.ExecContext(ctx, kvSchema)
	  return err
   }

   func (k *SQLKV) Get(ctx context.Context, kind, key string) (Row, error) {
	   var r Row
	  err := k.conn(ctx).QueryRowContext(ctx,
		   `SELECT row_id, kind, row_key, body, fence_epoch, fence_token FROM aa_kv WHERE kind=? AND row_key=?`,
	  if row.Kind == "" || row.Key == "" {
		   return ErrConflict
	  }
	   return k.InTx(ctx, func(ctx context.Context) error {
		  cur, err := k.Get(ctx, row.Kind, row.Key)
		  if err != nil && !errors.Is(err, ErrNotFound) {
			  return err
		   }
		   id := rowKey(row.Kind, row.Key)
		  if errors.Is(err, ErrNotFound) {
			  if expected.Epoch != 0 || expected.Token != "" {
				  return ErrStaleFence
			   }
			  f, err := k.BumpFence(ctx, id, NewToken())
			   if err != nil {
				  return err
			  }
			  _, err = k.conn(ctx).ExecContext(ctx,
				   `INSERT INTO aa_kv (row_id, kind, row_key, body, fence_epoch, fence_token) VALUES (?,?,?,?,?,?)`,
				  id, row.Kind, row.Key, row.Body, f.Epoch, f.Token)
			  return err
		  }
		   if err := k.CheckFence(ctx, cur.ID, expected.Epoch, expected.Token); err != nil {
			   return err
		   }
		   f, err := k.BumpFence(ctx, cur.ID, NewToken())
		   if err != nil {
			   return err
		   }
		  res, err := k.conn(ctx).ExecContext(ctx,
			  `UPDATE aa_kv SET body=?, fence_epoch=?, fence_token=? WHERE row_id=? AND fence_epoch=? AND fence_token=?`,
			  row.Body, f.Epoch, f.Token, cur.ID, expected.Epoch, expected.Token)
		  if err != nil {
			  return err
		  }
		  n, _ := res.RowsAffected()
		  if n == 0 {
			   return ErrStaleFence
		   }
		   return nil
	  })
  }

  func (k *SQLKV) Delete(ctx context.Context, kind, key string, expected Fence) error {
	  return k.InTx(ctx, func(ctx context.Context) error {
		  cur, err := k.Get(ctx, kind, key)
		  if err != nil {
			  return err
		  }
		   if err := k.CheckFence(ctx, cur.ID, expected.Epoch, expected.Token); err != nil {
			  return err
		   }
		  res,
			   err := k.conn(ctx).ExecContext(ctx,
			  `DELETE FROM aa_kv WHERE row_id=? AND fence_epoch=? AND fence_token=?`,
			   cur.ID,
			   expected.Epoch, expected.Token)
		   if err != nil {
			  return err
		  }
		   n, _ := res.RowsAffected()
		  if n == 0 {
			   return ErrStaleFence
		   }
		   return nil
	  })
  }

   func (k *SQLKV) List(ctx context.Context, kind string, limit int) ([]Row, error) {
	   return k.ListAfter(ctx, kind, "", limit)
   }

  func (k *SQLKV) ListAfter(ctx context.Context, kind, after string, limit int) ([]Row, error) {
	   if limit <= 0 {
		  limit = 50
	   }
	   rows,
		  err := k.conn(ctx).QueryContext(ctx,
		   `SELECT row_id, kind, row_key, body, fence_epoch, fence_token FROM aa_kv WHERE (?='' OR kind=?) AND row_key>? ORDER BY row_key LIMIT ?`,
		  kind, kind, after, limit)
	   if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   out := make([]Row, 0, limit)
	  for rows.Next() {
		   var r Row
		   if err := rows.Scan(&r.ID, &r.Kind, &r.Key, &r.Body, &r.Fence.Epoch, &r.Fence.Token); err != nil {
			   return nil, err
		  }
		   out = append(out, r)
	   }
	  return out, rows.Err()
   }
