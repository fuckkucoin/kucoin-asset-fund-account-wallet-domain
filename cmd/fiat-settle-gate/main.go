// 功能：fiat-settle-gate：法币轨道授权捕获结清与退款租约工作进程。
// 职责：
// 1. 过期 authorize 租约吊销
// 2. Capture 孤儿补偿泵（禁止二次 Capture）
// 3. 超限占用巡检
// 4. HTTP ops API（/ops/fiat-settle-gate/）
// 隔离：不改写 fiatadd Confirm、fiatjnl 不平衡挡写、fiatbreak 轧差 FSM。

// 作者：Cason
// 日期：2021.05.11

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
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/fiatrail"
	"account-assets.kucoin.net/funding/internal/fiatsettlegate"
	"account-assets.kucoin.net/funding/internal/persist"
)

   func main() {
	   apiAddr := flag.String("api-addr", ":8094", "ops HTTP API 监听地址")
	  metricsAddr := flag.String("metrics-addr", ":9302", "Prometheus 指标监听地址")
	   expireInterval := flag.Duration("expire-interval", 15*time.Second, "过期吊销间隔")
	   compInterval := flag.Duration("compensate-interval", 10*time.Second, "补偿泵间隔")
	   occupyInterval := flag.Duration("occupy-interval", 30*time.Second, "超限占用巡检间隔")
	   flag.Parse()
	   ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	  defer cancel()
	   rt, err := app.Open()
	   if err != nil {
		   fmt.Fprintf(os.Stderr, "fiat-settle-gate: open ledger: %v\n", err)
		   os.Exit(1)
	   }
	   if rt.Ledger == nil || rt.SQL == nil || rt.SQL.DB == nil {
		   fmt.Fprintf(os.Stderr, "fiat-settle-gate: ledger is not open\n")
		   os.Exit(1)
	   }
	   dialect := "mysql"
	   if rt.SQL.Dialect == persist.Postgres {
		   dialect = "postgres"
	   }
	  rail := fiatrail.New().BindLedger(rt.Ledger)
	   if psp := rail.PSP(); psp != nil {
		   // 进程内收单不作为结清依据。未接外部收单时授权与捕获失败。
		   psp.SetDown(true)
	   }
	  svc := fiatsettlegate.NewWithStore(fiatsettlegate.NewSQLStore(rt.SQL.DB, dialect), rail, rt.Ledger)
	   cfg := fiatsettlegate.WorkerConfig{
		  ExpireInterval: *expireInterval,
		   CompensateInterval: *compInterval,
		  OccupyInterval: *occupyInterval,
		  BatchSize: 50,
		  CompensateActor: "ops-settle-pump",
	  }
	   worker := fiatsettlegate.NewWorker(svc, cfg)
	   gin.SetMode(gin.ReleaseMode)
	  router := gin.New()
	  router.Use(gin.Recovery())
	  fiatsettlegate.NewHandler(svc).Register(router.Group("/ops"))
	   apiSrv := &http.Server{Addr: *apiAddr,
		  Handler: router}
	   metricsMux := http.NewServeMux()
	   metricsMux.Handle("/metrics", promhttp.Handler())
	  metricsMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		  w.WriteHeader(http.StatusOK)
		  _, _ = w.Write([]byte("ok"))
	   })
	   metricsSrv := &http.Server{Addr: *metricsAddr, Handler: metricsMux}
	  go func() {
		  if err := apiSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			   fmt.Fprintf(os.Stderr, "[fiat-settle-gate] api error: %v\n", err)
		   }
	  }()
	  go func() {
		  if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			   fmt.Fprintf(os.Stderr, "[fiat-settle-gate] metrics error: %v\n", err)
		  }
	  }()
	   fmt.Fprintf(os.Stderr, "[fiat-settle-gate] started api=%s metrics=%s\n", *apiAddr, *metricsAddr)
	  go worker.Run(ctx)
	   <-ctx.Done()
	  shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	  defer shutCancel()
	  _ = apiSrv.Shutdown(shutCtx)
	  _ = metricsSrv.Shutdown(shutCtx)
   }
