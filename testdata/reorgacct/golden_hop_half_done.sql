-- Golden fixture SQL: hop 半截并存场景（TRON 链）
-- 场景：确认数 20→10 回退，入金已贷记且触发了划转 hop，hop 在途需收口通知。
-- 验收：debit_reversal 腿（order=1）先执行，hop_close 腿（order=4）后执行，顺序正确。
-- 作者：Dax

-- ────────────────────────────────────────────────────────────────────────────
-- 1. 重组事件
-- ────────────────────────────────────────────────────────────────────────────
INSERT INTO reorg_events
  (id, chain, pivot_tx_hash, depth_before, depth_after, rollback_depth,
   affected_outputs, affected_tracks, evidence, state, source_kind, note,
   idem_key, created_at, updated_at)
VALUES (
  'evt-golden-hop-001',
  'TRON',
  'tron_tx_aaaa0000000000000000000000000000000000000000000000000000000001',
  20, 10, 10,
  '[{"tx_hash":"tron_tx_aaaa0000000000000000000000000000000000000000000000000000000001","output_index":0,"transfer_id":"tr-tron-hop-001","user_id":"user-F","asset":"USDT","amount":"500"}]',
  '[]',
  '[{"kind":"confirm_rollback","ref":"fundsafety:AdvanceConfirm:TRON:depth_before=20:depth_after=10","note":"TRON confirm rollback"}]',
  'scanned',
  'confirm_rollback',
  'TRON链确认回退，hop半截需收口',
  'tropidemlkey001',
  '2026-04-15 14:00:00.000',
  '2026-04-15 14:00:06.000'
);

-- ────────────────────────────────────────────────────────────────────────────
-- 2. 影响面行（已贷记 + hop 半截）
-- ────────────────────────────────────────────────────────────────────────────
INSERT INTO impact_rows
  (id, event_id, user_id, asset, kind, reference_id, affected_amount, note, created_at)
VALUES
  ('row-golden-hop-001', 'evt-golden-hop-001', 'user-F', 'USDT', 'credited',
   'tr-tron-hop-001', 500.000000000000000000,
   'deposit face phase=credited tx=tron_tx_…0001#0', '2026-04-15 14:00:05.000'),
  ('row-golden-hop-002', 'evt-golden-hop-001', 'user-F', 'USDT', 'hop_partial',
   'hop-001', 500.000000000000000000,
   'transfer hop partial transfer_id=tr-tron-hop-001 hop_id=hop-001', '2026-04-15 14:00:05.000');

-- ────────────────────────────────────────────────────────────────────────────
-- 3. 批次与腿（规划阶段）
-- ────────────────────────────────────────────────────────────────────────────
INSERT INTO reverse_batches
  (id, event_id, state, impact_row_ids, total_legs, done_legs, failed_legs,
   idem_key, note, created_at, updated_at)
VALUES (
  'batch-golden-hop-001', 'evt-golden-hop-001', 'planning',
  '["row-golden-hop-001","row-golden-hop-002"]',
  2, 0, 0,
  'hopbatchidel001', 'auto plan for event evt-golden-hop-001',
  '2026-04-15 14:00:10.000', '2026-04-15 14:00:10.000'
);

UPDATE impact_rows SET reverse_batch_id = 'batch-golden-hop-001'
WHERE id IN ('row-golden-hop-001', 'row-golden-hop-002');

-- 腿1：debit_reversal（order=1，先执行）
-- 腿2：hop_close（order=4，后执行，依赖腿1成功）
INSERT INTO reverse_legs
  (id, batch_id, impact_row_id, kind, execution_order, state, idem_key,
   reference_id, amount, user_id, asset, high_risk, result_note, created_at)
VALUES
  ('leg-golden-hop-001', 'batch-golden-hop-001', 'row-golden-hop-001',
   'debit_reversal', 1, 'pending',
   'hopidem001debit_reversal',
   'tr-tron-hop-001', 500.000000000000000000, 'user-F', 'USDT', 1, '', '2026-04-15 14:00:11.000'),
  ('leg-golden-hop-002', 'batch-golden-hop-001', 'row-golden-hop-002',
   'hop_close', 4, 'pending',
   'hopidem002hop_close',
   'hop-001', 500.000000000000000000, 'user-F', 'USDT', 0, '', '2026-04-15 14:00:11.000');

-- ────────────────────────────────────────────────────────────────────────────
-- 预期：执行后
-- ────────────────────────────────────────────────────────────────────────────
-- leg-golden-hop-001: state = done (debit_reversal 执行成功，order=1)
-- leg-golden-hop-002: state = done (hop_close 执行成功，order=4)
-- reverse_guards: 两条 (row-golden-hop-001, debit_reversal) + (row-golden-hop-002, hop_close)
-- batch: state = committed
-- 分录守恒：debit=1000 USDT = credit=1000 USDT（两对分录）

-- 分录验证 SQL:
-- SELECT SUM(affected_amount) FROM impact_rows WHERE event_id='evt-golden-hop-001' AND kind='credited'; → 500
-- SELECT SUM(affected_amount) FROM impact_rows WHERE event_id='evt-golden-hop-001' AND kind='hop_partial'; → 500
-- 总影响：1000 USDT（两种资金位各 500）
