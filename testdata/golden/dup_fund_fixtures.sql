-- 资金副作用恰好一次引擎 —  SQL 黄金夹具。
-- 用于 CI 集成环境（需配置 TEST_DSN）；
-- 数据仅用于读取验证，不写入库。
-- 作者：Knox
-- 时间列使用固定值（2026-02-01 UTC），确保确定性。

-- ────────────────────────────────────────────────────────────────────────────
-- fx_fund_effects 夹具
-- ────────────────────────────────────────────────────────────────────────────

-- 场景 1: 正常 applied effect（gen=1）
INSERT INTO fx_fund_effects
  (id, business_key, kind, generation, effect_key, state, payload, applied_at, reject_reason, poison_retries, created_at, updated_at)
VALUES
  ('fx-eff-0001', 'dep-golden-001', 'credit_intent', 1,
   'a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6',  -- MakeEffectKey('dep-golden-001', 1)
   'applied',
   '{"amount":"100.00","asset":"USDT","user_id":"u-001"}',
   '2026-02-01 00:00:01.000',
   '', 0,
   '2026-02-01 00:00:00.000',
   '2026-02-01 00:00:01.000');

-- 场景 2: Leader 切换后旧世代被 rejected
INSERT INTO fx_fund_effects
  (id, business_key, kind, generation, effect_key, state, payload, rejected_at, reject_reason, poison_retries, created_at, updated_at)
VALUES
  ('fx-eff-0002', 'dep-golden-002', 'credit_intent', 2,
   'b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7',
   'applied',
   '{"amount":"200.00","asset":"BTC","user_id":"u-002"}',
   NULL, '', 0,
   '2026-02-01 00:00:00.000',
   '2026-02-01 00:00:02.000'),
  ('fx-eff-0003', 'dep-golden-002', 'credit_intent', 1,
   'c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8',
   'rejected',
   '{"amount":"200.00","asset":"BTC","user_id":"u-002"}',
   '2026-02-01 00:00:03.000', 'stale_gen', 0,
   '2026-02-01 00:00:00.000',
   '2026-02-01 00:00:03.000');

-- 场景 4: 毒丸隔离 effect
INSERT INTO fx_fund_effects
  (id, business_key, kind, generation, effect_key, state, payload, rejected_at, reject_reason, poison_retries, created_at, updated_at)
VALUES
  ('fx-eff-0004', 'dep-golden-003', 'freeze_intent', 1,
   'd4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9',
   'poisoned',
   '{"amount":"50.00","asset":"ETH","user_id":"u-003"}',
   '2026-02-01 00:10:00.000', 'handler_error', 5,
   '2026-02-01 00:00:00.000',
   '2026-02-01 00:10:00.000');

-- ────────────────────────────────────────────────────────────────────────────
-- fx_delivery_gens 夹具
-- ────────────────────────────────────────────────────────────────────────────

-- gen=1 acked（对应 fx-eff-0001）
INSERT INTO fx_delivery_gens
  (id, business_key, kind, expected_gen, applied_gen, state, retry_count, max_retry, reject_reason, effect_id, outbox_event_id, acked_at, created_at, updated_at)
VALUES
  ('fx-gen-0001', 'dep-golden-001', 'credit_intent', 1, 1, 'acked', 0, 5, '',
   'fx-eff-0001', 'outbox-evt-0001',
   '2026-02-01 00:00:01.000',
   '2026-02-01 00:00:00.000',
   '2026-02-01 00:00:01.000');

-- gen=2 acked（对应 fx-eff-0002 — 胜者）
INSERT INTO fx_delivery_gens
  (id, business_key, kind, expected_gen, applied_gen, state, retry_count, max_retry, reject_reason, effect_id, outbox_event_id, acked_at, created_at, updated_at)
VALUES
  ('fx-gen-0002', 'dep-golden-002', 'credit_intent', 2, 2, 'acked', 0, 5, '',
   'fx-eff-0002', 'outbox-evt-0002',
   '2026-02-01 00:00:02.000',
   '2026-02-01 00:00:00.000',
   '2026-02-01 00:00:02.000');

-- gen=1 nacked（对应 fx-eff-0003 — 败者）
INSERT INTO fx_delivery_gens
  (id, business_key, kind, expected_gen, applied_gen, state, retry_count, max_retry, reject_reason, effect_id, outbox_event_id, nacked_at, created_at, updated_at)
VALUES
  ('fx-gen-0003', 'dep-golden-002', 'credit_intent', 1, 0, 'nacked', 1, 5, 'stale_gen',
   'fx-eff-0003', 'outbox-evt-0003',
   '2026-02-01 00:00:03.000',
   '2026-02-01 00:00:00.000',
   '2026-02-01 00:00:03.000');

