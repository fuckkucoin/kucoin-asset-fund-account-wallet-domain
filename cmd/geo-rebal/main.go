// 功能：cmd/geo-rebal — 跨区再平衡与库存隔离服务入口。
// 环境变量：
// GEO_REBAL_ADDR HTTP 监听地址（默认 :8097）
// GEO_REBAL_EXEC true 则启动执行 worker（默认 true）

// 作者：Cason
// 日期：2021.06.30

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"account-assets.kucoin.net/funding/internal/georebal"
	"account-assets.kucoin.net/funding/internal/regioinv"
)

  func main() {
	  addr := getenv("GEO_REBAL_ADDR", ":8097")
	  regioSt := regioinv.NewMemStore()
	  regioSvc := regioinv.New(regioSt)
	  rebalSt := georebal.NewMemStore()
	   svc := georebal.NewService(rebalSt, regioSvc, nil, 10*time.Second)
	   ctx,
		  stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	   defer stop()
	   // 后台 worker
	   go regioSvc.RunWorkers(ctx, 30*time.Second, 60*time.Second, 15*time.Second)
	  if getenv("GEO_REBAL_EXEC", "true") == "true" {
		   go svc.Worker.Run(ctx)
	   }
	  gin.SetMode(gin.ReleaseMode)
	   r := gin.New()
	  r.Use(gin.Recovery())
	  r.GET("/healthz", func(c *gin.Context) {
		  c.JSON(http.StatusOK, gin.H{"service": "geo-rebal", "ok": true})
	   })
	  r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	   v1 := r.Group("/v1")
	  regioinv.Mount(v1, regioSvc)
	  georebal.NewHandler(svc.Planner, svc.Exec).Mount(v1)
	  srv := &http.Server{Addr: addr, Handler: r, ReadHeaderTimeout: 5 * time.Second}
	  go func() {
		   <-ctx.Done()
		   sh, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		  defer cancel()
		   _ = srv.Shutdown(sh)
	   }()
	  log.Printf("geo-rebal listen %s", addr)
	  if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		  log.Fatal(err)
	   }
  }

   func getenv(k, def string) string {
	  if v := os.Getenv(k); v != "" {
		   return v
	   }
	  return def
  }
