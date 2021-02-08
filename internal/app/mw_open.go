package app

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/bus"
	"account-assets.kucoin.net/funding/internal/mwtopo"
	"account-assets.kucoin.net/funding/internal/outboxpump"
	"account-assets.kucoin.net/funding/internal/redisx"
	"account-assets.kucoin.net/funding/internal/slo"
	"account-assets.kucoin.net/funding/internal/svcreg"
)

// RedisX 可选门面（与 go-redis 原生客户端并存；新路径先）。
   func (rt *Runtime) openRedisX() (redisx.Client, error) {
	   if rt == nil {
		   return nil, redisx.ErrUnavailable
	   }
	   if strings.TrimSpace(os.Getenv("AA_REDIS_MEM")) == "1" {
		  return redisx.NewMem(), nil
	   }
	   return redisx.OpenFromEnv()
  }

// StartOutboxLeaderPump 后台启动出箱领导泵（非阻塞）。

   func (rt *Runtime) StartOutboxLeaderPump(ctx context.Context) {
	  if rt == nil || rt.Bus == nil || rt.KV == nil {
		   return
	  }
	  rdb, err := rt.openRedisX()
	  if err != nil {
		   rt.Log.Warn().Err(err).Msg("outbox leader redisx")
		   rdb = nil
	  }
	   cfg := outboxpump.DefaultLeaderPumpConfig()
	   cfg.LeaderKey = redisx.KeyOutboxLeader
	  go func() {
		  if err := outboxpump.RunLeaderPump(ctx, rt.KV, rt.Bus, rdb, cfg, rt.Log); err != nil && ctx.Err() == nil {
			  rt.Log.Error().Err(err).Msg("outbox leader pump exit")
		  }
	   }()
   }

// StartRegistryLeaseGuard 后台租约续期加固。
   func (rt *Runtime) StartRegistryLeaseGuard(ctx context.Context) {
	   if rt == nil {
		   return
	  }
	   et,
		  ok := rt.Registry.(*svcreg.Etcd)
	   if !ok || et == nil {
		  return
	   }
	   eps := []string{}
	  if rt.Cfg != nil {
		  eps = append(eps, rt.Cfg.Etcd.Endpoints...)
	  }
	  if len(eps) == 0 {
		   eps = mwtopo.EtcdEndpointsSG()
	  }
	   prefix := "/aa/svc"
	   if rt.Cfg != nil {
		   prefix = strings.TrimSuffix(svcreg.Key(rt.Cfg.Etcd.Namespace, rt.Cfg.Etcd.Env, ""), "/")
		  if prefix == "" {
			   prefix = "/aa/svc"
		  }
	  }
	   g := svcreg.NewLeaseGuard(et, eps, prefix, rt.Log)
	  g.OnLoss = func(err error) {
		  slo.FireMiddlewareAlertDetail("registry.quorum_loss", 1, 0, err.Error())
		   if rt.Bus != nil {
			   body, _ := json.Marshal(map[string]any{"err": err.Error(), "at": time.Now().UTC()})
			  _ = rt.Bus.Publish(context.Background(), bus.TopicRegistryLoss, []byte("quorum"), body)
		  }
	  }
	  go g.Run(ctx)
	  // 若守卫轮转了网关，运行时仍握旧指针；以首次成功 failover 为主，轮转后需进程重启接新指针。
	  _ = g
  }

// MiddlewareRegionSurface 只读区域就绪面（供探针扩展，不改既有 REST 字段名）。

   func (rt *Runtime) MiddlewareRegionSurface() mwtopo.RegionSurface {
	   sqlDSN := ""
	  kafkaOpt := true
	   if rt != nil && rt.Cfg != nil {
		   sqlDSN = strings.TrimSpace(rt.Cfg.Postgres.DSN)
		  if sqlDSN == "" {
			   sqlDSN = strings.TrimSpace(rt.Cfg.MySQLDSN)
		  }
		  kafkaOpt = rt.Cfg.KafkaOptional
	  }
	  sg := mwtopo.CatalogReadyInput("sg", sqlDSN, kafkaOpt)
	   tyo := mwtopo.CatalogReadyInput("tyo", sqlDSN, kafkaOpt)
	   return mwtopo.BuildSurface(sg, tyo, nil)
  }

// BindMiddlewareAlerts 把中间件告警钩到日志（可再挂 metrics）。
  func (rt *Runtime) BindMiddlewareAlerts() {
	   if rt == nil {
		  return
	  }
	   slo.BindMiddlewareAlertHook(func(ev slo.MiddlewareAlert) {
		   rt.Log.Warn().
			   Str("alert", ev.Name).
			   Str("severity", ev.Severity).
			   Float64("value", ev.Value).
			   Float64("threshold", ev.Threshold).
			   Str("detail", ev.Detail).
			  Msg("middleware alert")
	   })
  }

// EnsureFundingTopics 预检主题；缺主题记错误日志。
  func (rt *Runtime) EnsureFundingTopics(ctx context.Context) {
	  if rt == nil || rt.Cfg == nil {
		  return
	  }
	  bros := append([]string(nil),
		   rt.Cfg.KafkaBrokers...)
	   if len(bros) == 0 {
		  pc := bus.LoadProdConfig()
		  bros = pc.Brokers
	   }
	  if len(bros) == 0 {
		  return
	  }
	  checks, err := bus.EnsureTopics(ctx, bros, bus.AllManagedTopics())
	  if err != nil {
		   rt.Log.Warn().Err(err).Msg("kafka ensure topics")
		   return
	  }
	  if !bus.AllTopicsOK(checks) {
		   for _, c := range checks {
			  if !c.OK {
				   rt.Log.Error().Str("topic", c.Topic).Str("detail", c.Detail).Msg("kafka topic missing")
			  }
		   }
	  }
  }
