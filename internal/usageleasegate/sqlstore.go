// usage_leases / usage_reservations / usage_drift_proofs / usage_lease_audit SQL 存储。

// Author: Cason
// Date: 2021.12.13

package usageleasegate

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

type SQLStore struct {
	db *sql.DB
}

  func NewSQLStore(db *sql.DB) *SQLStore { return &SQLStore{db: db} }

   func (s *SQLStore) SaveLease(ctx context.Context, l UsageLease) (UsageLease, error) {
	  if s == nil || s.db == nil {
		  return UsageLease{}, ErrStoreUnavailable
	   }
	   body, err := json.Marshal(l)
	  if err != nil {
		   return UsageLease{}, err
	  }
	   _, err = s.db.ExecContext(ctx, `
INSERT INTO usage_leases (
  id, occ_kind, user_id, asset, dest, amount, today_sum, velocity_1h, velocity_24h,
  dest_count, daily_cap, velocity_cap, window_print, cap_print, state, ttl_ms,
  issued_at, expires_at, reserved_at, committed_at, expired_at, revoked_at,
  reservation_id, reservation_seq, proof_id, hit_id, idem, revoke_reason, body, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  state=VALUES(state), reservation_id=VALUES(reservation_id), reservation_seq=VALUES(reservation_seq),
  proof_id=VALUES(proof_id), hit_id=VALUES(hit_id), reserved_at=VALUES(reserved_at),
  committed_at=VALUES(committed_at), expired_at=VALUES(expired_at), revoked_at=VALUES(revoked_at),
  revoke_reason=VALUES(revoke_reason), body=VALUES(body), updated_at=VALUES(updated_at)`,
		  l.ID,
		  string(l.Kind), l.UserID, l.Asset, l.Dest, l.Amount, l.TodaySum, l.Velocity1h, l.Velocity24,
		  l.DestN,
		  l.DailyCap, l.VelocityCap, l.WindowPrint, l.CapPrint, string(l.State), l.TTL.Milliseconds(),
		   l.IssuedAt, l.ExpiresAt, nullTime(l.ReservedAt), nullTime(l.CommittedAt), nullTime(l.ExpiredAt), nullTime(l.RevokedAt),
		   l.ReservationID, l.ReservationSeq, l.ProofID, l.HitID, l.Idem, l.RevokeReason, body, l.CreatedAt, l.UpdatedAt,
	   )
	  if err != nil {
		  return UsageLease{}, err
	  }
	  return l, nil
   }

   func (s *SQLStore) GetLease(ctx context.Context, id string) (UsageLease, error) {
	  return s.scanLease(ctx, `SELECT body FROM usage_leases WHERE id=?`, strings.TrimSpace(id))
  }

   func (s *SQLStore) GetLeaseByIdem(ctx context.Context, idem string) (UsageLease, error) {
	  return s.scanLease(ctx, `SELECT body FROM usage_leases WHERE idem=? ORDER BY issued_at DESC LIMIT 1`, strings.TrimSpace(idem))
   }

  func (s *SQLStore) scanLease(ctx context.Context, q string, args ...any) (UsageLease, error) {
	   if s == nil || s.db == nil {
		  return UsageLease{}, ErrStoreUnavailable
	  }
	   var body []byte
	  if err := s.db.QueryRowContext(ctx, q, args...).Scan(&body); err != nil {
		  if err == sql.ErrNoRows {
			  return UsageLease{}, ErrLeaseNotFound
		  }
		   return UsageLease{}, err
	  }
	   var l UsageLease
	   if err := json.Unmarshal(body, &l); err != nil || l.ID == "" {
		   return UsageLease{},
			   ErrLeaseNotFound
	   }
	   return l, nil
  }

   func (s *SQLStore) listLeases(ctx context.Context, q string, args ...any) ([]UsageLease, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrStoreUnavailable
	  }
	   rows, err := s.db.QueryContext(ctx, q, args...)
	   if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   out := make([]UsageLease, 0)
	   for rows.Next() {
		   var body []byte
		  if err := rows.Scan(&body); err != nil {
			   return nil, err
		   }
		   var l UsageLease
		   if err := json.Unmarshal(body, &l); err != nil {
			   continue
		   }
		   out = append(out, l)
	   }
	  return out, rows.Err()
  }

  func (s *SQLStore) ListLeasesByUser(ctx context.Context, userID string, limit int) ([]UsageLease, error) {
	   if limit <= 0 {
		   limit = 50
	  }
	   return s.listLeases(ctx, `SELECT body FROM usage_leases WHERE user_id=? ORDER BY issued_at DESC LIMIT ?`, userID, limit)
  }

   func (s *SQLStore) ListLeasesByState(ctx context.Context, state LeaseState, limit int) ([]UsageLease, error) {
	  if limit <= 0 {
		  limit = DefaultExpiryBatch
	  }
	   return s.listLeases(ctx, `SELECT body FROM usage_leases WHERE state=? ORDER BY issued_at ASC LIMIT ?`, string(state), limit)
  }

  func (s *SQLStore) ListLeasesByWindow(ctx context.Context, windowKey string, limit int) ([]UsageLease, error) {
	  if limit <= 0 {
		   limit = DefaultPatrolBatch
	  }
	  parts := strings.Split(windowKey, "|")
	  if len(parts) < 4 {
		  return nil, nil
	  }
	  return s.listLeases(ctx, `SELECT body FROM usage_leases WHERE occ_kind=? AND user_id=? AND asset=? AND dest=? ORDER BY issued_at ASC LIMIT ?`,
		  parts[0], parts[1], parts[2], parts[3], limit)
   }

  func (s *SQLStore) ListExpiredLive(ctx context.Context, before time.Time, limit int) ([]UsageLease, error) {
	   if limit <= 0 {
		   limit = DefaultExpiryBatch
	   }
	  return s.listLeases(ctx, `SELECT body FROM usage_leases WHERE state IN ('snapshotted','reserved') AND expires_at<=? ORDER BY expires_at ASC LIMIT ?`, before, limit)
   }

  func (s *SQLStore) CountByState(ctx context.Context, state LeaseState) (int, error) {
	  if s == nil || s.db == nil {
		  return 0, ErrStoreUnavailable
	  }
	   var n int
	  err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM usage_leases WHERE state=?`, string(state)).Scan(&n)
	   return n, err
   }

  func (s *SQLStore) SaveReservation(ctx context.Context, r UsageReservation) (UsageReservation, error) {
	   if s == nil || s.db == nil {
		  return UsageReservation{}, ErrStoreUnavailable
	   }
	   body, err := json.Marshal(r)
	   if err != nil {
		   return UsageReservation{}, err
	  }
	   _, err = s.db.ExecContext(ctx, `
INSERT INTO usage_reservations (
  id, lease_id, window_key, occ_kind, user_id, asset, dest, amount, state, seq,
  held_at, expires_at, released_at, consumed_at, reason, body, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  state=VALUES(state), released_at=VALUES(released_at), consumed_at=VALUES(consumed_at),
  reason=VALUES(reason), body=VALUES(body), updated_at=VALUES(updated_at)`,
		   r.ID, r.LeaseID, r.WindowKey, string(r.Kind), r.UserID, r.Asset, r.Dest, r.Amount, string(r.State), r.Seq,
		  r.HeldAt,
		   r.ExpiresAt, nullTime(r.ReleasedAt), nullTime(r.ConsumedAt), r.Reason, body, r.CreatedAt, r.UpdatedAt,
	  )
	   if err != nil {
		   return UsageReservation{}, err
	  }
	   return r, nil
   }

  func (s *SQLStore) scanRes(ctx context.Context, q string, args ...any) (UsageReservation, error) {
	   if s == nil || s.db == nil {
		  return UsageReservation{}, ErrStoreUnavailable
	   }
	   var body []byte
	  if err := s.db.QueryRowContext(ctx, q, args...).Scan(&body); err != nil {
		  if err == sql.ErrNoRows {
			  return UsageReservation{}, ErrReservationMissing
		   }
		  return UsageReservation{}, err
	  }
	  var r UsageReservation
	  if err := json.Unmarshal(body, &r); err != nil || r.ID == "" {
		   return UsageReservation{}, ErrReservationMissing
	   }
	  return r, nil
   }

  func (s *SQLStore) GetReservation(ctx context.Context, id string) (UsageReservation, error) {
	   return s.scanRes(ctx, `SELECT body FROM usage_reservations WHERE id=?`, strings.TrimSpace(id))
  }

  func (s *SQLStore) GetReservationByLease(ctx context.Context, leaseID string) (UsageReservation, error) {
	   return s.scanRes(ctx, `SELECT body FROM usage_reservations WHERE lease_id=? ORDER BY held_at DESC LIMIT 1`, strings.TrimSpace(leaseID))
   }

   func (s *SQLStore) listRes(ctx context.Context, q string, args ...any) ([]UsageReservation, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrStoreUnavailable
	   }
	  rows, err := s.db.QueryContext(ctx, q, args...)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   out := make([]UsageReservation, 0)
	   for rows.Next() {
		  var body []byte
		  if err := rows.Scan(&body); err != nil {
			   return nil, err
		  }
		   var r UsageReservation
		  if json.Unmarshal(body, &r) == nil && r.ID != "" {
			  out = append(out, r)
		  }
	  }
	   return out, rows.Err()
  }

  func (s *SQLStore) ListHeldByWindow(ctx context.Context,
	  windowKey string) ([]UsageReservation, error) {
	   return s.listRes(ctx, `SELECT body FROM usage_reservations WHERE window_key=? AND state='held' ORDER BY seq ASC`, windowKey)
  }

   func (s *SQLStore) ListExpiredHeld(ctx context.Context, before time.Time, limit int) ([]UsageReservation, error) {
	   if limit <= 0 {
		   limit = DefaultExpiryBatch
	  }
	   return s.listRes(ctx, `SELECT body FROM usage_reservations WHERE state='held' AND expires_at<=? ORDER BY expires_at ASC LIMIT ?`, before, limit)
   }

  func (s *SQLStore) CountReservations(ctx context.Context, state ReservationState) (int, error) {
	  if s == nil || s.db == nil {
		   return 0, ErrStoreUnavailable
	   }
	   var n int
	  err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM usage_reservations WHERE state=?`, string(state)).Scan(&n)
	   return n, err
  }

   func (s *SQLStore) NextSeq(ctx context.Context, windowKey string) (int64, error) {
	   if s == nil || s.db == nil {
		  return 0, ErrStoreUnavailable
	   }
	   var n int64
	  err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0)+1 FROM usage_reservations WHERE window_key=?`, windowKey).Scan(&n)
	   return n, err
   }

   func (s *SQLStore) SaveProof(ctx context.Context, p UsageDriftProof) (UsageDriftProof, error) {
	   if s == nil || s.db == nil {
		   return UsageDriftProof{}, ErrStoreUnavailable
	   }
	   body, err := json.Marshal(p)
	   if err != nil {
		   return UsageDriftProof{}, err
	  }
	  _, err = s.db.ExecContext(ctx, `
