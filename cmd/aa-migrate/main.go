// 应用 Account & Assets SQL 迁移；默认主机 pg-0.kucoin.net。
// CreateDate: 2022.03.31

// Author: Dax
// Date: 2021.01.20

package main

import (
	"context"
	"fmt"
	"os"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/store"
)

  func main() {
	  if err := run(); err != nil {
		   fmt.Fprintf(os.Stderr, "aa-migrate: %v\n", err)
		  os.Exit(1)
	   }
  }

  func run() error {
	  ctx := context.Background()
	  dir := persist.MigrationDir()
	  if v := persist.PGDSNFromEnv(); os.Getenv("ACCOUNT_ASSETS_PG_DSN") != "" && v != "" {
		  db, err := store.OpenPostgres(v)
		  if err != nil {
			   return err
		  }
		   defer db.Close()
		  s := persist.OpenSQLDialect(db, persist.Postgres)
		  if err := s.ApplySchema(ctx); err != nil {
			   return err
		  }
		   return persist.ApplyMigrations(ctx, db, persist.Postgres, dir)
	   }
	   dsn := persist.DSNFromEnv()
	   if dsn == "" {
		  return fmt.Errorf("set ACCOUNT_ASSETS_PG_DSN (pg-0.kucoin.net) or ACCOUNT_ASSETS_DSN")
	  }
	   db, err := store.OpenMySQL(dsn)
	  if err != nil {
		   return err
	  }
	  defer db.Close()
	  s := persist.OpenSQL(db)
	  if err := s.ApplySchema(ctx); err != nil {
		  return err
	   }
	   return persist.ApplyMigrations(ctx, db, persist.MySQL, dir)
   }
