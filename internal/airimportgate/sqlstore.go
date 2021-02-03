package airimportgate

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// SQLStore 基于 *sql.DB 的 Store 实现。

type SQLStore struct {
	db    *sql.DB
	clock func() time.Time
}

// NewSQLStore 构造 SQLStore。
   func NewSQLStore(db *sql.DB) *SQLStore {
	  return &SQLStore{
		   db: db,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
   }

// WithClock 注入时钟（用）。

  func (s *SQLStore) WithClock(fn func() time.Time) *SQLStore {
	   if fn != nil {
		   s.clock = fn
	  }
	   return s
  }

  // isDuplicateEntry 判断是否为唯一索引冲突错误（MySQL/TiDB Error 1062）。
  func isDuplicateEntry(err error) bool {
	   if err == nil {
		  return false
	   }
	   return strings.Contains(err.Error(), "Duplicate entry") ||
		   strings.Contains(err.Error(), "duplicate key") ||
		   strings.Contains(err.Error(), "1062")
  }

// ── ImportLease ───────────────────────────────────────────────────────────────

const insertLease = `
INSERT INTO airgap_import_leases
  (lease_id, session_id, import_gen, blob_digest, actor, state,
   expires_at, imported_at, aborted_at, abort_reason, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`

  func (s *SQLStore) SaveLease(ctx context.Context, l *ImportLease) error {
	  if s == nil || l == nil {
		  return ErrUnavailable
	  }
	  _, err := s.db.ExecContext(ctx, insertLease,
		  l.LeaseID, l.SessionID, l.ImportGen, l.BlobDigest, l.Actor, string(l.State),
		  l.ExpiresAt, ptrTime(l.ImportedAt), ptrTime(l.AbortedAt), l.AbortReason,
		   l.CreatedAt, l.UpdatedAt,
	  )
	  return err
  }

const getLease = `
SELECT lease_id, session_id, import_gen, blob_digest, actor, state,
       expires_at, imported_at, aborted_at, abort_reason, created_at, updated_at
FROM airgap_import_leases WHERE lease_id=?`

   func (s *SQLStore) GetLease(ctx context.Context, id string) (*ImportLease, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	   row := s.db.QueryRowContext(ctx, getLease, id)
	   return scanLease(row)
   }

const updateLeaseState = `
UPDATE airgap_import_leases
SET state=?, imported_at=?, aborted_at=?, abort_reason=?, updated_at=?
WHERE lease_id=? AND state=?`

   func (s *SQLStore) UpdateLeaseState(ctx context.Context, id string, from, to LeaseState, update *ImportLease) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	  now := s.clock()
	   var importedAt, abortedAt interface{}
	  var abortReason string
	  if update != nil {
		  importedAt = ptrTime(update.ImportedAt)
		  abortedAt = ptrTime(update.AbortedAt)
		  abortReason = update.AbortReason
	   }
	   res, err := s.db.ExecContext(ctx, updateLeaseState,
		  string(to), importedAt, abortedAt, abortReason, now,
		  id, string(from),
	  )
	  if err != nil {
		  return err
	  }
	  n, err := res.RowsAffected()
	  if err != nil {
		   return err
	  }
	   if n == 0 {
		   return ErrInvalidTransition
	   }
	   return nil
   }

const listActiveLeasesForSession = `
SELECT lease_id, session_id, import_gen, blob_digest, actor, state,
       expires_at, imported_at, aborted_at, abort_reason, created_at, updated_at
FROM airgap_import_leases
WHERE session_id=? AND state NOT IN ('imported','aborted','expired')`

   func (s *SQLStore) ListActiveLeasesForSession(ctx context.Context, sessionID string) ([]*ImportLease, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	  rows,
		   err := s.db.QueryContext(ctx, listActiveLeasesForSession, sessionID)
	  if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   return scanLeases(rows)
  }

