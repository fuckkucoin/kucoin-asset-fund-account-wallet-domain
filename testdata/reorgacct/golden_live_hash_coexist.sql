-- Golden fixture SQL: live-hash 并存场景（ETH 链，普通冻结 + live-hash 出金）
-- 场景：重组波及一笔普通冻结出金（应回滚）和一笔 live-hash 出金（禁止自动解冻，只开事故单）。
-- 验收：freeze_rollback 腿正常执行；incident 腿只写事故单，FreezeRollbacker 不被调用于 live-hash。
-- 作者：Orin

-- ────────────────────────────────────────────────────────────────────────────
-- 1. 写入重组事件
-- ────────────────────────────────────────────────────────────────────────────
INSERT INTO reorg_events
  (id, chain, pivot_tx_hash, depth_before, depth_after, rollback_depth,
   affected_outputs, affected_tracks, evidence, state, source_kind, note,
   idem_key, created_at, updated_at)
VALUES (
  'evt-golden-lh-001',
  'ETH',
  '0xdeadbeef00000000000000000000000000000000000000000000000000000001',
  12, 3, 9,
  '[]',
  '[{"withdraw_id":"wd-normal-001","broadcast_id":"bc-normal-001","tx_hash":"0xdeadbeef00000000000000000000000000000000000000000000000000000001","live_hash":false,"freeze_id":"freeze-wd-001","asset":"ETH","user_id":"user-D","freeze_amount":"2.0"},{"withdraw_id":"wd-livehash-002","broadcast_id":"bc-livehash-002","tx_hash":"0xdeadbeef00000000000000000000000000000000000000000000000000000001","live_hash":true,"freeze_id":"freeze-wd-002","asset":"ETH","user_id":"user-E","freeze_amount":"1.0"}]',
  '[{"kind":"orphan_observe","ref":"orphan:0xdeadbeef_parent","note":"orphan observed; live-hash withdraw bc-livehash-002 coexists"}]',
  'pending',
  'orphan_observe',
  '普通冻结 + live-hash 并存场景',
  'lhidemlkey001',
  '2026-04-15 11:00:00.000',
  '2026-04-15 11:00:00.000'
);

-- ────────────────────────────────────────────────────────────────────────────
-- 2. 写入影响面行
-- ────────────────────────────────────────────────────────────────────────────
INSERT INTO impact_rows
  (id, event_id, user_id, asset, kind, reference_id, affected_amount, note, created_at)
VALUES
  ('row-golden-lh-001', 'evt-golden-lh-001', 'user-D', 'ETH', 'freeze',
   'freeze-wd-001', 2.000000000000000000,
   'withdraw freeze withdraw_id=wd-normal-001', '2026-04-15 11:00:05.000'),
  ('row-golden-lh-002', 'evt-golden-lh-001', 'user-E', 'ETH', 'live_hash',
   'wd-livehash-002', 1.000000000000000000,
   'live-hash broadcast_id=bc-livehash-002; manual review required', '2026-04-15 11:00:05.000');

UPDATE reorg_events SET state = 'scanned', updated_at = '2026-04-15 11:00:06.000'
WHERE id = 'evt-golden-lh-001';

-- ────────────────────────────────────────────────────────────────────────────
-- 3. 写入批次与腿
-- ────────────────────────────────────────────────────────────────────────────
INSERT INTO reverse_batches
  (id, event_id, state, impact_row_ids, total_legs, done_legs, failed_legs,
   idem_key, note, created_at, updated_at)
VALUES (
  'batch-golden-lh-001', 'evt-golden-lh-001', 'executing',
  '["row-golden-lh-001","row-golden-lh-002"]',
  2, 0, 0,
  'lhbatchidel001', 'auto plan for event evt-golden-lh-001',
  '2026-04-15 11:00:10.000', '2026-04-15 11:00:10.000'
);

UPDATE impact_rows SET reverse_batch_id = 'batch-golden-lh-001'
WHERE id IN ('row-golden-lh-001', 'row-golden-lh-002');

-- 腿1：普通冻结回滚（freeze_rollback，order=3）
-- 腿2：live-hash 事故单（incident，order=5，不解冻）
INSERT INTO reverse_legs
  (id, batch_id, impact_row_id, kind, execution_order, state, idem_key,
   reference_id, amount, user_id, asset, high_risk, result_note, created_at)
VALUES
  ('leg-golden-lh-001', 'batch-golden-lh-001', 'row-golden-lh-001',
   'freeze_rollback', 3, 'pending',
   'lhidem001freezerollback',
   'freeze-wd-001', 2.000000000000000000, 'user-D', 'ETH', 1, '', '2026-04-15 11:00:11.000'),
  ('leg-golden-lh-002', 'batch-golden-lh-001', 'row-golden-lh-002',
   'incident', 5, 'pending',
   'lhidem002incident',
   'wd-livehash-002', 1.000000000000000000, 'user-E', 'ETH', 0, '', '2026-04-15 11:00:11.000');

-- ────────────────────────────────────────────────────────────────────────────
-- 预期状态（执行 ExecuteBatch 后验证）
-- ────────────────────────────────────────────────────────────────────────────
-- leg-golden-lh-001: state = done    （freeze_rollback 成功执行）
-- leg-golden-lh-002: state = done    （incident 已开事故单）
-- reverse_guards: 仅 (row-golden-lh-001, freeze_rollback) 存在护栏
--                  (row-golden-lh-002, incident) 无护栏（live-hash 不写护栏）
-- incident_tickets: 1 条 kind=live_hash_manual
-- FreezeRollbacker: 仅被调用 1 次（freeze-wd-001），不调用 freeze-wd-002

-- SELECT l.id, l.kind, l.state FROM reverse_legs l WHERE l.batch_id = 'batch-golden-lh-001';
-- SELECT g.impact_row_id, g.leg_kind FROM reverse_guards g WHERE g.batch_id = 'batch-golden-lh-001';
-- SELECT t.kind, t.state FROM incident_tickets t WHERE t.batch_id = 'batch-golden-lh-001';