INSERT INTO usage_drift_proofs (
  id, lease_id, user_id, kind, occ_kind, asset, dest, lease_today, live_today,
  lease_print, live_print, lease_cap, live_cap, attempt_amount, reserved_amount,
  would_exceed, blocked, sealed, detail, body, created_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE detail=VALUES(detail), body=VALUES(body)`,
		  p.ID, p.LeaseID, p.UserID, string(p.Kind), p.OccKind, p.Asset, p.Dest, p.LeaseToday, p.LiveToday,
		   p.LeasePrint, p.LivePrint, p.LeaseCap, p.LiveCap, p.AttemptAmount, p.ReservedAmount,
		   boolToInt(p.WouldExceed), boolToInt(p.Blocked), boolToInt(p.Sealed), p.Detail, body, p.CreatedAt,
	   )
	   if err != nil {
		  return UsageDriftProof{}, err
	   }
	   return p, nil
   }

   func (s *SQLStore) GetProof(ctx context.Context, id string) (UsageDriftProof, error) {
	   if s == nil || s.db == nil {
		   return UsageDriftProof{}, ErrStoreUnavailable
	  }
	  var body []byte
	   if err := s.db.QueryRowContext(ctx, `SELECT body FROM usage_drift_proofs WHERE id=?`, strings.TrimSpace(id)).Scan(&body); err != nil {
		  if err == sql.ErrNoRows {
			  return UsageDriftProof{}, ErrProofNotFound
		  }
		  return UsageDriftProof{}, err
	  }
	  var p UsageDriftProof
	  if err := json.Unmarshal(body, &p); err != nil {
		   return UsageDriftProof{}, ErrProofNotFound
	  }
	   return p, nil
   }

   func (s *SQLStore) listProofs(ctx context.Context, q string, args ...any) ([]UsageDriftProof, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrStoreUnavailable
	  }
	  rows, err := s.db.QueryContext(ctx, q, args...)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   out := make([]UsageDriftProof, 0)
	   for rows.Next() {
		   var body []byte
		  if err := rows.Scan(&body); err != nil {
			  return nil, err
		   }
		  var p UsageDriftProof
		   if json.Unmarshal(body, &p) == nil {
			   out = append(out, p)
		   }
	   }
	   return out, rows.Err()
  }

  func (s *SQLStore) ListProofsByLease(ctx context.Context, leaseID string, limit int) ([]UsageDriftProof, error) {
	   if limit <= 0 {
		  limit = 50
	   }
	   return s.listProofs(ctx, `SELECT body FROM usage_drift_proofs WHERE lease_id=? ORDER BY created_at DESC LIMIT ?`, leaseID, limit)
   }

   func (s *SQLStore) ListProofsByUser(ctx context.Context, userID string, limit int) ([]UsageDriftProof, error) {
	  if limit <= 0 {
		   limit = 50
	   }
	  return s.listProofs(ctx, `SELECT body FROM usage_drift_proofs WHERE user_id=? ORDER BY created_at DESC LIMIT ?`, userID, limit)
  }

   func (s *SQLStore) ListProofs(ctx context.Context, kind DriftKind, limit int) ([]UsageDriftProof, error) {
	   if limit <= 0 {
		  limit = 50
	   }
	   if kind == "" {
		  return s.listProofs(ctx, `SELECT body FROM usage_drift_proofs ORDER BY created_at DESC LIMIT ?`, limit)
	   }
	   return s.listProofs(ctx, `SELECT body FROM usage_drift_proofs WHERE kind=? ORDER BY created_at DESC LIMIT ?`, string(kind), limit)
   }

  func (s *SQLStore) CountProofs(ctx context.Context) (int, error) {
	  if s == nil || s.db == nil {
		  return 0, ErrStoreUnavailable
	   }
	   var n int
	   err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM usage_drift_proofs`).Scan(&n)
	   return n, err
   }

  func (s *SQLStore) SaveAudit(ctx context.Context, a UsageLeaseAudit) (UsageLeaseAudit, error) {
	  if s == nil || s.db == nil {
		   return UsageLeaseAudit{}, ErrAuditUnavailable
	   }
	  body, err := json.Marshal(a)
	  if err != nil {
		   return UsageLeaseAudit{}, err
	   }
	   _, err = s.db.ExecContext(ctx, `
INSERT INTO usage_lease_audit (id, lease_id, event, from_state, to_state, actor, detail, body, at)
VALUES (?,?,?,?,?,?,?,?,?)`,
		   a.ID, a.LeaseID, a.Event, string(a.FromState), string(a.ToState), a.Actor, a.Detail, body, a.At,
	  )
	   if err != nil {
		  return UsageLeaseAudit{}, err
	  }
	   return a, nil
   }

   func (s *SQLStore) ListAudit(ctx context.Context, leaseID string, limit int) ([]UsageLeaseAudit, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrAuditUnavailable
	   }
	   if limit <= 0 {
		  limit = 50
	   }
	  rows, err := s.db.QueryContext(ctx, `SELECT body FROM usage_lease_audit WHERE lease_id=? ORDER BY at ASC LIMIT ?`, leaseID, limit)
	   if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   out := make([]UsageLeaseAudit, 0)
	   for rows.Next() {
		  var body []byte
		  if err := rows.Scan(&body); err != nil {
			   return nil, err
		   }
		   var a UsageLeaseAudit
		   if json.Unmarshal(body, &a) == nil {
			  out = append(out, a)
		  }
	  }
	   return out, rows.Err()
  }

  func (s *SQLStore) CountAudits(ctx context.Context) (int, error) {
	  if s == nil || s.db == nil {
		  return 0, ErrAuditUnavailable
	   }
	   var n int
	  err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM usage_lease_audit`).Scan(&n)
	  return n, err
   }

  func nullTime(t *time.Time) any {
	  if t == nil {
		  return nil
	  }
	  return *t
  }

  func boolToInt(b bool) int {
	   if b {
		  return 1
	   }
	  return 0
   }