const listExpiredLeases = `
SELECT lease_id, session_id, import_gen, blob_digest, actor, state,
       expires_at, imported_at, aborted_at, abort_reason, created_at, updated_at
FROM airgap_import_leases
WHERE expires_at < ? AND state NOT IN ('imported','aborted','expired')
LIMIT ?`

  func (s *SQLStore) ListExpiredLeases(ctx context.Context, limit int) ([]*ImportLease, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	   if limit <= 0 {
		  limit = 100
	   }
	  now := s.clock()
	  rows, err := s.db.QueryContext(ctx, listExpiredLeases, now, limit)
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  return scanLeases(rows)
  }

const listLeasesBySession = `
SELECT lease_id, session_id, import_gen, blob_digest, actor, state,
       expires_at, imported_at, aborted_at, abort_reason, created_at, updated_at
FROM airgap_import_leases WHERE session_id=? LIMIT ?`

   func (s *SQLStore) ListLeasesBySession(ctx context.Context, sessionID string, limit int) ([]*ImportLease, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	  if limit <= 0 {
		   limit = 50
	   }
	   rows, err := s.db.QueryContext(ctx, listLeasesBySession, sessionID, limit)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   return scanLeases(rows)
  }

   func scanLease(row *sql.Row) (*ImportLease, error) {
	   var l ImportLease
	   var state string
	  var importedAt, abortedAt sql.NullTime
	   err := row.Scan(
		   &l.LeaseID, &l.SessionID, &l.ImportGen, &l.BlobDigest, &l.Actor, &state,
		  &l.ExpiresAt, &importedAt, &abortedAt, &l.AbortReason, &l.CreatedAt, &l.UpdatedAt,
	  )
	   if err == sql.ErrNoRows {
		  return nil, ErrLeaseNotFound
	  }
	  if err != nil {
		   return nil, err
	  }
	   l.State = LeaseState(state)
	  if importedAt.Valid {
		  t := importedAt.Time
		   l.ImportedAt = &t
	   }
	   if abortedAt.Valid {
		   t := abortedAt.Time
		   l.AbortedAt = &t
	  }
	  return &l, nil
  }

  func scanLeases(rows *sql.Rows) ([]*ImportLease, error) {
	   var out []*ImportLease
	  for rows.Next() {
		   var l ImportLease
		   var state string
		   var importedAt,
			   abortedAt sql.NullTime
		  if err := rows.Scan(
			   &l.LeaseID, &l.SessionID, &l.ImportGen, &l.BlobDigest, &l.Actor, &state,
			   &l.ExpiresAt,
			  &importedAt, &abortedAt, &l.AbortReason, &l.CreatedAt, &l.UpdatedAt,
		  ); err != nil {
			  return nil, err
		  }
		   l.State = LeaseState(state)
		  if importedAt.Valid {
			   t := importedAt.Time
			   l.ImportedAt = &t
		   }
		   if abortedAt.Valid {
			   t := abortedAt.Time
			   l.AbortedAt = &t
		   }
		  out = append(out, &l)
	  }
	  return out, rows.Err()
  }

// ── SealedBlobDigest ──────────────────────────────────────────────────────────

const insertDigest = `
INSERT INTO airgap_blob_digests
  (digest_id, digest_hex, session_id, lease_id, imported_by, imported_at)
VALUES (?,?,?,?,?,?)`

   func (s *SQLStore) SaveDigest(ctx context.Context, d *SealedBlobDigest) error {
	   if s == nil || d == nil {
		  return ErrUnavailable
	  }
	  _, err := s.db.ExecContext(ctx, insertDigest,
		  d.DigestID, d.DigestHex, d.SessionID, d.LeaseID, d.ImportedBy, d.ImportedAt,
	   )
	   if isDuplicateEntry(err) {
		   return ErrBlobReplay
	  }
	  return err
  }

const getDigestByHex = `
SELECT digest_id, digest_hex, session_id, lease_id, imported_by, imported_at
FROM airgap_blob_digests WHERE digest_hex=?`

  func (s *SQLStore) GetDigestByHex(ctx context.Context, hex string) (*SealedBlobDigest, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	   row := s.db.QueryRowContext(ctx, getDigestByHex, hex)
	   var d SealedBlobDigest
	   err := row.Scan(&d.DigestID, &d.DigestHex, &d.SessionID, &d.LeaseID, &d.ImportedBy, &d.ImportedAt)
	   if err == sql.ErrNoRows {
		  return nil, ErrDigestNotFound
	  }
	   if err != nil {
		  return nil, err
	   }
	   return &d, nil
   }

