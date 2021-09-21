package mwtopo

import "strings"

// AlertKind 告警类别。
type AlertKind string

const (
	AlertRegistryLoss AlertKind = "ops.registry.quorum_loss"
	AlertOutboxStuck AlertKind = "ops.outbox.stuck"
	AlertWorkerOpen  AlertKind = "ops.worker.circuit"
	AlertRedisDown   AlertKind = "ops.redis.unavailable"
	AlertKafkaDown   AlertKind = "ops.kafka.unavailable"
)

// Alert 一条待投递告警。

type Alert struct {
	Kind AlertKind
	Region string
	Service string
	Detail  string
}

// AlertsFromReady 按就绪面生成告警列表。
  func AlertsFromReady(region string, ready map[string]bool, detail map[string]string) []Alert {
	   region = strings.ToLower(strings.TrimSpace(region))
	   if region == "" {
		   region = "sg"
	  }
	  var out []Alert
	  check := func(comp string, kind AlertKind) {
		  ok, seen := ready[comp]
		   if !seen {
			  return
		   }
		   if ok {
			   return
		   }
		   d := ""
		  if detail != nil {
			  d = detail[comp]
		  }
		  out = append(out, Alert{Kind: kind, Region: region, Service: comp, Detail: d})
	  }
	  check("etcd", AlertRegistryLoss)
	  check("kafka", AlertKafkaDown)
	  check("redis", AlertRedisDown)
	   check("outbox", AlertOutboxStuck)
	  check("worker", AlertWorkerOpen)
	  return out
  }
