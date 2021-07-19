package holdopsgate

const (
	sqlSelectGeneration = `SELECT id, ops_gen, fence_on, note, updated_at FROM hold_ops_generations WHERE id='singleton'`
	sqlCASBumpOps = `UPDATE hold_ops_generations
SET ops_gen = ops_gen + 1, fence_on=?, note=?, updated_at=?
WHERE id='singleton' AND ops_gen=?`
	sqlInsertLease = `INSERT INTO hold_ops_leases (
  id, ops_id, kind, user_id, asset, amount, token_id, gap_id, ref_id, reason, actor, nonce,
  ops_gen, status, isolation_mark, note, issued_at, bound_at, consumed_at, expires_at, terminal_at, bind_ticket
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	sqlSelectLease = `SELECT id, ops_id, kind, user_id, asset, amount, token_id, gap_id, ref_id, reason, actor, nonce, 
  ops_gen, status, isolation_mark, note, issued_at, bound_at, consumed_at, expires_at, terminal_at, bind_ticket
FROM hold_ops_leases WHERE id=?`
	sqlSelectLeaseNonce = `SELECT id, ops_id, kind, user_id, asset, amount, token_id, gap_id, ref_id, reason, actor, nonce,
  ops_gen, status, isolation_mark, note, issued_at, bound_at, consumed_at, expires_at, terminal_at, bind_ticket
FROM hold_ops_leases WHERE actor=? AND nonce=?`
	sqlListOpenLeases = `SELECT id, ops_id, kind, user_id, asset, amount, token_id, gap_id, ref_id, reason, actor, nonce,
  ops_gen, status, isolation_mark, note, issued_at, bound_at, consumed_at, expires_at, terminal_at, bind_ticket
FROM hold_ops_leases WHERE status IN ('open','bound')`
	sqlListTokenOpen = `SELECT id, ops_id, kind, user_id, asset, amount, token_id, gap_id, ref_id, reason, actor, nonce,
  ops_gen, status, isolation_mark, note, issued_at, bound_at, consumed_at, expires_at, terminal_at, bind_ticket
FROM hold_ops_leases WHERE token_id=? AND status IN ('open','bound')`
	sqlListActorOpen = `SELECT id, ops_id, kind, user_id, asset, amount, token_id, gap_id, ref_id, reason, actor, nonce,
  ops_gen, status, isolation_mark, note, issued_at, bound_at, consumed_at, expires_at, terminal_at, bind_ticket
FROM hold_ops_leases WHERE actor=? AND status IN ('open','bound')`
	sqlUpdateLease = `UPDATE hold_ops_leases SET status=?, note=?, bound_at=?, consumed_at=?, terminal_at=?, bind_ticket=? WHERE id=?`
	sqlInsertProof = `INSERT INTO hold_ops_proofs (
  id, lease_id, ops_id, token_id, gap_id, user_id, kind, status, expected_gen, observed_gen,
  winner_lease, loser_lease, open_token_count, note, isolation_mark, opened_at, proven_at, closed_at, eye_a, eye_b, clear_reason
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	sqlSelectProof = `SELECT id, lease_id, ops_id, token_id, gap_id, user_id, kind, status, expected_gen, observed_gen,
  winner_lease, loser_lease, open_token_count, note, isolation_mark, opened_at, proven_at, closed_at, eye_a, eye_b, clear_reason
FROM hold_ops_proofs WHERE id=?`
	sqlFindProof = `SELECT id, lease_id, ops_id, token_id, gap_id, user_id, kind, status, expected_gen, observed_gen,
  winner_lease, loser_lease, open_token_count, note, isolation_mark, opened_at, proven_at, closed_at, eye_a, eye_b, clear_reason
FROM hold_ops_proofs WHERE token_id=? AND kind=? AND status IN ('open','proven') LIMIT 1`
	sqlListOpenProofs = `SELECT id, lease_id, ops_id, token_id, gap_id, user_id, kind, status, expected_gen, observed_gen,
  winner_lease, loser_lease, open_token_count, note, isolation_mark, opened_at, proven_at, closed_at, eye_a, eye_b, clear_reason
FROM hold_ops_proofs WHERE status IN ('open','proven')`
	sqlListProofsToken = `SELECT id, lease_id, ops_id, token_id, gap_id, user_id, kind, status, expected_gen, observed_gen,
  winner_lease, loser_lease, open_token_count, note, isolation_mark, opened_at, proven_at, closed_at, eye_a, eye_b, clear_reason
FROM hold_ops_proofs WHERE token_id=?`
	sqlListProofsUser = `SELECT id, lease_id, ops_id, token_id, gap_id, user_id, kind, status, expected_gen, observed_gen,
  winner_lease, loser_lease, open_token_count, note, isolation_mark, opened_at, proven_at, closed_at, eye_a, eye_b, clear_reason
FROM hold_ops_proofs WHERE user_id=?`
	sqlUpdateProof = `UPDATE hold_ops_proofs SET status=?, eye_a=?, eye_b=?, clear_reason=?, proven_at=?, closed_at=? WHERE id=?`
	sqlInsertAudit = `INSERT INTO hold_ops_audit (
  id, ops_id, lease_id, token_id, gap_id, ops_gen, actor, kind, outcome, proof_id, note, at_ts, replay_key
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`
	sqlSelectAuditReplay = `SELECT id, ops_id, lease_id, token_id, gap_id, ops_gen, actor, kind, outcome, proof_id, note, at_ts, replay_key
FROM hold_ops_audit WHERE replay_key=?`
	sqlListAuditOps = `SELECT id, ops_id, lease_id, token_id, gap_id, ops_gen, actor, kind, outcome, proof_id, note, at_ts, replay_key
FROM hold_ops_audit WHERE ops_id=?`
	sqlUpsertToken = `INSERT INTO hold_ops_shadow_tokens (id, user_id, asset, amount, reason, ref_id, gap_id, is_open)
VALUES (?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE user_id=VALUES(user_id), asset=VALUES(asset), amount=VALUES(amount),
  reason=VALUES(reason), ref_id=VALUES(ref_id), gap_id=VALUES(gap_id), is_open=VALUES(is_open)`
	sqlSelectToken = `SELECT id, user_id, asset, amount, reason, ref_id, gap_id, is_open FROM hold_ops_shadow_tokens WHERE id=?`
	sqlListOpenTokGap = `SELECT id, user_id, asset, amount, reason, ref_id, gap_id, is_open FROM hold_ops_shadow_tokens WHERE gap_id=? AND is_open=1`
	sqlListOpenTokUser = `SELECT id, user_id, asset, amount, reason, ref_id, gap_id, is_open FROM hold_ops_shadow_tokens WHERE user_id=? AND is_open=1`
	sqlUpsertGap = `INSERT INTO hold_ops_shadow_gaps (id, user_id, asset, amount, acked, closed, open_token_count)
VALUES (?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE user_id=VALUES(user_id), asset=VALUES(asset), amount=VALUES(amount),
  acked=VALUES(acked), closed=VALUES(closed), open_token_count=VALUES(open_token_count)`
	sqlSelectGap = `SELECT id, user_id, asset, amount, acked, closed, open_token_count FROM hold_ops_shadow_gaps WHERE id=?`
)
