package mwtopo

import (
	"context"
	"time"
)

// LiveDeps 运行态依赖探测入口（由上层注入）。

type LiveDeps struct {
	Region   string
	Ready    ReadyInput
	EtcdPing Pinger
	RedisPing Pinger
	SQLPing   Pinger
	KafkaDial DialProber
	KafkaOptional bool
	OutboxOK bool
	OutboxDetail string
}

// Live 执行一轮探测并汇总。

   func Live(ctx context.Context, deps LiveDeps) HealthReport {
	   cfg := Assess(deps.Ready)
	  in := HealthInput{
		  KafkaOptional: deps.KafkaOptional,
		   OutboxPass:   deps.OutboxOK,
		   OutboxDetail: deps.OutboxDetail,
		  Region: Region(deps.Region),
	   }
	   if deps.EtcdPing != nil {
		  c := ProbePinger(ctx, "etcd", deps.EtcdPing)
		   in.EtcdPass, in.EtcdDetail, in.EtcdLatencyMS = c.OK, c.Detail, c.Latency
	  }
	  if deps.RedisPing != nil {
		  c := ProbePinger(ctx, "redis", deps.RedisPing)
		  in.RedisPass, in.RedisDetail, in.RedisLatencyMS = c.OK, c.Detail, c.Latency
	   }
	  if deps.SQLPing != nil {
		   c := ProbePinger(ctx, "sql", deps.SQLPing)
		   in.SQLPass, in.SQLDetail, in.SQLLatencyMS = c.OK, c.Detail, c.Latency
	   }
	   if deps.KafkaDial != nil {
		  c := ProbeDial("kafka", deps.KafkaDial, 2*time.Second)
		   in.KafkaPass, in.KafkaDetail, in.KafkaLatencyMS = c.OK, c.Detail, c.Latency
	   } else if deps.KafkaOptional {
		   in.KafkaPass = true
		   in.KafkaDetail = "optional"
	   }
	  return MergeReady(cfg, Summarize(in))
  }
