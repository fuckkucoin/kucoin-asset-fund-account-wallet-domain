package scoreleasegate

const (
	sqlSelectGen = `SELECT id, halt_gen, halt_on, pack_gen, pack, pack_fp, mode_gen, mode,
  window_gen, window_digest, updated_at
FROM risk_score_generations WHERE id='singleton'`
	sqlBumpHalt = `UPDATE risk_score_generations
SET halt_gen = halt_gen + 1, halt_on=?, updated_at=?
WHERE id='singleton' AND halt_gen=?`
	sqlBumpPack = `UPDATE risk_score_generations
SET pack_gen = pack_gen + 1, pack=?, pack_fp=?, updated_at=?
WHERE id='singleton' AND pack_gen=?`
	sqlBumpMode = `UPDATE risk_score_generations
SET mode_gen = mode_gen + 1, mode=?, updated_at=?
WHERE id='singleton' AND mode_gen=?`
	sqlBumpWindow = `UPDATE risk_score_generations
SET window_gen = window_gen + 1, window_digest=?, updated_at=?
WHERE id='singleton' AND window_gen=?`
	sqlInsertLease = `INSERT INTO risk_score_leases (
  id, user_id, chain, dest, asset, amount, kind, pack, mode, pack_fp,
  halt_gen, pack_gen, mode_gen, window_gen, window_digest,
  velocity_1h, velocity_24h, dest_count, dest_count_24h,
  cluster_size, correlated, new_dests, fingerprint, nonce, status,
  isolation_mark, note, issued_at, bound_at, consumed_at, expires_at,
  terminal_at, bind_ticket, eval_allow, eval_reason
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	sqlUpdateLease = `UPDATE risk_score_leases SET
  status=?, note=?, bound_at=?, consumed_at=?, terminal_at=?, bind_ticket=?
WHERE id=?`
	sqlSelectLease = `SELECT id, user_id, chain, dest, asset, amount, kind, pack, mode, pack_fp,
  halt_gen, pack_gen, mode_gen, window_gen, window_digest,
  velocity_1h, velocity_24h, dest_count, dest_count_24h,
  cluster_size, correlated, new_dests, fingerprint, nonce, status,
  isolation_mark, note, issued_at, bound_at, consumed_at, expires_at,
  terminal_at, bind_ticket, eval_allow, eval_reason
FROM risk_score_leases WHERE id=?`
	sqlSelectLeaseNonce = `SELECT id FROM risk_score_leases WHERE user_id=? AND nonce=? LIMIT 1`
	sqlSelectOpenLeases = `SELECT id FROM risk_score_leases WHERE status IN ('issued','bound')`
	sqlSelectUserOpen = `SELECT id FROM risk_score_leases WHERE user_id=? AND status IN ('issued','bound')`
	sqlInsertProof = `INSERT INTO risk_score_drift_proofs (
  id, lease_id, user_id, kind, status, expected_halt, observed_halt,
  expected_pack, observed_pack, expected_mode, observed_mode,
  expected_fp, observed_fp, expected_win, observed_win,
  expected_win_gen, observed_win_gen, digest, isolation_mark, note,
  closer_a, closer_b, close_reason, opened_at, proven_at, closed_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	sqlUpdateProof = `UPDATE risk_score_drift_proofs SET
  status=?, digest=?, note=?, closer_a=?, closer_b=?, close_reason=?, proven_at=?, closed_at=?
WHERE id=?`
	sqlSelectProof = `SELECT id, lease_id, user_id, kind, status, expected_halt, observed_halt,
  expected_pack, observed_pack, expected_mode, observed_mode,
  expected_fp, observed_fp, expected_win, observed_win,
  expected_win_gen, observed_win_gen, digest, isolation_mark, note,
  closer_a, closer_b, close_reason, opened_at, proven_at, closed_at
FROM risk_score_drift_proofs WHERE id=?`
	sqlSelectProofFind = `SELECT id FROM risk_score_drift_proofs
WHERE lease_id=? AND kind=? AND expected_halt=? AND observed_halt=? LIMIT 1`
	sqlSelectOpenProofs = `SELECT id FROM risk_score_drift_proofs WHERE status IN ('open','proven')`
	sqlInsertAudit = `INSERT INTO risk_score_lease_audit (
  id, kind, lease_id, proof_id, user_id, actor, note, at_ts
) VALUES (?,?,?,?,?,?,?,?)`
	sqlSelectAudit = `SELECT id, kind, lease_id, proof_id, user_id, actor, note, at_ts
FROM risk_score_lease_audit WHERE lease_id=? ORDER BY at_ts ASC`
)
