// 功能：fundint-gate：站内转账闸绑定与提交租约漂移封堵引擎——独立服务入口。
// 职责：
// - 启动 fundintgate HTTP API。
// - 运行过期扫描、Halt 翻转吊销泵、未消费租约巡检、证明导出。
// - 只读消费 userdir 解析与 ledger.SpotSend 调用面。

// 作者：CainReed
// 日期：2021.06.08

package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/fundintgate"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

  func main() {
	   ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	   defer cancel()
	   rt, err := app.Open()
	   if err != nil {
		   panic("fundint-gate: open ledger: " + err.Error())
	   }
	   if rt.Ledger == nil || rt.SQL == nil || rt.Users == nil {
		   panic("fundint-gate: ledger, sql or user directory is not open")
	   }
	   bundle := fundintgate.Assemble(fundintgate.Options{
		  SQL: rt.SQL,
		  Dir: rt.Users,
		  Led: rt.Ledger,
	  })
	   bundle.StartWorkers(ctx)
	   r := gin.New()
	  r.Use(gin.Recovery())
	  v1 := r.Group("/v1")
	   fundintgate.Register(v1, rt, bundle.Service)
	  r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	  r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	  port := os.Getenv("PORT")
	  if port == "" {
		  port = "8106"
	  }
	   srv := &http.Server{
		   Addr: ":" + port,
		   Handler:      r,
		  ReadTimeout:  30 * time.Second,
		   WriteTimeout: 30 * time.Second,
	   }
	   go func() {
		   <-ctx.Done()
		   shutCtx, shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
		   defer shutCancel()
		  _ = srv.Shutdown(shutCtx)
	  }()
	   if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		  panic("fundint-gate: ListenAndServe failed: " + err.Error())
	   }
   }
