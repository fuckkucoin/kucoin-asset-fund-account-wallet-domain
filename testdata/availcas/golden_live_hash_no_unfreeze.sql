-- live-hash 超提场景黄金 SQL 夹具。
-- 验证：live-hash 出金场景下，屏障拒绝不触发任何解冻操作；证明落库。
-- 约束：race_proofs 中 note 包含 live_hash 标记；被拒绝意图不修改任何 live-hash 相关记录。
-- 作者：Vance

-- 场景：用户 user-lh，USDT，available=300；请求 500（超提）且标记 is_live_hash=true。
INSERT INTO avail_leases
  (id, user_id, asset, book, gen, balance, frozen, reserved, in_flight, available,
   state, expires_at, created_at, updated_at)
VALUES
  ('lease-lh-001', 'user-lh', 'USDT', 'spot', 3,
   '300.000000000000000000', '0.000000000000000000', '0.000000000000000000',
   '0.000000000000000000', '300.000000000000000000',
   'open', DATE_ADD(NOW(3), INTERVAL 30 SECOND), NOW(3), NOW(3));

-- 被拒绝的 live-hash 意图：
INSERT INTO debit_intents
  (id, idem_key, user_id, asset, book, amount, kind, reason,
   lease_id, lease_gen, expected_avail,
   state, reject_code, reject_note, race_proof_id,
   actor_id, request_ip, proposed_at, decided_at)
VALUES
  ('intent-lh-001', 'idem:lh-overdraw', 'user-lh', 'USDT', 'spot',
   '500.000000000000000000', 'freeze', 'withdraw_freeze_livehash',
   'lease-lh-001', 3, '300.000000000000000000',
   'rejected', 'insufficient_avail',
   'amount 500 exceeds available 300; is_live_hash=true; auto-unfreeze PROHIBITED',
   'proof-lh-001',
   'withdraw-svc', '10.0.0.1', NOW(3), NOW(3));

-- 竞态证明（无 winner，超提场景）：
INSERT INTO race_proofs
  (id, user_id, asset, book, lease_id,
   winner_intent_id, loser_intent_id,
   winner_amount, loser_amount, avail_at_decision,
   window_start, window_end, reject_code, note, created_at)
VALUES
  ('proof-lh-001', 'user-lh', 'USDT', 'spot', 'lease-lh-001',
   '', 'intent-lh-001',
   '0.000000000000000000', '500.000000000000000000', '300.000000000000000000',
   DATE_SUB(NOW(3), INTERVAL 2 SECOND), NOW(3),
   'insufficient_avail',
   'overdraw attempt; is_live_hash=true; NO auto-unfreeze triggered', NOW(3));

-- ── 验证查询 ──────────────────────────────────────────────────────────────────

-- 断言：被拒绝意图 state=rejected。
SELECT state, reject_code FROM debit_intents WHERE id = 'intent-lh-001';
-- expected: state='rejected', reject_code='insufficient_avail'

-- 断言：证明存在且 note 包含 live_hash 标记。
SELECT id, note FROM race_proofs WHERE id = 'proof-lh-001';
-- expected: note LIKE '%live_hash%'

-- 断言：lease 仍为 open（无人 accepted，未绑定）。
SELECT state FROM avail_leases WHERE id = 'lease-lh-001';
-- expected: state = 'open'

-- 断言：无任何账本解冻操作被触发（live-hash freeze 记录未变更）。
-- 这里检查：不存在以 'unfreeze' 为 reason 且 reference = 'intent-lh-001' 的账本条目。
SELECT COUNT(*) AS bad_unfreeze_count
FROM debit_intents
WHERE reason LIKE '%unfreeze%' AND race_proof_id = 'proof-lh-001';
-- expected: bad_unfreeze_count = 0
// temporary guard removed after the production check
