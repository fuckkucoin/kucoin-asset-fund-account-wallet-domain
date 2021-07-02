package forbidgate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// Repo 关系型仓储（MySQL / Postgres）。

type Repo struct {
	db *persist.SQLDB
}

// NewRepo 构造仓储。
   func NewRepo(db *persist.SQLDB) *Repo {
	   return &Repo{db: db}
   }

// ---- ForbidLease ----
// InsertForbidLease 写入禁提世代租约（首次或覆盖）。

  func (r *Repo) InsertForbidLease(ctx context.Context, l ForbidLease) error {
	  q := `INSERT INTO fundsurf_forbid_leases
	(id, user_id, generation, state, actor, reason, proof_id,
	 issued_at, switched_at, revoked_at, fence_epoch, fence_token)
	VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT DO NOTHING`
	  q = persist.Rewrite(q, r.db.Dialect)
	   _,
		   err := r.db.DB.ExecContext(ctx, q,
		  l.ID, l.UserID, l.Generation, string(l.State),
		  l.Actor, l.Reason, l.ProofID,
		  l.IssuedAt,
		   ptrTime(l.SwitchedAt), ptrTime(l.RevokedAt),
		   l.FenceEpoch, l.FenceToken,
	   )
	   return wrapErr("insert forbid_lease", err)
   }

// GetLatestForbidLease 读取用户最新的禁提世代租约。
  func (r *Repo) GetLatestForbidLease(ctx context.Context, userID string) (ForbidLease, error) {
	  q := `SELECT id, user_id, generation, state, actor, reason, proof_id,
	       issued_at, switched_at, revoked_at, fence_epoch, fence_token
	  FROM fundsurf_forbid_leases
	 WHERE user_id = ?
	 ORDER BY generation DESC
	 LIMIT 1`
	  q = persist.Rewrite(q,
		   r.db.Dialect)
	   row := r.db.DB.QueryRowContext(ctx, q, userID)
	  return scanForbidLease(row)
   }

// ListForbidLeasesByUser 列出用户所有租约（世代降序）。
   func (r *Repo) ListForbidLeasesByUser(ctx context.Context, userID string, limit int) ([]ForbidLease, error) {
	  if limit <= 0 {
		  limit = 20
	   }
	  q := `SELECT id, user_id, generation, state, actor, reason, proof_id,
	       issued_at, switched_at, revoked_at, fence_epoch, fence_token
	  FROM fundsurf_forbid_leases
	 WHERE user_id = ?
	 ORDER BY generation DESC
	 LIMIT ?`
	   q = persist.Rewrite(q, r.db.Dialect)
	  rows, err := r.db.DB.QueryContext(ctx, q, userID, limit)
	   if err != nil {
		   return nil, wrapErr("list forbid_leases", err)
	   }
	  defer rows.Close()
	   return scanForbidLeaseRows(rows)
   }

// ---- WdPrefightBind ----
// InsertWdPrefightBind 写入预检绑定。

  func (r *Repo) InsertWdPrefightBind(ctx context.Context, b WdPrefightBind) error {
	   q := `INSERT INTO fundsurf_wd_binds
	(id, user_id, withdraw_ref, generation, forbid_lease_id, state,
	 issued_at, expires_at, revoked_at, revoke_reason, consumed_at,
	 fence_epoch, fence_token)
	VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT DO NOTHING`
	   q = persist.Rewrite(q, r.db.Dialect)
	  _, err := r.db.DB.ExecContext(ctx, q,
		   b.ID, b.UserID, b.WithdrawRef, b.Generation,
		   b.ForbidLeaseID, string(b.State),
		   b.IssuedAt, b.ExpiresAt,
		  ptrTime(b.RevokedAt), b.RevokeReason, ptrTime(b.ConsumedAt),
		   b.FenceEpoch, b.FenceToken,
	  )
	  return wrapErr("insert wd_bind", err)
  }

