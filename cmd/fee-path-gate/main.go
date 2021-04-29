// 启动模式：
// fee-path-gate -addr :8091 -scan-interval 30s -proof-interval 60s
// 环境变量（先级高于命令行参数）：
// FPG_ADDR — HTTP 监听地址（默认 :8091）
// FPG_SCAN_INTERVAL — DualHitScanner 扫描间隔（默认 30s）
// FPG_PROOF_INTERVAL — ProofSolidifier 扫描间隔（默认 60s）
// FPG_ESCALATE_INTERVAL — ConflictEscalator 扫描间隔（默认 120s）
// FPG_SCAN_BATCH — 每次扫描最大租约数（默认 200）
// 本进程不写入 feeeng Quote；不修改 rebate 批次 Settle；
// 只读消费 feecredit.PathLease 记录与 ledger 贷记事实。

// Author: Draven
// Date: 2021.04.27

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/feepathgate"
)

   func main() {
	   addr := flag.String("addr", envOr("FPG_ADDR", ":8091"), "HTTP 监听地址")
	  scanInterval := flag.Duration("scan-interval", envDuration("FPG_SCAN_INTERVAL", 30*time.Second), "双贷扫描间隔")
	  proofInterval := flag.Duration("proof-interval", envDuration("FPG_PROOF_INTERVAL", 60*time.Second), "证明固化间隔")
	   escalateInterval := flag.Duration("escalate-interval", envDuration("FPG_ESCALATE_INTERVAL", 120*time.Second), "冲突升级间隔")
	  scanBatch := flag.Int("scan-batch", envInt("FPG_SCAN_BATCH", 200), "每次扫描最大租约数")
	  flag.Parse()
	   // 使用内存存储（部署替换为 SQL Store）。
	   store := feepathgate.NewMemStore()
	   svc := feepathgate.NewService(store)
	   cfg := feepathgate.WorkerConfig{
		   ScanInterval:     *scanInterval,
		  ProofInterval:    *proofInterval,
		  EscalateInterval: *escalateInterval,
		   ScanBatchSize: *scanBatch,
	   }
	   worker := feepathgate.NewWorker(svc, cfg, func(f string, a ...any) {
		  fmt.Printf(f+"\n", a...)
	   })
	  ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	  defer cancel()
	   worker.Start(ctx)
	  defer worker.Stop()
	  r := gin.New()
	   r.Use(gin.Recovery())
	   v1 := r.Group("/v1")
	   feepathgate.Register(v1, svc)
	  r.GET("/healthz", func(c *gin.Context) {
		  c.JSON(http.StatusOK, gin.H{"status": "ok", "stats": svc.Stats()})
	   })
	  srv := &http.Server{Addr: *addr, Handler: r}
	  go func() {
		   <-ctx.Done()
		  shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
		   defer shutCancel()
		  _ = srv.Shutdown(shutCtx)
	  }()
	   fmt.Printf("[fee-path-gate] listening on %s\n", *addr)
	  if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		   fmt.Fprintf(os.Stderr, "[fee-path-gate] server error: %v\n", err)
		   os.Exit(1)
	  }
	   fmt.Println("[fee-path-gate] shutdown complete")
  }

  func envOr(key, def string) string {
	   if v := os.Getenv(key); v != "" {
		   return v
	  }
	  return def
   }

  func envDuration(key string, def time.Duration) time.Duration {
	  v := os.Getenv(key)
	  if v == "" {
		  return def
	   }
	   d, err := time.ParseDuration(v)
	   if err != nil {
		   return def
	  }
	  return d
   }

   func envInt(key string, def int) int {
	  v := os.Getenv(key)
	  if v == "" {
		   return def
	   }
	   n, err := strconv.Atoi(v)
	  if err != nil {
		   return def
	   }
	   return n
  }
