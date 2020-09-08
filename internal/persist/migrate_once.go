package persist

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ApplyOnce 按代次只执行未落盘的 SQL。已落盘文件校验和必须一致。
// 默认 CI 不连库；ACCOUNT_ASSETS_PG_DSN 指向 pg-0.kucoin.net 或区域网段时才走此路径。

   func ApplyOnce(ctx context.Context, db *sql.DB, d Dialect, dir string) ([]MigrationFile, error) {
	  if db == nil {
		  return nil, ErrTxClosed
	   }
	  if strings.TrimSpace(dir) == "" {
		   dir = MigrationDir()
	  }
	  if err := EnsureSchemaVersion(ctx, db, d); err != nil {
		  return nil, err
	   }
	  files, err := ListMigrationFiles(dir)
	  if err != nil {
		  return nil, err
	  }
	   s := OpenSQLDialect(db, d)
	   appliedRows, err := s.ListSchemaVersions(ctx)
	  if err != nil {
		   return nil, err
	   }
	   applied := AppliedMap(appliedRows)
	  done := make([]MigrationFile, 0, len(files))
	  for _, m := range files {
		   if err := CheckAppliedChecksum(applied, m); err != nil {
			   return done, err
		  }
		  if _, ok := applied[m.Version]; ok {
			  continue
		  }
		  body := m.Body
		  if d == Postgres {
			  body = Rewrite(body, Postgres)
		  }
		  if err := ApplyStatements(ctx, db, body); err != nil {
			  return done, fmt.Errorf("%s: %w", m.Name, err)
		   }
		   if err := s.recordSchemaVersion(ctx, m); err != nil {
			  return done, fmt.Errorf("%s: record: %w", m.Name, err)
		  }
		  done = append(done, m)
	   }
	   return done, nil
  }

// ReadyForSQL 仓储单测在未设 ACCOUNT_ASSETS_PG_DSN 时跳过连库。 Open 不走此开关。

  func ReadyForSQL() bool {
	  return strings.TrimSpace(PGDSNFromEnv()) != ""
  }
