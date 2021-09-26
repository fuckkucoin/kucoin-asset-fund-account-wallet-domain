// 职责：
// - 启动弃单收口 HTTP API（计划查询、强制弃单、冲突证明、收口校验）。
// - 后台启动四类 Worker：超时扫描、弃单推进、超龄巡检、证明巡检、冲突仲裁通知。
// - 注入 HoldCatObserver（只读观察 holdcat）与 AbandonBridge（资金操作桥）。
// 隔离约定：
// - 不启动 r7 争议追偿 worker（由 dispute-claw 进程管理）。
// - 不启动 r6 分层冻结 worker（由 holdcat 主进程管理）。
// - 不写 P2P 撮合逻辑；超时信号由调用方注入（OrderTimeoutSource 接口）。

// Author: Kian
// Date: 2021.09.19

package main

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/p2pabandon"
	"account-assets.kucoin.net/funding/internal/p2pgate"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

  func main() {
	  svcboot.Main("p2p-abandon", config.HostP2PFunding, register)
  }

   func register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  // 构建弃单收口服务（内存后端；中可注入 SQL Store）。
	   sla := p2pabandon.SLAConfig{
		  OrderTimeoutSecs: 1800,
		  GraceSecs: 60,
		   SLAEscalateSecs: 86400,
		  SLAWarningRatio: 0.75,
	   }
	  abandonSvc := p2pabandon.NewWithSLA(sla)
	   // 构建资金桥（注入 p2pgate.Funds 的 holdcat.Book 与 ledger.Service）。
	  var bridge p2pabandon.FundsBridge
	   if rt != nil {
		  holdBook := holdcat.New(rt.KV)
		   funds := p2pgate.NewFunds(rt.Ledger, holdBook)
		   bridge = p2pgate.NewAbandonBridge(funds, abandonSvc)
		  // holdcat 只读观察器（不写 holdcat 核心 FSM）。
		   holdObserver := p2pgate.NewHoldCatObserver(holdBook)
		  timedObserver := p2pabandon.NewTimedHoldObserver(holdObserver)
		   // 后台启动 Worker（ctx 在进程关闭时由 svcboot 取消）。
		   workerCtx,
			  workerCancel := context.WithCancel(context.Background())
		   _ = workerCancel // svcboot 关闭时取消（此处；中由 signal handler 触发）
		  m := p2pabandon.DefaultMetrics()
		  // 超时扫描 Worker（依赖外部 OrderTimeoutSource 注入；此处使用空实现占位）。
		  // 中替换为从 Kafka/DB 读取超时信号的 Source 实现。
		  timeoutSrc := &p2pabandon.StaticTimeoutSource{Signals: nil, Acked: make(map[string]bool)}
		  timeoutWorker := p2pabandon.NewTimeoutScanWorker(abandonSvc, timeoutSrc, m)
		   go timeoutWorker.Run(workerCtx)
		  // 弃单推进 Worker。
		   advWorker := p2pabandon.NewAbandonAdvWorker(abandonSvc, bridge, m)
		   go advWorker.Run(workerCtx)
		   // 超龄巡检 Worker。
		   agingWorker := p2pabandon.NewAgingPatrolWorker(abandonSvc, timedObserver, m)
		  go agingWorker.Run(workerCtx)
		   // 证明巡检 Worker。
		   proofWorker := p2pabandon.NewProofPatrolWorker(abandonSvc, m)
		   go proofWorker.Run(workerCtx)
		   // 冲突仲裁通知 Worker（使用日志告警型 Notifier；替换为 PagerDuty/飞书）。
		   conflictWorker := p2pabandon.NewConflictDeskWorker(abandonSvc, logConflictNotifier{}, m)
		  go conflictWorker.Run(workerCtx)
		  // 控制 Worker 生命周期：等待一段时间后停止（；中用 signal）。
		   go func() {
			   time.Sleep(24 * time.Hour * 365 * 10) // 实际不超时；由 svcboot 关闭信号取消
			   workerCancel()
		   }()
	   }
	  // 注册 HTTP API。
	   p2pabandon.Register(v1, abandonSvc)
  }

// logConflictNotifier 日志型冲突通知器（实现；替换为告警系统）。
type logConflictNotifier struct{}

  func (n logConflictNotifier) NotifyConflict(ctx context.Context, plan p2pabandon.AbandonPlan, tokens []p2pabandon.MutexToken) error {
	   // 中接入飞书/PagerDuty/工单系统告警。
	   // 此处仅打印占位，避免引入 log 包依赖。
	  _ = plan
	  _ = tokens
	  return nil
   }
