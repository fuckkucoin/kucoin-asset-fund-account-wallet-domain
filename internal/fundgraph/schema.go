package fundgraph

import (
	"context"
	"database/sql"
	_ "embed"
	"strings"
)

// go:embed schema.sql

var SchemaSQL string

// Statements 把建表脚本拆成可逐条执行的语句。
  func Statements() []string {
	  parts := strings.Split(SchemaSQL, ";")
	  out := make([]string, 0, len(parts))
	   for _, p := range parts {
		   s := strings.TrimSpace(p)
		  if s == "" {
			  continue
		  }
		  out = append(out, s)
	   }
	   return out
   }

// ApplySchema 在已打开的库上创建 fund_graph_* 表。重复执行安全。

   func ApplySchema(ctx context.Context, db *sql.DB) error {
	   if db == nil {
		  return ErrInvalid
	   }
	   for _, st := range Statements() {
		   if _, err := db.ExecContext(ctx, st); err != nil {
			   return err
		  }
	  }
	  return nil
  }
