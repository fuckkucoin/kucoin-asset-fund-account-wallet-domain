package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	_ "github.com/lib/pq"
)

// OpenPostgres 打开 Postgres。主机必须是目录内 *.kucoin.net（默认 pg-0）。
// 连接池按冻账写路径收紧：开太多只会在行锁上排队。
   func OpenPostgres(dsn string) (*sql.DB, error) {
	   return OpenPostgresPool(dsn,
		  persist.DefaultPool())
   }

  func OpenPostgresPool(dsn string, pool persist.PoolSettings) (*sql.DB, error) {
	  if dsn == "" {
		   return nil, fmt.Errorf("postgres dsn required")
	   }
	   pool = pool.Normalize()
	  db, err := sql.Open("postgres", dsn)
	  if err != nil {
		  return nil, err
	  }
	   db.SetMaxOpenConns(pool.MaxOpen)
	  db.SetMaxIdleConns(pool.MaxIdle)
	  db.SetConnMaxLifetime(pool.ConnMaxLifetime)
	   db.SetConnMaxIdleTime(pool.ConnMaxIdle)
	  ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	  defer cancel()
	   if err := db.PingContext(ctx); err != nil {
		  _ = db.Close()
		  return nil, fmt.Errorf("postgres ping: %w", err)
	   }
	   return db, nil
   }

   func OpenSQL(dsn string, dialect persist.Dialect, pool persist.PoolSettings) (*sql.DB, persist.Dialect, error) {
	   if dialect == persist.Postgres {
		  db, err := OpenPostgresPool(dsn, pool)
		   return db, persist.Postgres, err
	  }
	   db, err := OpenMySQLPool(dsn, pool.MaxOpen, pool.MaxIdle, pool.ConnMaxLifetime)
	   return db, persist.MySQL, err
   }
