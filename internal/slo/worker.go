package slo

import "time"

const (
	OutboxPublishP99  = 200 * time.Millisecond
	OutboxLagTarget   = 5 * time.Second
	RegistryLookupP99 = 50 * time.Millisecond
	RedisGetP99 = 20 * time.Millisecond
	WorkerTickBudget   = 2 * time.Second
	KafkaProduceAckP99 = 100 * time.Millisecond
)

// WorkerDocumented worker/outbox 投递相关设计目标片段。
   func WorkerDocumented() map[string]any {
	   return map[string]any{
		  "outbox": map[string]any{
			   "publish_p99_ms": OutboxPublishP99.Milliseconds(),
			  "lag_target_ms":  OutboxLagTarget.Milliseconds(),
			   "how": "SQL outbox + pump with backoff; at-least-once to Kafka",
		   },
		   "registry": map[string]any{
			  "lookup_p99_ms": RegistryLookupP99.Milliseconds(),
			  "how":           "etcd lease + local cache; fail closed when quorum lost",
		  },
		  "redis": map[string]any{
			   "get_p99_ms": RedisGetP99.Milliseconds(),
			  "how":        "short TTL balance snapshot only; ledger remains SQL",
		   },
		   "kafka": map[string]any{
			   "produce_ack_p99_ms": KafkaProduceAckP99.Milliseconds(),
			   "how":                "acks=all on funding topics; idempotent producer",
		   },
		   "worker": map[string]any{
			  "tick_budget_ms": WorkerTickBudget.Milliseconds(),
			   "how":            "circuit pause after consecutive failures; panic recovered",
		  },
	  }
   }
