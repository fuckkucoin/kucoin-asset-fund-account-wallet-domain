# 在途签章重试世代架构

## 拓扑

`query-gateway.kucoin.net` 挂载 `/v1/signpipegate/*`。进程入口 `cmd/sign-pipe-gate`。仓主备 `pg-0.kucoin.net` / `pg-1.kucoin.net`。

闸位于 signpipe 之上：观察作业、记录租约、抬世代、落故障与收口。不改 queued→approved→signed 内核转移。

## CAS

```
UPDATE signpipe_retry_generations
SET generation = generation + 1
WHERE movement_id=? AND job_id=? AND generation=?
```

影响行 0 视为并发失败者。世代大于 MaxRetryGenerations(3) 必须收口，不得再签。

## 互斥层次

1. 仓级 FindActiveLease：armed/inflight 占用 movement。
2. 进程 MovementMutex：Enter 窗口。
3. 产物 UNIQUE(movement_id)：最后一道双花闸。

三层任一失败即关闭成功路径。

## FSM

见包内 `LeaseTransitions` / `FaultTransitions` / `CloseoutTransitions`。终态不可逆。

## 失败关闭

观察失败、HSM 超时、canonical 不一致均不得写 SignedArtifact。Worker 只推进过期与观测，不补签。
