# Architecture

Account/asset process split is in [`docs/account-assets-architecture.md`](account-assets-architecture.md). System context, module map, fund flows, and dual-region topology: [`architecture/overview.md`](architecture/overview.md). Fund document state machine: [`architecture/fund-state.md`](architecture/fund-state.md). Subject-to-book map: [`architecture/book-subject.md`](architecture/book-subject.md). This note covers the shared spot + wallet stack.

## Domain rules (spot ↔ wallet)

Custodial spot book and on-chain wallet share freeze-then-move and decimal money.

| Movement | Freeze | Chain | Credit |
| --- | --- | --- | --- |
| `spot_to_wallet` / `wallet_to_spot` | Source book, same user lock | No | Dest book in the **same** DB tx (`internal:{id}` hash) |
| `spot_send` | Sender funding book | No | Counterparty funding book in the same lock pair (ordered user locks) |
| `wallet_withdraw` | Wallet book | Submit + confirm (destination decrypted from at-rest ciphertext) | Consume frozen only; spot unchanged; allowlist + listed asset; EVM family shares bound address; optional withdraw fee to `system.fee` |
| `wallet_deposit` | None | Observe hash + confirm | Wallet available from `system.bridge` |
| Spot CLOB (`POST /v1/spot/orders`) | Funding book | No | Price-time in this process; house backstop for leftover market IOC. **Not a matching-engine cluster.** |
| `fiat_onramp` | None | No | Funding from `system.bridge`. **Not a licensed fiat rail.** |
| `earn_subscribe` / `earn_redeem` | Earn hop | No | Funding ↔ `earn` book; redeem may pay interest from bridge. **Not a money-market product.** |
| `hot_cold_sweep` | None | No | `system.bridge` excess → `system.cold`. **Not an on-chain hot wallet sweeper.** |

Reconcile treats in-flight freeze on the source book for internal/withdraw, and forbids a deposit that froze user inventory. Swap quotes the shared spot cache and settles fees on the wallet book.

EVM L2s share one derived address (`secret:EVM`). `GET /v1/wallet/balances?view=unified` aggregates spot+wallet per asset plus chain natives.

## Layers

```
cmd/ process entry (api, walletd, marketd, worker) — boot, signals, no business rules
internal/app composition root: wires config, store, chain, bus, services
internal/* domain packages (ledger, swap, wallet, market)
internal/httpapi HTTP adapters — domain errors mapped in apierr
internal/grpcsvc gRPC adapters — same mapping
api/proto protobuf contracts
migrations/ versioned MySQL
deploy/ compose overlays
configs/ yaml + env overlay (limits and timeouts live in config)
```

Boundaries are interfaces: `ledger.Store`, `ledger.Locker`, `ledger.ChainBridge` (Submit/Status/Observe/Inspect), `wallet.Adapter`, `wallet.TxInspector`, `wallet.Fork`, `kms.Keeper`, `kyt.Screener`, `bus.Bus`, `swap.Pricer`. `cmd` does not import SQL or Redis drivers.

Account and assets add-ons:

- `internal/kms` PKCS#11 / cloud KMS `Keeper` + `Signer` (secp256k1 ECDSA)
- `internal/authn` bearer / HMAC
- `internal/kyt` + `internal/compliance` fail-closed withdraw screen, case queue, audit
- `internal/custody` hot/cold + dual-control + ceremony state machine
- `internal/opsapi` + `internal/ha` admin freeze/reconcile and leader fencing
- `internal/role` process-split plan
- `internal/cachettl` process L1 TTL
- `internal/conc` bounded For / singleflight Group / Chunk

## Domain vs transport

Domain errors live in `internal/ledger` (`ErrInsufficient`, `ErrPending`, `ErrBusy`, …). `internal/apierr` maps them:

| Domain | HTTP | gRPC |
| --- | --- | --- |
| `ErrPending` | 202 | — |
| `ErrNotFound` | 404 | NotFound |
| `ErrBusy` | 429 | ResourceExhausted |
| idempotency / drift / slippage / rolled_back / submitted_live / duplicate_hash | 409 | FailedPrecondition |
| risk / amount / insufficient / stale quote | 400 | InvalidArgument |

Handlers must not pick status codes by string matching business text.

## Write-path norms

- Idempotent POST: `idempotency_key` required; payload must match the original row.
- Money: `shopspring/decimal` + `ParseAmount` / `ParseRatio` / `ParseBook`. No float on books (mock ticker jitter uses integer bps).
- Explicit state machines (`ledger.Apply`); illegal transitions wrap `ErrIllegalTransition`.
- Write RPCs take `context.WithTimeout` (`http_write_timeout`, default 8s). Chain RPC has its own budget.
- Structured logs (`zerolog`) include `trace_id` from `X-Trace-Id`. Prometheus names: `aa_<subsystem>_<name>[_total|_seconds]`.

## Go style in this repo

- Small files by concern (`httpapi/{catalog,wallet,ledger,swap,market,respond,ops}.go`, `swap/{types,pricer,store,service}.go`, `ledger/{errors,lock,amount,…}`).
- Table-driven tests for parsers, state machine, HTTP status mapping.
- No `panic` on the request path (scan/parse return errors). Process boot uses `log.Fatal`, not panic.
- Comments describe invariants, not chronology. No dates in comments.

Package-level Prometheus `var`s and the Redis unlock Lua script are the documented exceptions to “no globals.”
