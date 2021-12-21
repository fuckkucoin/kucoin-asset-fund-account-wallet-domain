// 功能：warroom-cascade：战争室级联冻结协议独立工作进程。
// 职责：运行覆盖率巡检、波次推进、阶段超时升级、过期计划中止、过期回滚腿生成。
// 不接收 HTTP 请求；API 路由由主账户服务提供（/v1/wrfreeze/）。

// 作者：Vance
// 日期：2021.12.20

package main

import (
	"context"
	"database/sql"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "github.com/go-sql-driver/mysql"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"account-assets.kucoin.net/funding/internal/dualctl"
	"account-assets.kucoin.net/funding/internal/wrfreeze"
)

   func main() {
	   dsn := flag.String("dsn", os.Getenv("MYSQL_DSN"), "MySQL DSN")
	   tick := flag.Duration("tick", 30*time.Second, "Worker tick interval")
	  phaseTimeout := flag.Duration("phase-timeout", 10*time.Minute, "Unfreeze phase execution timeout")
	  metricsAddr := flag.String("metrics-addr", ":9191", "Prometheus metrics HTTP address")
	   flag.Parse()
	  ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	   defer cancel()
	   // 初始化存储
	   var store wrfreeze.Store
	   if *dsn != "" {
		  db, err := sql.Open("mysql", *dsn)
		  if err != nil {
			  panic("open mysql: " + err.Error())
		  }
		   db.SetMaxOpenConns(10)
		  db.SetConnMaxLifetime(5 * time.Minute)
		   store = wrfreeze.NewSQLStore(db, "mysql")
	   } else {
		   store = wrfreeze.NewMemStore()
	   }
	   // 空冻结器（工作进程不直接冻结；仅做巡检与状态推进）
	  freezer := &noopFreezer{}
	  // 双控簿（四眼审批）
	  dual := dualctl.New(4 * time.Hour)
	  // 服务层
	   planSvc := wrfreeze.NewPlanService(store, dual, freezer)
	   waveSvc := wrfreeze.NewWaveService(store,
		  freezer, 8)
	  unfreezeSvc := wrfreeze.NewUnfreezeService(store, freezer)
	  proofSvc := wrfreeze.NewProofService(store, freezer)
	   rollbackSvc := wrfreeze.NewRollbackService(store, freezer, wrfreeze.NoopHoldcatReader{})
	  metrics := wrfreeze.NewMetrics()
	  cfg := wrfreeze.WorkerConfig{
		  TickInterval: *tick,
		  PhaseTimeout: *phaseTimeout,
		   WaveRetryMax: 3,
	  }
	  worker := wrfreeze.NewWorker(planSvc, waveSvc, unfreezeSvc, proofSvc, rollbackSvc, metrics, cfg)
	  // Prometheus 指标端点
	   mux := http.NewServeMux()
	  mux.Handle("/metrics", promhttp.Handler())
	  mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		  w.WriteHeader(http.StatusOK)
		  _, _ = w.Write([]byte("ok"))
	   })
	  srv := &http.Server{Addr: *metricsAddr, Handler: mux}
	   go func() {
		   if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			   panic("metrics server: " + err.Error())
		   }
	   }()
	   // 启动工作循环
	  worker.Run(ctx)
	   shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	  defer shutCancel()
	   _ = srv.Shutdown(shutCtx)
   }

type noopFreezer struct{}

   func (n *noopFreezer) SetFrozen(_ context.Context, _ string, _ bool) error { return nil }

  func (n *noopFreezer) IsFrozen(_ context.Context, _ string) (bool, error) { return false, nil }
  func (n *noopFreezer) HasLiveHashWithdraw(_ context.Context, _ string) (bool, error) {
	   return false, nil
   }
