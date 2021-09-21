// 功能：main 模块 main。
// once-halt-gate：出箱一次性停机投递闸独立运维工具。
// 职责：
// - 读取 ACCOUNT_ASSETS_PG_DSN 环境变量，连接持久化层；
// - 启动 oncehaltgate 后台工人（在途 Deliver 吊销扫描、双 Mark 巡检）；
// - 暴露运维 HTTP 端点（/status、/halt-gen、/proof），供运维工具查询；
// - 接收 SIGTERM/SIGINT，优雅关停工人。
// 主机约定：ledger-transfer-freeze-reconciliation.kucoin.net
// 端口约定：8091（可由 ONCE_HALT_GATE_PORT 覆盖）

// 作者：ZaneCruz
// 日期：2021.09.08

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/oncehaltgate"
	"account-assets.kucoin.net/funding/internal/outboxonce"
)

  func main() {
	   if err := run(); err != nil {
		   fmt.Fprintf(os.Stderr, "once-halt-gate: %v\n", err)
		   os.Exit(1)
	   }
   }

   func run() error {
	   ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	  defer stop()
	   svc := outboxonce.New()
	   gate := oncehaltgate.NewGate(svc)
	  workers := oncehaltgate.NewWorkers(gate, 500*time.Millisecond, 2*time.Second)
	  workers.Start(ctx)
	  defer workers.Stop()
	  port := os.Getenv("ONCE_HALT_GATE_PORT")
	  if port == "" {
		  port = "8091"
	   }
	  gin.SetMode(gin.ReleaseMode)
	   r := gin.New()
	   r.Use(gin.Recovery())
	   v1 := r.Group("/v1")
	   // SetHalt 切换
	  v1.POST("/oncehaltgate/switch", func(c *gin.Context) {
		   var req oncehaltgate.SwitchRequest
		   if err := c.ShouldBindJSON(&req); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		   }
		  result, err := gate.Switch(c.Request.Context(), req)
		   if err != nil {
			   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(http.StatusOK, result)
	  })
	  // Deliver
	  v1.POST("/oncehaltgate/deliver", func(c *gin.Context) {
		   var req oncehaltgate.DeliverRequest
		   if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		  }
		  resp, err := gate.Deliver(c.Request.Context(), req)
		  if err != nil {
			   c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(http.StatusOK, resp)
	   })
	  // 状态查询
	   v1.GET("/oncehaltgate/status", func(c *gin.Context) {
		  c.JSON(http.StatusOK, gate.Status())
	   })
	   v1.GET("/oncehaltgate/halt-gen", func(c *gin.Context) {
		   c.JSON(http.StatusOK, gate.HaltGenView())
	   })
	   // 证明导出
	   v1.GET("/oncehaltgate/proof", func(c *gin.Context) {
		   var q oncehaltgate.ProofQuery
		   _ = c.ShouldBindQuery(&q)
		  c.JSON(http.StatusOK, gate.ExportProof(q))
	   })
	  // 工人状态
	   v1.GET("/oncehaltgate/workers", func(c *gin.Context) {
		   c.JSON(http.StatusOK, workers.Status())
	  })
	  // 健康探针
	   r.GET("/healthz", func(c *gin.Context) {
		   c.JSON(http.StatusOK, gin.H{"ok": true, "host": "ledger-transfer-freeze-reconciliation.kucoin.net"})
	   })
	  srv := &http.Server{
		  Addr:        ":" + port,
		  Handler:     r,
		  ReadTimeout: 10 * time.Second,
		  WriteTimeout: 10 * time.Second,
	   }
	  go func() {
		  <-ctx.Done()
		  shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		  defer cancel()
		   _ = srv.Shutdown(shutCtx)
	   }()
	   fmt.Printf("once-halt-gate listening on :%s (host: ledger-transfer-freeze-reconciliation.kucoin.net)\n", port)
	  if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		   return err
	   }
	   return nil
   }
