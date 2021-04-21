package dustquotegate

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// SQLStore 仓。
type SQLStore struct {
	db *sql.DB
}

// NewSQLStore 包装 *sql.DB。
  func NewSQLStore(db *sql.DB) *SQLStore {
	  return &SQLStore{db: db}
   }

const qInsertLease = ` 
INSERT IGNORE INTO dust_quote_leases (
  lease_id, user_id, asset, amount, usdt_want, rate, policy_gen, cap_bps,
  book_fp, digest, state, ttl_sec, idempotency_key, actor, scan_batch, scan_index,
  quoted_at, bound_at, settled_at, expired_at, revoked_at, revoke_why, flip_epoch,
  convert_id, floor_pinned, min_pinned, book_avail, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

  func (s *SQLStore) SaveLease(ctx context.Context, l DustQuoteLease) error {
	  if s == nil || s.db == nil {
		  return ErrUnavailable
	   }
	  _,
		   err := s.db.ExecContext(ctx, qInsertLease,
		   l.LeaseID, l.UserID, l.Asset, l.Amount, l.USDTWant, l.Rate, l.PolicyGen, l.CapBPS,
		   l.BookFP, l.Digest, string(l.State), l.TTLSec, l.Idem, l.Actor, l.ScanBatch, l.ScanIndex,
		  l.QuotedAt,
		   nullTime(l.BoundAt), nullTime(l.SettledAt), nullTime(l.ExpiredAt), nullTime(l.RevokedAt),
		   l.RevokeWhy, l.FlipEpoch, l.ConvertID, l.Fingerprint.FloorPinned, l.Fingerprint.MinPinned,
		  l.Fingerprint.BookAvail, l.CreatedAt, l.UpdatedAt,
	   )
	  if err != nil {
		  return ErrStore
	   }
	  return nil
  }

const qGetLease = `
SELECT lease_id, user_id, asset, amount, usdt_want, rate, policy_gen, cap_bps,
  book_fp, digest, state, ttl_sec, idempotency_key, actor, scan_batch, scan_index,
  quoted_at, bound_at, settled_at, expired_at, revoked_at, revoke_why, flip_epoch,
  convert_id, floor_pinned, min_pinned, book_avail, created_at, updated_at
FROM dust_quote_leases WHERE lease_id=?`

   func (s *SQLStore) GetLease(ctx context.Context, leaseID string) (DustQuoteLease, error) {
	   if s == nil || s.db == nil {
		  return DustQuoteLease{}, ErrUnavailable
	  }
	   row := s.db.QueryRowContext(ctx, qGetLease, strings.TrimSpace(leaseID))
	   return scanLease(row)
   }

   func (s *SQLStore) GetLeaseByIdem(ctx context.Context, idem string) (DustQuoteLease, error) {
	  if s == nil || s.db == nil {
		  return DustQuoteLease{}, ErrUnavailable
	  }
	   row := s.db.QueryRowContext(ctx,
		  strings.Replace(qGetLease, "WHERE lease_id=?", "WHERE idempotency_key=?", 1),
		  strings.TrimSpace(idem))
	  return scanLease(row)
   }

   func (s *SQLStore) ListLeasesByUserAsset(ctx context.Context, user, asset string) ([]DustQuoteLease, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	  }
	   q := `SELECT lease_id, user_id, asset, amount, usdt_want, rate, policy_gen, cap_bps,
  book_fp, digest, state, ttl_sec, idempotency_key, actor, scan_batch, scan_index,
  quoted_at, bound_at, settled_at, expired_at, revoked_at, revoke_why, flip_epoch,
  convert_id, floor_pinned, min_pinned, book_avail, created_at, updated_at
FROM dust_quote_leases WHERE (?='' OR user_id=?) AND (?='' OR asset=?)`
	   rows,
		   err := s.db.QueryContext(ctx, q, user, user, asset, asset)
	   if err != nil {
		  return nil, ErrStore
	  }
	   defer rows.Close()
	  return scanLeaseRows(rows)
   }

   func (s *SQLStore) ListOpenByAsset(ctx context.Context, asset string) ([]DustQuoteLease, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	   }
	  q := `SELECT lease_id, user_id, asset, amount, usdt_want, rate, policy_gen, cap_bps,
  book_fp, digest, state, ttl_sec, idempotency_key, actor, scan_batch, scan_index,
  quoted_at, bound_at, settled_at, expired_at, revoked_at, revoke_why, flip_epoch,
  convert_id, floor_pinned, min_pinned, book_avail, created_at, updated_at
