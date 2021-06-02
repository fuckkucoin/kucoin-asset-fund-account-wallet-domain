package forbidgate

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ListRecentActiveForbidUsers 列出近期有禁提操作的用户 ID（供吊销泵使用）。

   func (r *Repo) ListRecentActiveForbidUsers(ctx context.Context, limit int) ([]string, error) {
	  if limit <= 0 {
		  limit = 100
	  }
	  q := `SELECT DISTINCT user_id
	  FROM fundsurf_forbid_leases
	 ORDER BY issued_at DESC
	 LIMIT ?`
	  q = persist.Rewrite(q, r.db.Dialect)
	   rows, err := r.db.DB.QueryContext(ctx, q, limit)
	  if err != nil {
		  return nil, wrapErr("list recent active forbid users", err)
	   }
	   defer rows.Close()
	  var out []string
	  for rows.Next() {
		  var uid string
		   if err := rows.Scan(&uid); err != nil {
			   return nil, wrapErr("scan active forbid user", err)
		  }
		   out = append(out, uid)
	   }
	  return out, rows.Err()
  }

// ListExpiredOpenBinds 列出已过期的 open 预检绑定（供 BindScanWorker 使用）。
   func (r *Repo) ListExpiredOpenBinds(ctx context.Context, now time.Time, limit int) ([]WdPrefightBind, error) {
	   if limit <= 0 {
		  limit = 200
	  }
	  q := `SELECT id, user_id, withdraw_ref, generation, forbid_lease_id, state, 
	       issued_at, expires_at, revoked_at, revoke_reason, consumed_at,
	       fence_epoch, fence_token
	  FROM fundsurf_wd_binds
	 WHERE state='open' AND expires_at < ?
	 ORDER BY expires_at ASC
	 LIMIT ?`
	   q = persist.Rewrite(q, r.db.Dialect)
	   rows, err := r.db.DB.QueryContext(ctx, q, now, limit)
	  if err != nil {
		   return nil, wrapErr("list expired open binds", err)
	  }
	   defer rows.Close()
	  return scanWdBindRows(rows)
  }

// ListAllOpenDriftCases 全量列出 open/proven 状态漂移案例（供 DriftPatrolWorker 使用）。

  func (r *Repo) ListAllOpenDriftCases(ctx context.Context, limit int) ([]ForbidDriftCase, error) {
	   if limit <= 0 {
		   limit = 200
	  }
	   q := `SELECT id, user_id, withdraw_ref, bind_id, forbid_lease_id, kind,
	       gen_at_precheck, gen_at_detect, state, detected_at, proven_at, closed_at,
	       proof_id, proof_summary, closed_by, fence_epoch, fence_token
	  FROM fundsurf_forbid_drift_cases
	 WHERE state IN ('open','proven')
	 ORDER BY detected_at ASC
	 LIMIT ?`
	   q = persist.Rewrite(q, r.db.Dialect)
	  rows, err := r.db.DB.QueryContext(ctx, q, limit)
	   if err != nil {
		   return nil, wrapErr("list all open drift cases", err)
	   }
	  defer rows.Close()
	  return scanDriftCaseRows(rows)
  }

// ListDriftCasesByWithdrawRef 按出金 ref 查漂移案例（供广播前检查）。
  func (r *Repo) ListDriftCasesByWithdrawRef(ctx context.Context, withdrawRef string) ([]ForbidDriftCase, error) {
	   q := `SELECT id, user_id, withdraw_ref, bind_id, forbid_lease_id, kind,
	       gen_at_precheck, gen_at_detect, state, detected_at, proven_at, closed_at,
	       proof_id, proof_summary, closed_by, fence_epoch, fence_token
	  FROM fundsurf_forbid_drift_cases
	 WHERE withdraw_ref=?
	 ORDER BY detected_at ASC`
	   q = persist.Rewrite(q, r.db.Dialect)
	   rows, err := r.db.DB.QueryContext(ctx, q, withdrawRef)
	  if err != nil {
		   return nil, wrapErr("list drift cases by ref", err)
	   }
	   defer rows.Close()
	   return scanDriftCaseRows(rows)
  }

// ListDriftProofsByCase 列出某漂移案例的所有证明。

   func (r *Repo) ListDriftProofsByCase(ctx context.Context, caseID string) ([]ForbidDriftProof, error) {
	  q := `SELECT id, drift_case_id, submitted_by, confirmed_by, summary, evidence, submitted_at, confirmed_at
	  FROM fundsurf_forbid_drift_proofs
	 WHERE drift_case_id=?
	 ORDER BY submitted_at ASC`
	  q = persist.Rewrite(q, r.db.Dialect)
	   rows, err := r.db.DB.QueryContext(ctx, q, caseID)
	   if err != nil {
		  return nil, wrapErr("list drift proofs by case", err)
	   }
	   defer rows.Close()
	   var out []ForbidDriftProof
	  for rows.Next() {
		  var p ForbidDriftProof
		   var confirmedAt NullTime
		   if err := rows.Scan(&p.ID, &p.DriftCaseID, &p.SubmittedBy, &p.ConfirmedBy,
			   &p.Summary, &p.Evidence, &p.SubmittedAt, &confirmedAt,
		  ); err != nil {
			  return nil, wrapErr("scan drift_proof", err)
		   }
		  if confirmedAt.Valid {
			  t := confirmedAt.Time
			  p.ConfirmedAt = &t
		   }
		  out = append(out, p)
	  }
	  return out, rows.Err()
   }

// NullTime sql.NullTime 别名，避免 database/sql 循环依赖。
type NullTime struct {
	Time time.Time
	Valid bool
}

// Scan 实现 sql.Scanner。
  func (nt *NullTime) Scan(value any) error {
	   if value == nil {
		  nt.Valid = false
		  return nil
	  }
	   t, ok := value.(time.Time)
	   if !ok {
		  nt.Valid = false
		  return nil
	   }
	   nt.Time = t
	   nt.Valid = true
	  return nil
   }
