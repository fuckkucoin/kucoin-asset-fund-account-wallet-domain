// wd-partial — 出金批次部分失败残留与续提互斥收口服务入口。
// 1. 启动 ResidueScanner worker：周期性扫描 submitted 批次，发现残留项。
// 2. 启动 ReleaseExecutor worker：自动记录确认无占用项的 no_occupy 证明。
// 3. 启动 LeaseReaper worker：清理过期 ResumeLease。
// 4. 提供 HTTP ops API（/wdpartial/...），供运营手动触发检测与收口。
// 启动参数（环境变量）：
// WD_PARTIAL_ADDR HTTP 监听地址（默认 :8093）
// WD_PARTIAL_SCAN_LIMIT 每轮扫描批次数上限（默认 50）
// CreateDate: 2024.09.01

// Author: Finn
// Date: 2021.12.20

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"account-assets.kucoin.net/funding/internal/wdpartial"
	"github.com/gin-gonic/gin"
)

  func main() {
	   addr := getEnv("WD_PARTIAL_ADDR", ":8093")
	  svc := wdpartial.NewService()
	   // 构建路由
	   router := gin.New()
	   router.Use(gin.Recovery())
	  v1 := router.Group("/v1")
	   wdpartial.Register(v1, nil)
	   // 后台 worker
	   ctx, cancel := context.WithCancel(context.Background())
	  defer cancel()
	  lister := &wdpartial.MockBatchLister{}
	   go func() {
		  if err := runScanner(ctx, lister, svc); err != nil {
			  log.Printf("residue scanner exited: %v", err)
		   }
	  }()
	   srv := &http.Server{
		  Addr:        addr,
		  Handler:     router,
		   ReadTimeout: 15 * time.Second,
		   WriteTimeout: 30 * time.Second,
		  IdleTimeout: 60 * time.Second,
	  }
	  go func() {
		  log.Printf("wd-partial listening on %s", addr)
		   if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			   log.Fatalf("wd-partial listen: %v", err)
		   }
	   }()
	   quit := make(chan os.Signal, 1)
	  signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	  <-quit
	  log.Println("wd-partial shutting down")
	  shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	   defer shutCancel()
	   if err := srv.Shutdown(shutCtx); err != nil {
		  log.Printf("wd-partial shutdown: %v", err)
	   }
  }

   func runScanner(ctx context.Context, lister wdpartial.BatchLister, svc *wdpartial.Service) error {
	   ticker := time.NewTicker(30 * time.Second)
	   defer ticker.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			  return ctx.Err()
		  case <-ticker.C:
			   ids, err := lister.SubmittedBatchIDs(50)
			  if err != nil {
				   log.Printf("wd-partial scanner: list batches: %v", err)
				  continue
			   }
			   for _, id := range ids {
				  if _, err := svc.DetectResidues(id); err != nil {
					  log.Printf("wd-partial scanner: detect %s: %v", id, err)
				  }
			  }
		   }
	  }
  }

   func getEnv(key, def string) string {
	   if v := os.Getenv(key); v != "" {
		  return v
	  }
	   return def
  }
