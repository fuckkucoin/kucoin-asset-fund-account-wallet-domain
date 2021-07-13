-- 孤儿封堵 API 场景 SQL — 用于集成和冒烟数据填充。
--
-- 场景：
--   batch-sql-001: 5 items，其中 item-4 / item-5 为 hanging 孤儿，已封堵。
--   batch-sql-002: 3 items，其中 item-2 / item-3 为 missing 孤儿，待处置。
--
-- 验证点：
--   1. 按 batch_id 查询孤儿报告；
--   2. 孤儿状态过滤（detected / sealed）；
--   3. 封堵后 lo_idem_blocks 中存在对应行；
--   4. 完整性证明哈希可重算。
--
-- 作者：KianFord
-- 日期：2021.01.18

-- ── batch-sql-001 世代锁 ──────────────────────────────────────────────────────
INSERT INTO lo_batch_gens (id, batch_id, generation, worker_id, status, locked_at, expires_at, released_at, fence_epoch, created_at, updated_at) VALUES
  ('gen-sql-001', 'batch-sql-001', 2, 'worker-resume:pid-1001:uuid-sql', 'released',
   '2026-03-20 10:00:00.000', '2026-03-20 10:01:30.000', '2026-03-20 10:05:00.000',
   2, '2026-03-20 09:55:00.000', '2026-03-20 10:05:00.000');

-- ── batch-sql-001 孤儿报告 ────────────────────────────────────────────────────
INSERT INTO lo_orphan_reports (report_id, batch_id, generation, status, total_orphans, hanging_count, missing_count, intent_hash, ledger_hash, fence_epoch, created_at, updated_at) VALUES
  ('report-sql-001', 'batch-sql-001', 1, 'closed', 2, 2, 0,
   'a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2',
   'b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3',
   1, '2026-03-20 10:00:30.000', '2026-03-20 10:06:00.000');

-- ── batch-sql-001 孤儿明细（已封堵）────────────────────────────────────────────
INSERT INTO lo_orphan_items (orphan_id, batch_id, report_id, seq, user_id, asset, amount, direction, idem_key, kind, status, ledger_id, batch_ledger_id, note, detected_at, sealed_at, fence_epoch, updated_at) VALUES
  ('orp-sql-001-4', 'batch-sql-001', 'report-sql-001', 4, 'user-4001', 'USDT', 100.000000000000000000, 'funding_to_trading',
   'sql-hop-idem-004', 'hanging', 'sealed', '', 'ledger-sql-004-MISSING',
   '批次 batch-sql-001 孤儿 seq=4 已封堵', '2026-03-20 10:00:30.000', '2026-03-20 10:05:30.000', 2, '2026-03-20 10:05:30.000'),
  ('orp-sql-001-5', 'batch-sql-001', 'report-sql-001', 5, 'user-5001', 'USDT', 100.000000000000000000, 'funding_to_trading',
   'sql-hop-idem-005', 'hanging', 'sealed', '', 'ledger-sql-005-MISSING',
   '批次 batch-sql-001 孤儿 seq=5 已封堵', '2026-03-20 10:00:30.000', '2026-03-20 10:05:30.000', 2, '2026-03-20 10:05:30.000');

-- ── batch-sql-001 封堵动作证明 ────────────────────────────────────────────────
INSERT INTO lo_seal_actions (seal_id, batch_id, report_id, kind, status, orphan_ids, actor1, actor2, approved_at, applied_at, proof_hash, fence_epoch, created_at, updated_at) VALUES
  ('seal-sql-001', 'batch-sql-001', 'report-sql-001', 'block', 'applied',
   '["orp-sql-001-4","orp-sql-001-5"]', 'ops-charlie', 'ops-diana',
   '2026-03-20 10:05:00.000', '2026-03-20 10:05:30.000',
   'c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4',
   2, '2026-03-20 10:05:00.000', '2026-03-20 10:05:30.000');

-- ── batch-sql-001 idem 拒写记录（封堵后） ────────────────────────────────────
INSERT INTO lo_idem_blocks (idem_key, batch_id, orphan_id, seal_id, blocked_at) VALUES
  ('sql-hop-idem-004', 'batch-sql-001', 'orp-sql-001-4', 'seal-sql-001', '2026-03-20 10:05:30.000'),
  ('sql-hop-idem-005', 'batch-sql-001', 'orp-sql-001-5', 'seal-sql-001', '2026-03-20 10:05:30.000');

-- ── batch-sql-001 完整性证明 ──────────────────────────────────────────────────
INSERT INTO lo_integrity_proofs (proof_id, batch_id, report_id, seal_id, generation, status, hash, intent_count, ledger_count, seal_count, intent_idem_keys, ledger_idem_keys, seal_idem_keys, verified_at, fence_epoch, created_at) VALUES
  ('proof-sql-001', 'batch-sql-001', 'report-sql-001', 'seal-sql-001', 2, 'verified',
   'c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4',
   5, 3, 2,
   '["sql-hop-idem-001","sql-hop-idem-002","sql-hop-idem-003","sql-hop-idem-004","sql-hop-idem-005"]',
   '["sql-hop-idem-001","sql-hop-idem-002","sql-hop-idem-003"]',
   '["sql-hop-idem-004","sql-hop-idem-005"]',
   '2026-03-20 11:00:00.000', 2, '2026-03-20 10:06:00.000');

-- ── batch-sql-002 世代锁（active，待续提） ────────────────────────────────────
INSERT INTO lo_batch_gens (id, batch_id, generation, worker_id, status, locked_at, expires_at, fence_epoch, created_at, updated_at) VALUES
  ('gen-sql-002', 'batch-sql-002', 1, '', 'open', NULL, NULL, 1, '2026-03-21 08:00:00.000', '2026-03-21 08:00:00.000');

-- ── batch-sql-002 孤儿报告（open，待处置） ────────────────────────────────────
INSERT INTO lo_orphan_reports (report_id, batch_id, generation, status, total_orphans, hanging_count, missing_count, intent_hash, ledger_hash, fence_epoch, created_at, updated_at) VALUES
  ('report-sql-002', 'batch-sql-002', 0, 'open', 2, 0, 2,
   'd4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5',
   'e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6',
   1, '2026-03-21 08:05:00.000', '2026-03-21 08:05:00.000');

-- ── batch-sql-002 孤儿明细（detected，待处置） ───────────────────────────────
INSERT INTO lo_orphan_items (orphan_id, batch_id, report_id, seq, user_id, asset, amount, direction, idem_key, kind, status, ledger_id, batch_ledger_id, note, detected_at, fence_epoch, updated_at) VALUES
  ('orp-sql-002-2', 'batch-sql-002', 'report-sql-002', 2, 'user-6002', 'BTC', 0.010000000000000000, 'trading_to_funding',
   'sql-miss-idem-002', 'missing', 'detected', 'ledger-btc-sql-002', '',
   '', '2026-03-21 08:05:00.000', 1, '2026-03-21 08:05:00.000'),
  ('orp-sql-002-3', 'batch-sql-002', 'report-sql-002', 3, 'user-6003', 'BTC', 0.010000000000000000, 'trading_to_funding',
   'sql-miss-idem-003', 'missing', 'detected', 'ledger-btc-sql-003', '',
   '', '2026-03-21 08:05:00.000', 1, '2026-03-21 08:05:00.000');
