package stmtexpiry

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// SQLStore 基于 *sql.DB 的存储实现。
type SQLStore struct {
	db *sql.DB
	mu      sync.Mutex
	mutexes map[string]string
}

  func NewSQLStore(db *sql.DB) *SQLStore {
	  return &SQLStore{
		   db: db,
		  mutexes: make(map[string]string),
	   }
  }

// ──────────────────────────── 租约 ────────────────────────────

  func (s *SQLStore) InsertLease(ctx context.Context, l ExportLease) error {
	   const q = `INSERT INTO stmt_export_leases
		(id, user_id, format, root, high_water, state, ttl_sec,
		 issued_at, expires_at, created_at, updated_at,
		 window_start, window_end, revoked_by, proof_id)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	   _, err := s.db.ExecContext(ctx, q,
		   l.ID, l.UserID, l.Format, l.Root, l.HighWater, string(l.State), l.TTLSec,
		  nullTime(l.IssuedAt), nullTime(l.ExpiresAt), l.CreatedAt, l.UpdatedAt,
		  nullTime(l.WindowStart),
		  nullTime(l.WindowEnd), nullStr(l.RevokedBy), nullStr(l.ProofID))
	  return err
   }

   func (s *SQLStore) GetLease(ctx context.Context, id string) (ExportLease, error) {
	   const q = `SELECT id, user_id, format, root, high_water, state, ttl_sec,
		issued_at, expires_at, created_at, updated_at,
		window_start, window_end, revoked_by, proof_id
		FROM stmt_export_leases WHERE id = ?`
	   var l ExportLease
	  var state string
	   var issuedAt, expiresAt, wStart, wEnd sql.NullTime
	   var revokedBy, proofID sql.NullString
	   err := s.db.QueryRowContext(ctx, q, id).Scan(
		   &l.ID, &l.UserID, &l.Format, &l.Root, &l.HighWater, &state, &l.TTLSec,
		  &issuedAt, &expiresAt, &l.CreatedAt, &l.UpdatedAt,
		  &wStart, &wEnd, &revokedBy, &proofID)
	   if err != nil {
		   return l, err
	   }
	   l.State = LeaseState(state)
	  l.IssuedAt = fromNullTime(issuedAt)
	  l.ExpiresAt = fromNullTime(expiresAt)
	   l.WindowStart = fromNullTime(wStart)
	  l.WindowEnd = fromNullTime(wEnd)
	   l.RevokedBy = revokedBy.String
	  l.ProofID = proofID.String
	   return l, nil
   }

  func (s *SQLStore) UpdateLeaseState(ctx context.Context, id string, from, to LeaseState, updatedAt time.Time) error {
	   res, err := s.db.ExecContext(ctx,
		  `UPDATE stmt_export_leases SET state=?, updated_at=? WHERE id=? AND state=?`,
		   string(to), updatedAt, id, string(from))
	  if err != nil {
		  return err
	  }
	  return checkOneRow(res, "lease", id)
   }

  func (s *SQLStore) SetLeaseIssued(ctx context.Context, id, proofID string, issuedAt, expiresAt time.Time) error {
	  res, err := s.db.ExecContext(ctx,
		  `UPDATE stmt_export_leases SET state=?, proof_id=?, issued_at=?, expires_at=?, updated_at=?
		 WHERE id=? AND state=?`,
		  string(LeaseIssued),
		  proofID, issuedAt, expiresAt, time.Now().UTC(), id, string(LeaseBuilt))
	  if err != nil {
		   return err
	   }
	  return checkOneRow(res, "lease-issue", id)
   }

   func (s *SQLStore) SetLeaseRevoked(ctx context.Context, id, revokedBy string) error {
	   res, err := s.db.ExecContext(ctx,
		  `UPDATE stmt_export_leases SET state=?, revoked_by=?, updated_at=?
		 WHERE id=? AND state IN (?,?)`,
		  string(LeaseRevoked), revokedBy, time.Now().UTC(), id, string(LeaseBuilt), string(LeaseIssued))
	  if err != nil {
		   return err
	   }
	   return checkOneRow(res, "lease-revoke", id)
   }

  func (s *SQLStore) ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]ExportLease, error) {
	  if limit <= 0 {
		   limit = 100
	   }
	   const q = `SELECT id, user_id, format, root, high_water, state, ttl_sec,
		issued_at, expires_at, created_at, updated_at,
		window_start, window_end, revoked_by, proof_id
		FROM stmt_export_leases WHERE state=? AND expires_at < ? ORDER BY expires_at LIMIT ?`
	   rows,
		  err := s.db.QueryContext(ctx, q, string(LeaseIssued), before, limit)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   return scanLeases(rows)
  }

  func (s *SQLStore) ListActiveLeasesByUser(ctx context.Context, userID string) ([]ExportLease, error) {
	   const q = `SELECT id, user_id, format, root, high_water, state, ttl_sec,
		issued_at, expires_at, created_at, updated_at,
		window_start, window_end, revoked_by, proof_id
		FROM stmt_export_leases WHERE user_id=? AND state IN (?,?) ORDER BY created_at DESC`
	  rows, err := s.db.QueryContext(ctx, q, userID, string(LeaseBuilt), string(LeaseIssued))
	   if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	   return scanLeases(rows)
  }

  func (s *SQLStore) ListIssuedLeases(ctx context.Context, limit int) ([]ExportLease, error) {
	   if limit <= 0 {
		   limit = 200
	  }
	  const q = `SELECT id, user_id, format, root, high_water, state, ttl_sec,
		issued_at, expires_at, created_at, updated_at,
		window_start, window_end, revoked_by, proof_id
		FROM stmt_export_leases WHERE state=? ORDER BY issued_at LIMIT ?`
	   rows, err := s.db.QueryContext(ctx, q, string(LeaseIssued), limit)
	  if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	  return scanLeases(rows)
  }

  func scanLeases(rows *sql.Rows) ([]ExportLease, error) {
	   var out []ExportLease
	   for rows.Next() {
		   var l ExportLease
		  var state string
		   var issuedAt,
			   expiresAt, wStart, wEnd sql.NullTime
		  var revokedBy,
			  proofID sql.NullString
		   if err := rows.Scan(
			  &l.ID, &l.UserID, &l.Format, &l.Root, &l.HighWater, &state, &l.TTLSec,
			  &issuedAt, &expiresAt, &l.CreatedAt, &l.UpdatedAt,
			   &wStart, &wEnd, &revokedBy, &proofID); err != nil {
			   return nil, err
		   }
		  l.State = LeaseState(state)
		   l.IssuedAt = fromNullTime(issuedAt)
		   l.ExpiresAt = fromNullTime(expiresAt)
		   l.WindowStart = fromNullTime(wStart)
		  l.WindowEnd = fromNullTime(wEnd)
		  l.RevokedBy = revokedBy.String
		   l.ProofID = proofID.String
		  out = append(out, l)
	  }
	  return out, rows.Err()
   }

// ──────────────────────────── 漂移 ────────────────────────────

  func (s *SQLStore) InsertDrift(ctx context.Context, d RootDriftCase) error {
	  seqsJSON,
		  _ := json.Marshal(d.DriftBillSeqs)
	   const q = `INSERT INTO stmt_root_drifts
		(id, lease_id, user_id, original_root, current_root,
		 high_water_old, high_water_new, drift_bill_seqs, state, created_at,
		 proven_at, closed_at, resolution)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`
	  _, err := s.db.ExecContext(ctx, q,
		  d.ID, d.LeaseID, d.UserID, d.OriginalRoot, d.CurrentRoot,
		   d.HighWaterOld, d.HighWaterNew, string(seqsJSON), string(d.State), d.CreatedAt,
		   nullTime(d.ProvenAt), nullTime(d.ClosedAt), nullStr(d.Resolution))
	   return err
  }

   func (s *SQLStore) GetDrift(ctx context.Context, id string) (RootDriftCase, error) {
	  const q = `SELECT id, lease_id, user_id, original_root, current_root,
		high_water_old, high_water_new, drift_bill_seqs, state, created_at,
		proven_at, closed_at, resolution
		FROM stmt_root_drifts WHERE id = ?`
	   var d RootDriftCase
	  var state string
	   var seqsJSON string
	  var provenAt, closedAt sql.NullTime
	   var resolution sql.NullString
	   err := s.db.QueryRowContext(ctx, q, id).Scan(
		  &d.ID, &d.LeaseID, &d.UserID, &d.OriginalRoot, &d.CurrentRoot,
		   &d.HighWaterOld,
		  &d.HighWaterNew, &seqsJSON, &state, &d.CreatedAt,
		   &provenAt, &closedAt, &resolution)
	  if err != nil {
		  return d, err
	   }
	   d.State = DriftState(state)
	   d.ProvenAt = fromNullTime(provenAt)
	  d.ClosedAt = fromNullTime(closedAt)
	   d.Resolution = resolution.String
	   if seqsJSON != "" {
		   _ = json.Unmarshal([]byte(seqsJSON), &d.DriftBillSeqs)
	  }
	  return d, nil
   }

  func (s *SQLStore) UpdateDriftState(ctx context.Context, id string, from, to DriftState) error {
	  now := time.Now().UTC()
	  var setExtra string
	   switch to {
	   case DriftProven:
		  setExtra = ", proven_at='" + now.Format(time.RFC3339) + "'"
	   case DriftClosed:
		  setExtra = ", closed_at='" + now.Format(time.RFC3339) + "'"
	   }
	   q := fmt.Sprintf(`UPDATE stmt_root_drifts SET state=?%s WHERE id=? AND state=?`, setExtra)
	  res, err := s.db.ExecContext(ctx, q, string(to), id, string(from))
	  if err != nil {
		   return err
	   }
	  return checkOneRow(res, "drift", id)
  }

  func (s *SQLStore) ListOpenDrifts(ctx context.Context, limit int) ([]RootDriftCase, error) {
	   if limit <= 0 {
		   limit = 100
	   }
	   const q = `SELECT id, lease_id, user_id, original_root, current_root,
		high_water_old, high_water_new, drift_bill_seqs, state, created_at,
		proven_at, closed_at, resolution
		FROM stmt_root_drifts WHERE state IN (?,?) ORDER BY created_at LIMIT ?`
	   rows, err := s.db.QueryContext(ctx, q, string(DriftOpen), string(DriftProven), limit)
	  if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	  return scanDrifts(rows)
  }

   func (s *SQLStore) ListDriftsByLease(ctx context.Context, leaseID string) ([]RootDriftCase, error) {
	   const q = `SELECT id, lease_id, user_id, original_root, current_root,
		high_water_old, high_water_new, drift_bill_seqs, state, created_at,
		proven_at, closed_at, resolution
		FROM stmt_root_drifts WHERE lease_id=? ORDER BY created_at`
	   rows, err := s.db.QueryContext(ctx, q, leaseID)
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   return scanDrifts(rows)
  }

  func scanDrifts(rows *sql.Rows) ([]RootDriftCase, error) {
	  var out []RootDriftCase
	  for rows.Next() {
		   var d RootDriftCase
		  var state string
		  var seqsJSON string
		   var provenAt, closedAt sql.NullTime
		  var resolution sql.NullString
		   if err := rows.Scan(
			   &d.ID, &d.LeaseID, &d.UserID, &d.OriginalRoot, &d.CurrentRoot,
			  &d.HighWaterOld, &d.HighWaterNew, &seqsJSON, &state, &d.CreatedAt,
			  &provenAt, &closedAt, &resolution); err != nil {
			   return nil, err
		   }
		   d.State = DriftState(state)
		  d.ProvenAt = fromNullTime(provenAt)
		   d.ClosedAt = fromNullTime(closedAt)
		  d.Resolution = resolution.String
		  if seqsJSON != "" {
			   _ = json.Unmarshal([]byte(seqsJSON), &d.DriftBillSeqs)
		  }
		   out = append(out, d)
	   }
	  return out, rows.Err()
   }

// ──────────────────────────── 撤销 ────────────────────────────

   func (s *SQLStore) InsertRevocation(ctx context.Context, r Revocation) error {
	  const q = `INSERT INTO stmt_revocations
		(id, lease_id, user_id, kind, reason, drift_id, created_at, actor)
		VALUES (?,?,?,?,?,?,?,?)`
	   _, err := s.db.ExecContext(ctx, q,
		  r.ID, r.LeaseID, r.UserID, string(r.Kind), r.Reason, nullStr(r.DriftID), r.CreatedAt, r.Actor)
	  return err
  }

  func (s *SQLStore) GetRevocation(ctx context.Context, id string) (Revocation, error) {
	  const q = `SELECT id, lease_id, user_id, kind, reason, drift_id, created_at, actor
		FROM stmt_revocations WHERE id = ?`
	   var r Revocation
	   var kind string
	   var driftID sql.NullString
	  err := s.db.QueryRowContext(ctx, q, id).Scan(
		   &r.ID,
		   &r.LeaseID, &r.UserID, &kind, &r.Reason, &driftID, &r.CreatedAt, &r.Actor)
	  if err != nil {
		  return r, err
	   }
	   r.Kind = RevocationKind(kind)
	   r.DriftID = driftID.String
	  return r, nil
   }

  func (s *SQLStore) ListRevocationsByLease(ctx context.Context, leaseID string) ([]Revocation, error) {
	   const q = `SELECT id, lease_id, user_id, kind, reason, drift_id, created_at, actor
		FROM stmt_revocations WHERE lease_id=? ORDER BY created_at`
	  rows,
		  err := s.db.QueryContext(ctx, q, leaseID)
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  var out []Revocation
	   for rows.Next() {
		   var r Revocation
		  var kind string
		  var driftID sql.NullString
		   if err := rows.Scan(
			   &r.ID, &r.LeaseID, &r.UserID, &kind, &r.Reason, &driftID, &r.CreatedAt, &r.Actor); err != nil {
			   return nil, err
		   }
		  r.Kind = RevocationKind(kind)
		  r.DriftID = driftID.String
		  out = append(out, r)
	  }
	  return out, rows.Err()
  }

// ──────────────────────────── 证明 ────────────────────────────

  func (s *SQLStore) InsertProof(ctx context.Context, p IssueProof) error {
	  const q = `INSERT INTO stmt_issue_proofs
		(id, lease_id, user_id, root, ttl_sec, digest, issued_at, expires_at,
		 window_start, window_end)
		VALUES (?,?,?,?,?,?,?,?,?,?)`
	   _, err := s.db.ExecContext(ctx, q,
		   p.ID, p.LeaseID, p.UserID, p.Root, p.TTLSec, p.Digest,
		  p.IssuedAt,
		   p.ExpiresAt, p.WindowStart, p.WindowEnd)
	  return err
  }

   func (s *SQLStore) GetProof(ctx context.Context, id string) (IssueProof, error) {
	  const q = `SELECT id, lease_id, user_id, root, ttl_sec, digest, issued_at, expires_at, 
		window_start, window_end
		FROM stmt_issue_proofs WHERE id = ?`
	   var p IssueProof
	  err := s.db.QueryRowContext(ctx, q, id).Scan(
		  &p.ID, &p.LeaseID, &p.UserID, &p.Root, &p.TTLSec, &p.Digest,
		   &p.IssuedAt,
		  &p.ExpiresAt, &p.WindowStart, &p.WindowEnd)
	  return p, err
  }

   func (s *SQLStore) GetProofByLease(ctx context.Context, leaseID string) (IssueProof, error) {
	  const q = `SELECT id, lease_id, user_id, root, ttl_sec, digest, issued_at, expires_at,
		window_start, window_end
		FROM stmt_issue_proofs WHERE lease_id = ? ORDER BY issued_at DESC LIMIT 1`
	  var p IssueProof
	   err := s.db.QueryRowContext(ctx, q, leaseID).Scan(
		  &p.ID, &p.LeaseID, &p.UserID, &p.Root, &p.TTLSec, &p.Digest,
		  &p.IssuedAt, &p.ExpiresAt, &p.WindowStart, &p.WindowEnd)
	  return p, err
   }

// ──────────────────────────── 互斥 ────────────────────────────

  func (s *SQLStore) TryAcquireExportMutex(ctx context.Context, userID, leaseID string, ttl time.Duration) (bool, error) {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  key := "export:" + userID
	  if holder, ok := s.mutexes[key]; ok && holder != leaseID {
		   return false, nil
	  }
	   s.mutexes[key] = leaseID
	   go func() {
		  <-time.After(ttl)
		   s.mu.Lock()
		   if s.mutexes[key] == leaseID {
			   delete(s.mutexes, key)
		   }
		   s.mu.Unlock()
	  }()
	   return true, nil
   }

   func (s *SQLStore) ReleaseExportMutex(ctx context.Context, userID, leaseID string) error {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   key := "export:" + userID
	   if s.mutexes[key] == leaseID {
		  delete(s.mutexes, key)
	   }
	   return nil
   }

// ──────────────────────────── 辅助 ────────────────────────────

  func nullTime(t time.Time) sql.NullTime {
	  if t.IsZero() {
		  return sql.NullTime{}
	  }
	  return sql.NullTime{Time: t, Valid: true}
   }

   func fromNullTime(nt sql.NullTime) time.Time {
	  if nt.Valid {
		   return nt.Time
	  }
	  return time.Time{}
  }

  func nullStr(s string) sql.NullString {
	  if s == "" {
		  return sql.NullString{}
	   }
	   return sql.NullString{String: s, Valid: true}
  }

  func checkOneRow(res sql.Result, entity, id string) error {
	  n, err := res.RowsAffected()
	  if err != nil {
		  return err
	   }
	   if n == 0 {
		   return fmt.Errorf("%s %s: not found or state conflict", entity, id)
	  }
	   return nil
  }
