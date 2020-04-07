# 服务间 HTTP 调用约定（不走对外 API 网关）

## 默认规则

- **Go ↔ Java 编排/入账通知**：etcd 服务名 + `internal/svcdial` / `internal/platformjava`，HTTP JSON 调用对端进程。
- **不经过** query-gateway 二次转发，**不携带** 终端用户 `Authorization`（除非下文「例外」）。
- Java 调 Go：按 `internal/config/delegate_route.go` 选 **行业主机**（`ledger`、`deposit_withdraw`、`query_gateway` 等），表示「哪台 Go 进程」，不是公司级 API 网关多跳。

## 已落地示例

| 场景 | 调用方 | 发现 | Path |
| --- | --- | --- | --- |
| 链上入账后通知 Java 落平台单 | Go deposit-withdraw | `funding-bff-service` | `POST /internal/v1/deposit-rails/arrivals` |
| 实现 | `internal/depositrails` | `platformjava.New(rt.Dial)` | 见 `platformjava.PathDepositRailsArrivals` |

入账触发点：`deposit.Service` 在 `credited` 后 `emitCredited` → outbox `deposit.credited` + 上述 BFF 回调。

## 允许走「网关层」的例外（特殊情况）

1. **终端用户访问 Classic `/api/*`** 
 流量进入 **query-gateway 进程**（G7 部署名），由该进程挂 `kcfund` 路由。这是 **对外 HTTP 入口**，不是 Go 调 Java 时再绕一层网关。

2. **IAM**（`iam-gateway.kucoin.net`） 
 外部登录/鉴权；服务间回调 **不用** IAM。

3. **query-gateway 代用户读 Java 产品面**（`internal/gateway/product_bff.go`） 
 浏览器/API Key 请求带 `Authorization` 时，query-gateway **透传** 到 `funding-bff-service` 做产品编排。属于 **用户请求代理**，不是 deposit-withdraw 后台回调。

4. **`profile=mem` / 单测** 
 不发出网 HTTP；deposit-rails 回调自动跳过。

5. **`config.AllowOutbound`** 
 安全白名单拦截非法外呼主机，不是业务网关。

6. **delegate 里的 `query_gateway` HostKey** 
 仅表示「Classic 对外 REST 落在哪台 Go 主机」，Java Feign 应对 **目录主机 + path**，勿理解成必须先打统一 API Gateway。

## 命名

- 服务拨号客户端：`fundmesh.Client`（避免与「对关」混淆）。
- Java Feign 名（如 `GoDepositRailsGateway`）在 **Java 仓**；Go 侧只认 **HTTP path + 服务注册名**。
