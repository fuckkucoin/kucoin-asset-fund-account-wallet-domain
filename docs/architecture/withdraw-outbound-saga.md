# 提现出站 Saga 持久化与失败封闭

本文说明出金 `withdraw.Book` 在装配中的 SQL 附着、拒纯内存状态机、按自然键续跑与失败封闭语义。不涉及入金 Book、门控 Register 或其他字母面。

表定义见 `migrations/0222_aa_wd_outbound_saga.sql`（避开 main 既有 `0139_aa_scoreleasegate`）。

## 装配顺序

1. 运行配置判定：`authn.ProductionProfile`（`Auth.Kind` / `Profile` 为 iam、prod、production）。
2. `Runtime.Mem==false` 且 `Runtime.SQL` 可用时，对广播跟踪、抬价与 **出站 Book** 执行既有 `BindSQL`。
3. Book 执行 `Hydrate`，自 `aa_wd_outbound` 按 `transfer_id` 装回 Run。
4. 再对外暴露写路径（`PutErr` / `Request` / `Recover`）。

且（`Runtime.Mem` 或 Book 未附着）时返回稳定错误 `refuse production: withdraw outbound book bound to in-process mem`，在装配或首次写入前失败即关，禁止先内存后异步切 SQL。

非（显式 mem / 开发档）允许纯进程内 Book。

## 状态机

`withdraw.Step`：`requested` → `frozen` → `allowlisted` → `decrypted` → `kyt` →（`signed` | `review`）→ `submitted` → `confirmed`。任意未终态可进 `failed`。

非法边由既有 `Advance` 拒绝，落库 `step` 不得改写。已 `fail_sealed` 禁止再记 `submitted` / `confirmed`。

未终态（冻账至已提交）进入 `Pending` / `StuckFrozen`，由 `Recover` 按自然键续跑。终态（`confirmed` / `failed`）可按 `transfer_id` 读取，不得当成功重入扣款。

```
requested ──► frozen ──► allowlisted ──► decrypted ──► kyt
 │
 signed ◄───┴───► review
 │ │
 └──► submitted ──► confirmed
任意未终态 ──► failed（fail_seal）
frozen…signed 失败：compensate=unfreeze_required
submitted 失败：compensate=submitted_live（不得静默解冻）
```

## 自然键与幂等

出站 saga 行唯一键为 `transfer_id`。业务幂等键 `idem` 唯一：同键回放原单，异 `transfer_id` 冲突。`debit_once` 置位后，重启重入不得再冻一笔。

## 失败封闭

| 条件 | 结果 |
|------|------|
| 步骤失败 | `FailSeal` 写 `failed` + `fail_sealed`，禁止静默成功 |
| 已冻未广播 | 封闭时记 `unfreeze_required`，须补偿解冻 |
| 已广播 | 记 `submitted_live`，禁止当取消成功 |
| 已确认 | 拒绝失败封闭冒充失败成功 |
| 补偿未先封闭 | 拒绝 |
| + 未 BindSQL / `Runtime.Mem` | 装配与 `Put`/`Recover`/`Request` 失败 |
| Hydrate 读库失败 | 健康面记录错误，不得假装空簿 |
| 同 idem 异 transfer | 冲突，不双扣 |

## 健康面

`Book.Health` 与广播跟踪健康并列：未绑定或 Hydrate/装配失败则 `ok=false`，不得对外报健康可写。