-- gen=1 poisoned（对应 fx-eff-0004 — 毒丸）
INSERT INTO fx_delivery_gens
  (id, business_key, kind, expected_gen, applied_gen, state, retry_count, max_retry, reject_reason, effect_id, outbox_event_id, nacked_at, created_at, updated_at)
VALUES
  ('fx-gen-0004', 'dep-golden-003', 'freeze_intent', 1, 0, 'poisoned', 5, 5, 'handler_error',
   'fx-eff-0004', 'outbox-evt-0004',
   '2026-02-01 00:10:00.000',
   '2026-02-01 00:00:00.000',
   '2026-02-01 00:10:00.000');

-- ────────────────────────────────────────────────────────────────────────────
-- fx_dup_proofs 夹具
-- ────────────────────────────────────────────────────────────────────────────

-- dep-golden-002: gen=1（败） vs gen=2（胜）
INSERT INTO fx_dup_proofs
  (id, business_key, kind, winner_gen, loser_gen,
   winner_effect_id, loser_effect_id,
   winner_delivery_id, loser_delivery_id,
   winner_at, loser_at, window_ms,
   reject_code, reviewed, reviewed_by, reviewed_at, created_at)
VALUES
  ('fx-dup-0001', 'dep-golden-002', 'credit_intent', 2, 1,
   'fx-eff-0002', 'fx-eff-0003',
   'fx-gen-0002', 'fx-gen-0003',
   '2026-02-01 00:00:02.000',
   '2026-02-01 00:00:03.000',
   1000,
   'stale_gen', 0, '', NULL,
   '2026-02-01 00:00:03.000');

-- dep-golden-004: 已审阅的 DupProof（desk 已处理）
INSERT INTO fx_dup_proofs
  (id, business_key, kind, winner_gen, loser_gen,
   winner_effect_id, loser_effect_id,
   winner_delivery_id, loser_delivery_id,
   winner_at, loser_at, window_ms,
   reject_code, reviewed, reviewed_by, reviewed_at, created_at)
VALUES
  ('fx-dup-0002', 'dep-golden-004', 'hop_intent', 3, 1,
   'fx-eff-0010', 'fx-eff-0011',
   'fx-gen-0010', 'fx-gen-0011',
   '2026-02-01 01:00:00.000',
   '2026-02-01 01:00:05.000',
   5000,
   'stale_gen', 1, 'desk-operator-alice', '2026-02-01 02:00:00.000',
   '2026-02-01 01:00:05.000');

-- dep-golden-005: 乱序投递（loser 先到 winner 后到，window_ms < 0）
INSERT INTO fx_dup_proofs
  (id, business_key, kind, winner_gen, loser_gen,
   winner_effect_id, loser_effect_id,
   winner_delivery_id, loser_delivery_id,
   winner_at, loser_at, window_ms,
   reject_code, reviewed, reviewed_by, reviewed_at, created_at)
VALUES
  ('fx-dup-0003', 'dep-golden-005', 'notify_fund', 5, 7,
   'fx-eff-0020', 'fx-eff-0021',
   'fx-gen-0020', 'fx-gen-0021',
   '2026-02-01 02:00:10.000',
   '2026-02-01 02:00:08.000',
   -2000,
   'stale_gen', 0, '', NULL,
   '2026-02-01 02:00:10.000');

-- ────────────────────────────────────────────────────────────────────────────
-- fx_kind_policies 夹具（覆盖默认策略）
-- ────────────────────────────────────────────────────────────────────────────

-- 验证默认资金 kind 策略已存在
SELECT kind, gate_required, blocked, max_retry
FROM fx_kind_policies
WHERE kind IN ('credit_intent','freeze_intent','hop_intent','notify_fund','debit_intent','settle_intent')
ORDER BY kind;

-- 验证 fx_dup_proofs 未审阅计数
SELECT COUNT(*) AS unreviewed_count FROM fx_dup_proofs WHERE reviewed = 0;

-- 验证 dep-golden-002 只有一个 applied effect
SELECT COUNT(*) AS applied_count FROM fx_fund_effects
WHERE business_key = 'dep-golden-002' AND state = 'applied';

-- 验证 dep-golden-001 的 effect_key 格式（32位小写hex）
SELECT LENGTH(effect_key) AS key_len,
       effect_key REGEXP '^[0-9a-f]{32}$' AS is_valid_hex
FROM fx_fund_effects
WHERE id = 'fx-eff-0001';