FROM dust_quote_leases WHERE asset=? AND state IN ('quoted','bound')`
	   rows, err := s.db.QueryContext(ctx, q, asset)
	  if err != nil {
		   return nil, ErrStore
	  }
	   defer rows.Close()
	  return scanLeaseRows(rows)
  }

  func (s *SQLStore) ListQuotedOrBound(ctx context.Context, now time.Time, limit int) ([]DustQuoteLease, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	  }
	  if limit <= 0 {
		   limit = 200
	  }
	   q := `SELECT lease_id, user_id, asset, amount, usdt_want, rate, policy_gen, cap_bps,
  book_fp, digest, state, ttl_sec, idempotency_key, actor, scan_batch, scan_index,
  quoted_at, bound_at, settled_at, expired_at, revoked_at, revoke_why, flip_epoch,
  convert_id, floor_pinned, min_pinned, book_avail, created_at, updated_at
FROM dust_quote_leases WHERE state IN ('quoted','bound') AND quoted_at<=? LIMIT ?`
	   rows,
		   err := s.db.QueryContext(ctx, q, now, limit)
	  if err != nil {
		   return nil, ErrStore
	  }
	  defer rows.Close()
	  return scanLeaseRows(rows)
  }

   func (s *SQLStore) CASState(ctx context.Context, leaseID string, from, to LeaseState, patch DustQuoteLease) error {
	   if s == nil || s.db == nil {
		  return ErrUnavailable
	   }
	  res, err := s.db.ExecContext(ctx, `
UPDATE dust_quote_leases SET state=?, bound_at=COALESCE(?, bound_at), settled_at=COALESCE(?, settled_at),
  expired_at=COALESCE(?, expired_at), revoked_at=COALESCE(?, revoked_at),
  revoke_why=IF(?='', revoke_why, ?), convert_id=IF(?='', convert_id, ?), updated_at=?
WHERE lease_id=? AND state=?`,
		   string(to), nullTime(patch.BoundAt), nullTime(patch.SettledAt), nullTime(patch.ExpiredAt),
		   nullTime(patch.RevokedAt),
		   patch.RevokeWhy, patch.RevokeWhy, patch.ConvertID, patch.ConvertID,
		  patch.UpdatedAt, leaseID, string(from),
	   )
	   if err != nil {
		  return ErrStore
	  }
	  n, _ := res.RowsAffected()
	  if n == 0 {
		   return ErrInvalidTransition
	  }
	  return nil
   }

   func (s *SQLStore) SaveProof(ctx context.Context, p DustQuoteDriftProof) error {
	  if s == nil || s.db == nil {
		   return ErrUnavailable
	  }
	   _, err := s.db.ExecContext(ctx, `
