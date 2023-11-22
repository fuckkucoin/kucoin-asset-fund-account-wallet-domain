package legmovefence

import (
	"context"
	"database/sql"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// SQLStore 基于 persist.SQLDB。

type SQLStore struct {
	db *persist.SQLDB
}

  func NewSQLStore(db *persist.SQLDB) *SQLStore {
	  if db == nil {
		  panic("legmovefence: SQLStore requires non-nil SQLDB")
	  }
	   return &SQLStore{db: db}
  }

   func (s *SQLStore) q(ctx context.Context) persist.Queryer {
	   return s.db.Conn(ctx)
  }

const fenceCols = `fence_id, plan_id, leg_id, idempotency_key, src_region, dst_region, asset, amount,
		fingerprint, generation, state, ttl_seconds, issued_at, enqueued_at, moved_at, expired_at,
		revoked_at, revoke_reason, intent_ref, transfer_ref, move_count, created_at, updated_at`

   func (s *SQLStore) SaveFence(ctx context.Context, f LegMoveFence) error {
	   const q = `INSERT INTO georebal_leg_move_fences (` + fenceCols + `) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	   _, err := s.q(ctx).ExecContext(ctx, q,
		   f.FenceID,
		  f.PlanID, f.LegID, f.IdempotencyKey, f.SrcRegion, f.DstRegion, f.Asset, f.Amount,
		  f.Fingerprint, f.Generation, string(f.State), f.TTLSeconds, f.IssuedAt, nullTime(f.EnqueuedAt),
		   nullTime(f.MovedAt), nullTime(f.ExpiredAt), nullTime(f.RevokedAt), f.RevokeReason,
		   f.IntentRef, f.TransferRef, f.MoveCount, f.CreatedAt, f.UpdatedAt,
	  )
	   if err != nil {
		   return WrapStore("save_fence", err)
	   }
	   return nil
  }

   func (s *SQLStore) GetFence(ctx context.Context, fenceID string) (LegMoveFence, error) {
	   row := s.q(ctx).QueryRowContext(ctx, `SELECT `+fenceCols+` FROM georebal_leg_move_fences WHERE fence_id=?`, fenceID)
	  f, err := scanFence(row)
	   if err == sql.ErrNoRows {
		  return LegMoveFence{}, ErrFenceNotFound
	   }
	   if err != nil {
		   return LegMoveFence{}, WrapStore("get_fence", err)
	   }
	  return f, nil
  }

  func (s *SQLStore) GetFenceByIdem(ctx context.Context, key string) (LegMoveFence, bool, error) {
	   row := s.q(ctx).QueryRowContext(ctx, `SELECT `+fenceCols+` FROM georebal_leg_move_fences WHERE idempotency_key=? LIMIT 1`, key)
	   f, err := scanFence(row)
	   if err == sql.ErrNoRows {
		  return LegMoveFence{}, false, nil
	   }
	  if err != nil {
		   return LegMoveFence{}, false, WrapStore("get_fence_idem", err)
	   }
	   return f, true, nil
  }

  func (s *SQLStore) GetFenceByLeg(ctx context.Context, planID, legID string) (LegMoveFence, bool, error) {
	  row := s.q(ctx).QueryRowContext(ctx, `SELECT `+fenceCols+` FROM georebal_leg_move_fences WHERE plan_id=? AND leg_id=? LIMIT 1`, planID, legID)
	   f, err := scanFence(row)
	   if err == sql.ErrNoRows {
		  return LegMoveFence{}, false, nil
	   }
	  if err != nil {
		   return LegMoveFence{}, false, WrapStore("get_fence_leg", err)
	  }
	   return f, true, nil
   }

   func (s *SQLStore) UpdateFence(ctx context.Context, f LegMoveFence) error {
	  const q = `UPDATE georebal_leg_move_fences SET plan_id=?, leg_id=?, idempotency_key=?, src_region=?,
		dst_region=?, asset=?, amount=?, fingerprint=?, generation=?, state=?, ttl_seconds=?, issued_at=?,
		enqueued_at=?, moved_at=?, expired_at=?, revoked_at=?, revoke_reason=?, intent_ref=?, transfer_ref=?,
		move_count=?, updated_at=? WHERE fence_id=?`
	  res, err := s.q(ctx).ExecContext(ctx, q,
		   f.PlanID, f.LegID, f.IdempotencyKey, f.SrcRegion, f.DstRegion, f.Asset, f.Amount, f.Fingerprint,
		   f.Generation, string(f.State), f.TTLSeconds, f.IssuedAt, nullTime(f.EnqueuedAt), nullTime(f.MovedAt),
		   nullTime(f.ExpiredAt), nullTime(f.RevokedAt), f.RevokeReason, f.IntentRef, f.TransferRef,
		  f.MoveCount, f.UpdatedAt, f.FenceID,
	  )
	   if err != nil {
		  return WrapStore("update_fence", err)
	  }
	   n, _ := res.RowsAffected()
	  if n == 0 {
		   return ErrFenceNotFound
	   }
	  return nil
   }

   func (s *SQLStore) ListFencesByState(ctx context.Context, state FenceState, limit int) ([]LegMoveFence, error) {
	   if limit <= 0 {
		  limit = 200
	  }
	   rows, err := s.q(ctx).QueryContext(ctx, `SELECT `+fenceCols+` FROM georebal_leg_move_fences WHERE state=? LIMIT ?`, string(state), limit)
	  if err != nil {
		  return nil, WrapStore("list_fence_state", err)
	   }
	   defer rows.Close()
	  return scanFences(rows)
  }

  func (s *SQLStore) ListExpiredActive(ctx context.Context, now time.Time, limit int) ([]LegMoveFence, error) {
	   if limit <= 0 {
		   limit = 200
	   }
	   rows, err := s.q(ctx).QueryContext(ctx,
		   `SELECT `+fenceCols+` FROM georebal_leg_move_fences WHERE state IN ('issued','enqueued') AND DATE_ADD(issued_at, INTERVAL ttl_seconds SECOND) < ? LIMIT ?`,
		  now, limit)
	  if err != nil {
		   return nil, WrapStore("list_expired", err)
	  }
	  defer rows.Close()
	   return scanFences(rows)
   }

  func (s *SQLStore) ListByRegion(ctx context.Context, regionID string, limit int) ([]LegMoveFence, error) {
	  if limit <= 0 {
		   limit = 200
	  }
	  rows,
		  err := s.q(ctx).QueryContext(ctx,
		   `SELECT `+fenceCols+` FROM georebal_leg_move_fences WHERE src_region=? OR dst_region=? LIMIT ?`,
		   regionID, regionID, limit)
	   if err != nil {
		   return nil, WrapStore("list_region", err)
	   }
	  defer rows.Close()
	  return scanFences(rows)
  }

const proofCols = `proof_id, kind, fence_id, plan_id, leg_id, idempotency_key, src_region, dst_region, 
		amount, bind_fingerprint, live_fingerprint, bind_gen, live_gen, evidence, detected_at, exported_at, created_at`

   func (s *SQLStore) SaveProof(ctx context.Context, p GeoDoubleMoveProof) error {
	   const q = `INSERT INTO georebal_double_move_proofs (` + proofCols + `) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	   _, err := s.q(ctx).ExecContext(ctx, q,
		   p.ProofID, string(p.Kind), p.FenceID, p.PlanID, p.LegID, p.IdemKey, p.SrcRegion, p.DstRegion,
		  p.Amount, p.BindFP, p.LiveFP, p.BindGen, p.LiveGen, p.Evidence, p.DetectedAt, nullTime(p.ExportedAt), p.CreatedAt,
	  )
	   if err != nil {
		  return WrapStore("save_proof", err)
	  }
	   return nil
   }

  func (s *SQLStore) GetProof(ctx context.Context, proofID string) (GeoDoubleMoveProof, error) {
	   row := s.q(ctx).QueryRowContext(ctx, `SELECT `+proofCols+` FROM georebal_double_move_proofs WHERE proof_id=?`, proofID)
	   p, err := scanProof(row)
	  if err == sql.ErrNoRows {
		   return GeoDoubleMoveProof{}, ErrProofNotFound
	   }
	   if err != nil {
		  return GeoDoubleMoveProof{}, WrapStore("get_proof", err)
	   }
	   return p, nil
  }

  func (s *SQLStore) ListProofs(ctx context.Context, filter ExportFilter) ([]GeoDoubleMoveProof, error) {
	   limit := filter.Limit
	   if limit <= 0 {
		  limit = 200
	   }
	   q := `SELECT ` + proofCols + ` FROM georebal_double_move_proofs WHERE 1=1`
	  args := make([]any, 0)
	   if filter.Kind != "" {
		  q += ` AND kind=?`
		   args = append(args, string(filter.Kind))
	  }
	   if filter.FenceID != "" {
		   q += ` AND fence_id=?`
		  args = append(args, filter.FenceID)
	   }
	  if filter.LegID != "" {
		  q += ` AND leg_id=?`
		  args = append(args, filter.LegID)
	   }
	  if filter.UnexportedOnly {
		   q += ` AND exported_at IS NULL`
	  }
	  q += ` LIMIT ?`
	  args = append(args, limit)
	   rows,
		   err := s.q(ctx).QueryContext(ctx, q, args...)
	   if err != nil {
		   return nil, WrapStore("list_proofs", err)
	   }
	   defer rows.Close()
	   out := make([]GeoDoubleMoveProof, 0)
	   for rows.Next() {
		  p, err := scanProof(rows)
		   if err != nil {
			   return nil, err
		   }
		   out = append(out, p)
	  }
	   return out, rows.Err()
   }

   func (s *SQLStore) MarkProofsExported(ctx context.Context, ids []string, at time.Time) error {
	  for _, id := range ids {
		   _, err := s.q(ctx).ExecContext(ctx, `UPDATE georebal_double_move_proofs SET exported_at=? WHERE proof_id=?`, at, id)
		  if err != nil {
			   return WrapStore("export_proof", err)
		  }
	   }
	  return nil
  }

const halfCols = `case_id, kind, fence_id, plan_id, leg_id, state, resume_lock, locked_at, reason,
		opened_at, closed_at, actor, created_at, updated_at`

   func (s *SQLStore) SaveHalf(ctx context.Context, c HalfMoveCase) error {
	  const q = `INSERT INTO georebal_half_move_cases (` + halfCols + `) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	  _,
		  err := s.q(ctx).ExecContext(ctx, q,
		  c.CaseID, string(c.Kind), c.FenceID, c.PlanID, c.LegID, string(c.State), c.ResumeLock,
		   nullTime(c.LockedAt), c.Reason, c.OpenedAt, nullTime(c.ClosedAt), c.Actor, c.CreatedAt, c.UpdatedAt,
	  )
	  if err != nil {
		   return WrapStore("save_half", err)
	   }
	   return nil
   }

   func (s *SQLStore) GetHalf(ctx context.Context, caseID string) (HalfMoveCase, error) {
	  row := s.q(ctx).QueryRowContext(ctx, `SELECT `+halfCols+` FROM georebal_half_move_cases WHERE case_id=?`, caseID)
	   c, err := scanHalf(row)
	   if err == sql.ErrNoRows {
		   return HalfMoveCase{}, ErrHalfNotFound
	  }
	  if err != nil {
		   return HalfMoveCase{}, WrapStore("get_half", err)
	   }
	   return c, nil
  }

  func (s *SQLStore) GetOpenHalfByFence(ctx context.Context, fenceID string) (HalfMoveCase, bool, error) {
	  row := s.q(ctx).QueryRowContext(ctx, `SELECT `+halfCols+` FROM georebal_half_move_cases WHERE fence_id=? AND state='open' LIMIT 1`, fenceID)
	  c, err := scanHalf(row)
	   if err == sql.ErrNoRows {
		  return HalfMoveCase{}, false, nil
	   }
	  if err != nil {
		   return HalfMoveCase{}, false, WrapStore("get_open_half", err)
	  }
	  return c, true, nil
  }

   func (s *SQLStore) UpdateHalf(ctx context.Context, c HalfMoveCase) error {
	  const q = `UPDATE georebal_half_move_cases SET kind=?, fence_id=?, plan_id=?, leg_id=?, state=?,
		resume_lock=?, locked_at=?, reason=?, opened_at=?, closed_at=?, actor=?, updated_at=? WHERE case_id=?`
	   res, err := s.q(ctx).ExecContext(ctx, q,
		  string(c.Kind), c.FenceID, c.PlanID, c.LegID, string(c.State), c.ResumeLock, nullTime(c.LockedAt),
		  c.Reason, c.OpenedAt, nullTime(c.ClosedAt), c.Actor, c.UpdatedAt, c.CaseID,
	   )
	   if err != nil {
		   return WrapStore("update_half", err)
	  }
	   n, _ := res.RowsAffected()
	  if n == 0 {
		  return ErrHalfNotFound
	  }
	  return nil
  }

  func (s *SQLStore) ListOpenHalf(ctx context.Context, limit int) ([]HalfMoveCase, error) {
	   if limit <= 0 {
		  limit = 200
	  }
	  rows, err := s.q(ctx).QueryContext(ctx, `SELECT `+halfCols+` FROM georebal_half_move_cases WHERE state='open' LIMIT ?`, limit)
	  if err != nil {
		  return nil, WrapStore("list_half", err)
	  }
	   defer rows.Close()
	  out := make([]HalfMoveCase, 0)
	   for rows.Next() {
		   c, err := scanHalf(rows)
		  if err != nil {
			   return nil, err
		  }
		   out = append(out, c)
	   }
	  return out, rows.Err()
   }

  func (s *SQLStore) SaveAudit(ctx context.Context, a FenceAudit) error {
	  const q = `INSERT INTO georebal_leg_move_audit (audit_id, fence_id, plan_id, leg_id, action, actor, detail, created_at)
		VALUES (?,?,?,?,?,?,?,?)`
	  _, err := s.q(ctx).ExecContext(ctx, q, a.AuditID, a.FenceID, a.PlanID, a.LegID, string(a.Action), a.Actor, a.Detail, a.CreatedAt)
	  if err != nil {
		  return WrapStore("save_audit", err)
	  }
	   return nil
   }

   func (s *SQLStore) ListAudit(ctx context.Context, fenceID string, limit int) ([]FenceAudit, error) {
	   if limit <= 0 {
		  limit = 100
	   }
	  rows, err := s.q(ctx).QueryContext(ctx,
		  `SELECT audit_id, fence_id, plan_id, leg_id, action, actor, detail, created_at FROM georebal_leg_move_audit WHERE fence_id=? ORDER BY created_at DESC LIMIT ?`,
		   fenceID, limit)
	  if err != nil {
		  return nil, WrapStore("list_audit", err)
	   }
	  defer rows.Close()
	   out := make([]FenceAudit, 0)
	  for rows.Next() {
		  var a FenceAudit
		  var act string
		   if err := rows.Scan(&a.AuditID, &a.FenceID, &a.PlanID, &a.LegID, &act, &a.Actor, &a.Detail, &a.CreatedAt); err != nil {
			   return nil, err
		   }
		  a.Action = AuditAction(act)
		  out = append(out, a)
	   }
	  return out, rows.Err()
  }

type rowScanner interface {
	Scan(dest ...any) error
}

   func scanFence(sc rowScanner) (LegMoveFence, error) {
	  var f LegMoveFence
	  var state string
	   var enq, mov, exp, rev sql.NullTime
	  err := sc.Scan(&f.FenceID, &f.PlanID, &f.LegID, &f.IdempotencyKey, &f.SrcRegion, &f.DstRegion, &f.Asset, &f.Amount,
		   &f.Fingerprint,
		  &f.Generation, &state, &f.TTLSeconds, &f.IssuedAt, &enq, &mov, &exp, &rev,
		   &f.RevokeReason, &f.IntentRef, &f.TransferRef, &f.MoveCount, &f.CreatedAt, &f.UpdatedAt)
	  if err != nil {
		  return LegMoveFence{}, err
	   }
	   f.State = FenceState(state)
	  f.EnqueuedAt = ntime(enq)
	  f.MovedAt = ntime(mov)
	   f.ExpiredAt = ntime(exp)
	   f.RevokedAt = ntime(rev)
	   return f, nil
  }

  func scanFences(rows *sql.Rows) ([]LegMoveFence, error) {
	  out := make([]LegMoveFence, 0)
	   for rows.Next() {
		   f, err := scanFence(rows)
		  if err != nil {
			   return nil, err
		  }
		  out = append(out, f)
	   }
	  return out, rows.Err()
   }

  func scanProof(sc rowScanner) (GeoDoubleMoveProof, error) {
	  var p GeoDoubleMoveProof
	  var kind string
	  var exp sql.NullTime
	  err := sc.Scan(&p.ProofID,
		   &kind, &p.FenceID, &p.PlanID, &p.LegID, &p.IdemKey, &p.SrcRegion, &p.DstRegion,
		  &p.Amount, &p.BindFP, &p.LiveFP, &p.BindGen, &p.LiveGen, &p.Evidence, &p.DetectedAt, &exp, &p.CreatedAt)
	   if err != nil {
		  return GeoDoubleMoveProof{}, err
	   }
	   p.Kind = ProofKind(kind)
	  p.ExportedAt = ntime(exp)
	   return p, nil
  }

   func scanHalf(sc rowScanner) (HalfMoveCase, error) {
	   var c HalfMoveCase
	   var kind, state string
	  var locked, closed sql.NullTime
	   err := sc.Scan(&c.CaseID, &kind, &c.FenceID, &c.PlanID, &c.LegID, &state, &c.ResumeLock, &locked, &c.Reason,
		   &c.OpenedAt,
		   &closed, &c.Actor, &c.CreatedAt, &c.UpdatedAt)
	   if err != nil {
		   return HalfMoveCase{}, err
	  }
	  c.Kind = HalfKind(kind)
	  c.State = HalfState(state)
	   c.LockedAt = ntime(locked)
	   c.ClosedAt = ntime(closed)
	   return c, nil
   }

   func nullTime(t *time.Time) any {
	   if t == nil || t.IsZero() {
		   return nil
	  }
	  return *t
   }

  func ntime(n sql.NullTime) *time.Time {
	  if !n.Valid {
		   return nil
	  }
	   t := n.Time
	   return &t
  }
