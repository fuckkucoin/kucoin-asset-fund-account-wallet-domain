// @FileName: main.go
// @Summary: 站内转账闸服务（int-xfer-gate）。
// 职责：
// - 启动 HTTP 服务，暴露解析快照查询、误路由确认、效果证明导出与发起含闸转账等接口。
// - 启动三个后台 worker：patrol（双扣巡检）、scanner（串户扫描）、sealer（证明固化）。
// - 只读挂钩 userdir.Directory 与 ledger.SpotSend 观察；不改写 ledger 内核。
// 启动参数：
// -addr HTTP 监听地址（默认 :8180）
// -patrol-ms patrol worker 间隔毫秒（默认 5000）
// -scanner-ms scanner worker 间隔毫秒（默认 30000）
// -sealer-ms sealer worker 间隔毫秒（默认 15000）
// -snap-ttl-s 快照有效期秒（默认 300）
// -lease-ttl-s 租约有效期秒（默认 30）
// 不包含：链上出金广播、P2P 通道、跨 book 半截镜像（r6 D）。
// @Author: Talon
// @Created: 2021.07.27

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/fundint"
	"account-assets.kucoin.net/funding/internal/intxfergate"
)

  func main() {
	  addr := flag.String("addr", ":8180", "HTTP listen address")
	  patrolMS := flag.Int("patrol-ms", 5000, "patrol worker interval ms")
	   scannerMS := flag.Int("scanner-ms", 30000, "scanner worker interval ms")
	   sealerMS := flag.Int("sealer-ms", 15000, "sealer worker interval ms")
	   snapTTLS := flag.Int("snap-ttl-s", 300, "resolve snap TTL seconds")
	  leaseTTLS := flag.Int("lease-ttl-s", 30, "debit lease TTL seconds")
	  flag.Parse()
	  if err := run(
		   *addr,
		   time.Duration(*patrolMS)*time.Millisecond,
		   time.Duration(*scannerMS)*time.Millisecond,
		   time.Duration(*sealerMS)*time.Millisecond,
		   time.Duration(*snapTTLS)*time.Second,
		  time.Duration(*leaseTTLS)*time.Second,
	  ); err != nil {
		  fmt.Fprintf(os.Stderr, "int-xfer-gate: %v\n", err)
		  os.Exit(1)
	   }
  }

   func run(addr string, patrolInterval, scannerInterval, sealerInterval, snapTTL, leaseTTL time.Duration) error {
	  ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	   defer cancel()
	  // 闸自身的快照与证明仍记在本进程。划转与收款方解析走已打开的账本和用户目录。
	  store := intxfergate.NewMemStore()
	   rt, err := app.Open()
	   if err != nil {
		  return fmt.Errorf("open ledger: %w", err)
	   }
	   if rt.Ledger == nil || rt.Users == nil {
		  return fmt.Errorf("ledger or user directory is not open")
	   }
	  dir := rt.Users
	  // 装配 fundint.Service
	   svc := fundint.New(rt.Ledger, dir)
	  // 装配 fundint.ServiceExt（含 ResolveGate）
	   ext := fundint.NewExt(svc,
		   store,
		   fundint.WithSnapTTL(snapTTL),
		  fundint.WithLeaseTTL(leaseTTL),
	   )
	   // 装配 Engine
	  engine := intxfergate.NewEngine(ext, store)
	   // 装配 ResolveGate（供 WorkerSet 使用）
	  gate := fundint.NewResolveGate(store,
		  fundint.WithSnapTTL(snapTTL),
		   fundint.WithLeaseTTL(leaseTTL),
	  )
	  // 装配 WorkerSet
	   ws := intxfergate.NewWorkerSet(gate, engine, store, dir, 200, 100, 100, 2*time.Minute)
	   // 启动 HTTP 服务
	   mux := http.NewServeMux()
	  h := intxfergate.NewHandler(engine)
	  h.Register(mux)
	   // 健康检查
	  mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		   w.WriteHeader(http.StatusOK)
		   _, _ = w.Write([]byte("ok"))
	   })
	  srv := &http.Server{
		   Addr:    addr,
		  Handler: mux,
		   ReadTimeout:  10 * time.Second,
		  WriteTimeout: 30 * time.Second,
		  IdleTimeout:  60 * time.Second,
	  }
	   // 启动 worker goroutine
	  startWorker(ctx, "patrol", patrolInterval, func() {
		  if _, _, err := ws.Patrol.RunOnce(ctx); err != nil {
			   fmt.Fprintf(os.Stderr, "patrol: %v\n", err)
		  }
	   })
	  startWorker(ctx, "scanner", scannerInterval, func() {
		  if _, _, err := ws.Scanner.RunOnce(ctx); err != nil {
			   fmt.Fprintf(os.Stderr, "scanner: %v\n", err)
		  }
	  })
	  startWorker(ctx, "sealer", sealerInterval, func() {
		   if _, _, err := ws.Sealer.RunOnce(ctx); err != nil {
			  fmt.Fprintf(os.Stderr, "sealer: %v\n", err)
		  }
	  })
	   // 启动 HTTP server
	   go func() {
		  <-ctx.Done()
		   shutCtx, shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
		  defer shutCancel()
		   _ = srv.Shutdown(shutCtx)
	  }()
	   fmt.Fprintf(os.Stdout, "int-xfer-gate: listening on %s\n", addr)
	   if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		   return fmt.Errorf("http server: %w", err)
	  }
	  return nil
  }

// startWorker 启动定时 worker goroutine。

  func startWorker(ctx context.Context, name string, interval time.Duration, fn func()) {
	   go func() {
		  ticker := time.NewTicker(interval)
		   defer ticker.Stop()
		   for {
			  select {
			   case <-ctx.Done():
				  return
			  case <-ticker.C:
				  fn()
			   }
		   }
	   }()
}
