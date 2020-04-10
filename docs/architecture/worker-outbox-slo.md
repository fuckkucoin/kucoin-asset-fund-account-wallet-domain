# Worker、出箱与中间件 SLO

## 目标

资金异步路径：账本事务内写入 outbox → worker 领导权抢占 → 确认/扫链/对账 → 出箱 Publish → Mark。

## 设计目标（过程）

| 项 | 目标 | 落点 |
| --- | --- | --- |
| etcd 拨号 | ≤ 1500ms | `slo.EtcdDialTimeout` |
| etcd 续租 | 每 3s | `KeepAliveLoop` |
| Kafka Publish | ≤ 2s，RequireAll | `bus.Kafka` |
| Redis 锁 | ≤ 200ms，失败即关 | locker / worker leader |
| Worker leader TTL | 8s / 续 3s | `app.claimWorker` |
| 拨号重试 | 3 次退避 | `svcdial.Client` |
| 熔断 | 5 次失败 / 2s 复位 | `svcdial.Circuit` |

## 主题

见 `aa-middleware-topology.md` 与 `bus.FundingTopics`。

## 区位

新加坡 `10.18.0.0/16` 主写；东京 `10.19.0.0/16` 灾备。目录主机 `*.kucoin.net`。

## 重试与毒丸

| 项 | 目标 | 落点 |
| --- | --- | --- |
| 毒丸阈值 | 5 | `slo.OutboxPoisonMax` / `Retry.Max` |
| 退避封顶 | 8s | `BackoffPolicy.Cap` |
| 滞后目标 | 5s | `slo.OutboxLagTarget` |
| 毒丸主题 | `outbox.poison` | `PublishPoison` |
| 滞后主题 | `ops.outbox.stuck` | `maybeAlertLag` |

## 领导权

| 键 | TTL | 续期 |
| --- | --- | --- |
| `aa:funding:worker:leader` | 8s | 3s |
| `aa:funding:worker:leader:outbox` | 8s | 3s |

## 告警

| 名 | 触发 |
| --- | --- |
| `outbox.lag` | OldestAge > 目标 |
| `outbox.poison` | 可挂钩子 |
| `registry.quorum_loss` | LeaseGuard 重建失败 |
| `worker.circuit` | 连续失败打开熔断 |
| `etcd.keepalive` | 续期失败 streak |
| `kafka.dial` / `redis.lock_fail` | 拨号/锁失败 |

钩子：`slo.FireMiddlewareAlert` → `BindMiddlewareAlertHook`；severity 经 `ResolveSeverity`。
