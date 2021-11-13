package slo

import "time"

const (
	EtcdDialTimeout = 1500 * time.Millisecond
	EtcdKeepAliveEvery = 3 * time.Second
	KafkaPublishTimeout = 2 * time.Second
	RedisLockTimeout    = 200 * time.Millisecond
	OutboxPollDefault   = 400 * time.Millisecond
	WorkerLeaderTTL     = 8 * time.Second
	WorkerLeaderRefresh = 3 * time.Second
	SvcDialTries        = 3
	SvcCircuitThreshold = 5
	SvcCircuitReset = 2 * time.Second
	OutboxPoisonMax = 5
	OutboxBackoffCap     = 8 * time.Second
	EtcdLeaseTTLSecs     = 15
	KafkaConsumerMaxWait = 500 * time.Millisecond
)

// MiddlewareDocumented 中间件相关设计目标片段。

   func MiddlewareDocumented() map[string]any {
			   "region": "SG 10.18 / TYO 10.19; hosts etcd-0/1/2.kucoin.net",
		   },
		   "kafka": map[string]any{
			   "publish_timeout_ms": KafkaPublishTimeout.Milliseconds(),
			  "acks": "RequireAll",
			  "how":  "outbox journal first; bus notify only; poison topic outbox.poison",
			  "region": "kafka-0/1/2.kucoin.net KRaft",
		   },
		  "redis": map[string]any{
			   "lock_timeout_ms": RedisLockTimeout.Milliseconds(),
			   "how":             "user lock + worker leader; lock fail closed",
			  "region": "redis-0/1/2.kucoin.net Sentinel",
		   },
		   "outbox_worker": map[string]any{
			  "poll_ms":           OutboxPollDefault.Milliseconds(),
			   "leader_ttl_ms":     WorkerLeaderTTL.Milliseconds(),
			   "leader_refresh_ms": WorkerLeaderRefresh.Milliseconds(),
			   "confirm_concurrency": "config worker.confirm_concurrency",
			   "how":                 "Redis SetNX leader; drain outbox then Mark published",
		   },
		   "svcdial": map[string]any{
			   "tries":             SvcDialTries,
			  "circuit_threshold": SvcCircuitThreshold,
			   "circuit_reset_ms":  SvcCircuitReset.Milliseconds(),
			   "how":               "registry pick + backoff; circuit open returns error",
		   },
		  "alerts": map[string]any{
			   "names": MiddlewareAlertNames(),
			  "how": "BindMiddlewareAlertHook; FireMiddlewareAlert non-blocking",
		  },
		  "outbox_retry": map[string]any{
			   "poison_max": OutboxPoisonMax,
			   "backoff_cap_ms": OutboxBackoffCap.Milliseconds(),
			  "how":            "exponential backoff; poison to outbox.poison; lag to ops.outbox.stuck",
		   },
		  "kafka_consumer": map[string]any{
			  "max_wait_ms": KafkaConsumerMaxWait.Milliseconds(),
			  "how": "manual commit after handler OK; at-least-once",
		  },
		   "etcd_lease": map[string]any{
			   "ttl_secs": EtcdLeaseTTLSecs,
			  "how": "keepalive every 3s; streak fail then failover rebind",
		   },
	  }
  }