const listDigestsBySession = `
SELECT digest_id, digest_hex, session_id, lease_id, imported_by, imported_at
FROM airgap_blob_digests WHERE session_id=?`

  func (s *SQLStore) ListDigestsBySession(ctx context.Context, sessionID string) ([]*SealedBlobDigest, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	  }
	  rows, err := s.db.QueryContext(ctx, listDigestsBySession, sessionID)
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  var out []*SealedBlobDigest
	   for rows.Next() {
		   var d SealedBlobDigest
		   if err := rows.Scan(&d.DigestID, &d.DigestHex, &d.SessionID, &d.LeaseID, &d.ImportedBy, &d.ImportedAt); err != nil {
			   return nil, err
		   }
		  out = append(out, &d)
	   }
	  return out, rows.Err()
  }

// ── PhaseConflictProof ────────────────────────────────────────────────────────

const insertConflict = ` 
INSERT INTO airgap_phase_conflicts
  (conflict_id, session_id, conflict_kind, state,
   winner_op, winner_actor, loser_op, loser_actor,
   phase_at_conflict, note, proven_at, closed_at, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

  func (s *SQLStore) SaveConflict(ctx context.Context, p *PhaseConflictProof) error {
	  if s == nil || p == nil {
		  return ErrUnavailable
	  }
	  _, err := s.db.ExecContext(ctx, insertConflict,
		   p.ConflictID, p.SessionID, string(p.ConflictKind), string(p.State),
		   p.WinnerOp, p.WinnerActor, p.LoserOp, p.LoserActor,
		  p.PhaseAtConflict, p.Note, ptrTime(p.ProvenAt), ptrTime(p.ClosedAt),
		   p.CreatedAt, p.UpdatedAt,
	   )
	   return err
  }

const getConflict = `
SELECT conflict_id, session_id, conflict_kind, state,
       winner_op, winner_actor, loser_op, loser_actor,
       phase_at_conflict, note, proven_at, closed_at, created_at, updated_at
FROM airgap_phase_conflicts WHERE conflict_id=?`

  func (s *SQLStore) GetConflict(ctx context.Context, id string) (*PhaseConflictProof, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	  row := s.db.QueryRowContext(ctx, getConflict, id)
	   return scanConflict(row)
   }

const updateConflictState = `
UPDATE airgap_phase_conflicts
SET state=?, proven_at=?, closed_at=?, updated_at=?
WHERE conflict_id=? AND state=?`

   func (s *SQLStore) UpdateConflictState(ctx context.Context, id string, from, to ConflictState, update *PhaseConflictProof) error {
	  if s == nil {
		   return ErrUnavailable
	   }
	  now := s.clock()
	  var provenAt, closedAt interface{}
	  if update != nil {
		   provenAt = ptrTime(update.ProvenAt)
		  closedAt = ptrTime(update.ClosedAt)
	  }
	  res, err := s.db.ExecContext(ctx, updateConflictState,
		   string(to), provenAt, closedAt, now,
		  id, string(from),
	   )
	   if err != nil {
		   return err
	  }
	   n, _ := res.RowsAffected()
	  if n == 0 {
		  return ErrInvalidTransition
	   }
	  return nil
  }

const listOpenConflictsBySession = `
SELECT conflict_id, session_id, conflict_kind, state,
       winner_op, winner_actor, loser_op, loser_actor,
       phase_at_conflict, note, proven_at, closed_at, created_at, updated_at
