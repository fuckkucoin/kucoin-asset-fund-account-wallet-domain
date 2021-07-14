package guardlease

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// SQLStore SQL 仓储实现。

type SQLStore struct {
	db *persist.SQLDB
}

// NewSQLStore 创建 SQL 仓储，db 为已初始化的数据库连接池。
  func NewSQLStore(db *persist.SQLDB) *SQLStore {
	  return &SQLStore{db: db}
   }

// ---- SnapLease ----

   func (s *SQLStore) InsertLease(ctx context.Context, l SnapLease) error {
	   q := `INSERT INTO fg_snap_leases 
		(lease_id, generation, user_id, asset, snapshot_hash, state, issued_at, expires_at, actor_id)
		VALUES (?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE lease_id=lease_id`
	  _, err := s.db.Conn(ctx).ExecContext(ctx, q,
		  l.LeaseID,
		  l.Generation, l.UserID, l.Asset, l.SnapshotHash,
		  string(l.State), l.IssuedAt, l.ExpiresAt, l.ActorID,
	  )
	  return err
  }

   func (s *SQLStore) GetLease(ctx context.Context, leaseID string) (SnapLease, error) {
	   q := `SELECT lease_id, generation, user_id, asset, snapshot_hash, state,
		issued_at, expires_at, bound_at, consumed_at, actor_id
		FROM fg_snap_leases WHERE lease_id=? LIMIT 1`
	  row := s.db.Conn(ctx).QueryRowContext(ctx, q, leaseID)
	  return scanLease(row)
   }

  func scanLease(row *sql.Row) (SnapLease, error) {
	  var l SnapLease
	  var state string
	   var boundAt, consumedAt sql.NullTime
	  err := row.Scan(
		   &l.LeaseID, &l.Generation, &l.UserID, &l.Asset, &l.SnapshotHash,
		  &state, &l.IssuedAt, &l.ExpiresAt, &boundAt, &consumedAt, &l.ActorID,
	  )
	  if err == sql.ErrNoRows {
		   return SnapLease{},
			  ErrLeaseNotFound
	  }
	   if err != nil {
		   return SnapLease{}, err
	   }
	  l.State = LeaseState(state)
	   if boundAt.Valid {
		   l.BoundAt = &boundAt.Time
	  }
	  if consumedAt.Valid {
		   l.ConsumedAt = &consumedAt.Time
	  }
	   return l, nil
  }

   func (s *SQLStore) UpdateLeaseState(ctx context.Context, leaseID string, old, new LeaseState, extra map[string]any) error {
	   clauses := []string{"state=?"}
	  args := []any{string(new)}
	  if t, ok := extra["bound_at"]; ok {
		   clauses = append(clauses, "bound_at=?")
		  args = append(args, t)
	   }
	  if t, ok := extra["consumed_at"]; ok {
		  clauses = append(clauses, "consumed_at=?")
		  args = append(args, t)
	  }
	  args = append(args, leaseID, string(old))
	  q := fmt.Sprintf("UPDATE fg_snap_leases SET %s WHERE lease_id=? AND state=?", strings.Join(clauses, ","))
	   res,
		   err := s.db.Conn(ctx).ExecContext(ctx, q, args...)
	   if err != nil {
		   return err
	  }
	   n, _ := res.RowsAffected()
	  if n == 0 {
		  return ErrLeaseInvalidState
	   }
	   return nil
  }

   func (s *SQLStore) ListActiveLeases(ctx context.Context, limit int) ([]SnapLease, error) {
	  q := `SELECT lease_id, generation, user_id, asset, snapshot_hash, state,
		issued_at, expires_at, bound_at, consumed_at, actor_id
		FROM fg_snap_leases WHERE state='open' AND expires_at>? ORDER BY issued_at DESC LIMIT ?`
	  rows, err := s.db.Conn(ctx).QueryContext(ctx, q, time.Now().UTC(), limit)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  return scanLeases(rows)
  }

   func scanLeases(rows *sql.Rows) ([]SnapLease, error) {
	  var out []SnapLease
	  for rows.Next() {
		   var l SnapLease
		   var state string
		   var boundAt, consumedAt sql.NullTime
		   if err := rows.Scan(
			  &l.LeaseID, &l.Generation, &l.UserID, &l.Asset, &l.SnapshotHash,
			   &state, &l.IssuedAt, &l.ExpiresAt, &boundAt, &consumedAt, &l.ActorID,
		   ); err != nil {
			   return nil, err
		  }
		   l.State = LeaseState(state)
		  if boundAt.Valid {
			   l.BoundAt = &boundAt.Time
		   }
		  if consumedAt.Valid {
			  l.ConsumedAt = &consumedAt.Time
		  }
		  out = append(out, l)
	  }
	  return out, rows.Err()
  }

   func (s *SQLStore) MarkExpiredLeases(ctx context.Context) (int64, error) {
	  q := `UPDATE fg_snap_leases SET state='expired' WHERE state='open' AND expires_at<?`
	  res, err := s.db.Conn(ctx).ExecContext(ctx, q, time.Now().UTC())
	   if err != nil {
		   return 0, err
	   }
	  return res.RowsAffected()
   }

   func (s *SQLStore) GetOrInitGeneration(ctx context.Context, userID, asset string) (GuardGeneration, error) {
	  q := `INSERT INTO fg_guard_generations (user_id, asset, generation, updated_at) 
		VALUES (?,?,0,?) ON DUPLICATE KEY UPDATE user_id=user_id`
	  if _, err := s.db.Conn(ctx).ExecContext(ctx, q, userID, asset, time.Now().UTC()); err != nil {
		  return GuardGeneration{}, err
	   }
	   row := s.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT user_id, asset, generation, updated_at FROM fg_guard_generations WHERE user_id=? AND asset=?`,
		  userID, asset)
	  var g GuardGeneration
	  if err := row.Scan(&g.UserID, &g.Asset, &g.Generation, &g.UpdatedAt); err != nil {
		   return GuardGeneration{}, err
	  }
	   return g, nil
   }

   func (s *SQLStore) IncrGeneration(ctx context.Context, userID, asset string) (int64, error) {
	   now := time.Now().UTC()
	  q := `INSERT INTO fg_guard_generations (user_id, asset, generation, updated_at)
		VALUES (?,?,1,?) ON DUPLICATE KEY UPDATE generation=generation+1, updated_at=?`
	  if _, err := s.db.Conn(ctx).ExecContext(ctx, q, userID, asset, now, now); err != nil {
		   return 0, err
	   }
	  row := s.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT generation FROM fg_guard_generations WHERE user_id=? AND asset=?`,
		   userID, asset)
	   var gen int64
	  if err := row.Scan(&gen); err != nil {
		   return 0, err
	  }
	  return gen, nil
   }

