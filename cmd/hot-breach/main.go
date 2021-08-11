// @FileName: main.go
// @Summary: main 模块 main。
// cmd/hot-breach/main.go 热库击穿守护进程入口。
// 启动三类 Worker：
// 1. LeaseExpiryWorker — 租约过期回收
// 2. BreachPatrolWorker — 击穿巡检（open→gated）
// 3. WriteBlockClearWorker — gated 超期告警
// 环境变量：
// HOT_BREACH_KV_DSN KV 后端 DSN（空则内存模式）
// HOT_BREACH_LEASE_TTL 租约 TTL（如 "30s"，默认 30s）
// HOT_BREACH_PATROL_INT 巡检周期（如 "15s"，默认 15s）
// HOT_BREACH_EXPIRY_INT 过期回收周期（如 "10s"，默认 10s）
// HOT_BREACH_ALERT_AGE gated 超期告警阈值（如 "10m"，默认 10m）
// 日志输出至 stderr（JSON 形态）。
// @Author: Cason
// @Created: 2021.07.20

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
	"account-assets.kucoin.net/funding/internal/hotbreach"
)

  func main() {
	  if err := run(); err != nil {
		  fmt.Fprintf(os.Stderr, "hot-breach: fatal: %v\n", err)
		   os.Exit(1)
	   }
  }

   func run() error {
	   ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	  defer cancel()
	   // 从环境变量读取配置
	   patrolInt := parseDuration(os.Getenv("HOT_BREACH_PATROL_INT"), 15*time.Second)
	   expiryInt := parseDuration(os.Getenv("HOT_BREACH_EXPIRY_INT"), 10*time.Second)
	   alertAge := parseDuration(os.Getenv("HOT_BREACH_ALERT_AGE"), 10*time.Minute)
	  // 构造 Service（内存模式；注入KV）
	  svc := hotbreach.NewService(nil, nil, nil)
	  // 构造 Worker Group
	   wg := hotbreach.NewWorkerGroup(svc)
	  _ = wg
	  // 单独启动三 Worker（可自定义周期）
	  expiryWorker := hotbreach.NewLeaseExpiryWorker(svc, expiryInt)
	   patrolWorker := hotbreach.NewBreachPatrolWorker(svc,
		   patrolInt)
	  monitorWorker := hotbreach.NewWriteBlockClearWorker(svc, 60*time.Second, alertAge)
	   monitorWorker.OnAlert(func(breachID string, age time.Duration) {
		  fmt.Fprintf(os.Stderr, "{\"level\":\"warn\",\"msg\":\"breach gated too long\",\"breach_id\":%q,\"age\":%q}\n",
			   breachID, age.String())
	  })
	  errc := make(chan error, 3)
	   go func() { errc <- expiryWorker.Run(ctx) }()
	  go func() { errc <- patrolWorker.Run(ctx) }()
	   go func() { errc <- monitorWorker.Run(ctx) }()
	   fmt.Fprintf(os.Stderr, "{\"level\":\"info\",\"msg\":\"hot-breach workers started\",\"expiry_int\":%q,\"patrol_int\":%q,\"alert_age\":%q}\n",
		  expiryInt.String(), patrolInt.String(), alertAge.String())
	   select {
	  case <-ctx.Done():
		   fmt.Fprintf(os.Stderr, "{\"level\":\"info\",\"msg\":\"hot-breach shutting down\"}\n")
		  return nil
	   case err := <-errc:
		   if err != nil && err != context.Canceled {
			  return fmt.Errorf("worker error: %w", err)
		   }
		   return nil
	  }
  }

// parseDuration 解析时长字符串，失败返回 fallback。

   func parseDuration(s string, fallback time.Duration) time.Duration {
	  if s == "" {
		   return fallback
	  }
	   d, err := time.ParseDuration(s)
	  if err != nil || d <= 0 {
		  return fallback
	  }
	   return d
  }