INSERT IGNORE INTO dust_quote_drift_proofs (
  proof_id, lease_id, user_id, asset, kind, quoted_rate, live_rate,
  quoted_gen, live_gen, quoted_book, live_book, quoted_cap, live_cap,
  detail, actor, scan_batch, at, created_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		  p.ProofID, p.LeaseID, p.UserID, p.Asset, string(p.Kind), p.QuotedRate, p.LiveRate,
		   p.QuotedGen, p.LiveGen, p.QuotedBook, p.LiveBook, p.QuotedCap, p.LiveCap,
		  p.Detail, p.Actor, p.ScanBatch, p.At, p.CreatedAt,
	   )
	  if err != nil {
		  return ErrStore
	   }
	  return nil
   }

  func (s *SQLStore) GetProof(ctx context.Context, proofID string) (DustQuoteDriftProof, error) {
	  if s == nil || s.db == nil {
		  return DustQuoteDriftProof{}, ErrUnavailable
	   }
	  row := s.db.QueryRowContext(ctx, `
SELECT proof_id, lease_id, user_id, asset, kind, quoted_rate, live_rate,
  quoted_gen, live_gen, quoted_book, live_book, quoted_cap, live_cap,
  detail, actor, scan_batch, at, created_at
FROM dust_quote_drift_proofs WHERE proof_id=?`, proofID)
	   return scanProof(row)
  }

   func (s *SQLStore) ListProofsByLease(ctx context.Context, leaseID string) ([]DustQuoteDriftProof, error) {
	  return s.listProofs(ctx, `SELECT proof_id, lease_id, user_id, asset, kind, quoted_rate, live_rate,
  quoted_gen, live_gen, quoted_book, live_book, quoted_cap, live_cap,
  detail, actor, scan_batch, at, created_at FROM dust_quote_drift_proofs WHERE lease_id=?`, leaseID)
  }

  func (s *SQLStore) ListProofsByUser(ctx context.Context, user string, limit int) ([]DustQuoteDriftProof, error) {
	   if limit <= 0 {
		  limit = 50
	   }
	   return s.listProofs(ctx, `SELECT proof_id, lease_id, user_id, asset, kind, quoted_rate, live_rate,
  quoted_gen, live_gen, quoted_book, live_book, quoted_cap, live_cap,
  detail, actor, scan_batch, at, created_at FROM dust_quote_drift_proofs WHERE (?='' OR user_id=?) LIMIT ?`, user, user, limit)
  }

   func (s *SQLStore) ListProofsByKind(ctx context.Context, kind ProofKind, limit int) ([]DustQuoteDriftProof, error) {
	  if limit <= 0 {
		  limit = 50
	   }
	  return s.listProofs(ctx, `SELECT proof_id, lease_id, user_id, asset, kind, quoted_rate, live_rate,
  quoted_gen, live_gen, quoted_book, live_book, quoted_cap, live_cap,
  detail, actor, scan_batch, at, created_at FROM dust_quote_drift_proofs WHERE kind=? LIMIT ?`, string(kind), limit)
  }

   func (s *SQLStore) listProofs(ctx context.Context, q string, args ...any) ([]DustQuoteDriftProof, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	   }
	   rows, err := s.db.QueryContext(ctx, q, args...)
	   if err != nil {
		  return nil, ErrStore
	   }
	   defer rows.Close()
	  out := make([]DustQuoteDriftProof, 0)
	  for rows.Next() {
		   p, err := scanProof(rows)
		   if err != nil {
			  return nil, err
		   }
		  out = append(out, p)
	  }
	   return out, nil
   }

  func (s *SQLStore) AppendAudit(ctx context.Context, a DustQuoteAudit) error {
	   if s == nil || s.db == nil {
		   return ErrUnavailable
	  }
	   _, err := s.db.ExecContext(ctx, ` 
INSERT INTO dust_quote_audit (audit_id, lease_id, action, from_state, to_state, actor, detail, at)
VALUES (?,?,?,?,?,?,?,?)`, a.AuditID, a.LeaseID, a.Action, a.From, a.To, a.Actor, a.Detail, a.At)
	  if err != nil {
		  return ErrAudit
	   }
	  return nil
  }

  func (s *SQLStore) ListAudit(ctx context.Context, leaseID string) ([]DustQuoteAudit, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	   }
	  rows, err := s.db.QueryContext(ctx, `
SELECT audit_id, lease_id, action, from_state, to_state, actor, detail, at
FROM dust_quote_audit WHERE lease_id=? ORDER BY at ASC`, leaseID)
	  if err != nil {
		   return nil, ErrStore
	  }
	  defer rows.Close()
	   out := make([]DustQuoteAudit, 0)
	   for rows.Next() {
		  var a DustQuoteAudit
		   if err := rows.Scan(&a.AuditID, &a.LeaseID, &a.Action, &a.From, &a.To, &a.Actor, &a.Detail, &a.At); err != nil {
			  return nil, ErrStore
		  }
		   out = append(out, a)
	  }
	   return out, nil
  }

  func (s *SQLStore) SaveCursor(ctx context.Context, c ScanCursor) error {
	  if s == nil || s.db == nil {
		  return ErrUnavailable
	  }
	  _, err := s.db.ExecContext(ctx, `
INSERT INTO dust_quote_scan_cursors (batch_id, user_id, next_index, halted, halt_reason, settled_n, rejected_n, flip_epoch, actor, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE next_index=VALUES(next_index), halted=VALUES(halted), halt_reason=VALUES(halt_reason),
  settled_n=VALUES(settled_n), rejected_n=VALUES(rejected_n), flip_epoch=VALUES(flip_epoch), actor=VALUES(actor), updated_at=VALUES(updated_at)`,
		  c.BatchID, c.UserID, c.NextIndex, boolToInt(c.Halted), c.HaltReason, c.SettledN, c.RejectedN, c.FlipEpoch, c.Actor, c.UpdatedAt)
	  if err != nil {
		  return ErrScanCursor
	  }
	   return nil
  }

  func (s *SQLStore) GetCursor(ctx context.Context, batchID string) (ScanCursor, error) {
	  if s == nil || s.db == nil {
		  return ScanCursor{},
			  ErrUnavailable
	   }
	  var c ScanCursor
	  var halted int
	   err := s.db.QueryRowContext(ctx, `
SELECT batch_id, user_id, next_index, halted, halt_reason, settled_n, rejected_n, flip_epoch, actor, updated_at
FROM dust_quote_scan_cursors WHERE batch_id=?`, batchID).Scan(
		   &c.BatchID, &c.UserID, &c.NextIndex, &halted, &c.HaltReason, &c.SettledN, &c.RejectedN, &c.FlipEpoch, &c.Actor, &c.UpdatedAt)
	  if err == sql.ErrNoRows {
		   return ScanCursor{},
			   ErrScanCursor
	   }
	   if err != nil {
		   return ScanCursor{}, ErrStore
	   }
	   c.Halted = halted != 0
	   return c, nil
   }

  func (s *SQLStore) ListHaltedCursors(ctx context.Context) ([]ScanCursor, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	  }
	  rows, err := s.db.QueryContext(ctx, `
SELECT batch_id, user_id, next_index, halted, halt_reason, settled_n, rejected_n, flip_epoch, actor, updated_at
FROM dust_quote_scan_cursors WHERE halted=1`)
	  if err != nil {
		  return nil, ErrStore
	   }
	   defer rows.Close()
	  out := make([]ScanCursor, 0)
	   for rows.Next() {
		   var c ScanCursor
		   var halted int
		  if err := rows.Scan(&c.BatchID, &c.UserID, &c.NextIndex, &halted, &c.HaltReason, &c.SettledN, &c.RejectedN, &c.FlipEpoch, &c.Actor, &c.UpdatedAt); err != nil {
			   return nil, ErrStore
		   }
		  c.Halted = halted != 0
		  out = append(out, c)
	   }
	   return out, nil
  }

  func (s *SQLStore) SaveFlip(ctx context.Context, e FlipEvent) error {
	   if s == nil || s.db == nil {
		  return ErrUnavailable
	  }
	   _, err := s.db.ExecContext(ctx, ` 
INSERT IGNORE INTO dust_quote_flip_events (event_id, asset, kind, old_rate, new_rate, old_gen, new_gen, epoch, at, done)
VALUES (?,?,?,?,?,?,?,?,?,0)`, e.EventID, e.Asset, string(e.Kind), e.OldRate, e.NewRate, e.OldGen, e.NewGen, e.Epoch, e.At)
	  if err != nil {
		  return ErrStore
	   }
	   return nil
   }

  func (s *SQLStore) ListPendingFlips(ctx context.Context, limit int) ([]FlipEvent, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	  }
	  if limit <= 0 {
		  limit = 100
	  }
	   rows, err := s.db.QueryContext(ctx, `
SELECT event_id, asset, kind, old_rate, new_rate, old_gen, new_gen, epoch, at
FROM dust_quote_flip_events WHERE done=0 LIMIT ?`, limit)
	   if err != nil {
		  return nil, ErrStore
	  }
	   defer rows.Close()
	  out := make([]FlipEvent, 0)
	   for rows.Next() {
		  var e FlipEvent
		   var kind string
		   if err := rows.Scan(&e.EventID, &e.Asset, &kind, &e.OldRate, &e.NewRate, &e.OldGen, &e.NewGen, &e.Epoch, &e.At); err != nil {
			   return nil, ErrStore
		  }
		   e.Kind = FlipKind(kind)
		  out = append(out, e)
	   }
	   return out, nil
  }

   func (s *SQLStore) MarkFlipDone(ctx context.Context, eventID string) error {
	   if s == nil || s.db == nil {
		  return ErrUnavailable
	   }
	   _, err := s.db.ExecContext(ctx, `UPDATE dust_quote_flip_events SET done=1 WHERE event_id=?`, eventID)
	   if err != nil {
		  return ErrStore
	  }
	   return nil
   }

   func (s *SQLStore) NextFlipEpoch(ctx context.Context, asset string) (int64, error) {
	  if s == nil || s.db == nil {
		   return 0, ErrUnavailable
	  }
	  _, err := s.db.ExecContext(ctx, `
INSERT INTO dust_quote_flip_epochs (asset, epoch) VALUES (?,1)
ON DUPLICATE KEY UPDATE epoch=epoch+1`, strings.ToUpper(strings.TrimSpace(asset)))
	  if err != nil {
		   return 0, ErrFlipEpoch
	   }
	   return s.CurrentFlipEpoch(ctx, asset)
  }

   func (s *SQLStore) CurrentFlipEpoch(ctx context.Context, asset string) (int64, error) {
	  if s == nil || s.db == nil {
		  return 0, ErrUnavailable
	  }
	   var n int64
	  err := s.db.QueryRowContext(ctx, `SELECT epoch FROM dust_quote_flip_epochs WHERE asset=?`, strings.ToUpper(strings.TrimSpace(asset))).Scan(&n)
	  if err == sql.ErrNoRows {
		   return 0, nil
	  }
	  if err != nil {
		   return 0, ErrFlipEpoch
	   }
	   return n, nil
   }

