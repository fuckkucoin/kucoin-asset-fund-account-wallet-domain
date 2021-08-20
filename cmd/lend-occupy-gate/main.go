// - 启动 lendoccupygate Worker（半截扫描、重复释放巡检、metrics 采集）。
// - 暴露 /v1/lendoccupygate/ HTTP API（PlaceBound、Settle、Cancel、证明导出、租约查询）。
// - 不写出金链路；不操作 loanbook/marginhook/P2P 保证金主路径；仅通过 lending.LendingAdapter 接口。
// - 未配置 DB 时使用内存 Store（单节点模式，不具备持久化能力）。
// 不变量：
// - SIGTERM/SIGINT 触发 graceful shutdown；Worker goroutine 通过 context cancel 退出。
// - 环境须通过环境变量注入 DB_DSN；未注入时服务以内存模式启动并打印警告。

// Author: Bronson
// Date: 2021.08.19

package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"account-assets.kucoin.net/funding/internal/lending"
	"account-assets.kucoin.net/funding/internal/lendoccupygate"
	"account-assets.kucoin.net/funding/internal/holdcat"
)

  func main() {
	  ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	   defer cancel()
	   nodeID := os.Getenv("NODE_ID")
	   if nodeID == "" {
		  nodeID = "lend-occupy-gate-default"
	   }
	  addr := os.Getenv("ADDR")
	  if addr == "" {
		  addr = ":8110"
	   }
	   var db *sql.DB
	   if dsn := os.Getenv("DB_DSN"); dsn != "" {
		  var err error
		  db, err = sql.Open("mysql", dsn)
		   if err != nil {
			  panic("lend-occupy-gate: failed to open DB: " + err.Error())
		   }
		   db.SetMaxOpenConns(20)
		   db.SetMaxIdleConns(5)
		   db.SetConnMaxLifetime(5 * time.Minute)
	  } else {
		   // 内存模式（开发/）
		  _, _ = os.Stderr.WriteString("lend-occupy-gate: DB_DSN not set, running in memory-store mode\n")
	   }
	  // lending.Service 在无 DB 时无法完整初始化；此处构造最小存根以供。
	   // 环境须注入完整 lending.Service（含 ledger.Service 与 holdcat.Book）。
	   lendSvc := buildLendingService(db)
	  lendAdapter := lending.NewLendingAdapter(lendSvc)
	   r := gin.New()
	   r.Use(gin.Recovery())
	   v1 := r.Group("/v1")
	   lendoccupygate.Register(ctx, v1, lendoccupygate.RegisterConfig{
		   DB: db,
		  Lending: lendAdapter,
		  WorkerCfg: lendoccupygate.WorkerConfig{
			   NodeID:            nodeID,
			   HalfPlaceInterval: 60 * time.Second,
			   DupReleaseInterval: 90 * time.Second,
			  MetricsInterval: 30 * time.Second,
			  StaleTTL:        5 * time.Minute,
		  },
		  ScannerCfg: lendoccupygate.ScannerConfig{
			  ScannerID: nodeID + ".scanner",
			  StaleTTL:  5 * time.Minute,
		  },
	   })
	   srv := &http.Server{
		   Addr:    addr,
		   Handler: r,
		  ReadTimeout: 10 * time.Second,
		   WriteTimeout: 30 * time.Second,
	   }
	   go func() {
		  <-ctx.Done()
		   shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
		  defer shutCancel()
		   _ = srv.Shutdown(shutCtx)
	   }()
	  if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		   panic("lend-occupy-gate: server error: " + err.Error())
	   }
   }

// buildLendingService 构造 lending.Service（简化版，须完整注入）。

  func buildLendingService(_ *sql.DB) *lending.Service {
	  // 此处返回 nil-safe Service：内存 KV（无持久化）。
	  // 环境须注入 ledger.Service 与 holdcat.Book（含 SQL backend）。
	   return lending.New(nil, holdcat.New(nil))
   }