// GetWdPrefightBind 读取单个预检绑定。

   func (r *Repo) GetWdPrefightBind(ctx context.Context, bindID string) (WdPrefightBind, error) {
	   q := `SELECT id, user_id, withdraw_ref, generation, forbid_lease_id, state,
	       issued_at, expires_at, revoked_at, revoke_reason, consumed_at,
	       fence_epoch, fence_token
	  FROM fundsurf_wd_binds
	 WHERE id = ?`
	   q = persist.Rewrite(q,
		  r.db.Dialect)
	  row := r.db.DB.QueryRowContext(ctx, q, bindID)
	  return scanWdBind(row)
  }

// ListOpenBindsByUser 列出用户 open 状态绑定（用于吊销泵）。

   func (r *Repo) ListOpenBindsByUser(ctx context.Context, userID string) ([]WdPrefightBind, error) {
	  q := `SELECT id, user_id, withdraw_ref, generation, forbid_lease_id, state,
	       issued_at, expires_at, revoked_at, revoke_reason, consumed_at,
	       fence_epoch, fence_token
	  FROM fundsurf_wd_binds
	 WHERE user_id = ? AND state = 'open'
	 ORDER BY issued_at ASC`
	   q = persist.Rewrite(q, r.db.Dialect)
	   rows, err := r.db.DB.QueryContext(ctx, q, userID)
	   if err != nil {
		  return nil, wrapErr("list open wd_binds", err)
	   }
	  defer rows.Close()
	   return scanWdBindRows(rows)
  }

// UpdateWdBindState 更新预检绑定状态（原子 CAS 用 fence_epoch）。
  func (r *Repo) UpdateWdBindState(ctx context.Context, b WdPrefightBind) error {
	  q := `UPDATE fundsurf_wd_binds 
	   SET state=?, revoked_at=?, revoke_reason=?, consumed_at=?,
	       fence_epoch=?, fence_token=?
	 WHERE id=? AND fence_epoch=?`
	  q = persist.Rewrite(q, r.db.Dialect)
	   _, err := r.db.DB.ExecContext(ctx, q,
		  string(b.State), ptrTime(b.RevokedAt), b.RevokeReason, ptrTime(b.ConsumedAt),
		   b.FenceEpoch+1, persist.NewToken(),
		  b.ID, b.FenceEpoch,
	  )
	  return wrapErr("update wd_bind state", err)
   }

// ListOpenBindsByGenLT 列出世代小于 gen 的 open 绑定（跨用户，供吊销泵全量扫）。
   func (r *Repo) ListOpenBindsByGenLT(ctx context.Context, userID string, gen int64, limit int) ([]WdPrefightBind, error) {
	   if limit <= 0 {
		   limit = 100
	   }
	   q := `SELECT id, user_id, withdraw_ref, generation, forbid_lease_id, state, 
	       issued_at, expires_at, revoked_at, revoke_reason, consumed_at,
	       fence_epoch, fence_token
	  FROM fundsurf_wd_binds
	 WHERE user_id=? AND state='open' AND generation < ?
	 ORDER BY issued_at ASC
	 LIMIT ?`
	  q = persist.Rewrite(q, r.db.Dialect)
	  rows, err := r.db.DB.QueryContext(ctx, q, userID, gen, limit)
	  if err != nil {
		   return nil, wrapErr("list open wd_binds by gen", err)
	   }
	  defer rows.Close()
	  return scanWdBindRows(rows)
   }

// ---- ForbidDriftCase ----
// InsertForbidDriftCase 写入漂移案例。

   func (r *Repo) InsertForbidDriftCase(ctx context.Context, dc ForbidDriftCase) error {
	   q := `INSERT INTO fundsurf_forbid_drift_cases
	(id, user_id, withdraw_ref, bind_id, forbid_lease_id, kind,
	 gen_at_precheck, gen_at_detect, state, detected_at, proven_at, closed_at,
	 proof_id, proof_summary, closed_by, fence_epoch, fence_token)
	VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT DO NOTHING`
	  q = persist.Rewrite(q, r.db.Dialect)
	   _, err := r.db.DB.ExecContext(ctx, q,
		   dc.ID, dc.UserID, dc.WithdrawRef, dc.BindID, dc.ForbidLeaseID,
		   string(dc.Kind), dc.GenAtPrecheck, dc.GenAtDetect,
		  string(dc.State), dc.DetectedAt,
		  ptrTime(dc.ProvenAt), ptrTime(dc.ClosedAt),
		  dc.ProofID, dc.ProofSummary, dc.ClosedBy,
		   dc.FenceEpoch, dc.FenceToken,
	  )
	   return wrapErr("insert forbid_drift_case", err)
  }

