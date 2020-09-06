package persist

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ApplyMigrations 按文件名顺序执行 migrations/*.sql。
// 一律按分号拆条；Postgres 先 Rewrite 占位与类型。
  func ApplyMigrations(ctx context.Context, db *sql.DB, d Dialect, dir string) error {
	   if db == nil {
		   return ErrTxClosed
	   }
	  if strings.TrimSpace(dir) == "" {
		   dir = "migrations"
	  }
	  ents, err := os.ReadDir(dir)
	   if err != nil {
		   return err
	   }
	  names := make([]string, 0, len(ents))
	   for _, e := range ents {
		   if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".sql") {
			  continue
		  }
		  names = append(names, e.Name())
	  }
	   sort.Strings(names)
	  if len(names) == 0 {
		  return fmt.Errorf("no sql migrations in %s", dir)
	   }
	   for _, name := range names {
		  raw, err := os.ReadFile(filepath.Join(dir, name))
		   if err != nil {
			   return err
		   }
		   body := string(raw)
		   if d == Postgres {
			   body = Rewrite(body, Postgres)
		  }
		  if err := ApplyStatements(ctx, db, body); err != nil {
			   return fmt.Errorf("%s: %w", name, err)
		   }
	  }
	   return nil
   }

  func MigrationDir() string {
	  if v := os.Getenv("ACCOUNT_ASSETS_MIGRATIONS"); v != "" {
		   return v
	  }
	  return "migrations"
   }
