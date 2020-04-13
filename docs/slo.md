# Design targets

Process design targets for account and asset services. Numbers are enforcement knobs, not measured SLAs.

| Area | Target | How it is enforced |
| --- | --- | --- |
| Wallet-core availability | 99.95% HA aim | `/healthz` liveness, `/readyz` dependency checks, graceful degrade when a chain RPC is down, retries with backoff, node circuit + failover |
| Multi-chain balance / history | p99 ≤ 500ms | Redis cache, parallel chain fan-out, per-RPC timeout, circuit/failover; histogram `aa_wallet_query_seconds` |
| Spot ↔ wallet + swap | No silent book drift | Reconcile job compares books vs in-flight freeze/credit/swap journals; mismatches are audited and block new movements until clean |
| Wallet market ticker / 1m K-line | Sub-second push; stale/error ≤ 0.5% | Same spot feed as swap quotes: bus → memory/Redis → WebSocket. Lag histogram + stale drop counters |
| Chain tx submit | ≥ 99.9% success | Prefer healthy low-latency endpoints; open circuit on consecutive failures; half-open probe; retry/failover. `aa_chain_rpc_total` / `aa_chain_submit_total` |
| Split services | Alert-friendly localization | `api` / `walletd` / `marketd` / `worker`, Prometheus `/metrics`, `GET /v1/ops/alerts`, `deploy/alerts.yml`, structured logs, `X-Trace-Id` |

Fund-safety rules (ledger + swap): idempotency with payload match, freeze-before-move, no negative balances, 18-decimal precision, dual-side state machine, rollback on chain failure, daily/single/velocity caps, anomaly flags, audit trail.

Spot ↔ wallet books vs open orders: `GET /v1/ledger/statement` (unified balances + open transfers/swaps + `books_match`). Live process rates: `GET /v1/slo` `live` object.

Observability: `aa_reconcile_mismatches`, `aa_frozen_stuck_total`, `aa_idempotency_conflicts_total`, `aa_quote_rejected_total`, `aa_chain_submit_deduped_total`, `aa_chain_rpc_total` / `aa_chain_submit_total`. Rule file: [`../deploy/alerts.yml`](../deploy/alerts.yml).
