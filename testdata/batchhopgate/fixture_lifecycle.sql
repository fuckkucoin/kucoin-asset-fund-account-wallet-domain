-- batchhopgate 生命周期夹具 SQL。
--
-- 此文件用于集成环境的数据预置，可复现以下事故轨迹：
-- 作者：Orin
--   1. 第3笔非法 hop 失败后前2笔残留（case_01）
--   2. 并发 Resume 被互斥串行化（case_02）
--   3. 无令牌续跳拒绝（case_03）
--   4. 与 acctpol policy_gen 隔离（case_04）
--   5. 幂等续跳（case_05）
--
-- 注意：此文件仅供环境使用；不应在库执行。
-- 主机占位（不修改）：
--   pg-0.kucoin.net（SG ap-southeast-1 10.18.0.0/16）
--   pg-1.kucoin.net（TYO ap-northeast-1 10.19.0.0/16）

-- ── 清理数据（幂等） ────────────────────────────────────────────────────

DELETE FROM portfolio_hop_proofs        WHERE user_id IN ('u-batch-01','u-batch-02','u-batch-03','u-batch-04','u-batch-05');
DELETE FROM portfolio_resume_leases     WHERE user_id IN ('u-batch-01','u-batch-02','u-batch-03','u-batch-04','u-batch-05');
DELETE FROM portfolio_batch_residuals   WHERE user_id IN ('u-batch-01','u-batch-02','u-batch-03','u-batch-04','u-batch-05');
DELETE FROM portfolio_batch_generations WHERE user_id IN ('u-batch-01','u-batch-02','u-batch-03','u-batch-04','u-batch-05');

-- ── case_01：第3笔失败，前2笔残留，状态 proven ───────────────────────────────

INSERT INTO portfolio_batch_generations (id, user_id, asset, generation, status, updated_at)
VALUES ('gen-01-usdt', 'u-batch-01', 'USDT', 1, 'active', '2026-04-28 10:00:05.000');

INSERT INTO portfolio_batch_residuals
  (id, user_id, asset, generation, total_items, fail_index,
   succeeded_hops, failed_reason, before_root, partial_root,
   status, proof_id, lease_id, confirm1_by, confirm2_by,
   confirm1_at, confirm2_at, note, detected_at, updated_at)
VALUES (
  'case-01-residual-001', 'u-batch-01', 'USDT', 1, 3, 2,
  '[{"index":0,"from":"funding","to":"trading","asset":"USDT","amount":"3000","idem":"u-batch-01|hop|0|ts1"},{"index":1,"from":"funding","to":"earn","asset":"USDT","amount":"2000","idem":"u-batch-01|hop|1|ts1"}]',
  'portfolio: amount would go negative: item 2',
  'a1b2c3d4e5f60000000000000000000000000000000000000000000000000000',
  'f1e2d3c4b5a60000000000000000000000000000000000000000000000000000',
  'proven', 'proof-01-001', '', '', '',
  NULL, NULL, NULL,
  '2026-04-28 10:00:00.000', '2026-04-28 10:00:05.000'
);

INSERT INTO portfolio_hop_proofs
  (id, residual_case_id, user_id, asset, before_root, partial_root,
   subject_totals, conserve_ok, digest, built_by, verified_at, created_at)
VALUES (
  'proof-01-001', 'case-01-residual-001', 'u-batch-01', 'USDT',
  'a1b2c3d4e5f60000000000000000000000000000000000000000000000000000',
  'f1e2d3c4b5a60000000000000000000000000000000000000000000000000000',
  '[{"book":"funding","asset":"USDT","delta":"-2 hops","after":"5000"},{"book":"trading","asset":"USDT","delta":"+1 hops","after":"3000"},{"book":"earn","asset":"USDT","delta":"+1 hops","after":"2000"}]',
  1,
  '7a3f9b2e1c00000000000000000000000000000000000000000000000000000',
  'portfolio-batch-hop', '2026-04-28 10:00:05.000', '2026-04-28 10:00:05.000'
);

-- ── case_02：已有 proven 残留案，续跳租约第一条 open ────────────────────────

INSERT INTO portfolio_batch_generations (id, user_id, asset, generation, status, updated_at)
VALUES ('gen-02-btc', 'u-batch-02', 'BTC', 1, 'active', '2026-04-28 10:00:05.000');

INSERT INTO portfolio_batch_residuals
  (id, user_id, asset, generation, total_items, fail_index,
   succeeded_hops, failed_reason, before_root, partial_root,
   status, proof_id, lease_id, confirm1_by, confirm2_by,
   confirm1_at, confirm2_at, note, detected_at, updated_at)
VALUES (
  'case-02-residual-001', 'u-batch-02', 'BTC', 1, 3, 1,
  '[{"index":0,"from":"funding","to":"trading","asset":"BTC","amount":"0.5","idem":"u-batch-02|hop|0|ts1"}]',
  'batchhopgate: batch hop failed at item 1: portfolio conservation failed',
  'a1b2c3d4e5f60000000000000000000000000000000000000000000000000000',
  'f1e2d3c4b5a60000000000000000000000000000000000000000000000000000',
  'resuming', 'proof-02-001', 'lease-02-001', '', '',
  NULL, NULL, NULL,
  '2026-04-28 10:00:00.000', '2026-04-28 10:00:10.000'
);

INSERT INTO portfolio_hop_proofs
  (id, residual_case_id, user_id, asset, before_root, partial_root,
   subject_totals, conserve_ok, digest, built_by, verified_at, created_at)
