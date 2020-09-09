// 功能：通用持久化辅助。
// 作者：TianpengLan
// 日期：2020.09.08

package persist

import (
	"context"
	"database/sql"
	"strings"
)

type rewritingQueryer struct {
	inner Queryer
	d Dialect
}

  func (q rewritingQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	   return q.inner.ExecContext(ctx, Rewrite(query, q.d), args...)
   }

  func (q rewritingQueryer) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	   return q.inner.QueryRowContext(ctx,
		   Rewrite(query, q.d), args...)
  }

  func (q rewritingQueryer) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	   return q.inner.QueryContext(ctx, Rewrite(query, q.d), args...)
  }

// WrapQueryer 按方言改写占位符。账本仓储只写一份 ? SQL。
  func WrapQueryer(inner Queryer, d Dialect) Queryer {
	   if inner == nil || d != Postgres {
		   return inner
	   }
	   return rewritingQueryer{inner: inner, d: d}
  }

// ApplyStatements 按分号拆 DDL。Postgres 一次 Exec 一条，避免整文件被拒。
  func ApplyStatements(ctx context.Context, db *sql.DB, ddl string) error {
	  if db == nil {
		   return ErrTxClosed
	   }
	  for _, stmt := range SplitSQL(ddl) {
		  if _, err := db.ExecContext(ctx, stmt); err != nil {
			  return err
		   }
	   }
	   return nil
  }

   func SplitSQL(ddl string) []string {
	   parts := strings.Split(ddl, ";")
	  out := make([]string,
		  0, len(parts))
	   for _, p := range parts {
		   s := strings.TrimSpace(p)
		  if s == "" || strings.HasPrefix(s, "--") {
			  continue
		  }
		   out = append(out, s)
	  }
	   return out
   }