// GetDriftCase 读取单个漂移案例。
  func (r *Repo) GetDriftCase(ctx context.Context, caseID string) (ForbidDriftCase, error) {
	  q := `SELECT id, user_id, withdraw_ref, bind_id, forbid_lease_id, kind, 
	       gen_at_precheck, gen_at_detect, state, detected_at, proven_at, closed_at,
	       proof_id, proof_summary, closed_by, fence_epoch, fence_token
	  FROM fundsurf_forbid_drift_cases
	 WHERE id=?`
	   q = persist.Rewrite(q,
		   r.db.Dialect)
	  row := r.db.DB.QueryRowContext(ctx, q, caseID)
	   return scanDriftCase(row)
  }

// ListDriftCasesByUserState 列出用户指定状态的漂移案例。
   func (r *Repo) ListDriftCasesByUserState(ctx context.Context, userID string, state DriftState) ([]ForbidDriftCase, error) {
	   q := `SELECT id, user_id, withdraw_ref, bind_id, forbid_lease_id, kind,
	       gen_at_precheck, gen_at_detect, state, detected_at, proven_at, closed_at,
	       proof_id, proof_summary, closed_by, fence_epoch, fence_token
	  FROM fundsurf_forbid_drift_cases
	 WHERE user_id=? AND state=?
	 ORDER BY detected_at ASC`
	  q = persist.Rewrite(q, r.db.Dialect)
	  rows, err := r.db.DB.QueryContext(ctx, q, userID, string(state))
	   if err != nil {
		  return nil, wrapErr("list drift cases", err)
	  }
	   defer rows.Close()
	  return scanDriftCaseRows(rows)
   }

// UpdateDriftCaseState 更新漂移案例状态（含证明字段）。
   func (r *Repo) UpdateDriftCaseState(ctx context.Context, dc ForbidDriftCase) error {
	  q := `UPDATE fundsurf_forbid_drift_cases
	   SET state=?, proven_at=?, closed_at=?,
	       proof_id=?, proof_summary=?, closed_by=?
	 WHERE id=?`
	   q = persist.Rewrite(q, r.db.Dialect)
	  _, err := r.db.DB.ExecContext(ctx, q,
		   string(dc.State),
		   ptrTime(dc.ProvenAt), ptrTime(dc.ClosedAt),
		   dc.ProofID, dc.ProofSummary, dc.ClosedBy,
		  dc.ID,
	  )
	   return wrapErr("update drift_case state", err)
   }

// ---- ForbidDriftProof ----
// InsertDriftProof 写入漂移证明。

  func (r *Repo) InsertDriftProof(ctx context.Context, p ForbidDriftProof) error {
	   q := `INSERT INTO fundsurf_forbid_drift_proofs 
	(id, drift_case_id, submitted_by, confirmed_by, summary, evidence, submitted_at, confirmed_at)
	VALUES (?,?,?,?,?,?,?,?)
	ON CONFLICT DO NOTHING`
	  q = persist.Rewrite(q, r.db.Dialect)
	  _, err := r.db.DB.ExecContext(ctx, q,
		   p.ID, p.DriftCaseID, p.SubmittedBy, p.ConfirmedBy,
		   p.Summary, p.Evidence, p.SubmittedAt, ptrTime(p.ConfirmedAt),
	   )
	  return wrapErr("insert drift_proof", err)
  }

// GetDriftProof 读取单个证明。

   func (r *Repo) GetDriftProof(ctx context.Context, proofID string) (ForbidDriftProof, error) {
	  q := `SELECT id, drift_case_id, submitted_by, confirmed_by, summary, evidence, submitted_at, confirmed_at
	  FROM fundsurf_forbid_drift_proofs
	 WHERE id=?`
	  q = persist.Rewrite(q, r.db.Dialect)
	   row := r.db.DB.QueryRowContext(ctx, q, proofID)
	  return scanDriftProof(row)
   }

