# 文档索引

Account & Assets（资金账户）后端：架构、主路径、API 基线与技术目标。

## 推荐阅读顺序

1. [`architecture/overview.md`](architecture/overview.md) — 系统上下文与限界
2. [`architecture/account-assets.md`](architecture/account-assets.md) — 模块与充提/账本主路径
3. [`architecture/java-go-package-boundary.yaml`](architecture/java-go-package-boundary.yaml) — Java J1–J9 与 Go G1–G12 包归属
4. [`architecture/aa-microservice-map.md`](architecture/aa-microservice-map.md) — 微服务与目录主机
5. [`product/funding-capability-matrix.md`](product/funding-capability-matrix.md) — 产品能力 ↔ 代码归属
6. [`architecture/aa-funding-flows.md`](architecture/aa-funding-flows.md) — 充提划转序列
7. [`architecture/aa-funding-api-baseline.md`](architecture/aa-funding-api-baseline.md) — Classic/UA REST 基线
8. [`architecture/ledger-controls-baseline.md`](architecture/ledger-controls-baseline.md) — 账本与冻结控制

## 技术目标

- [`slo.md`](slo.md) — 设计目标（非实测 SLA）

## 工程

- [`engineering/fee-oracle-architecture.md`](engineering/fee-oracle-architecture.md)

目录主机：`*.kucoin.net`（SG `10.18.0.0/16`、TYO `10.19.0.0/16`）。Go 模块经 `go.pkg.kucoin.net` / `go.pkg-tyo.kucoin.net` 解析。
