package signpipegate

import "strings"

const SQLBumpGeneration = ` 
UPDATE signpipe_retry_generations
SET generation = generation + 1, updated_at = NOW(3)
WHERE movement_id = ? AND job_id = ? AND generation = ?`

const SQLInsertGenerationFirst = `
INSERT INTO signpipe_retry_generations (id, movement_id, job_id, generation, updated_at)
VALUES (?, ?, ?, 1, NOW(3))
ON DUPLICATE KEY UPDATE id = id`

const SQLInsertLease = ` 
INSERT INTO signpipe_inflight_leases (
  id, movement_id, job_id, chain, dest, amount, canonical, nonce,
  generation, status, actor, retries, fence_epoch, fence_token,
  sig_hex, isolation_mark, note, armed_at, inflight_at, sealed_at, expires_at, terminal_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

const SQLUpdateLeaseStatus = `
UPDATE signpipe_inflight_leases
SET status = ?, sig_hex = ?, note = ?, inflight_at = ?, sealed_at = ?, terminal_at = ?
WHERE id = ? AND status = ?`

const SQLInsertFault = `
INSERT INTO signpipe_fault_cases (
  id, lease_id, movement_id, job_id, kind, status, observed_state, note,
  operator, isolation_mark, injected_at, observed_at, closed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

const SQLInsertCloseout = `
INSERT INTO signpipe_closeout_proofs (
  id, lease_id, movement_id, job_id, kind, status, digest, note,
  operator, isolation_mark, opened_at, proven_at, closed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

const SQLInsertArtifact = `
INSERT INTO signpipe_signed_artifacts (
  id, lease_id, movement_id, job_id, canonical, sig_hex, digest,
  generation, isolation_mark, sealed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

const SQLSelectLeaseByNonce = `
SELECT id FROM signpipe_inflight_leases
WHERE movement_id = ? AND nonce = ?
ORDER BY armed_at DESC LIMIT 1`

const SQLSelectActiveLease = `
SELECT id FROM signpipe_inflight_leases
WHERE movement_id = ? AND status IN ('armed','inflight')
LIMIT 1`

const SQLSelectLeaseByID = ` 
SELECT id, movement_id, job_id, chain, dest, amount, canonical, nonce,
  generation, status, actor, retries, fence_epoch, fence_token,
  sig_hex, isolation_mark, note, armed_at, inflight_at, sealed_at, expires_at, terminal_at
FROM signpipe_inflight_leases WHERE id = ?`

const SQLUpsertLease = `
INSERT INTO signpipe_inflight_leases (
  id, movement_id, job_id, chain, dest, amount, canonical, nonce,
  generation, status, actor, retries, fence_epoch, fence_token,
  sig_hex, isolation_mark, note, armed_at, inflight_at, sealed_at, expires_at, terminal_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  status=VALUES(status), actor=VALUES(actor), retries=VALUES(retries),
  sig_hex=VALUES(sig_hex), note=VALUES(note), inflight_at=VALUES(inflight_at),
  sealed_at=VALUES(sealed_at), expires_at=VALUES(expires_at), terminal_at=VALUES(terminal_at)`

  func RenderNamed(sql string, names []string) string {
	  out := sql
	   for _, n := range names {
		  out = strings.Replace(out, "?", "/*"+n+"*/?", 1)
	  }
	   return out
   }

   func IsolationSQLGuard(ddl string) error {
	   low := strings.ToLower(ddl)
	   if strings.Contains(low, "por_root") || strings.Contains(low, "stmt_manifest") || strings.Contains(low, "balproof_root") {
		   return ErrIsolationViolation
	  }
	  return nil
  }
