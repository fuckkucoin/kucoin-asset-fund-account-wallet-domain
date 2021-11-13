-- Golden fixture SQL: 二次冲正护栏场景
-- 场景：同一 ImpactRow 被两个不同批次尝试冲正，第二次被 ErrGuardConflict 阻断。
-- 验收：reverse_guards 表只有一条 (row_id, leg_kind) 记录；
-- 作者：Marston
--        第二个批次的对应腿状态为 skipped；double_reverse_audit 中有告警记录。

-- ────────────────────────────────────────────────────────────────────────────
-- 前置状态：第一次冲正已完成（batch-guard-001 committed）
-- ────────────────────────────────────────────────────────────────────────────
INSERT INTO reorg_events
  (id, chain, pivot_tx_hash, depth_before, depth_after, rollback_depth,
   affected_outputs, affected_tracks, evidence, state, source_kind, note,
   idem_key, created_at, updated_at)
VALUES (
  'evt-golden-guard-001', 'ETH',
  '0xguard00000000000000000000000000000000000000000000000000000000001',
  12, 5, 7,
  '[{"tx_hash":"0xguard00000000000000000000000000000000000000000000000000000000001","output_index":0,"transfer_id":"tr-guard-001","user_id":"user-I","asset":"ETH","amount":"0.5"}]',
  '[]', '[]', 'closed', 'confirm_rollback', '护栏事件',
  'guardidemlkey001',
  '2026-04-15 15:00:00.000', '2026-04-15 15:30:00.000'
);

INSERT INTO impact_rows
  (id, event_id, user_id, asset, kind, reference_id, affected_amount,
   reverse_batch_id, note, created_at)
VALUES
  ('row-golden-guard-001', 'evt-golden-guard-001', 'user-I', 'ETH', 'credited',
   'tr-guard-001', 0.500000000000000000,
   'batch-guard-001', 'deposit face phase=credited; first reverse complete', '2026-04-15 15:00:05.000');

-- 第一次批次（已 committed）
INSERT INTO reverse_batches
  (id, event_id, state, impact_row_ids, total_legs, done_legs, failed_legs,
   idem_key, note, created_at, updated_at)
VALUES (
  'batch-guard-001', 'evt-golden-guard-001', 'committed',
  '["row-golden-guard-001"]', 1, 1, 0,
  'guardbatchidel001', 'first reversal (committed)',
  '2026-04-15 15:00:10.000', '2026-04-15 15:05:00.000'
);

-- 第一次腿（已 done）
INSERT INTO reverse_legs
  (id, batch_id, impact_row_id, kind, execution_order, state, idem_key,
   reference_id, amount, user_id, asset, high_risk, result_note, created_at)
VALUES (
  'leg-guard-001-first', 'batch-guard-001', 'row-golden-guard-001',
  'debit_reversal', 1, 'done',
  'guardideml001debit_reversal',
  'tr-guard-001', 0.500000000000000000, 'user-I', 'ETH', 1, 'ok',
  '2026-04-15 15:00:11.000'
);

-- 第一次护栏（已写入）
INSERT INTO reverse_guards (id, impact_row_id, leg_kind, batch_id, leg_id, locked_at)
VALUES (
  'guard-row-001-debit', 'row-golden-guard-001', 'debit_reversal',
  'batch-guard-001', 'leg-guard-001-first', '2026-04-15 15:01:00.000'
);

-- ────────────────────────────────────────────────────────────────────────────
-- 二次冲正尝试（batch-guard-002，由 ops 错误触发）
-- ────────────────────────────────────────────────────────────────────────────
INSERT INTO reverse_batches
  (id, event_id, state, impact_row_ids, total_legs, done_legs, failed_legs,
   idem_key, note, created_at, updated_at)
VALUES (
  'batch-guard-002', 'evt-golden-guard-001', 'executing',
  '["row-golden-guard-001"]', 1, 0, 0,
  'guardbatchidel002', 'second reversal attempt (should be blocked)',
  '2026-04-15 15:10:00.000', '2026-04-15 15:10:00.000'
);

-- 第二次腿（pending → 执行时护栏命中 → skipped）
INSERT INTO reverse_legs
  (id, batch_id, impact_row_id, kind, execution_order, state, idem_key,
   reference_id, amount, user_id, asset, high_risk, result_note, created_at)
VALUES (
  'leg-guard-002-second', 'batch-guard-002', 'row-golden-guard-001',
  'debit_reversal', 1, 'pending',
  'guardideml002debit_reversal',
  'tr-guard-001', 0.500000000000000000, 'user-I', 'ETH', 1, '',
  '2026-04-15 15:10:01.000'
);

-- ────────────────────────────────────────────────────────────────────────────
-- 预期（执行 leg-guard-002-second 的 ExecuteBatch）
-- ────────────────────────────────────────────────────────────────────────────
-- GuardManager.Lock("row-golden-guard-001", "debit_reversal", "batch-guard-002", "leg-guard-002-second")
--   → reverse_guards 中 (row-golden-guard-001, debit_reversal) 已存在
--   → 返回 ErrGuardConflict
-- leg-guard-002-second: state = skipped，result_note = "guard conflict: already reversed"
-- reverse_guards: 仍只有 1 条记录（batch-guard-001 的）
-- LedgerReverser: 未被调用（护栏提前阻断）
-- batch-guard-002: state = committed（唯一腿被 skipped，视为完成）

-- 验证查询:
-- SELECT COUNT(*) FROM reverse_guards WHERE impact_row_id = 'row-golden-guard-001'; → 1
-- SELECT state, result_note FROM reverse_legs WHERE id = 'leg-guard-002-second'; → skipped, guard conflict
-- SELECT state FROM reverse_batches WHERE id = 'batch-guard-002'; → committed