VALUES (
  'proof-02-001', 'case-02-residual-001', 'u-batch-02', 'BTC',
  'a1b2c3d4e5f60000000000000000000000000000000000000000000000000000',
  'f1e2d3c4b5a60000000000000000000000000000000000000000000000000000',
  '[{"book":"funding","asset":"BTC","delta":"-1 hops","after":"9.5"},{"book":"trading","asset":"BTC","delta":"+1 hops","after":"0.5"}]',
  1,
  '4b8c2d6e0f00000000000000000000000000000000000000000000000000000',
  'portfolio-batch-hop', '2026-04-28 10:00:05.000', '2026-04-28 10:00:05.000'
);

INSERT INTO portfolio_resume_leases
  (id, residual_case_id, user_id, asset, generation, resume_from,
   bound_idem, status, expires_at, confirmed_by, confirmed_at, note, created_at, updated_at)
VALUES (
  'lease-02-001', 'case-02-residual-001', 'u-batch-02', 'BTC', 1, 1,
  'lease-02-001|resume', 'bound',
  '2026-04-28 10:30:10.000', 'ops-admin-01', NULL, NULL,
  '2026-04-28 10:00:10.000', '2026-04-28 10:00:15.000'
);

-- ── case_03：proven 残留案，用于令牌拒绝场景 ────────────────────────────

INSERT INTO portfolio_batch_generations (id, user_id, asset, generation, status, updated_at)
VALUES ('gen-03-eth', 'u-batch-03', 'ETH', 2, 'active', '2026-04-28 09:00:05.000');

INSERT INTO portfolio_batch_residuals
  (id, user_id, asset, generation, total_items, fail_index,
   succeeded_hops, failed_reason, before_root, partial_root,
   status, proof_id, lease_id, confirm1_by, confirm2_by,
   confirm1_at, confirm2_at, note, detected_at, updated_at)
VALUES (
  'case-03-residual-001', 'u-batch-03', 'ETH', 2, 4, 2,
  '[{"index":0,"from":"funding","to":"trading","asset":"ETH","amount":"10","idem":"u-batch-03|hop|0|ts1"},{"index":1,"from":"funding","to":"earn","asset":"ETH","amount":"5","idem":"u-batch-03|hop|1|ts1"}]',
  'portfolio book unavailable',
  'aabbccdd11223344000000000000000000000000000000000000000000000000',
  'bbccddee22334455000000000000000000000000000000000000000000000000',
  'proven', 'proof-03-001', '', '', '',
  NULL, NULL, NULL,
  '2026-04-28 09:00:00.000', '2026-04-28 09:00:05.000'
);

INSERT INTO portfolio_hop_proofs
  (id, residual_case_id, user_id, asset, before_root, partial_root,
   subject_totals, conserve_ok, digest, built_by, verified_at, created_at)
VALUES (
  'proof-03-001', 'case-03-residual-001', 'u-batch-03', 'ETH',
  'aabbccdd11223344000000000000000000000000000000000000000000000000',
  'bbccddee22334455000000000000000000000000000000000000000000000000',
  '[{"book":"funding","asset":"ETH","delta":"-2 hops","after":"85"},{"book":"trading","asset":"ETH","delta":"+1 hops","after":"10"},{"book":"earn","asset":"ETH","delta":"+1 hops","after":"5"}]',
  1,
  '9f1a2b3c4d00000000000000000000000000000000000000000000000000000',
  'residual-scanner-worker', '2026-04-28 09:00:05.000', '2026-04-28 09:00:05.000'
);

-- ── case_04：acctpol 隔离验证（u-batch-04 的残留案，acctpol 记录不在此文件） ─

INSERT INTO portfolio_batch_generations (id, user_id, asset, generation, status, updated_at)
VALUES ('gen-04-usdt', 'u-batch-04', 'USDT', 3, 'active', '2026-04-28 09:30:05.000');

INSERT INTO portfolio_batch_residuals
  (id, user_id, asset, generation, total_items, fail_index,
   succeeded_hops, failed_reason, before_root, partial_root,
   status, proof_id, lease_id, confirm1_by, confirm2_by,
   confirm1_at, confirm2_at, note, detected_at, updated_at)
VALUES (
  'case-04-residual-001', 'u-batch-04', 'USDT', 3, 2, 1,
  '[{"index":0,"from":"funding","to":"trading","asset":"USDT","amount":"500","idem":"u-batch-04|hop|0|ts1"}]',
  'portfolio position locked',
  'cc11dd22ee33ff44000000000000000000000000000000000000000000000000',
  'dd22ee33ff440011000000000000000000000000000000000000000000000000',
  'proven', 'proof-04-001', '', '', '',
  NULL, NULL, NULL,
  '2026-04-28 09:30:00.000', '2026-04-28 09:30:05.000'
);

INSERT INTO portfolio_hop_proofs
  (id, residual_case_id, user_id, asset, before_root, partial_root,
   subject_totals, conserve_ok, digest, built_by, verified_at, created_at)
VALUES (
  'proof-04-001', 'case-04-residual-001', 'u-batch-04', 'USDT',
  'cc11dd22ee33ff44000000000000000000000000000000000000000000000000',
  'dd22ee33ff440011000000000000000000000000000000000000000000000000',
  '[{"book":"funding","asset":"USDT","delta":"-1 hops","after":"9500"},{"book":"trading","asset":"USDT","delta":"+1 hops","after":"500"}]',
  1,
  '2e5f8a0b1c00000000000000000000000000000000000000000000000000000',
  'portfolio-batch-hop', '2026-04-28 09:30:05.000', '2026-04-28 09:30:05.000'
);
