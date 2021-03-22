-- 事故回放夹具：签发后贷记抬 Available。不改 por / stmt 表。
-- 作者：Capson
-- 日期：2021.01.20

INSERT INTO balproof_generations (id, user_id, book, asset, generation, updated_at)
VALUES ('g-r14-001', 'u-r14-a001', 'main', 'USDT', 1, '2026-04-28 10:00:00.000');

INSERT INTO balproof_leases (
  id, user_id, book, asset, nonce, generation, status,
  available, frozen, total, observe_gen, root_digest,
  canonical, signature, issued_at, bound_at, expires_at, isolation_mark
) VALUES (
  'lease-r14-credit', 'u-r14-a001', 'main', 'USDT', 'n-credit-001', 1, 'bound',
  100, 0, 100, 1, '00placeholderdigest00placeholderdigest00placeholderdigest0000',
  'canon-placeholder', 'sig-placeholder',
  '2026-04-28 10:00:00.000', '2026-04-28 10:00:00.100', '2026-04-28 10:15:00.000',
  'balproof-live-hmac;not-por-merkle;not-stmt-manifest;not-leddiff'
);

-- 期望：复核时 live available=150 落 proven drift，consume 不得成功。
INSERT INTO balproof_drift_proofs (
  id, lease_id, user_id, book, asset, kind, status,
  issued_avail, issued_frozen, issued_observe_gen,
  live_avail, live_frozen, live_observe_gen,
  digest, opened_at, proven_at, isolation_mark
) VALUES (
  'drift-r14-credit', 'lease-r14-credit', 'u-r14-a001', 'main', 'USDT', 'avail_credit', 'proven',
  100, 0, 1, 150, 0, 2,
  'drift-digest-placeholder', '2026-04-28 10:01:00.000', '2026-04-28 10:01:00.010',
  'balproof-live-hmac;not-por-merkle;not-stmt-manifest;not-leddiff'
);
