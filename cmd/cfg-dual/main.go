// 功能：资金配置灰度双写与策略脑裂封堵进程入口。
// 服务职责：
// 1. 挂载 cfgdual HTTP API（灰度配置、差分看板、强制 baseline、回滚）；
// 2. 启动四类 Worker（决策泵、差分聚合、封堵执行、回滚巡检）；
// 3. 向 cfgcenter.Store 订阅事件，在包代次更新时刷新 Cohort 世代。
// 部署模型：单副本或双副本（竞争 Redis leader 锁）；
// 无状态 eval 可多副本部署；Worker 争抢单实例。
// 内部主机：iam-gateway.kucoin.net（与 cfgcenter 共用鉴权域）。

// 作者：TateVoss
// 日期：2021.03.11

package main

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/cfgcenter"
	"account-assets.kucoin.net/funding/internal/cfgdual"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/logx"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"account-assets.kucoin.net/funding/internal/workerloop"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

   func main() {
	  svcboot.MainEngine("cfg-dual", config.HostIAMGateway, cfgdual.Register, startWorkers)
   }

// startWorkers 在 HTTP 服务启动后异步运行四类 Worker。
  func startWorkers(r *gin.Engine, rt *app.Runtime) {
	   log := logx.New("info")
	   cfgSt := cfgcenter.NewStore()
	   if rt != nil && !rt.Mem && rt.SQL != nil {
		  cfgSt.BindSQL(rt.SQL)
	  }
	  idgen := func() string { return uuid.NewString() }
	   var mgr *cfgdual.Manager
	  if rt != nil && !rt.Mem && rt.SQL != nil {
		  mgr = cfgdual.NewManager(cfgSt, rt.SQL, idgen)
	   } else {
		  mgr = cfgdual.NewManager(cfgSt, nil, idgen)
	   }
	   if err := mgr.Bootstrap(context.Background()); err != nil {
		   log.Error().Err(err).Msg("cfg-dual: bootstrap failed")
	   }
	   ws := cfgdual.BuildWorkerSet(mgr, log)
	   workerCfg := func(name string, interval time.Duration) workerloop.Config {
		   return workerloop.Config{
			  Name: name,
			   Interval:   interval,
			  MaxBackoff: 60 * time.Second,
			   FailThresh: 5,
			  CircuitPause: 30 * time.Second,
		   }
	  }
	   go func() {
		   _ = workerloop.Run(context.Background(),
			  workerCfg("cfgdual-decision-pump", 2*time.Second), log, ws.DecisionPump)
	   }()
	  go func() {
		   _ = workerloop.Run(context.Background(),
			  workerCfg("cfgdual-diff-aggregate", 30*time.Second), log, ws.DiffAggregate)
	  }()
	  go func() {
		   _ = workerloop.Run(context.Background(),
			  workerCfg("cfgdual-seal-executor", 15*time.Second), log, ws.SealExecutor)
	  }()
	   go func() {
		  _ = workerloop.Run(context.Background(),
			  workerCfg("cfgdual-rollback-patrol", 60*time.Second), log, ws.RollbackPatrol)
	  }()
   }
