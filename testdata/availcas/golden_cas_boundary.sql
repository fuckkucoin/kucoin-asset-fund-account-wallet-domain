-- CAS 屏障边界场景黄金 SQL 夹具。
-- 验证：同窗口两意图同时提交，数据库层面只有一条 accepted 行，另一条 rejected。
-- balance=1000, frozen=0, reserved=0, available=1000。
-- 作者：Gunner

-- 场景初始化：插入租约行（gen=7，state=open，available=1000）
INSERT INTO avail_leases
  (id, user_id, asset, book, gen, balance, frozen, reserved, in_flight, available,
   state, expires_at, created_at, updated_at)
VALUES
  ('lease-cas-001', 'user-cas', 'USDT', 'spot', 7,
   '1000.000000000000000000', '0.000000000000000000', '0.000000000000000000',
   '0.000000000000000000', '1000.000000000000000000',
   'open', DATE_ADD(NOW(3), INTERVAL 30 SECOND), NOW(3), NOW(3));

-- Winner 意图（500 USDT，gen=7，expected_avail=1000）：
INSERT INTO debit_intents
  (id, idem_key, user_id, asset, book, amount, kind, reason,
   lease_id, lease_gen, expected_avail,
   state, reject_code, reject_note, race_proof_id,
   actor_id, request_ip, proposed_at, decided_at)
VALUES
  ('intent-winner-001', 'idem:cas-winner', 'user-cas', 'USDT', 'spot',
   '500.000000000000000000', 'freeze', 'withdraw_freeze',
   'lease-cas-001', 7, '1000.000000000000000000',
   'accepted', '', '', '',
   'withdraw-svc', '10.0.0.1', NOW(3), NOW(3));

-- Loser 意图（600 USDT，同 gen=7，expected_avail=1000，但租约已 bound）：
INSERT INTO debit_intents
  (id, idem_key, user_id, asset, book, amount, kind, reason,
   lease_id, lease_gen, expected_avail,
   state, reject_code, reject_note, race_proof_id,
   actor_id, request_ip, proposed_at, decided_at)
VALUES
  ('intent-loser-001', 'idem:cas-loser', 'user-cas', 'USDT', 'spot',
   '600.000000000000000000', 'freeze', 'withdraw_freeze',
   'lease-cas-001', 7, '1000.000000000000000000',
   'rejected', 'cas_conflict', 'lease already bound to accepted intent', 'proof-cas-001',
   'withdraw-svc', '10.0.0.2', NOW(3), NOW(3));

-- 更新租约为 bound 状态：
UPDATE avail_leases SET state = 'bound', updated_at = NOW(3) WHERE id = 'lease-cas-001';

-- 竞态证明：
INSERT INTO race_proofs
  (id, user_id, asset, book, lease_id,
   winner_intent_id, loser_intent_id,
   winner_amount, loser_amount, avail_at_decision,
   window_start, window_end, reject_code, note, created_at)
VALUES
  ('proof-cas-001', 'user-cas', 'USDT', 'spot', 'lease-cas-001',
   'intent-winner-001', 'intent-loser-001',
   '500.000000000000000000', '600.000000000000000000', '1000.000000000000000000',
   DATE_SUB(NOW(3), INTERVAL 5 SECOND), NOW(3),
   'cas_conflict', 'concurrent withdraw: winner=500, loser=600', NOW(3));

-- ── 验证查询 ──────────────────────────────────────────────────────────────────

-- 断言：lease-cas-001 应为 bound。
SELECT id, state FROM avail_leases WHERE id = 'lease-cas-001';
-- expected: state = 'bound'

-- 断言：只有一条 accepted 意图。
SELECT COUNT(*) AS accepted_count FROM debit_intents
WHERE lease_id = 'lease-cas-001' AND state = 'accepted';
-- expected: accepted_count = 1

-- 断言：只有一条 rejected 意图。
SELECT COUNT(*) AS rejected_count FROM debit_intents
WHERE lease_id = 'lease-cas-001' AND state = 'rejected';
-- expected: rejected_count = 1

-- 断言：存在一条竞态证明。
SELECT id, reject_code, winner_intent_id, loser_intent_id FROM race_proofs
WHERE lease_id = 'lease-cas-001';
-- expected: reject_code = 'cas_conflict', winner = intent-winner-001, loser = intent-loser-001

-- 断言：∑ accepted amount（500）≤ initial available（1000）。
SELECT SUM(amount) AS total_accepted FROM debit_intents
WHERE lease_id = 'lease-cas-001' AND state = 'accepted';
-- expected: total_accepted = 500, which is <= 1000