type rowScanner interface {
	Scan(dest ...any) error
}

   func scanLease(row rowScanner) (DustQuoteLease, error) {
	   var l DustQuoteLease
	   var state string
	   var bound,
		  settled, expired, revoked sql.NullTime
	  err := row.Scan(
		  &l.LeaseID, &l.UserID, &l.Asset, &l.Amount, &l.USDTWant, &l.Rate, &l.PolicyGen, &l.CapBPS,
		   &l.BookFP,
		   &l.Digest, &state, &l.TTLSec, &l.Idem, &l.Actor, &l.ScanBatch, &l.ScanIndex,
		  &l.QuotedAt, &bound, &settled, &expired, &revoked, &l.RevokeWhy, &l.FlipEpoch,
		   &l.ConvertID, &l.Fingerprint.FloorPinned, &l.Fingerprint.MinPinned, &l.Fingerprint.BookAvail,
		   &l.CreatedAt, &l.UpdatedAt,
	  )
	   if err == sql.ErrNoRows {
		   return DustQuoteLease{}, ErrLeaseNotFound
	  }
	  if err != nil {
		  return DustQuoteLease{}, ErrStore
	   }
	   l.State = LeaseState(state)
	  l.Fingerprint.Asset = l.Asset
	   l.Fingerprint.Rate = l.Rate
	  l.Fingerprint.PolicyGen = l.PolicyGen
	  l.Fingerprint.CapBPS = l.CapBPS
	  l.Fingerprint.BookFP = l.BookFP
	   l.Fingerprint.Digest = l.Digest
	   if bound.Valid {
		  l.BoundAt = bound.Time
	   }
	  if settled.Valid {
		  l.SettledAt = settled.Time
	   }
	   if expired.Valid {
		  l.ExpiredAt = expired.Time
	  }
	  if revoked.Valid {
		  l.RevokedAt = revoked.Time
	  }
	  return l, nil
   }

  func scanLeaseRows(rows *sql.Rows) ([]DustQuoteLease, error) {
	  out := make([]DustQuoteLease, 0)
	   for rows.Next() {
		  l, err := scanLease(rows)
		  if err != nil {
			  return nil, err
		  }
		  out = append(out, l)
	  }
	   return out, nil
  }

   func scanProof(row rowScanner) (DustQuoteDriftProof, error) {
	  var p DustQuoteDriftProof
	   var kind string
	   err := row.Scan(
		  &p.ProofID,
		  &p.LeaseID, &p.UserID, &p.Asset, &kind, &p.QuotedRate, &p.LiveRate,
		   &p.QuotedGen, &p.LiveGen, &p.QuotedBook, &p.LiveBook, &p.QuotedCap, &p.LiveCap,
		   &p.Detail, &p.Actor, &p.ScanBatch, &p.At, &p.CreatedAt,
	   )
	   if err == sql.ErrNoRows {
		  return DustQuoteDriftProof{}, ErrProofNotFound
	   }
	  if err != nil {
		  return DustQuoteDriftProof{}, ErrStore
	   }
	  p.Kind = ProofKind(kind)
	   return p, nil
  }

   func nullTime(t time.Time) any {
	  if t.IsZero() {
		  return nil
	  }
	   return t
   }

   func boolToInt(v bool) int {
	   if v {
		   return 1
	   }
	   return 0
   }