// ---- PartialImbalance ----

  func (s *SQLStore) InsertImbalance(ctx context.Context, im PartialImbalance) error {
	   q := `INSERT INTO fg_partial_imbalances
		(imbalance_id, batch_id, user_id, asset, expected_total, actual_total, delta, state, detected_at)
		VALUES (?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE imbalance_id=imbalance_id`
	  _, err := s.db.Conn(ctx).ExecContext(ctx, q,
		  im.ImbalanceID,
		   im.BatchID, im.UserID, im.Asset,
		  im.ExpectedTotal, im.ActualTotal, im.Delta,
		  string(im.State), im.DetectedAt,
	  )
	   return err
   }

  func (s *SQLStore) GetImbalance(ctx context.Context, imbalanceID string) (PartialImbalance, error) {
	   q := `SELECT imbalance_id, batch_id, user_id, asset, expected_total, actual_total, delta,
		state, detected_at, proven_at, closed_at, actor1, actor2
		FROM fg_partial_imbalances WHERE imbalance_id=? LIMIT 1`
	  row := s.db.Conn(ctx).QueryRowContext(ctx,
		  q, imbalanceID)
	  return scanImbalance(row)
  }

   func scanImbalance(row *sql.Row) (PartialImbalance, error) {
	  var im PartialImbalance
	   var state string
	   var provenAt, closedAt sql.NullTime
	   var actor1, actor2 sql.NullString
	   err := row.Scan(
		   &im.ImbalanceID, &im.BatchID, &im.UserID, &im.Asset,
		   &im.ExpectedTotal, &im.ActualTotal, &im.Delta,
		   &state, &im.DetectedAt, &provenAt, &closedAt, &actor1, &actor2,
	  )
	  if err == sql.ErrNoRows {
		   return PartialImbalance{}, ErrImbalanceNotFound
	  }
	  if err != nil {
		   return PartialImbalance{}, err
	  }
	  im.State = ImbalanceState(state)
	  if provenAt.Valid {
		   im.ProvenAt = &provenAt.Time
	   }
	   if closedAt.Valid {
		   im.ClosedAt = &closedAt.Time
	   }
	  if actor1.Valid {
		   im.Actor1 = actor1.String
	  }
	   if actor2.Valid {
		   im.Actor2 = actor2.String
	  }
	   return im, nil
   }

  func (s *SQLStore) UpdateImbalanceState(ctx context.Context, imbalanceID string, old, new ImbalanceState, extra map[string]any) error {
	  clauses := []string{"state=?"}
	  args := []any{string(new)}
	  for _, k := range []string{"proven_at", "closed_at", "actor1", "actor2"} {
		  if v, ok := extra[k]; ok {
			   clauses = append(clauses, k+"=?")
			   args = append(args, v)
		  }
	  }
	  args = append(args, imbalanceID, string(old))
	   q := fmt.Sprintf("UPDATE fg_partial_imbalances SET %s WHERE imbalance_id=? AND state=?",
		  strings.Join(clauses, ","))
	   res, err := s.db.Conn(ctx).ExecContext(ctx, q, args...)
	   if err != nil {
		  return err
	   }
	  n, _ := res.RowsAffected()
	  if n == 0 {
		   return ErrLeaseInvalidState
	   }
	  return nil
  }

   func (s *SQLStore) ListOpenImbalances(ctx context.Context, limit int) ([]PartialImbalance, error) {
	   q := `SELECT imbalance_id, batch_id, user_id, asset, expected_total, actual_total, delta,
		state, detected_at, proven_at, closed_at, actor1, actor2
		FROM fg_partial_imbalances WHERE state='open' ORDER BY detected_at DESC LIMIT ?`
	   rows,
		  err := s.db.Conn(ctx).QueryContext(ctx, q, limit)
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  var out []PartialImbalance
	  for rows.Next() {
		  var im PartialImbalance
		  var state string
		   var provenAt, closedAt sql.NullTime
		  var actor1, actor2 sql.NullString
		   if err := rows.Scan(
			  &im.ImbalanceID, &im.BatchID, &im.UserID, &im.Asset,
			  &im.ExpectedTotal, &im.ActualTotal, &im.Delta,
			   &state, &im.DetectedAt, &provenAt, &closedAt, &actor1, &actor2,
		  ); err != nil {
			  return nil, err
		   }
		   im.State = ImbalanceState(state)
		  if provenAt.Valid {
			  im.ProvenAt = &provenAt.Time
		   }
		   if closedAt.Valid {
			  im.ClosedAt = &closedAt.Time
		   }
		  if actor1.Valid {
			  im.Actor1 = actor1.String
		   }
		  if actor2.Valid {
			   im.Actor2 = actor2.String
		   }
		  out = append(out, im)
	  }
	   return out, rows.Err()
   }

