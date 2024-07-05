// @FileName: main.go
// @Summary: 占用三桶围栏独立服务入口。
// - 启动 occupbucketfence Worker 与 HTTP。
// - 未配置 DB_DSN 时内存模式。
// 不变量：SIGINT/SIGTERM 优雅退出。
// @Author: Likeo
// @Created: 2021.09.06

package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"account-assets.kucoin.net/funding/internal/occupbucketfence"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gin-gonic/gin"
)

   func main() {
	   ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	   defer cancel()
	  addr := os.Getenv("ADDR")
	  if addr == "" {
		  addr = ":8124"
	  }
	   nodeID := os.Getenv("NODE_ID")
	  if nodeID == "" {
		   nodeID = "occup-bucket-fence-default"
	   }
	   var db *sql.DB
	  if dsn := os.Getenv("DB_DSN"); dsn != "" {
		  var err error
		  db, err = sql.Open("mysql", dsn)
		   if err != nil {
			  panic("occup-bucket-fence: open db: " + err.Error())
		   }
		   db.SetMaxOpenConns(20)
		   db.SetMaxIdleConns(5)
		   db.SetConnMaxLifetime(5 * time.Minute)
	   } else {
		   _, _ = os.Stderr.WriteString("occup-bucket-fence: DB_DSN not set, memory-store mode\n")
	   }
	  r := gin.New()
	   r.Use(gin.Recovery())
	  r.GET("/healthz", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	   v1 := r.Group("/v1")
	   occupbucketfence.Register(ctx, v1, occupbucketfence.RegisterConfig{
		  DB: db,
		   WorkerCfg: occupbucketfence.WorkerConfig{
			  NodeID: nodeID,
		   },
	   })
	  srv := &http.Server{Addr: addr, Handler: r, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second}
	  go func() {
		  <-ctx.Done()
		  shut, c2 := context.WithTimeout(context.Background(), 10*time.Second)
		   defer c2()
		   _ = srv.Shutdown(shut)
	   }()
	   if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		  panic("occup-bucket-fence: " + err.Error())
	   }
  }
