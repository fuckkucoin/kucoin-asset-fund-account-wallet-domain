// 功能：法币上分确认闸进程入口。
// 挂载：
// - fiatadd 口径 Open/Get/ListForUser 端点（兼容现有 fiat-rail 路由）
// - fiataddgate GateConfirm / fork-cases / confirm-leases 端点
// - 后台 worker：分叉巡检 + 未决 confirming 回收
// 配置要求：
// - FUNDS_FIAT_VENDOR + FUNDS_FIAT_API_KEY：厂商配置
// - 签名密钥复用 FiatAPIKey 或独立注入

// 作者：Blaise
// 日期：2021.05.11

package main

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/fiatadd"
	"account-assets.kucoin.net/funding/internal/fiataddgate"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

   func main() {
	   svcboot.MainEngine("fiat-add-gate", config.HostFiatOnramp, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   fiatadd.Register(v1, rt)
		  fiataddgate.Register(v1, rt)
	   }, func(_ *gin.Engine, rt *app.Runtime) {
		   startWorkers(rt)
	  })
  }

   func startWorkers(rt *app.Runtime) {
	   kv := rt.KV
	  if kv == nil {
		   kv = persist.NewMemKV()
	  }
	  log := rt.Log
	  forks := fiataddgate.NewKVForkCaseStore(kv)
	  leases := fiataddgate.NewKVLeaseStore(kv)
	   book := fiatadd.FromRuntime(rt)
	   patrolWorker := fiataddgate.NewForkPatrolWorker(forks, log, fiataddgate.ForkPatrolConfig{
		  Interval:   30 * time.Second,
		   BatchLimit: 50,
	   }, nil)
	  recycleWorker := fiataddgate.NewPendingConfirmingRecycler(
		  book, leases, forks, nil, log,
		  fiataddgate.RecycleConfig{
			   Interval:   60 * time.Second,
			   BatchLimit: 20,
			  StaleAfter: 90 * time.Second,
		   },
	  )
	   ctx := context.Background()
	  go func() { _ = patrolWorker.Run(ctx) }()
	  go func() { _ = recycleWorker.Run(ctx) }()
   }
