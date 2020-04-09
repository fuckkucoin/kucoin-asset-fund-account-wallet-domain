# 门控 Register： SQL 与 RefuseProduction

作者：Likeo
日期：2020.04.08

## 装配

档（`APP_PROFILE=prod|production` 或 `AUTH_KIND=iam`）下，`screenleasegate` / `holdopsgate` / `signpipegate` 的 HTTP `Register` 必须经 `BindStore` 绑定既有租约表（列探测见 `0144_aa_gate_register_probe`）：

- `risk_screen_leases`（`expires_at`、`user_id`）
- `hold_ops_leases`（`expires_at`、`actor`）
- `signpipe_inflight_leases`（`expires_at`、`actor`）

`STORAGE` / `GATE_STORAGE=mem` 或运行时仍绑定进程内 Mem 时，`persist.RefuseMemRegister` 返回稳定错误 `refuse production: register bound to in-process mem`，在 `app.RefuseProduction` 或首次 `Register` 前失败即关。禁止先 Mem 再异步切 SQL。

非（`profile=mem|dev`）允许 Mem，供进程内表驱动用例。

## 租约语义

`persist.Leases` 与 `persist.TableRegister`：`Register(key,holder,ttl)` 同键不同持有者且未过期为冲突；`Get` / `ListActive` 过滤过期；`Renew` 仅当前 holder；`Release` 删行。共享表夹具的二次 `Open` 重启重载。

缺列探测：`ProbeLeaseColumns` 现行迁移列名，缺 `expires_at` 等必要列则失败（不另起表族，不引入 `internal/gate/register`）。

## 进程内验收

见 `internal/persist/gaterun_test.go`、`leasecols_test.go` 与各 `*gate/bindstore_test.go`（A-T01–A-T10）。
