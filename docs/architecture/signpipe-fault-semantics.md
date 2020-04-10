# 签章故障注入失败语义

## 必须失败的种类

目录见 `FaultCatalog`。每一条 `MustFail=true`：注入路径不得返回成功签名。

- signer： signer 失败，作业应停在 failed。
- expire：时钟越过 TTL，租约 expired。
- fence：旧 epoch/token，ErrFenceStale。
- actor：开单/复核/执行重叠，ErrDualActor。
- hsm_timeout：管线不可用。
- canonical_mismatch：规范串不一致。
- dual_broadcast：已有产物，双花禁止。
- retry_exhausted：世代耗尽。
- generation_stale / inflight_race：并发互斥失败者。

## 挡写

`BlocksRetry` 种类开立 CloseoutProof 后，同 movement 新 Arm 失败，直至运维关闭收口与故障案。

## 与资金的关系

本包不借记账本。唯一资金不变量：同一 movement 最多一份密封签名。第二次 Seal 必须失败并开 double_guard。
