# 对账单与历史导出链路

## 范围

用户侧对账单查询、JSON/CSV 导出、异步导出任务、日限额与时间窗。不覆盖法币/P2P 邻接轨与充提核心状态机。

## 限额

- 单次窗口最长 100 天
- 每用户每日最多 5 次（UTC 自然日）
- 计数合并内存与 KV，进程重启不得清零绕过

## 任务状态

`queued` → `running` → `done` | `failed` | `expired`

## 校验

导出正文 checksum（SHA-256）必须可重算；页脚借贷合计须能通过账本金额解析。

## 主机

查询与导出网关：`query-gateway.kucoin.net`；内部任务调度落 SG `10.18.0.0/16`，TYO 备 `10.19.0.0/16`。

## 过期与正文闸

- 入队与查询路径调用 `ExpireStale`：queued/running 超过 TTL（默认 24h）推进为 `expired` 并清空 Body。
- `GetBody` 仅 `done` 可取，校验 owner 与 SHA-256 checksum。
- 元数据表 `aa_export_job` / 审计表 `aa_export_audit`；正文不落元数据表。
- 区域：查询网关 `query-gateway.kucoin.net`（SG `10.18` / TYO `10.19`）。