// ---- ConserveProof ----

  func (s *SQLStore) InsertProof(ctx context.Context, p ConserveProof) error {
	   q := `INSERT INTO fg_conserve_proofs
		(proof_id, lease_id, user_id, asset, before_total, after_total, snap_hash,
		 state, created_at, fail_reason, escalated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE proof_id=proof_id`
	   var failReason sql.NullString
	  if p.FailReason != "" {
		   failReason = sql.NullString{String: p.FailReason,
			  Valid: true}
	   }
	   var escalatedAt sql.NullTime
	   if p.EscalatedAt != nil {
		   escalatedAt = sql.NullTime{Time: *p.EscalatedAt, Valid: true}
	  }
	  _, err := s.db.Conn(ctx).ExecContext(ctx, q,
		  p.ProofID, p.LeaseID, p.UserID, p.Asset,
		  p.BeforeTotal, p.AfterTotal, p.SnapHash,
		  string(p.State), p.CreatedAt, failReason, escalatedAt,
	   )
	  return err
  }

  func (s *SQLStore) GetProof(ctx context.Context, proofID string) (ConserveProof, error) {
	   q := `SELECT proof_id, lease_id, user_id, asset, before_total, after_total, snap_hash,
		state, created_at, resolved_at, fail_reason, cleared_by, escalated_at
		FROM fg_conserve_proofs WHERE proof_id=? LIMIT 1`
	   row := s.db.Conn(ctx).QueryRowContext(ctx, q, proofID)
	   return scanProof(row)
  }

  func scanProof(row *sql.Row) (ConserveProof, error) {
	  var p ConserveProof
	  var state string
	   var resolvedAt, escalatedAt sql.NullTime
	   var failReason, clearedBy sql.NullString
	   err := row.Scan(
		   &p.ProofID, &p.LeaseID, &p.UserID, &p.Asset,
		   &p.BeforeTotal,
		   &p.AfterTotal, &p.SnapHash,
		  &state, &p.CreatedAt, &resolvedAt, &failReason, &clearedBy, &escalatedAt,
	  )
	  if err == sql.ErrNoRows {
		   return ConserveProof{}, ErrProofNotFound
	   }
	   if err != nil {
		   return ConserveProof{}, err
	   }
	  p.State = ProofState(state)
	  if resolvedAt.Valid {
		  p.ResolvedAt = &resolvedAt.Time
	   }
	   if failReason.Valid {
		  p.FailReason = failReason.String
	   }
	  if clearedBy.Valid {
		   p.ClearedBy = clearedBy.String
	   }
	  if escalatedAt.Valid {
		  p.EscalatedAt = &escalatedAt.Time
	  }
	  return p, nil
  }

  func (s *SQLStore) UpdateProofState(ctx context.Context, proofID string, old, new ProofState, extra map[string]any) error {
	   clauses := []string{"state=?"}
	   args := []any{string(new)}
	  if t, ok := extra["resolved_at"]; ok {
		   clauses = append(clauses, "resolved_at=?")
		   args = append(args, t)
	  }
	   if v, ok := extra["cleared_by"]; ok {
		  clauses = append(clauses, "cleared_by=?")
		  args = append(args, v)
	   }
	   args = append(args, proofID, string(old))
	  q := fmt.Sprintf("UPDATE fg_conserve_proofs SET %s WHERE proof_id=? AND state=?",
		   strings.Join(clauses, ","))
	  res, err := s.db.Conn(ctx).ExecContext(ctx, q, args...)
	   if err != nil {
		  return err
	  }
	  n, _ := res.RowsAffected()
	  if n == 0 {
		  return ErrLeaseInvalidState
	   }
	   return nil
  }

   func (s *SQLStore) ListFailedProofs(ctx context.Context, limit int) ([]ConserveProof, error) {
	   q := `SELECT proof_id, lease_id, user_id, asset, before_total, after_total, snap_hash,
		state, created_at, resolved_at, fail_reason, cleared_by, escalated_at
		FROM fg_conserve_proofs WHERE state IN ('fail','halted') ORDER BY created_at DESC LIMIT ?`
	  rows, err := s.db.Conn(ctx).QueryContext(ctx, q, limit)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	  var out []ConserveProof
	   for rows.Next() {
		  var p ConserveProof
		  var state string
		  var resolvedAt, escalatedAt sql.NullTime
		  var failReason,
			   clearedBy sql.NullString
		   if err := rows.Scan(
			   &p.ProofID, &p.LeaseID, &p.UserID, &p.Asset,
			   &p.BeforeTotal, &p.AfterTotal, &p.SnapHash,
			   &state, &p.CreatedAt, &resolvedAt, &failReason, &clearedBy, &escalatedAt,
		   ); err != nil {
			   return nil, err
		  }
		  p.State = ProofState(state)
		   if resolvedAt.Valid {
			  p.ResolvedAt = &resolvedAt.Time
		  }
		   if failReason.Valid {
			  p.FailReason = failReason.String
		   }
		  if clearedBy.Valid {
			  p.ClearedBy = clearedBy.String
		  }
		  if escalatedAt.Valid {
			  p.EscalatedAt = &escalatedAt.Time
		   }
		  out = append(out, p)
	  }
	  return out, rows.Err()
   }

