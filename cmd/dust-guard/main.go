// @FileName: main.go
// @Summary: 粉尘/灰尘攻击资金域防护 worker 进程。
// 此进程启动防护服务并挂载到入金 Observe 路径；
// 检测泵、隔离超时巡检和水位触发轮换作为后台 goroutine 运行。
// HTTP 路由提供运维管理 API（隔离/轮换/策略/防护单）。
// 环境变量：
// ACCOUNT_ASSETS_DSN — MySQL DSN（必填）
// ACCOUNT_ASSETS_PG_DSN — 也支持 Postgres
// DUST_GUARD_DETECTOR_SEC — 检测器 flush 间隔秒数（默认 30）
// DUST_GUARD_QUARANTINE_SEC— 隔离巡检间隔秒数（默认 60）
// DUST_GUARD_WATERLEVEL_SEC— 水位检测间隔秒数（默认 120）
// @Author: Likeo
// @Created: 2021.04.15

package main

import (
	"context"
	"os"
	"strconv"
	"time"
	"account-assets.kucoin.net/funding/internal/addrpool"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/approvestore"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/dustguard"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

   func main() {
	  var startGuard func()
	   svcboot.MainEngine("dust-guard", config.HostAddress, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   deposit.Register(v1, rt)
		  dustguard.RegisterGuardRoutes(v1,
			  rt)
		  svc := buildGuardService(rt)
		   rt.DustGuard = svc
		  // 把防护钩子注入 deposit.Service
		  depSvc := deposit.FromRuntime(rt)
		  if depSvc != nil && svc != nil {
			   depSvc.BindDustGuard(dustguard.NewDepositAdapter(svc.ObserveHook()))
		  }
		   startGuard = func() {
			  detSec := envInt("DUST_GUARD_DETECTOR_SEC", 30)
			  quaSec := envInt("DUST_GUARD_QUARANTINE_SEC", 60)
			  wlSec := envInt("DUST_GUARD_WATERLEVEL_SEC", 120)
			   svc.StartWorkers(
				  context.Background(),
				  nil, // WaterLevelReader 可由运营插入具体实现
				  time.Duration(detSec)*time.Second,
				  time.Duration(quaSec)*time.Second,
				  time.Duration(wlSec)*time.Second,
			  )
		   }
	   }, func(_ *gin.Engine, _ *app.Runtime) {
		   if startGuard != nil {
			   go startGuard()
		  }
	   })
   }

  func buildGuardService(rt *app.Runtime) *dustguard.GuardService {
	  if rt == nil {
		   return nil
	   }
	   pool := addrpool.New(nil,
		   24*time.Hour)
	   if rt.SQL != nil {
		   pool.BindSQL(rt.SQL)
	  }
	  inv := addrpool.NewInventory(nil, 24*time.Hour)
	   if rt.SQL != nil {
		   inv.BindSQL(rt.SQL)
	   }
	  shared := addrpool.NewSharedMemoPool(nil)
	  if rt.SQL != nil {
		   shared.BindSQL(rt.SQL)
	   }
	   var apprStore *approvestore.Store
	   if rt.KV != nil {
		  apprStore = approvestore.New(rt.KV)
		  if rt.SQL != nil {
			   apprStore.BindSQL(rt.SQL)
		  }
	  }
	   opts := dustguard.GuardOptions{
		  DB: rt.SQL,
		  PolicyTTL: 60 * time.Second,
		  Pool:   addrpool.NewPoolOwnerAdapter(pool),
		   Inv:    addrpool.NewInventoryAdapter(inv),
		   Shared: shared,
		  SharedChk: shared,
		  Approvals: apprStore,
	  }
	   return dustguard.New(opts)
  }

  func envInt(key string, def int) int {
	   if v := os.Getenv(key); v != "" {
		   if n, err := strconv.Atoi(v); err == nil && n > 0 {
			  return n
		  }
	  }
	   return def
   }
