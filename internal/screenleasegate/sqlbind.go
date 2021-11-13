package screenleasegate

const (
	sqlSelectGen = `SELECT id, halt_gen, halt_on, pack_gen, pack, mode, pack_fp, updated_at
FROM risk_screen_generations WHERE id='singleton'`
	sqlBumpHalt = `UPDATE risk_screen_generations 
SET halt_gen = halt_gen + 1, halt_on=?, updated_at=?
WHERE id='singleton' AND halt_gen=?`
	sqlBumpPack = `UPDATE risk_screen_generations
SET pack_gen = pack_gen + 1, pack=?, mode=?, pack_fp=?, updated_at=?
WHERE id='singleton' AND pack_gen=?`
	sqlInsertLease = `INSERT INTO risk_screen_leases (
  id, user_id, dest, asset, amount, kind, pack, mode, pack_fp,
  halt_gen, pack_gen, window_digest, velocity_1h, velocity_24h,
  dest_count, dest_count_24h, nonce, status, isolation_mark, note,
  issued_at, bound_at, consumed_at, expires_at, terminal_at, bind_ticket
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	sqlUpdateLease = `UPDATE risk_screen_leases SET
  status=?, note=?, bound_at=?, consumed_at=?, terminal_at=?, bind_ticket=?
WHERE id=?`
	sqlSelectLease = `SELECT id, user_id, dest, asset, amount, kind, pack, mode, pack_fp,
  halt_gen, pack_gen, window_digest, velocity_1h, velocity_24h,
  dest_count, dest_count_24h, nonce, status, isolation_mark, note,
  issued_at, bound_at, consumed_at, expires_at, terminal_at, bind_ticket
FROM risk_screen_leases WHERE id=?`
	sqlSelectLeaseNonce = `SELECT id FROM risk_screen_leases WHERE user_id=? AND nonce=? LIMIT 1`
	sqlSelectOpenLeases = `SELECT id FROM risk_screen_leases WHERE status IN ('open','bound')`
	sqlInsertProof = `INSERT INTO risk_screen_proofs (
  id, lease_id, user_id, kind, status, expected_halt, observed_halt,
  expected_pack, observed_pack, expected_fp, observed_fp, digest,
  isolation_mark, note, operator_a, operator_b, opened_at, proven_at, closed_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	sqlUpdateProof = `UPDATE risk_screen_proofs SET
  status=?, digest=?, note=?, operator_a=?, operator_b=?, proven_at=?, closed_at=?
WHERE id=?`
	sqlSelectProof = `SELECT id, lease_id, user_id, kind, status, expected_halt, observed_halt,
  expected_pack, observed_pack, expected_fp, observed_fp, digest,
  isolation_mark, note, operator_a, operator_b, opened_at, proven_at, closed_at
FROM risk_screen_proofs WHERE id=?`
)
