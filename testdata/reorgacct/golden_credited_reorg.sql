-- Golden fixture SQL: 已贷记后重组（ETH 链，确认数 12→4 回退）
-- 场景：两笔提前入账已贷记的入金在确认窗回退后需冲正。
-- 用途：集成中注入初始状态，验证 Ingestor + InventoryScanner + ReversalJournal 全链路。
-- 作者：Finn
--
-- 前置条件（需已存在）：
--   - 用户 user-A, user-B 存在；
--   - aa_transfers 中 tr-eth-001, tr-eth-002 状态为 confirming/credited；
--   - deposit Face 相位为 confirming（user-A）和 credited（user-B）。
--
-- 执行顺序：1→2→3→4（顺序依赖）

-- ────────────────────────────────────────────────────────────────────────────
-- 1. 写入重组事件
-- ────────────────────────────────────────────────────────────────────────────
INSERT INTO reorg_events
  (id, chain, pivot_tx_hash, depth_before, depth_after, rollback_depth,
   affected_outputs, affected_tracks, evidence, state, source_kind, note,
   idem_key, created_at, updated_at)
VALUES (
  'evt-golden-eth-001',
  'ETH',
  '0xabc123def456abc123def456abc123def456abc123def456abc123def456abc1',
  12, 4, 8,
  '[{"tx_hash":"0xabc123def456abc123def456abc123def456abc123def456abc123def456abc1","output_index":0,"transfer_id":"tr-eth-001","user_id":"user-A","asset":"ETH","amount":"1.5"},{"tx_hash":"0xabc123def456abc123def456abc123def456abc123def456abc123def456abc1","output_index":1,"transfer_id":"tr-eth-002","user_id":"user-B","asset":"ETH","amount":"0.3"}]',
  '[]',
  '[{"kind":"confirm_rollback","ref":"fundsafety:AdvanceConfirm:ETH:depth_before=12:depth_after=4","note":"AdvanceConfirm returned ErrReorg"}]',
  'pending',
  'confirm_rollback',
  'ETH 链确认数回退 12→4，两笔提前入账需冲正',
  'abc123idemkey001',
  '2026-04-15 10:00:00.000',
  '2026-04-15 10:00:00.000'
);

-- ────────────────────────────────────────────────────────────────────────────
-- 2. 写入影响面行（盘点产出）
-- ────────────────────────────────────────────────────────────────────────────
INSERT INTO impact_rows
  (id, event_id, user_id, asset, kind, reference_id, affected_amount, note, created_at)
VALUES
  ('row-golden-eth-001', 'evt-golden-eth-001', 'user-A', 'ETH', 'credited',
   'tr-eth-001', 1.500000000000000000,
   'deposit face phase=confirming tx=0xabc12…abc1#0', '2026-04-15 10:00:05.000'),
  ('row-golden-eth-002', 'evt-golden-eth-001', 'user-B', 'ETH', 'credited',
   'tr-eth-002', 0.300000000000000000,
   'deposit face phase=credited tx=0xabc12…abc1#1', '2026-04-15 10:00:05.000');

-- 更新事件状态为 scanned
UPDATE reorg_events SET state = 'scanned', updated_at = '2026-04-15 10:00:06.000'
WHERE id = 'evt-golden-eth-001';

-- ────────────────────────────────────────────────────────────────────────────
-- 3. 写入冲正批次
-- ────────────────────────────────────────────────────────────────────────────
INSERT INTO reverse_batches
  (id, event_id, state, impact_row_ids, total_legs, done_legs, failed_legs,
   idem_key, note, created_at, updated_at)
VALUES (
  'batch-golden-eth-001',
  'evt-golden-eth-001',
  'planning',
  '["row-golden-eth-001","row-golden-eth-002"]',
  2, 0, 0,
  'batchidemet001',
  'auto plan for event evt-golden-eth-001',
  '2026-04-15 10:00:10.000',
  '2026-04-15 10:00:10.000'
);

-- 回写 impact_rows.reverse_batch_id
UPDATE impact_rows SET reverse_batch_id = 'batch-golden-eth-001'
WHERE id IN ('row-golden-eth-001', 'row-golden-eth-002');

-- ────────────────────────────────────────────────────────────────────────────
-- 4. 写入冲正腿（pending，按依赖序）
-- ────────────────────────────────────────────────────────────────────────────
INSERT INTO reverse_legs
  (id, batch_id, impact_row_id, kind, execution_order, state, idem_key,
   reference_id, amount, user_id, asset, high_risk, result_note, created_at)
VALUES
  ('leg-golden-eth-001', 'batch-golden-eth-001', 'row-golden-eth-001',
   'debit_reversal', 1, 'pending',
   'abc123idemleg001debit_reversal',
   'tr-eth-001', 1.500000000000000000, 'user-A', 'ETH', 1, '', '2026-04-15 10:00:11.000'),
  ('leg-golden-eth-002', 'batch-golden-eth-001', 'row-golden-eth-002',
   'debit_reversal', 1, 'pending',
   'abc123idemleg002debit_reversal',
   'tr-eth-002', 0.300000000000000000, 'user-B', 'ETH', 1, '', '2026-04-15 10:00:11.000');

-- ────────────────────────────────────────────────────────────────────────────
-- 验证查询（运行后手动核对）
-- ────────────────────────────────────────────────────────────────────────────
-- SELECT e.id, e.state, COUNT(r.id) AS impact_rows, COUNT(l.id) AS legs
-- FROM reorg_events e
-- LEFT JOIN impact_rows r ON r.event_id = e.id
-- LEFT JOIN reverse_batches b ON b.event_id = e.id
-- LEFT JOIN reverse_legs l ON l.batch_id = b.id
-- WHERE e.id = 'evt-golden-eth-001'
-- GROUP BY e.id, e.state;
-- 期望：state=planning, impact_rows=2, legs=2

-- SELECT SUM(affected_amount) AS total_credited FROM impact_rows
-- WHERE event_id = 'evt-golden-eth-001' AND kind = 'credited';
-- 期望：1.8（= 1.5 + 0.3）