// UpdateDriftProofConfirmed 确认证明（第二人）。
   func (r *Repo) UpdateDriftProofConfirmed(ctx context.Context, p ForbidDriftProof) error {
	   q := `UPDATE fundsurf_forbid_drift_proofs
	   SET confirmed_by=?, confirmed_at=?
	 WHERE id=?`
	   q = persist.Rewrite(q, r.db.Dialect)
	  _, err := r.db.DB.ExecContext(ctx, q,
		  p.ConfirmedBy,
		   ptrTime(p.ConfirmedAt), p.ID,
	  )
	  return wrapErr("update drift_proof confirmed", err)
  }

// ---- scan helpers ----

   func scanForbidLease(row *sql.Row) (ForbidLease, error) {
	   var l ForbidLease
	   var state string
	   var switchedAt, revokedAt sql.NullTime
	   err := row.Scan(&l.ID,
		   &l.UserID, &l.Generation, &state,
		  &l.Actor,
		   &l.Reason, &l.ProofID,
		  &l.IssuedAt, &switchedAt, &revokedAt,
		  &l.FenceEpoch, &l.FenceToken,
	  )
	   if errors.Is(err, sql.ErrNoRows) {
		  return ForbidLease{}, persist.ErrNotFound
	  }
	   if err != nil {
		   return ForbidLease{}, wrapErr("scan forbid_lease", err)
	  }
	  l.State = ForbidState(state)
	  if switchedAt.Valid {
		  t := switchedAt.Time
		   l.SwitchedAt = &t
	   }
	  if revokedAt.Valid {
		   t := revokedAt.Time
		   l.RevokedAt = &t
	  }
	   return l, nil
   }

  func scanForbidLeaseRows(rows *sql.Rows) ([]ForbidLease, error) {
	   var out []ForbidLease
	   for rows.Next() {
		   var l ForbidLease
		  var state string
		   var switchedAt, revokedAt sql.NullTime
		  err := rows.Scan(&l.ID, &l.UserID, &l.Generation, &state,
			   &l.Actor, &l.Reason, &l.ProofID,
			  &l.IssuedAt,
			   &switchedAt, &revokedAt,
			  &l.FenceEpoch, &l.FenceToken,
		  )
		   if err != nil {
			   return nil, wrapErr("scan forbid_lease rows", err)
		  }
		   l.State = ForbidState(state)
		   if switchedAt.Valid {
			  t := switchedAt.Time
			   l.SwitchedAt = &t
		   }
		   if revokedAt.Valid {
			   t := revokedAt.Time
			  l.RevokedAt = &t
		   }
		  out = append(out, l)
	  }
	   return out, rows.Err()
  }

   func scanWdBind(row *sql.Row) (WdPrefightBind, error) {
	  var b WdPrefightBind
	   var state string
	  var revokedAt,
		  consumedAt sql.NullTime
	  err := row.Scan(&b.ID, &b.UserID, &b.WithdrawRef, &b.Generation, &b.ForbidLeaseID, &state,
		  &b.IssuedAt, &b.ExpiresAt, &revokedAt, &b.RevokeReason, &consumedAt,
		   &b.FenceEpoch, &b.FenceToken,
	   )
	  if errors.Is(err, sql.ErrNoRows) {
		   return WdPrefightBind{}, persist.ErrNotFound
	  }
	   if err != nil {
		  return WdPrefightBind{}, wrapErr("scan wd_bind", err)
	  }
	   b.State = BindState(state)
	   if revokedAt.Valid {
		   t := revokedAt.Time
		   b.RevokedAt = &t
	   }
	  if consumedAt.Valid {
		  t := consumedAt.Time
		   b.ConsumedAt = &t
	   }
	   return b, nil
   }

  func scanWdBindRows(rows *sql.Rows) ([]WdPrefightBind, error) {
	   var out []WdPrefightBind
	   for rows.Next() {
		   var b WdPrefightBind
		   var state string
		   var revokedAt, consumedAt sql.NullTime
		  err := rows.Scan(&b.ID, &b.UserID, &b.WithdrawRef, &b.Generation, &b.ForbidLeaseID, &state,
			  &b.IssuedAt, &b.ExpiresAt, &revokedAt, &b.RevokeReason, &consumedAt,
			   &b.FenceEpoch, &b.FenceToken,
		   )
		   if err != nil {
			   return nil, wrapErr("scan wd_bind rows", err)
		  }
		  b.State = BindState(state)
		  if revokedAt.Valid {
			  t := revokedAt.Time
			  b.RevokedAt = &t
		  }
		  if consumedAt.Valid {
			  t := consumedAt.Time
			   b.ConsumedAt = &t
		   }
		  out = append(out, b)
	  }
	   return out, rows.Err()
   }

  func scanDriftCase(row *sql.Row) (ForbidDriftCase, error) {
	  var dc ForbidDriftCase
	   var kind, state string
	   var provenAt, closedAt sql.NullTime
	   err := row.Scan(
		  &dc.ID, &dc.UserID, &dc.WithdrawRef, &dc.BindID, &dc.ForbidLeaseID,
		  &kind, &dc.GenAtPrecheck, &dc.GenAtDetect,
		   &state, &dc.DetectedAt, &provenAt, &closedAt,
		  &dc.ProofID, &dc.ProofSummary, &dc.ClosedBy,
		   &dc.FenceEpoch, &dc.FenceToken,
	   )
	   if errors.Is(err, sql.ErrNoRows) {
		  return ForbidDriftCase{}, persist.ErrNotFound
	   }
	  if err != nil {
		  return ForbidDriftCase{}, wrapErr("scan drift_case", err)
	  }
	  dc.Kind = DriftKind(kind)
	  dc.State = DriftState(state)
	  if provenAt.Valid {
		   t := provenAt.Time
		  dc.ProvenAt = &t
	  }
	  if closedAt.Valid {
		  t := closedAt.Time
		   dc.ClosedAt = &t
	  }
	  return dc, nil
  }

   func scanDriftCaseRows(rows *sql.Rows) ([]ForbidDriftCase, error) {
	  var out []ForbidDriftCase
	   for rows.Next() {
		   var dc ForbidDriftCase
		  var kind, state string
		   var provenAt, closedAt sql.NullTime
		   err := rows.Scan(
			   &dc.ID, &dc.UserID, &dc.WithdrawRef, &dc.BindID, &dc.ForbidLeaseID,
			   &kind, &dc.GenAtPrecheck, &dc.GenAtDetect,
			   &state, &dc.DetectedAt, &provenAt, &closedAt,
			  &dc.ProofID, &dc.ProofSummary, &dc.ClosedBy,
			   &dc.FenceEpoch, &dc.FenceToken,
		  )
		  if err != nil {
			  return nil, wrapErr("scan drift_case rows", err)
		  }
		   dc.Kind = DriftKind(kind)
		   dc.State = DriftState(state)
		  if provenAt.Valid {
			   t := provenAt.Time
			   dc.ProvenAt = &t
		   }
		  if closedAt.Valid {
			   t := closedAt.Time
			  dc.ClosedAt = &t
		   }
		  out = append(out, dc)
	   }
	  return out, rows.Err()
   }

   func scanDriftProof(row *sql.Row) (ForbidDriftProof, error) {
	   var p ForbidDriftProof
	   var confirmedAt sql.NullTime
	  err := row.Scan(&p.ID, &p.DriftCaseID, &p.SubmittedBy, &p.ConfirmedBy,
		  &p.Summary, &p.Evidence, &p.SubmittedAt, &confirmedAt,
	  )
	   if errors.Is(err, sql.ErrNoRows) {
		  return ForbidDriftProof{}, persist.ErrNotFound
	  }
	   if err != nil {
		   return ForbidDriftProof{}, wrapErr("scan drift_proof", err)
	  }
	   if confirmedAt.Valid {
		   t := confirmedAt.Time
		   p.ConfirmedAt = &t
	  }
	   return p, nil
   }

// ---- helpers ----

  func ptrTime(t *time.Time) any {
	  if t == nil {
		  return nil
	  }
	  return *t
   }

  func wrapErr(op string, err error) error {
	   if err == nil {
		  return nil
	  }
	   return fmt.Errorf("forbidgate repo %s: %w", op, err)
   }
