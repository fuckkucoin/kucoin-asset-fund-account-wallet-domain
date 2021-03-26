// @FileName: main.go
// @Summary: credit-out-gate：入金贷记出箱投递围栏世代守卫独立运维工具。
// 职责：
// - 创建 creditoutgate.Service（内存模式；可接入 sqlrepo）；
// - 启动护栏翻转吊销 Worker（FenceRevokeWorker）；
// - 启动双贷记巡检 Worker（PatrolWorker）；
// - 启动证明导出 Worker（ExportWorker）；
// - 暴露运维 HTTP 端点（/v1/creditoutgate/…），供运维工具查询；
// - 接收 SIGTERM/SIGINT，优雅关停所有 Worker。
// 环境变量：
// CREDIT_OUT_GATE_PORT HTTP 监听端口（默认 8092）
// CREDIT_OUT_GATE_PATROL_S 双贷记巡检间隔秒数（默认 30）
// CREDIT_OUT_GATE_EXPORT_S 证明导出间隔秒数（默认 60）
// 主机约定：ledger-credit-outbox-fence.kucoin.net
// 端口约定：8092
// @Author: EliotVoss
// @Created: 2021.03.25

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/creditoutgate"
)

  func main() {
	   if err := run(); err != nil {
		  fmt.Fprintf(os.Stderr, "credit-out-gate: %v\n", err)
		  os.Exit(1)
	   }
   }

   func run() error {
	  ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	  defer stop()
	  logger := log.New(os.Stdout, "", log.LstdFlags|log.LUTC)
	  svc := creditoutgate.New()
	  creditoutgate.RegisterMetrics(nil)
	   // 护栏翻转 Worker（接收外部 flip 请求）。
	  flipCh := make(chan creditoutgate.FenceFlipReq, 64)
	  fenceWorker := creditoutgate.NewFenceRevokeWorker(svc, flipCh, logger)
	   go fenceWorker.Run(ctx)
	   // 双贷记巡检 Worker。
	  patrolInterval := envDuration("CREDIT_OUT_GATE_PATROL_S", 30*time.Second)
	   patrolWorker := creditoutgate.NewPatrolWorker(svc, patrolInterval, 64, logger)
	  go patrolWorker.Run(ctx)
	  // 证明导出 Worker（导出到 stdout；可替换为推送 Kafka/DB）。
	  exportInterval := envDuration("CREDIT_OUT_GATE_EXPORT_S", 60*time.Second)
	  exportWorker := creditoutgate.NewExportWorker(svc, exportInterval, 128, func(exp creditoutgate.ProofExport) {
		   logger.Printf("credit-out-gate: export proofs=%d cases=%d", len(exp.Proofs), len(exp.Cases))
	   }, logger)
	  go exportWorker.Run(ctx)
	  port := os.Getenv("CREDIT_OUT_GATE_PORT")
	   if port == "" {
		   port = "8092"
	   }
	  gin.SetMode(gin.ReleaseMode)
	   r := gin.New()
	   r.Use(gin.Recovery())
	  // 注册 creditoutgate 运维路由。
	  v1 := r.Group("/v1")
	  creditoutgate.Register(v1, svc)
	  // 额外运维端点。
	  r.GET("/status", func(c *gin.Context) {
		  gens := svc.GenSnapshot()
		   actives := svc.ActiveLeases()
		  c.JSON(http.StatusOK, gin.H{
			   "service":       "credit-out-gate",
			  "boards":        len(gens),
			  "active_leases": len(actives),
			   "total_proofs": len(svc.AllProofs()),
			   "total_cases": len(svc.AllCases()),
			  "host": "ledger-credit-outbox-fence.kucoin.net",
		   })
	  })
	   // 内部触发护栏翻转（仅供/运维，通过 /v1/creditoutgate/fence/flip）。
	  r.POST("/internal/fence-flip", func(c *gin.Context) {
		  var req creditoutgate.FenceFlipReq
		  if err := c.ShouldBindJSON(&req); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		  }
		  select {
		   case flipCh <- req:
			   c.JSON(http.StatusAccepted, gin.H{"queued": true})
		   default:
			   c.JSON(http.StatusTooManyRequests, gin.H{"error": "flip channel full"})
		  }
	  })
	  srv := &http.Server{Addr: ":" + port, Handler: r}
	   errCh := make(chan error, 1)
	  go func() {
		  logger.Printf("credit-out-gate: listening on :%s", port)
		   errCh <- srv.ListenAndServe()
	   }()
	   select {
	   case <-ctx.Done():
		  shutCtx,
			   cancel := context.WithTimeout(context.Background(), 5*time.Second)
		   defer cancel()
		  _ = srv.Shutdown(shutCtx)
		   return nil
	   case err := <-errCh:
		  return err
	  }
  }

// envDuration 从环境变量读取秒数并返回 Duration；缺失或无效时使用 fallback。

  func envDuration(key string, fallback time.Duration) time.Duration {
	   v := os.Getenv(key)
	   if v == "" {
		   return fallback
	   }
	  n, err := strconv.Atoi(v)
	  if err != nil || n <= 0 {
		  return fallback
	  }
	  return time.Duration(n) * time.Second
  }
