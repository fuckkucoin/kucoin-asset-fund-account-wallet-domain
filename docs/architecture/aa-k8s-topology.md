# Account & Assets Kubernetes 拓扑

在既有微服务骨架上按角色部署。不新增 `*.kucoin.net` 主机名。新加坡为主写区，东京为备用区。地址段不变。

## 参考

- https://docs.platform.kucoin.local/vendor/kubernetes/workloads/deployment
- https://docs.platform.kucoin.local/vendor/kubernetes/scheduling/pod-affinity
- https://docs.platform.kucoin.local/vendor/kubernetes/autoscale/hpa
- https://docs.platform.kucoin.local/vendor/etcd/v3.5/clustering
- https://docs.platform.kucoin.local/vendor/kafka/kraft-operations
- `aa-middleware-topology.md`、`deployment.md`

## 区域

| 区 | CIDR | 角色 |
| --- | --- | --- |
| Singapore | `10.18.0.0/16` | 主写。MySQL / Postgres 主库、Kafka 主集群、etcd 法定人数、服务与 VIP/LB |
| Tokyo | `10.19.0.0/16` | 少数灾备：只读副本 + 热备。`internal/ha.Pair` 选主失败才上写 |

中间件、库、Kafka、Redis、etcd、服务主机与 VIP/LB 只使用上表地址段。链上 JSON-RPC / 浏览器仍走配置中的公网 URL。目录主机两侧同名，跨区 DNS / 私网解析到对应区地址。

## 服务角色

| 角色 | 目录主机 | 副本 | 反亲和 | CPU / 内存提示 | 说明 |
| --- | --- | --- | --- | --- | --- |
| API gateway | `query-gateway.kucoin.net` | 3 | hostname | 500m / 512Mi | HTTP/gRPC 入口；Classic + UTA `/api` |
| account | `account-book.kucoin.net` | 3 | hostname | 500m / 512Mi | 科目、hop、子账户、逐仓/全仓分账 |
| ledger | `ledger-transfer-freeze-reconciliation.kucoin.net` | 3 | hostname | 1 / 1Gi | 分录、冻结、流水、对账 |
| deposit | `deposit-withdraw.kucoin.net` | 2 | hostname | 500m / 512Mi | 入金 observe / credit |
| withdraw | `deposit-withdraw.kucoin.net` | 2 | hostname | 500m / 512Mi | 出金 freeze / broadcast |
| address | `address.kucoin.net` | 2 | hostname | 250m / 256Mi | 充值地址、地址簿 |
| custody | `custody-boundary-lab.kucoin.net` | 2 | hostname | 500m / 512Mi | 热冷、双控、PKCS#11 / cloud KMS |
| kyt | `kyt.kucoin.net` | 2 | hostname | 250m / 256Mi | 筛查；拨号失败记日志重试 |
| transfer | `ledger-transfer-freeze-reconciliation.kucoin.net` | 与 ledger 同进程或 2 副本 | hostname | 500m / 512Mi | Flex / 内部划转 |
| asset-query | `query-gateway.kucoin.net` | 2 | hostname | 250m / 256Mi | 总览、导出作业 |

同主机名多进程（deposit / withdraw）用注册实例 `name` + `addr` 区分。反亲和：`podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution`，`topologyKey=kubernetes.io/hostname`。

## 中间件（同集群）

| 组件 | 副本 | 私网地址（SG） | 私网地址（TYO） |
| --- | --- | --- | --- |
| etcd | 3 | `10.18.0.10` / `.14` / 第三节点见 compose | `10.19.0.10` 族 |
| Kafka KRaft | 3 | `10.18.0.20`–`10.18.0.22` | `10.19.0.20`–`10.19.0.22` |
| Redis Sentinel | 1 主 2 从 + 3 sentinel | `10.18.0.12`–`10.18.0.17` | `10.19.0.12`–`10.19.0.17` |
| MySQL | 1 主 1 从 | `10.18.0.10:3306` / `10.18.0.13:3307` | `10.19.0.10` / `10.19.0.13` |
| Postgres（可选） | 1 主 1 从 | `10.18.0.4:5432` / `10.18.0.5:5433` | `10.19.0.4` / `10.19.0.5` |

 etcd 缺失失败即关。编排见 `deploy/compose-middleware.yml`。

## 性能约定

- SQL / Redis 连接池：`PoolSettings`（默认 MaxOpen 16 / MaxIdle 8），拨号与查询超时走配置。
- 划转 / 出金 `clientOid` 幂等；账本 outbox 投递 Kafka 主题。
- 热路径 bench：ledger post、account transfer、isolated transfer、deposit credit、withdraw freeze/broadcast、address allocate。

## 清单落点

仓库说明级 YAML：

- `deploy/k8s/etcd-statefulset.yaml` — etcd 3 节点 StatefulSet + headless Service
- `deploy/k8s/industry-services.yaml` — ConfigMap（ETCD_ENDPOINTS）+ gateway Deployment/Service 样例

行业进程（account/ledger/deposit/withdraw/address/custody/kyt）复制 gateway 模板，改 `args` 与 `HA_SERVICE`、资源提示见上表。TYO 命名空间复用同名 Service，靠区域 DNS / 私网解析到 `10.19` 族地址；写权由 `internal/ha.Pair` 围栏裁定。
