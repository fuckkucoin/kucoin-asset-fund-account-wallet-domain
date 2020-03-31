# 入金 Saga 簿持久化与门禁

本文说明入金 `Book` / saga 在装配中的 SQL 附着、拒纯 Mem 与重启装回语义。费率字段本面为 N/A。不涉及出金、划转或门控 Register。

## 装配顺序

1. 运行配置判定：`authn.ProductionProfile`（`Auth.Kind` / `Profile` 为 iam、prod、production）。
2. `Runtime.Mem==false` 且 `Runtime.SQL` 可用时，对 Desk、归因与 **Book** 执行既有 `BindSQL`。
3. Book 执行 `Hydrate`，自 `aa_deposits` 装回 Run。
4. 再对外暴露写路径（`Put` / `Recover`）。

且（`Runtime.Mem` 或 Book 未附着）时返回稳定错误 `refuse production: deposit book bound to in-process mem`，禁止先 Mem 后异步切 SQL。

非（显式 mem / 开发档）允许纯进程内 Book。

## 状态机与落库

`deposit.Step`：`observed` → `accepted` → `confirmed` → `credited`；`held` 可回 `accepted` 或 `unwound`；`credited` 仅可 `unwound`。非法边由既有 `Advance` 拒绝，落库 `step` 不得改写。

未终态（`accepted` / `confirmed` / `held`）进入 `Pending` 并由 `Recover` 推进。终态（`credited` / `unwound` / `failed`）可按自然键读取，不得入队。

## 自然键

认领与 Saga 行唯一键为 `(chain, tx_hash, output_index)`。同一交易多输出必须分行，禁止退回仅 `(chain, tx_hash)`。

## 健康面

`Book.Health` 与 `Desk.Health` 并列：未绑定或 Hydrate/装配失败则 `ok=false`，不得对外报健康可写。

## 失败语义

| 条件 | 结果 |
|------|------|
| + 未 BindSQL / `Runtime.Mem` | 装配与 `Put`/`Recover` 失败，错误码稳定 |
| Hydrate 读库失败 | 健康面记录错误，不得假装空簿 |
| 同键异用户 Claim | `ErrTaken` |
| 同键同用户 Claim | 幂等回放，不双贷 |
| 非法 Advance | 拒绝；库内 Step 不变 |
