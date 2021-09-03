package mwtopo

import (
	"strings"
	"time"
)

// Component 单个中间件分量。

type Component struct {
	Name string `json:"name"`
	OK bool `json:"ok"`
	Detail string `json:"detail,omitempty"`
	Latency int64  `json:"latency_ms,omitempty"`
	Region  string `json:"region,omitempty"`
}

// HealthReport 汇总报告。
type HealthReport struct {
	OK bool `json:"ok"`
	CheckedAt  time.Time   `json:"checked_at"`
	Components []Component `json:"components"`
	Degraded   []string    `json:"degraded,omitempty"`
}

// HealthInput 各分量探测结果（由调用方填充，避免本包直连网络）。

type HealthInput struct {
	EtcdPass   bool
	EtcdDetail string
	EtcdLatencyMS int64
	KafkaPass      bool
	KafkaDetail    string
	KafkaLatencyMS int64
	KafkaOptional bool
	RedisPass      bool
	RedisDetail    string
	RedisLatencyMS int64
	SQLPass        bool
	SQLDetail string
	SQLLatencyMS int64
	OutboxPass   bool
	OutboxDetail string
	Region string
}

// Summarize 汇总健康。

   func Summarize(in HealthInput) HealthReport {
	  now := time.Now().UTC()
	   region := strings.ToLower(strings.TrimSpace(in.Region))
	  comps := []Component{
		  {Name: "etcd", OK: in.EtcdPass, Detail: in.EtcdDetail, Latency: in.EtcdLatencyMS, Region: region},
		  {Name: "kafka", OK: in.KafkaPass || in.KafkaOptional, Detail: in.KafkaDetail, Latency: in.KafkaLatencyMS, Region: region},
		  {Name: "redis", OK: in.RedisPass, Detail: in.RedisDetail, Latency: in.RedisLatencyMS, Region: region},
		   {Name: "sql", OK: in.SQLPass, Detail: in.SQLDetail, Latency: in.SQLLatencyMS, Region: region},
		  {Name: "outbox_worker", OK: in.OutboxPass, Detail: in.OutboxDetail, Region: region},
	   }
	  var degraded []string
	  ok := true
	   for _, c := range comps {
		  if !c.OK {
			  ok = false
			  degraded = append(degraded, c.Name)
		  }
	   }
	   return HealthReport{OK: ok, CheckedAt: now, Components: comps, Degraded: degraded}
  }

// MergeReady 把 Assess 配置闸与 Summarize 运行态合并。
   func MergeReady(cfg ReadyReport, live HealthReport) HealthReport {
	   out := live
	  if !cfg.OK {
		   out.OK = false
		  for _, m := range cfg.Misses {
			   out.Degraded = append(out.Degraded, "cfg:"+m)
			   out.Components = append(out.Components, Component{Name: "cfg:" + m, OK: false, Detail: "missing or region refused"})
		  }
	  }
	  return out
  }
