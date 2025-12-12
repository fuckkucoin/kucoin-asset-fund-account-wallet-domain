-- viptiergate 夹具 SQL——可复现档位漂移事故轨迹与期望证明摘要。
--
-- 此文件包含三个主要场景的基线数据：
--   Scenario A: 在途冻额 + 抬档 → 漂移检测
--   Scenario B: 并发 RecordVolume + QuoteWithdraw → 旧世代 Quote 拒结算
--   Scenario C: feecredit path_lease_id 隔离验证（只读引用，不写 fee_path_leases）
--
-- 用于集成时预置数据库初始状态；
-- 各表字段与 migrations/0101_aa_viptiergate.sql 定义一致。
--
-- 作者：KianFord
-- 日期：2021.01.01

-- ─────────────────────────────────────────────────────────────────────────────
-- Scenario A：在途冻额 + 抬档 → 漂移检测
-- ─────────────────────────────────────────────────────────────────────────────

-- 世代状态：世代 2
INSERT INTO vip_tier_generations (id, user_id, generation, last_tier, updated_at)
VALUES ('gen-sa-001', 'u-fixture-A', 2, 'vip', NOW(3))
ON DUPLICATE KEY UPDATE generation=2, last_tier='vip';

-- 活跃在途冻额（旧世代 gen=2，抬档后世代变为 3，冻额世代不匹配）
INSERT INTO vip_inflight_freezes (id, user_id, asset, amount, bps, tier, generation, created_at, released_at)
VALUES ('frz-fixture-A-001', 'u-fixture-A', 'BTC', 0.5, 5, 'vip', 2, NOW(3), NULL);

-- 抬档租约（世代 2→3，trigger=pin）
INSERT INTO vip_tier_leases
  (id, user_id, from_tier, to_tier, generation, trigger_kind, trigger_ref, status, created_at, updated_at)
VALUES
  ('lease-fixture-A-001', 'u-fixture-A', 'vip', 'vip+', 3, 'pin', 'u-fixture-A|pin|20260921100000', 'bound', NOW(3), NOW(3));

-- 期望漂移案（由 DriftPatrol worker 在扫描后开立）
INSERT INTO vip_drift_cases
  (id, user_id, asset, kind, observed_tier, expected_tier, observed_bps, expected_bps,
   amount, lease_id, generation, status, note, detected_at, updated_at)
VALUES
  ('dc-fixture-A-001', 'u-fixture-A', 'BTC', 'freeze_stale',
   'vip', 'vip+', 5, 2,
   0.5, 'lease-fixture-A-001', 3, 'open',
   'freeze_id=frz-fixture-A-001 has stale generation 2 vs current 3',
   NOW(3), NOW(3));

-- ─────────────────────────────────────────────────────────────────────────────
-- Scenario B：旧世代 QuoteBind 拒结算
-- ─────────────────────────────────────────────────────────────────────────────

-- 世代状态：世代 8（已从 7 切换）
INSERT INTO vip_tier_generations (id, user_id, generation, last_tier, updated_at)
VALUES ('gen-sb-001', 'u-fixture-B', 8, 'vip', NOW(3))
ON DUPLICATE KEY UPDATE generation=8, last_tier='vip';

-- 旧世代 QuoteBind（gen=7，已被 VoidAll 作废）
INSERT INTO vip_quote_binds
  (id, user_id, asset, gross, fee, net, bps, tier, generation, status, expires_at, voided_at, created_at)
VALUES
  ('qb-fixture-B-001', 'u-fixture-B', 'ETH', 5.0, 0.02, 4.98, 4, 'vip+', 7, 'voided',
   DATE_ADD(NOW(3), INTERVAL 5 MINUTE), NOW(3), NOW(3));

-- 抬档租约（世代 7→8，trigger=clear_pin）
INSERT INTO vip_tier_leases
  (id, user_id, from_tier, to_tier, generation, trigger_kind, trigger_ref, status, created_at, updated_at)
VALUES
  ('lease-fixture-B-001', 'u-fixture-B', 'vip+', 'vip', 8, 'clear_pin', 'u-fixture-B|clear_pin|ops1', 'switched',
   NOW(3), NOW(3));

-- 自动开立的漂移案（SettleQuote 时世代不符触发）
INSERT INTO vip_drift_cases
  (id, user_id, asset, kind, observed_tier, expected_tier, observed_bps, expected_bps,
   amount, quote_bind_id, lease_id, generation, status, note, detected_at, updated_at)
VALUES
  ('dc-fixture-B-001', 'u-fixture-B', 'ETH', 'quote_gen_stale',
   'vip+', 'vip', 4, 0,
   5.0, 'qb-fixture-B-001', 'lease-fixture-B-001', 8, 'open',
   'auto-opened by SettleQuote: quote_gen=7 current_gen=8',
   NOW(3), NOW(3));

-- ─────────────────────────────────────────────────────────────────────────────
-- Scenario C：feecredit 隔离验证
-- （viptiergate 仅生成 tier_lease_id；不写 fee_path_leases）
-- ─────────────────────────────────────────────────────────────────────────────

-- 世代状态（VIP 钉档，世代 5）
INSERT INTO vip_tier_generations (id, user_id, generation, last_tier, updated_at)
VALUES ('gen-sc-001', 'u-fixture-C', 5, 'vip', NOW(3))
ON DUPLICATE KEY UPDATE generation=5, last_tier='vip';

-- 已 switched 的租约（仅供 tier_lease_id 只读引用）
INSERT INTO vip_tier_leases
  (id, user_id, from_tier, to_tier, generation, trigger_kind, trigger_ref, status, switched_at, created_at, updated_at)
VALUES
  ('lease-fixture-C-001', 'u-fixture-C', 'standard', 'vip', 5, 'pin', 'u-fixture-C|pin|20260921120000', 'switched',
   NOW(3), NOW(3), NOW(3));

-- 验证：fee_path_leases 表中不应有 user_id='u-fixture-C' 的记录
-- （此 SELECT 应返回 0 行，在集成中用于断言隔离性）
-- SELECT COUNT(*) FROM fee_path_leases WHERE business_key LIKE 'u-fixture-C|%';
-- 期望结果：0

-- ─────────────────────────────────────────────────────────────────────────────
-- 全量证明记录（供导出验证）
-- ─────────────────────────────────────────────────────────────────────────────

-- Scenario A 的漂移证明（verified 状态，摘要在运行时重计算）
INSERT INTO vip_drift_proofs
  (id, drift_case_id, user_id, asset, observed_bps, expected_bps, amount,
   quote_bind_id, lease_id, generation, digest, status, verified_at, created_at)
VALUES
  ('proof-fixture-A-001', 'dc-fixture-A-001', 'u-fixture-A', 'BTC',
   5, 2, 0.5,
   '', 'lease-fixture-A-001', 3,
   'PLACEHOLDER_DIGEST_COMPUTED_AT_TEST_RUNTIME',
   'verified', NOW(3), NOW(3));

-- Scenario B 的漂移证明（pending 状态，等待 worker 验证）
INSERT INTO vip_drift_proofs
  (id, drift_case_id, user_id, asset, observed_bps, expected_bps, amount,
   quote_bind_id, lease_id, generation, digest, status, verified_at, created_at)
VALUES
  ('proof-fixture-B-001', 'dc-fixture-B-001', 'u-fixture-B', 'ETH',
   4, 0, 5.0,
   'qb-fixture-B-001', 'lease-fixture-B-001', 8,
   'PLACEHOLDER_DIGEST_COMPUTED_AT_TEST_RUNTIME',
   'pending', NULL, NOW(3));
