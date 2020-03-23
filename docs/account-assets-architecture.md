# Account & assets process map

Books, freeze-then-move, and KYT fail-closed are encoded here.

Internal endpoints (`*.kucoin.net` only):

| Process | Endpoint | Role |
| --- | --- | --- |
| account-service | https://account-book.kucoin.net | funding / trading / margin / futures / earn lifecycle + book hops |
| ledger-service | https://ledger-transfer-freeze-reconciliation.kucoin.net | transfers, user freeze, journal, reconcile; no negative balances |
| deposit-service + withdraw-service | https://deposit-withdraw.kucoin.net | inbound observe→confirm→credit; outbound freeze→allowlist→KYT→sign→submit |
| address-service | https://address.kucoin.net | deposit address derive + withdraw address book |
| custody-service | https://custody-boundary-lab.kucoin.net | hot (`system.bridge`) / cold (`system.cold`); PKCS#11 or cloud KMS; dual-control |
| kyt-service | https://kyt.kucoin.net | allow / review / deny, case queue, audit trail, policy hooks |
| query-gateway | https://query-gateway.kucoin.net | unified balances + bills; Bearer from `configs/app.yaml` |

Infra aliases used by `config.Load`: MySQL `mysql-0.kucoin.net`, Redis `redis-0.kucoin.net`, Kafka `kafka-0.kucoin.net`. Registry `etcd-0.kucoin.net`. KMS catalog host `kms-vault.kucoin.net`, IAM `iam-gateway.kucoin.net`.

## Packages

| Package | Responsibility |
| --- | --- |
| `pkg/money` | DECIMAL(36,18) parse; refuse scientific notation / negative available; journal balance check |
| `internal/policy` | Append-only policy versions; withdraw/transfer/deposit gates stamped on the command |
| `internal/perm` | Role grants; `X-Actor` / `X-Role`; custody sign, ceremony, KYT decide, ops admin are not default-user |
| `internal/command` | Idempotency log + outbox/inbox once; payload hash mismatch → conflict |
| `internal/cases` | KYT review queue; maker-checker decide; expire stale; append-only audit |
| `internal/compliance` | Fail-closed withdraw gate: rules engine → KYT → case + trail |
| `internal/account` | funding/trading/margin/futures/earn lifecycle + book hops |
| `internal/deposit` | Observe (chain amount) → confirm → reorg unwind; saga recover |
| `internal/withdraw` | Freeze → allowlist → compliance gate → KYT → sign → submit; expired review cannot confirm |
| `internal/ledgersvc` | Transfers, user freeze, journal, reconcile job, KYT ops decide |
| `internal/opsapi` | Admin freeze / reconcile triggers; `/v1/ops/ha`; leader-fenced writes |
| `internal/addresssvc` | Deposit derive + withdraw allowlist |
| `internal/custody` | Hot/cold inventory, dual-control approvals, PKCS#11 / cloud KMS sign, ceremony state machine |
| `internal/ha` | Redis/mem SET NX leader + fence epoch; shared by ops admin |
| `internal/assetquery` | Unified books + bills + running statement |
| `internal/usage` | Daily/velocity occupancy for policy gates |
| `internal/addrpool` | Deposit address bind, EVM share, reuse cooldown |
| `internal/stmt` | Running-balance statement plus JSON/CSV export and period/batch builders |
| `internal/limitgate` | Policy + usage check before freeze |
| `internal/acctjob` | Leader-fenced recover / expire / reconcile / sweep |
| `internal/mw` | Write timeout + actor gate |
| `internal/snap` | Point-in-time book snapshot + checksum + live diff |
| `internal/feeeng` | Withdraw/deposit/hop fee quote; fee+net = gross; volume tiers |
| `internal/earn` | Flexible/locked earn products, daily accrual, subscribe/redeem hops |
| `internal/marginhook` | Collateral/borrow freeze hooks + LTV cap (no liquidation engine) |
| `internal/notify` | Outbox consumers, retry, dead-letter; known event kinds only |
| `internal/investigate` | Ops case + timeline + book tracer for a single user |

