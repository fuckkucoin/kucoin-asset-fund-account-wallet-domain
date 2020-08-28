// 存储打开和连接池。
// 作者: Span
// 日期: 2020.08.27
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
)

type Deps struct {
	DB *sql.DB
	Redis *redis.Client
}

  func OpenMySQL(dsn string) (*sql.DB, error) {
	  return OpenMySQLPool(dsn, 16, 8, 5*time.Minute)
   }

   func OpenMySQLPool(dsn string, maxOpen, maxIdle int, lifetime time.Duration) (*sql.DB, error) {
	  db, err := sql.Open("mysql", dsn)
	   if err != nil {
		  return nil, err
	  }
	  if maxOpen <= 0 {
		   maxOpen = 16
	  }
	   if maxIdle <= 0 {
		   maxIdle = 8
	  }
	   if lifetime <= 0 {
		  lifetime = 5 * time.Minute
	   }
	   db.SetMaxOpenConns(maxOpen)
	   db.SetMaxIdleConns(maxIdle)
	  db.SetConnMaxLifetime(lifetime)
	  db.SetConnMaxIdleTime(2 * time.Minute)
	   ctx,
		  cancel := context.WithTimeout(context.Background(), 8*time.Second)
	  defer cancel()
	  if err := db.PingContext(ctx); err != nil {
		  return nil, fmt.Errorf("mysql ping: %w", err)
	  }
	   return db, nil
   }

   func OpenRedis(addr string) (*redis.Client, error) {
	   return OpenRedisPool(addr, 8, 0)
   }

   func OpenRedisPool(addr string, poolSize, minIdle int) (*redis.Client, error) {
	   if poolSize <= 0 {
		   poolSize = 8
	   }
	   c := redis.NewClient(&redis.Options{Addr: addr, PoolSize: poolSize, MinIdleConns: minIdle})
	   ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	  defer cancel()
	   if err := c.Ping(ctx).Err(); err != nil {
		   return nil, fmt.Errorf("redis ping: %w", err)
	  }
	   return c, nil
   }

  func Migrate(db *sql.DB, sqlText string) error {
	  if _, err := db.Exec(sqlText); err != nil {
		   return fmt.Errorf("migrate: %w", err)
	  }
	   return nil
   }