FROM airgap_phase_conflicts
WHERE session_id=? AND state IN ('open','proven')`

   func (s *SQLStore) ListOpenConflictsBySession(ctx context.Context, sessionID string) ([]*PhaseConflictProof, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	   rows, err := s.db.QueryContext(ctx, listOpenConflictsBySession, sessionID)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   var out []*PhaseConflictProof
	   for rows.Next() {
		   row := rows
		  _ = row
		  var p PhaseConflictProof
		  var kind, state string
		   var provenAt, closedAt sql.NullTime
		   if err := rows.Scan(
			  &p.ConflictID, &p.SessionID, &kind, &state,
			  &p.WinnerOp, &p.WinnerActor, &p.LoserOp, &p.LoserActor,
			  &p.PhaseAtConflict,
			   &p.Note, &provenAt, &closedAt,
			  &p.CreatedAt, &p.UpdatedAt,
		  ); err != nil {
			  return nil, err
		   }
		   p.ConflictKind = ConflictKind(kind)
		   p.State = ConflictState(state)
		  if provenAt.Valid {
			  t := provenAt.Time
			   p.ProvenAt = &t
		  }
		   if closedAt.Valid {
			  t := closedAt.Time
			  p.ClosedAt = &t
		   }
		   out = append(out, &p)
	   }
	   return out, rows.Err()
  }

   func scanConflict(row *sql.Row) (*PhaseConflictProof, error) {
	   var p PhaseConflictProof
	   var kind, state string
	   var provenAt, closedAt sql.NullTime
	   err := row.Scan(
		  &p.ConflictID,
		   &p.SessionID, &kind, &state,
		   &p.WinnerOp, &p.WinnerActor, &p.LoserOp, &p.LoserActor,
		  &p.PhaseAtConflict, &p.Note, &provenAt, &closedAt,
		  &p.CreatedAt, &p.UpdatedAt,
	   )
	  if err == sql.ErrNoRows {
		  return nil, ErrConflictNotFound
	  }
	  if err != nil {
		   return nil, err
	   }
	  p.ConflictKind = ConflictKind(kind)
	  p.State = ConflictState(state)
	  if provenAt.Valid {
		   t := provenAt.Time
		   p.ProvenAt = &t
	  }
	  if closedAt.Valid {
		  t := closedAt.Time
		  p.ClosedAt = &t
	  }
	  return &p, nil
  }

// ── QuorumBind ────────────────────────────────────────────────────────────────

const insertBind = `
INSERT INTO airgap_quorum_binds
  (bind_id, session_id, lease_id, bind_gen,
   threshold, actual_quorum, quorum_passed,
   bound_by, bound_at, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?)`

  func (s *SQLStore) SaveBind(ctx context.Context, b *QuorumBind) error {
	   if s == nil || b == nil {
		  return ErrUnavailable
	   }
	  _, err := s.db.ExecContext(ctx, insertBind,
		   b.BindID, b.SessionID, b.LeaseID, b.BindGen,
		  b.Threshold, b.ActualQuorum, b.QuorumPassed,
		   b.BoundBy, b.BoundAt, b.CreatedAt,
	  )
	  return err
   }

const getBind = `
SELECT bind_id, session_id, lease_id, bind_gen,
       threshold, actual_quorum, quorum_passed,
       bound_by, bound_at, created_at
FROM airgap_quorum_binds WHERE bind_id=?`

   func (s *SQLStore) GetBind(ctx context.Context, id string) (*QuorumBind, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	  row := s.db.QueryRowContext(ctx, getBind, id)
	   return scanBind(row)
   }

const getBindByLease = `
SELECT bind_id, session_id, lease_id, bind_gen,
       threshold, actual_quorum, quorum_passed,
       bound_by, bound_at, created_at
FROM airgap_quorum_binds WHERE lease_id=? ORDER BY created_at DESC LIMIT 1`

   func (s *SQLStore) GetBindByLease(ctx context.Context, leaseID string) (*QuorumBind, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	   row := s.db.QueryRowContext(ctx, getBindByLease, leaseID)
	  return scanBind(row)
  }

  func scanBind(row *sql.Row) (*QuorumBind, error) {
	   var b QuorumBind
	  err := row.Scan(
		   &b.BindID, &b.SessionID, &b.LeaseID, &b.BindGen,
		  &b.Threshold, &b.ActualQuorum, &b.QuorumPassed,
		   &b.BoundBy, &b.BoundAt, &b.CreatedAt,
	   )
	  if err == sql.ErrNoRows {
		  return nil, ErrBindNotFound
	   }
	  if err != nil {
		   return nil, err
	   }
	  return &b, nil
  }

// ptrTime 将 *time.Time 转为 interface{}；nil 转为 nil（SQL NULL）。
   func ptrTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}