## Sequences

### Book hop (funding → trading)

1. `POST /v1/accounts/open` with `book=trading` (funding is already open).
2. `POST /v1/accounts/transfers` with `idempotency_key`. Frozen or closed dest rejects.
3. ledger freezes funding available, hops `system.bridge`, credits trading. Same user lock, no chain RPC.
4. Replay of the same key returns the original row.

### Deposit

1. Chain `Inspect` runs **outside** the DB tx. Credit amount is the observed quantity, not the caller’s guess.
2. `wallet_deposit` insert; unique `(chain, tx_hash)` — second hash is `ErrDuplicateHash`.
3. Confirm depth per chain (ETH 12 / BTC 3 / L2 20+). Credit wallet from `system.bridge`.
4. Reorg on a credited row: unwind journal (wallet debit, bridge credit), state `reorged`.

### Withdraw (fail-closed)

1. Freeze wallet available (after idempotency begin).
2. Allowlist + decrypt dest (ciphertext at rest).
3. `compliance.GateWithdraw`: policy / sanction / velocity hooks, then KYT. Deny or screener error → no `Submit`. Review opens a case and keeps the freeze.
4. File-backed KMS `SignWithdraw` then `Submit` (RPC still outside the DB tx). Persist hash before `chain_submitted`.
5. Confirm consumes frozen only; spot/funding unchanged.
6. Stale review cases expire; expired is not allow. Recover will not confirm an expired case.

### Custody dual-control

1. Custody actor A opens an approval (`sweep` or `sign`).
2. A different custody actor B approves.
3. Execute sweep/sign; requester cannot execute their own request.
4. Ceremony `init → generate → split → combine → activate` needs a second actor on activate. Records are labeled `ceremony`.

## Fund-safety invariants (kept)

- Decimal money (`shopspring/decimal`, `DECIMAL(36,18)`); no float on books.
- Idempotency key + payload hash; conflict is 409.
- Freeze-then-move; user lock via Redis `SET NX` or in-process mutex.
- Chain RPC (observe / submit / status) stays outside the DB transaction.
- Deposit credit from observation, not the request amount.
- Reorg unwind reverses a credited journal.
- KYT / compliance error or deny is fail-closed (no broadcast).
- Row mutations carry `fence_epoch` / `fence_token`; stale workers cannot unfreeze.

## Delivery boundary

| Area | In this tree | Outside this tree |
| --- | --- | --- |
| Account books | funding/trading/margin/futures/earn lifecycle + hops | Separate risk/liquidation; no empty futures catalog |
| Ledger | freeze/unfreeze, journal, reconcile, no-negative, fencing | Dedicated DB per service, blast-radius limits, signed-off SOX/audit |
| Deposit / withdraw | Observe→credit, unique hash, reorg unwind, withdraw saga | Chain adapters, confirmation policy per asset, indexer SLA |
| Compliance / KYT | Case queue, maker-checker, audit trail, fail-closed gate, rules engine | Licensed program, vendor SLA, case tooling, independent audit |
| Custody | Hot/cold books, dual-control, PKCS#11 or cloud KMS ECDSA, ceremony state machine | Independent key domains, officer ceremony, certification |
| Ops | Prometheus, traces, `/readyz`, admin freeze/reconcile, runbook | On-call, paging, change management, customer comms |
| Multi-active | Dual-region compose, leader election, fence epoch | Runbook-signed DR, split-brain tests, RPO/RTO evidence |
| Auth | Process bearer / HMAC, `X-Role` | SSO, scoped service tokens, break-glass |

`cmd/api` still mounts the same HTTP routes for a single-process checkout. Compose profile `apps` runs the split binaries. Profiles `region-a` / `region-b` add a second process copy on published ports **without** inventing new catalog hostnames.
