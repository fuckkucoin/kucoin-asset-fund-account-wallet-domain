// @FileName: sqlstore.go
// @Summary: SQL 存储：fiat_settle_leases / fiat_limits_reservations / fiat_settle_proofs / fiat_settle_audit。
// @Author: EliotVoss
// @Created: 2021.05.18

package fiatsettlegate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SQLStore MySQL / PostgreSQL。

type SQLStore struct {
	db *sql.DB
	dialect string
}

// NewSQLStore 创建 SQL 仓。

  func NewSQLStore(db *sql.DB, dialect string) *SQLStore {
	  return &SQLStore{db: db, dialect: dialect}
   }

  func (s *SQLStore) rewrite(q string) string {
	  if s.dialect != "postgres" {
		  return q
	  }
	  n := 0
	  out := make([]byte, 0, len(q)+16)
	  for i := 0; i < len(q); i++ {
		   if q[i] == '?' {
			  n++
			  out = append(out, '$')
			   out = append(out, []byte(fmt.Sprintf("%d", n))...)
		  } else {
			   out = append(out, q[i])
		  }
	  }
	  return string(out)
  }

  func (s *SQLStore) InsertLease(ctx context.Context, l FiatSettleLease) error {
	   q := `INSERT INTO fiat_settle_leases (
		lease_id, booking_id, user_id, kind, asset, amount, currency,
		psp_intent, kyc_tier, kyc_fingerprint, limits_fingerprint, reservation_id,
		capture_ref, capture_sealed, ledger_id, state, generation, ttl_seconds,
		issued_at, expires_at, bound_at, settled_at, revoked_at, actor, note, compensate_reason
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	   sealed := 0
	  if l.CaptureSealed {
		  sealed = 1
	  }
	  _, err := s.db.ExecContext(ctx, s.rewrite(q),
		  l.LeaseID,
		   l.BookingID, l.UserID, l.Kind, l.Asset, l.Amount, l.Currency,
		   l.PSPIntent, l.KYCTier, l.KYCFingerprint, l.LimitsFingerprint, nullStr(l.ReservationID),
		  nullStr(l.CaptureRef), sealed, nullStr(l.LedgerID), string(l.State), l.Generation, l.TTLSeconds,
		  l.IssuedAt, l.ExpiresAt, nullTime(l.BoundAt), nullTime(l.SettledAt), nullTime(l.RevokedAt),
		  l.Actor,
		   l.Note, l.CompensateReason,
	  )
	   return err
   }

  func (s *SQLStore) GetLease(ctx context.Context, leaseID string) (*FiatSettleLease, error) {
	   q := leaseSelect + ` WHERE lease_id=?`
	  return scanLease(s.db.QueryRowContext(ctx, s.rewrite(q), leaseID))
   }

   func (s *SQLStore) GetLeaseByBooking(ctx context.Context, bookingID string) (*FiatSettleLease, error) {
	  q := leaseSelect + ` WHERE booking_id=? ORDER BY issued_at DESC LIMIT 1`
	   return scanLease(s.db.QueryRowContext(ctx,
		   s.rewrite(q), bookingID))
   }

   func (s *SQLStore) CASLease(ctx context.Context, leaseID string, expectedGen int64, expectedState LeaseState, next FiatSettleLease) error {
	   q := `UPDATE fiat_settle_leases SET
		booking_id=?, user_id=?, kind=?, asset=?, amount=?, currency=?,
		psp_intent=?, kyc_tier=?, kyc_fingerprint=?, limits_fingerprint=?, reservation_id=?,
		capture_ref=?, capture_sealed=?, ledger_id=?, state=?, generation=?, ttl_seconds=?,
		issued_at=?, expires_at=?, bound_at=?, settled_at=?, revoked_at=?, actor=?, note=?, compensate_reason=?
		WHERE lease_id=? AND generation=? AND state=?`
	   sealed := 0
	   if next.CaptureSealed {
		  sealed = 1
	  }
	  res, err := s.db.ExecContext(ctx, s.rewrite(q),
		  next.BookingID, next.UserID, next.Kind, next.Asset, next.Amount, next.Currency,
		   next.PSPIntent,
		  next.KYCTier, next.KYCFingerprint, next.LimitsFingerprint, nullStr(next.ReservationID),
		   nullStr(next.CaptureRef), sealed, nullStr(next.LedgerID), string(next.State), next.Generation, next.TTLSeconds,
		  next.IssuedAt, next.ExpiresAt, nullTime(next.BoundAt), nullTime(next.SettledAt), nullTime(next.RevokedAt),
		   next.Actor,
		  next.Note, next.CompensateReason,
		  leaseID, expectedGen, string(expectedState),
	  )
	   if err != nil {
		   return err
	   }
	   n, _ := res.RowsAffected()
	   if n == 0 {
		   return ErrCAS
	   }
	   return nil
   }

   func (s *SQLStore) ListLeasesByState(ctx context.Context, state LeaseState, limit int) ([]FiatSettleLease, error) {
	  if limit <= 0 {
		  limit = 100
	  }
	  q := leaseSelect + ` WHERE state=? LIMIT ?`
	  rows, err := s.db.QueryContext(ctx, s.rewrite(q), string(state), limit)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   return scanLeases(rows)
   }

   func (s *SQLStore) ListExpiredAuthorized(ctx context.Context, now time.Time, limit int) ([]FiatSettleLease, error) {
	   if limit <= 0 {
		   limit = 100
	   }
	  q := leaseSelect + ` WHERE state='authorized' AND expires_at<=? LIMIT ?`
	   rows,
		  err := s.db.QueryContext(ctx, s.rewrite(q), now, limit)
	   if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  return scanLeases(rows)
   }

   func (s *SQLStore) ListCompensate(ctx context.Context, limit int) ([]FiatSettleLease, error) {
	   return s.ListLeasesByState(ctx,
		   LeaseCompensate, limit)
   }

const leaseSelect = `SELECT lease_id, booking_id, user_id, kind, asset, amount, currency,
	psp_intent, kyc_tier, kyc_fingerprint, limits_fingerprint, reservation_id,
	capture_ref, capture_sealed, ledger_id, state, generation, ttl_seconds,
	issued_at, expires_at, bound_at, settled_at, revoked_at, actor, note, compensate_reason
	FROM fiat_settle_leases`

   func scanLease(row *sql.Row) (*FiatSettleLease, error) {
	   var l FiatSettleLease
	  var res, capRef, led, actor, note, comp sql.NullString
	   var bound, settled, revoked sql.NullTime
	   var sealed int
	  err := row.Scan(
		  &l.LeaseID, &l.BookingID, &l.UserID, &l.Kind, &l.Asset, &l.Amount, &l.Currency,
		  &l.PSPIntent,
		  &l.KYCTier, &l.KYCFingerprint, &l.LimitsFingerprint, &res,
		   &capRef, &sealed, &led, &l.State, &l.Generation, &l.TTLSeconds,
		   &l.IssuedAt, &l.ExpiresAt, &bound, &settled, &revoked, &actor, &note, &comp,
	  )
	  if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrLeaseNotFound
		  }
		   return nil, err
	  }
	  l.ReservationID = res.String
	   l.CaptureRef = capRef.String
	   l.CaptureSealed = sealed == 1
	   l.LedgerID = led.String
	  l.Actor = actor.String
	  l.Note = note.String
	   l.CompensateReason = comp.String
	   if bound.Valid {
		   l.BoundAt = &bound.Time
	  }
	  if settled.Valid {
		   l.SettledAt = &settled.Time
	   }
	   if revoked.Valid {
		  l.RevokedAt = &revoked.Time
	  }
	  return &l, nil
   }

   func scanLeases(rows *sql.Rows) ([]FiatSettleLease, error) {
	   out := make([]FiatSettleLease, 0)
	   for rows.Next() {
		  var l FiatSettleLease
		  var res, capRef, led, actor, note, comp sql.NullString
		   var bound, settled, revoked sql.NullTime
		  var sealed int
		  if err := rows.Scan(
			   &l.LeaseID, &l.BookingID, &l.UserID, &l.Kind, &l.Asset, &l.Amount, &l.Currency,
			   &l.PSPIntent, &l.KYCTier, &l.KYCFingerprint, &l.LimitsFingerprint, &res,
			  &capRef, &sealed, &led, &l.State, &l.Generation, &l.TTLSeconds,
			   &l.IssuedAt, &l.ExpiresAt, &bound, &settled, &revoked, &actor, &note, &comp,
		  ); err != nil {
			  return nil, err
		   }
		   l.ReservationID = res.String
		   l.CaptureRef = capRef.String
		   l.CaptureSealed = sealed == 1
		   l.LedgerID = led.String
		   l.Actor = actor.String
		  l.Note = note.String
		  l.CompensateReason = comp.String
		  if bound.Valid {
			  t := bound.Time
			  l.BoundAt = &t
		   }
		   if settled.Valid {
			   t := settled.Time
			  l.SettledAt = &t
		  }
		   if revoked.Valid {
			   t := revoked.Time
			   l.RevokedAt = &t
		  }
		  out = append(out, l)
	  }
	   return out, rows.Err()
   }

   func (s *SQLStore) InsertReservation(ctx context.Context, r LimitsReservation) error {
	   q := `INSERT INTO fiat_limits_reservations (
		reservation_id, booking_id, user_id, kind, amount, limits_fingerprint, state, lease_id,
		created_at, updated_at, recorded_at, released_at, release_reason
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`
	   _, err := s.db.ExecContext(ctx, s.rewrite(q),
		  r.ReservationID,
		  r.BookingID, r.UserID, r.Kind, r.Amount, r.LimitsFingerprint, string(r.State), nullStr(r.LeaseID),
		  r.CreatedAt, r.UpdatedAt, nullTime(r.RecordedAt), nullTime(r.ReleasedAt), r.ReleaseReason,
	  )
	   return err
   }

  func (s *SQLStore) GetReservation(ctx context.Context, id string) (*LimitsReservation, error) {
	   q := resSelect + ` WHERE reservation_id=?`
	   return scanRes(s.db.QueryRowContext(ctx,
		   s.rewrite(q), id))
  }

   func (s *SQLStore) GetReservationByBooking(ctx context.Context, bookingID string) (*LimitsReservation, error) {
	   q := resSelect + ` WHERE booking_id=? ORDER BY created_at DESC LIMIT 1`
	   return scanRes(s.db.QueryRowContext(ctx, s.rewrite(q), bookingID))
  }

   func (s *SQLStore) CASReservation(ctx context.Context, id string, expected ReservationState, next LimitsReservation) error {
	   q := `UPDATE fiat_limits_reservations SET
		booking_id=?, user_id=?, kind=?, amount=?, limits_fingerprint=?, state=?, lease_id=?,
		updated_at=?, recorded_at=?, released_at=?, release_reason=?
		WHERE reservation_id=? AND state=?`
	  res, err := s.db.ExecContext(ctx, s.rewrite(q),
		  next.BookingID, next.UserID, next.Kind, next.Amount, next.LimitsFingerprint, string(next.State), nullStr(next.LeaseID),
		   next.UpdatedAt, nullTime(next.RecordedAt), nullTime(next.ReleasedAt), next.ReleaseReason,
		  id, string(expected),
	   )
	  if err != nil {
		  return err
	  }
	   n, _ := res.RowsAffected()
	  if n == 0 {
		   return ErrCAS
	  }
	  return nil
   }

  func (s *SQLStore) ListReservationsByState(ctx context.Context, state ReservationState, limit int) ([]LimitsReservation, error) {
	   if limit <= 0 {
		  limit = 100
	  }
	   q := resSelect + ` WHERE state=? LIMIT ?`
	  rows, err := s.db.QueryContext(ctx, s.rewrite(q), string(state), limit)
	   if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	  out := make([]LimitsReservation, 0)
	   for rows.Next() {
		  r, err := scanResRow(rows)
		  if err != nil {
			  return nil, err
		  }
		  out = append(out, *r)
	  }
	   return out, rows.Err()
  }

   func (s *SQLStore) ListOpenReservationsByUser(ctx context.Context, userID, kind string) ([]LimitsReservation, error) {
	   q := resSelect + ` WHERE user_id=? AND kind=? AND state='reserved'`
	  rows, err := s.db.QueryContext(ctx, s.rewrite(q), userID, kind)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  out := make([]LimitsReservation, 0)
	  for rows.Next() {
		  r, err := scanResRow(rows)
		   if err != nil {
			   return nil, err
		  }
		   out = append(out, *r)
	   }
	   return out, rows.Err()
   }

const resSelect = `SELECT reservation_id, booking_id, user_id, kind, amount, limits_fingerprint, state, lease_id,
	created_at, updated_at, recorded_at, released_at, release_reason FROM fiat_limits_reservations`

   func scanRes(row *sql.Row) (*LimitsReservation, error) {
	   var r LimitsReservation
	  var lease, relReason sql.NullString
	   var rec, rel sql.NullTime
	  err := row.Scan(&r.ReservationID,
		  &r.BookingID, &r.UserID, &r.Kind, &r.Amount, &r.LimitsFingerprint, &r.State, &lease,
		   &r.CreatedAt,
		   &r.UpdatedAt, &rec, &rel, &relReason)
	  if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrReservationNotFound
		   }
		   return nil, err
	   }
	  r.LeaseID = lease.String
	   r.ReleaseReason = relReason.String
	   if rec.Valid {
		   r.RecordedAt = &rec.Time
	  }
	  if rel.Valid {
		  r.ReleasedAt = &rel.Time
	  }
	  return &r, nil
  }

   func scanResRow(rows *sql.Rows) (*LimitsReservation, error) {
	   var r LimitsReservation
	  var lease, relReason sql.NullString
	   var rec, rel sql.NullTime
	  err := rows.Scan(&r.ReservationID, &r.BookingID, &r.UserID, &r.Kind, &r.Amount, &r.LimitsFingerprint, &r.State, &lease,
		  &r.CreatedAt, &r.UpdatedAt, &rec, &rel, &relReason)
	  if err != nil {
		  return nil, err
	   }
	   r.LeaseID = lease.String
	  r.ReleaseReason = relReason.String
	  if rec.Valid {
		   r.RecordedAt = &rec.Time
	   }
	  if rel.Valid {
		  r.ReleasedAt = &rel.Time
	   }
	  return &r, nil
   }

  func (s *SQLStore) InsertRefundLease(ctx context.Context, r RefundLease) error {
	  q := `INSERT INTO fiat_refund_leases (
		refund_lease_id, settle_lease_id, booking_id, psp_intent, reverse_ledger, psp_refund_ref,
		state, actor, reason, issued_at, completed_at, fail_reason
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`
	   _, err := s.db.ExecContext(ctx, s.rewrite(q),
		   r.RefundLeaseID, r.SettleLeaseID, r.BookingID, r.PSPIntent, nullStr(r.ReverseLedger), nullStr(r.PSPRefundRef),
		   string(r.State), r.Actor, r.Reason, r.IssuedAt, nullTime(r.CompletedAt), r.FailReason,
	   )
	   return err
   }

  func (s *SQLStore) GetRefundLease(ctx context.Context, id string) (*RefundLease, error) {
	  q := refSelect + ` WHERE refund_lease_id=?`
	  return scanRef(s.db.QueryRowContext(ctx, s.rewrite(q), id))
   }

  func (s *SQLStore) GetRefundLeaseByBooking(ctx context.Context, bookingID string) (*RefundLease, error) {
	   q := refSelect + ` WHERE booking_id=? ORDER BY issued_at DESC LIMIT 1`
	  return scanRef(s.db.QueryRowContext(ctx, s.rewrite(q), bookingID))
   }

   func (s *SQLStore) CASRefundLease(ctx context.Context, id string, expected RefundLeaseState, next RefundLease) error {
	   q := `UPDATE fiat_refund_leases SET reverse_ledger=?, psp_refund_ref=?, state=?, actor=?, reason=?, completed_at=?, fail_reason=? 
		WHERE refund_lease_id=? AND state=?`
	  res, err := s.db.ExecContext(ctx, s.rewrite(q),
		   nullStr(next.ReverseLedger), nullStr(next.PSPRefundRef), string(next.State), next.Actor, next.Reason,
		  nullTime(next.CompletedAt), next.FailReason, id, string(expected),
	   )
	  if err != nil {
		  return err
	  }
	  n, _ := res.RowsAffected()
	   if n == 0 {
		   return ErrCAS
	   }
	  return nil
  }

const refSelect = `SELECT refund_lease_id, settle_lease_id, booking_id, psp_intent, reverse_ledger, psp_refund_ref,
	state, actor, reason, issued_at, completed_at, fail_reason FROM fiat_refund_leases`

  func scanRef(row *sql.Row) (*RefundLease, error) {
	  var r RefundLease
	   var rev, pref, fail sql.NullString
	   var done sql.NullTime
	  err := row.Scan(&r.RefundLeaseID,
		  &r.SettleLeaseID, &r.BookingID, &r.PSPIntent, &rev, &pref,
		  &r.State, &r.Actor, &r.Reason, &r.IssuedAt, &done, &fail)
	  if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrRefundLeaseNotFound
		  }
		  return nil, err
	   }
	  r.ReverseLedger = rev.String
	  r.PSPRefundRef = pref.String
	  r.FailReason = fail.String
	  if done.Valid {
		   r.CompletedAt = &done.Time
	   }
	  return &r, nil
   }

   func (s *SQLStore) InsertProof(ctx context.Context, p FiatSettleProof) error {
	  q := `INSERT INTO fiat_settle_proofs ( 
		proof_id, lease_id, booking_id, kind, fail_closed, winner, loser, summary,
		capture_sealed, settled_held, opened_at, exported_at, detail
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`
	   fc, cap, set := b2i(p.FailClosed), b2i(p.CaptureSealed), b2i(p.SettledHeld)
	  _, err := s.db.ExecContext(ctx, s.rewrite(q),
		   p.ProofID, p.LeaseID, p.BookingID, string(p.Kind), fc, p.Winner, p.Loser, p.Summary,
		  cap, set, p.OpenedAt, nullTime(p.ExportedAt), p.Detail,
	  )
	   return err
   }

   func (s *SQLStore) GetProof(ctx context.Context, id string) (*FiatSettleProof, error) {
	   q := proofSelect + ` WHERE proof_id=?`
	   return scanProof(s.db.QueryRowContext(ctx, s.rewrite(q), id))
  }

  func (s *SQLStore) ListProofs(ctx context.Context, limit int) ([]FiatSettleProof, error) {
	  if limit <= 0 {
		  limit = 200
	  }
	   q := proofSelect + ` ORDER BY opened_at DESC LIMIT ?`
	  rows, err := s.db.QueryContext(ctx, s.rewrite(q), limit)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   return scanProofs(rows)
   }

  func (s *SQLStore) ListProofsByLease(ctx context.Context, leaseID string) ([]FiatSettleProof, error) {
	  q := proofSelect + ` WHERE lease_id=? ORDER BY opened_at`
	   rows,
		   err := s.db.QueryContext(ctx, s.rewrite(q), leaseID)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   return scanProofs(rows)
   }

  func (s *SQLStore) MarkProofExported(ctx context.Context, id string, now time.Time) error {
	   q := `UPDATE fiat_settle_proofs SET exported_at=? WHERE proof_id=?`
	  res, err := s.db.ExecContext(ctx, s.rewrite(q), now, id)
	  if err != nil {
		   return err
	  }
	  n, _ := res.RowsAffected()
	  if n == 0 {
		   return ErrProofNotFound
	   }
	  return nil
  }

const proofSelect = `SELECT proof_id, lease_id, booking_id, kind, fail_closed, winner, loser, summary,
	capture_sealed, settled_held, opened_at, exported_at, detail FROM fiat_settle_proofs`

  func scanProof(row *sql.Row) (*FiatSettleProof, error) {
	  var p FiatSettleProof
	   var fc, cap, set int
	  var exp sql.NullTime
	  err := row.Scan(&p.ProofID, &p.LeaseID, &p.BookingID, &p.Kind, &fc, &p.Winner, &p.Loser, &p.Summary,
		   &cap, &set, &p.OpenedAt, &exp, &p.Detail)
	   if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrProofNotFound
		   }
		  return nil, err
	   }
	   p.FailClosed, p.CaptureSealed, p.SettledHeld = fc == 1, cap == 1, set == 1
	   if exp.Valid {
		   p.ExportedAt = &exp.Time
	   }
	  return &p, nil
  }

  func scanProofs(rows *sql.Rows) ([]FiatSettleProof, error) {
	   out := make([]FiatSettleProof,
		  0)
	   for rows.Next() {
		   var p FiatSettleProof
		  var fc, cap, set int
		   var exp sql.NullTime
		   if err := rows.Scan(&p.ProofID,
			   &p.LeaseID, &p.BookingID, &p.Kind, &fc, &p.Winner, &p.Loser, &p.Summary,
			   &cap, &set, &p.OpenedAt, &exp, &p.Detail); err != nil {
			  return nil, err
		  }
		  p.FailClosed, p.CaptureSealed, p.SettledHeld = fc == 1, cap == 1, set == 1
		   if exp.Valid {
			   t := exp.Time
			   p.ExportedAt = &t
		  }
		   out = append(out, p)
	  }
	  return out, rows.Err()
   }

   func (s *SQLStore) InsertAudit(ctx context.Context, a AuditEvent) error {
	  q := `INSERT INTO fiat_settle_audit (audit_id, lease_id, booking_id, action, actor, from_state, to_state, detail, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`
	   _, err := s.db.ExecContext(ctx, s.rewrite(q),
		   a.AuditID, nullStr(a.LeaseID), a.BookingID, a.Action, a.Actor, a.FromState, a.ToState, a.Detail, a.CreatedAt,
	   )
	  return err
  }

  func (s *SQLStore) ListAuditsByLease(ctx context.Context, leaseID string, limit int) ([]AuditEvent, error) {
	  if limit <= 0 {
		  limit = 100
	   }
	  q := auditSelect + ` WHERE lease_id=? ORDER BY created_at LIMIT ?`
	   rows, err := s.db.QueryContext(ctx, s.rewrite(q), leaseID, limit)
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  return scanAudits(rows)
   }

   func (s *SQLStore) ListAuditsByBooking(ctx context.Context, bookingID string, limit int) ([]AuditEvent, error) {
	   if limit <= 0 {
		  limit = 100
	   }
	  q := auditSelect + ` WHERE booking_id=? ORDER BY created_at LIMIT ?`
	   rows, err := s.db.QueryContext(ctx, s.rewrite(q), bookingID, limit)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   return scanAudits(rows)
  }

const auditSelect = `SELECT audit_id, lease_id, booking_id, action, actor, from_state, to_state, detail, created_at FROM fiat_settle_audit`

   func scanAudits(rows *sql.Rows) ([]AuditEvent, error) {
	  out := make([]AuditEvent, 0)
	   for rows.Next() {
		   var a AuditEvent
		   var lease sql.NullString
		  if err := rows.Scan(&a.AuditID, &lease, &a.BookingID, &a.Action, &a.Actor, &a.FromState, &a.ToState, &a.Detail, &a.CreatedAt); err != nil {
			  return nil, err
		   }
		   a.LeaseID = lease.String
		   out = append(out, a)
	  }
	  return out, rows.Err()
   }

  func (s *SQLStore) PutIdem(ctx context.Context, key, resultID string) error {
	  if key == "" {
		  return nil
	  }
	  q := `INSERT INTO fiat_settle_idem (idem_key, result_id) VALUES (?,?)`
	  _, err := s.db.ExecContext(ctx, s.rewrite(q), key, resultID)
	   return err
   }

  func (s *SQLStore) GetIdem(ctx context.Context, key string) (string, bool, error) {
	   q := `SELECT result_id FROM fiat_settle_idem WHERE idem_key=?`
	  var id string
	   err := s.db.QueryRowContext(ctx, s.rewrite(q), key).Scan(&id)
	   if errors.Is(err, sql.ErrNoRows) {
		  return "", false, nil
	   }
	   if err != nil {
		   return "", false, err
	   }
	  return id, true, nil
  }

   func (s *SQLStore) TryLockBooking(ctx context.Context, bookingID, holder, purpose string, now time.Time, ttl time.Duration) error {
	   q := `INSERT INTO fiat_settle_booking_locks (booking_id, holder, purpose, expires_at)
		VALUES (?,?,?,?) ON DUPLICATE KEY UPDATE
		holder=IF(expires_at<? OR holder=?, VALUES(holder), holder),
		purpose=IF(expires_at<? OR holder=?, VALUES(purpose), purpose),
		expires_at=IF(expires_at<? OR holder=?, VALUES(expires_at), expires_at)`
	  exp := now.Add(ttl)
	   _, err := s.db.ExecContext(ctx, s.rewrite(q), bookingID, holder, purpose, exp, now, holder, now, holder, now, holder)
	   if err != nil {
		  return err
	  }
	   var got string
	   err = s.db.QueryRowContext(ctx, s.rewrite(`SELECT holder FROM fiat_settle_booking_locks WHERE booking_id=?`), bookingID).Scan(&got)
	  if err != nil {
		   return err
	  }
	   if got != holder {
		  return ErrMutexSettleRefund
	  }
	  return nil
  }

   func (s *SQLStore) UnlockBooking(ctx context.Context, bookingID, holder string) error {
	   q := `DELETE FROM fiat_settle_booking_locks WHERE booking_id=? AND holder=?`
	   _, err := s.db.ExecContext(ctx, s.rewrite(q), bookingID, holder)
	  return err
  }

  func (s *SQLStore) GetBookingLock(ctx context.Context, bookingID string) (string, string, bool) {
	  var h, p string
	  err := s.db.QueryRowContext(ctx, s.rewrite(`SELECT holder, purpose FROM fiat_settle_booking_locks WHERE booking_id=?`), bookingID).Scan(&h, &p)
	  if err != nil {
		  return "", "", false
	   }
	  return h, p, true
   }

   func (s *SQLStore) Snapshot() StoreSnapshot { return StoreSnapshot{} }

  func (s *SQLStore) Restore(StoreSnapshot) {}

   func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

  func nullTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}

  func b2i(v bool) int {
	   if v {
		   return 1
	  }
	  return 0
  }