// ---- FenceRequest ----

  func (s *SQLStore) InsertFence(ctx context.Context, f FenceRequest) error {
	   q := `INSERT INTO fg_fence_requests
		(fence_id, lease_id, user_id, asset, kind, amount, queued_at)
		VALUES (?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE fence_id=fence_id`
	   _, err := s.db.Conn(ctx).ExecContext(ctx, q,
		  f.FenceID, f.LeaseID, f.UserID, f.Asset, f.Kind, f.Amount, f.QueuedAt,
	   )
	  return err
   }

   func (s *SQLStore) ClearFence(ctx context.Context, leaseID string) error {
	  q := `UPDATE fg_fence_requests SET cleared_at=? WHERE lease_id=? AND cleared_at IS NULL`
	  _, err := s.db.Conn(ctx).ExecContext(ctx, q, time.Now().UTC(), leaseID)
	  return err
   }

  func (s *SQLStore) HasActiveFence(ctx context.Context, userID, asset, leaseID string) (bool, error) {
	  q := `SELECT COUNT(*) FROM fg_fence_requests
		WHERE user_id=? AND asset=? AND lease_id=? AND cleared_at IS NULL LIMIT 1`
	  row := s.db.Conn(ctx).QueryRowContext(ctx, q, userID, asset, leaseID)
	  var count int
	  if err := row.Scan(&count); err != nil {
		  return false, err
	  }
	   return count > 0, nil
  }
