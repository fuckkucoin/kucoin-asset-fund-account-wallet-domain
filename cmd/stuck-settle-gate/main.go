// @FileName: main.go
// @Summary: 滞留回收世代闸门独立服务入口。
// - 启动 stucksettle worker（双结巡检、租约超时清扫、Advance 锁清扫）。
// - 暴露 /v1/stucksettle/ HTTP API（效果世代查询、双结确认、证明导出）。
// - 不写出金链路；不操作 withdraw 主路径；仅通过 Gate 接口完成回收闸逻辑。
// 不变量：
// - SIGTERM/SIGINT 触发 graceful shutdown；worker goroutine 通过 context cancel 退出。
// - 未配置 DB 时使用内存 Store（单节点模式，不具备持久化能力）。
// @Author: Draven
// @Created: 2021.11.22

package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"account-assets.kucoin.net/funding/internal/stucksettle"
	"github.com/gin-gonic/gin"
)

   func main() {
	  ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	  defer cancel()
	   nodeID := os.Getenv("NODE_ID")
	  if nodeID == "" {
		   nodeID = "stuck-settle-gate-default"
	  }
	  // 内存 Store（环境替换为 SQLStore）
	  store := stucksettle.NewMemStore()
	   gate := stucksettle.NewGate(stucksettle.GateConfig{
		  Store:  store,
		  NodeID: nodeID,
	   })
	  workerCfg := stucksettle.WorkerConfig{
		  DupScanInterval: 60 * time.Second,
		   ExpiryInterval:     30 * time.Second,
		  MutexSweepInterval: 30 * time.Second,
		  // TransferListFn / LeaseListFn 未注入时 worker 空跑（需绑定 SQL 扫描）
	  }
	  worker := stucksettle.NewWorker(gate, workerCfg)
	  worker.RunAll(ctx)
	  r := gin.New()
	  r.Use(gin.Recovery())
	  v1 := r.Group("/v1")
	   stucksettle.Register(v1, nil)
	   addr := os.Getenv("ADDR")
	   if addr == "" {
		  addr = ":8099"
	  }
	   srv := &http.Server{Addr: addr, Handler: r}
	   go func() {
		   <-ctx.Done()
		   shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
		   defer shutCancel()
		   _ = srv.Shutdown(shutCtx)
	  }()
	   _ = srv.ListenAndServe()
  }
