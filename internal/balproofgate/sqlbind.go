package balproofgate

import "strings"

// SQLBumpGeneration CAS 抬世代。
const SQLBumpGeneration = `
UPDATE balproof_generations
SET generation = generation + 1, updated_at = NOW(3)
WHERE user_id = ? AND book = ? AND asset = ? AND generation = ?`

// SQLInsertGenerationFirst 首次插入。
const SQLInsertGenerationFirst = `
INSERT INTO balproof_generations (id, user_id, book, asset, generation, updated_at)
VALUES (?, ?, ?, ?, 1, NOW(3))
ON DUPLICATE KEY UPDATE id = id`

// SQLInsertLease 落租约。
const SQLInsertLease = `
INSERT INTO balproof_leases (
  id, user_id, book, asset, nonce, generation, status,
  available, frozen, total, observe_gen, root_digest,
  canonical, signature, issued_at, bound_at, expires_at, isolation_mark
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

// SQLUpdateLeaseStatus 推进状态。
const SQLUpdateLeaseStatus = `
UPDATE balproof_leases
SET status = ?, consumed_at = ?, revoked_at = ?, revoke_reason = ?, bound_at = ?
WHERE id = ? AND status = ?`

// SQLInsertDrift 落漂移证明。
const SQLInsertDrift = `
INSERT INTO balproof_drift_proofs (
  id, lease_id, user_id, book, asset, kind, status,
  issued_avail, issued_frozen, issued_observe_gen,
  live_avail, live_frozen, live_observe_gen,
  digest, opened_at, proven_at, isolation_mark
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

// SQLInsertRevocation 落吊销。
const SQLInsertRevocation = `
INSERT INTO balproof_revocations (
  id, lease_id, user_id, book, asset, nonce, reason, operator, revoked_at, propagated
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

// SQLSelectLeaseByNonce 幂等查找。
const SQLSelectLeaseByNonce = `
SELECT id FROM balproof_leases
WHERE user_id = ? AND book = ? AND asset = ? AND nonce = ?
ORDER BY issued_at DESC LIMIT 1`

// RenderNamed 将 ? 占位替换为命名注释，供文档导出，不拼用户输入。

  func RenderNamed(sql string, names []string) string {
	   out := sql
	   for _, n := range names {
		   out = strings.Replace(out, "?", "/*"+n+"*/?", 1)
	  }
	   return out
   }

// IsolationSQLGuard 文档断言：迁移不得创建 por_/stmt_ 列。

   func IsolationSQLGuard(ddl string) error {
	   low := strings.ToLower(ddl)
	   if strings.Contains(low, "por_root") || strings.Contains(low, "stmt_manifest") {
		  return ErrIsolationViolation
	  }
	   return nil
  }
