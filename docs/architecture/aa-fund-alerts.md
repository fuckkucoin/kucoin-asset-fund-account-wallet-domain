# Account & Assets 资金路径告警

资金闸、对账积压、审计缺口，以及写区与备区 CIDR 探针，接入 Prometheus 规则与进程内 SLO 评估。

`deploy/alerts.yml` 与 `internal/slo.Evaluate` 使用同一组名称。`GatherPrometheus` 读取进程内序列。`GET /v1/ops/alerts` 的 `live.fund` 与 `live.region` 带回闸状态与 CIDR 探针。存储故障与 CIDR 失败按 page 升级；未关对账工单按 ticket 升级。

资金闸 `result=store` 不得当成未登记。对账积压不得被存储错误清零。`aa_region_write_ok` 仅在探针调用后才有序列。新加坡写区 `10.18.0.0/16`，东京备区 `10.19.0.0/16`。本规则骨架不是监管报送通道。

## 规则

| 名称 | 序列 | 作用 |
| --- | --- | --- |
| AAFundGateStore | `aa_fund_gates_total{result="store"}` | 白名单/日限额/速度闸读库失败 |
| AAReconBacklog | `aa_recon_backlog` | 开单或已指派未结案 |
| AAAuditGaps | `aa_audit_gaps_total{result!="ok"}` | 审计链缺号、缺 kind、哈希断 |
| AASGWriteCIDR | `aa_region_write_ok{region="sg"}` | 写区 IP 不在 `10.18.0.0/16` |
| AATYOStandbyCIDR | `aa_region_write_ok{region="tyo"}` | 备区 IP 不在 `10.19.0.0/16` |
| AATravelFXStale | `aa_travel_fx_stale_count` / `aa_travel_fx_stale` | 旅行规则 USD 牌价陈旧（超越 fx-health JSON） |

进程内探针：`internal/ha.ProbeSGWrite` / `ProbeTYOStandby` / `ProbeFailover`。
| AATravelFXVendorBurn | `aa_travel_fx_burn_rate` / `aa_travel_fx_vendor_total` | Live FX 厂商错误预算燃烧；可用 fx-severity 调谐 |
- `GET /v1/ops/admin/wd-track-health`：出金 fee-bump/broadcast Hydrate 与 SQL ；落库失败上浮。
| AAWDTrackHealth | `aa_wd_track_ok` / `aa_wd_track_drift` / hydrate|persist err | 出金 fee-bump/broadcast Hydrate 与 SQL |
| AAWDReorgSagaStuck | `aa_wd_reorg_saga_max_age_seconds` ≥ 600 | 重组→重提 saga 软阈 stuck ticket（10m+） |
| AAWDReorgSagaOverTimeout | `aa_wd_reorg_saga_max_age_seconds` ≥ 1800 | 超 DefaultSagaTimeout 仍开放 → page（worker 未推进） |
| AAWDReorgSagaTimeouts | `aa_wd_reorg_saga_timeout_total` increase | TickSagas 超时失败；须人工重开 |
| AATravelFXVendorBurn5m | `aa_travel_fx_burn_rate_window{window="5m"}` | Live FX 5m 短窗燃烧 |

- `GET /v1/ops/admin/wd-track-health`：出金 fee-bump/broadcast Hydrate 与 SQL ；落库失败上浮；推 Prometheus。
- `GET /v1/withdrawals/broadcast/sagas`：开放 reorg→resubmit saga。

| AAEarnProductAudit | `aa_earn_product_audit_write_err` / `aa_earn_product_audit_max_age_seconds` / `aa_earn_product_audit_cold_*` | 理财目录软删审计写失败或超 365d 未清扫；先 cold-archive 再 purge |

- `GET /v1/ops/earn/products/audit?format=csv`：审计 CSV（100 天导出窗 / 365d 保留）。
- `POST /v1/ops/earn/products/audit/purge`：清扫超保留窗审计行。

