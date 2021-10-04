package prefroute

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/fundpref"
)

// SQLStore prefroute SQL Store 实现。
type SQLStore struct {
	db *sql.DB
}

// NewSQLStore 构造 SQLStore。

   func NewSQLStore(db *sql.DB) *SQLStore {
	  return &SQLStore{db: db}
  }

// ── PrefGeneration ─────────────────────────────────────────────────────────

   func (s *SQLStore) GetGeneration(ctx context.Context, userID string) (fundpref.PrefGeneration, error) {
	  const q = `SELECT id, user_id, generation, default_book, bumped_by, source_req_id, bumped_at
		FROM fp_pref_generations WHERE user_id=? ORDER BY generation DESC LIMIT 1`
	  row := s.db.QueryRowContext(ctx, q, userID)
	   var g fundpref.PrefGeneration
	  var bumpedAt string
	  err := row.Scan(&g.ID,
		  &g.UserID, &g.Generation, &g.DefaultBook,
		  &g.BumpedBy, &g.SourceReqID, &bumpedAt)
	   if err == sql.ErrNoRows {
		   return fundpref.PrefGeneration{}, fundpref.ErrNotFound
	   }
	  if err != nil {
		  return fundpref.PrefGeneration{}, err
	  }
	   g.BumpedAt, _ = time.Parse(time.RFC3339Nano, bumpedAt)
	   return g, nil
   }

   func (s *SQLStore) SaveGeneration(ctx context.Context, g fundpref.PrefGeneration) error {
	   const q = `INSERT INTO fp_pref_generations
		(id, user_id, generation, default_book, bumped_by, source_req_id, bumped_at)
		VALUES (?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE default_book=VALUES(default_book),
		bumped_by=VALUES(bumped_by), source_req_id=VALUES(source_req_id), bumped_at=VALUES(bumped_at)`
	   _, err := s.db.ExecContext(ctx, q, g.ID, g.UserID, g.Generation, string(g.DefaultBook),
		  g.BumpedBy, g.SourceReqID, g.BumpedAt.Format(time.RFC3339Nano))
	  return err
   }

  func (s *SQLStore) ListGenerations(ctx context.Context, userID string, limit int) ([]fundpref.PrefGeneration, error) {
	   if limit <= 0 {
		  limit = 20
	   }
	  const q = `SELECT id, user_id, generation, default_book, bumped_by, source_req_id, bumped_at
		FROM fp_pref_generations WHERE user_id=? ORDER BY generation DESC LIMIT ?`
	   rows, err := s.db.QueryContext(ctx, q, userID, limit)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   var out []fundpref.PrefGeneration
	   for rows.Next() {
		   var g fundpref.PrefGeneration
		   var bumpedAt string
		  if err := rows.Scan(&g.ID, &g.UserID, &g.Generation, &g.DefaultBook,
			  &g.BumpedBy, &g.SourceReqID, &bumpedAt); err != nil {
			  return nil, err
		  }
		   g.BumpedAt, _ = time.Parse(time.RFC3339Nano, bumpedAt)
		  out = append(out, g)
	   }
	   return out, rows.Err()
   }

// ── DefaultAccountPref ────────────────────────────────────────────────────

  func (s *SQLStore) GetDefaultPref(ctx context.Context, userID string) (fundpref.DefaultAccountPref, error) {
	  const q = `SELECT user_id, book, version, updated_at, updated_by
		FROM fp_default_account_prefs WHERE user_id=?`
	  row := s.db.QueryRowContext(ctx, q, userID)
	  var p fundpref.DefaultAccountPref
	   var updatedAt string
	   err := row.Scan(&p.UserID, &p.Book, &p.Version, &updatedAt, &p.UpdatedBy)
	   if err == sql.ErrNoRows {
		   return fundpref.DefaultAccountPref{}, fundpref.ErrNotFound
	   }
	  if err != nil {
		   return fundpref.DefaultAccountPref{}, err
	  }
	   p.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	   return p, nil
  }

  func (s *SQLStore) SaveDefaultPref(ctx context.Context, p fundpref.DefaultAccountPref) error {
	   const q = `INSERT INTO fp_default_account_prefs
		(user_id, book, version, updated_at, updated_by)
		VALUES (?,?,?,?,?)
		ON DUPLICATE KEY UPDATE book=VALUES(book), version=VALUES(version),
		updated_at=VALUES(updated_at), updated_by=VALUES(updated_by)`
	   _, err := s.db.ExecContext(ctx, q, p.UserID, string(p.Book), p.Version,
		   p.UpdatedAt.Format(time.RFC3339Nano), p.UpdatedBy)
	   return err
  }

// ── PendingSwitch ─────────────────────────────────────────────────────────

  func (s *SQLStore) SavePendingSwitch(ctx context.Context, ps fundpref.PendingSwitch) error {
	   const q = `INSERT INTO fp_pending_switches
		(id, user_id, from_book, to_book, state, requested_by, requested_at,
		 applied_at, cancelled_at, cancel_note, fence_epoch, fence_token)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE state=VALUES(state), applied_at=VALUES(applied_at),
		cancelled_at=VALUES(cancelled_at), cancel_note=VALUES(cancel_note),
		fence_epoch=VALUES(fence_epoch), fence_token=VALUES(fence_token)`
	   var appliedAt,
		   cancelledAt interface{}
	  if ps.AppliedAt != nil {
		  appliedAt = ps.AppliedAt.Format(time.RFC3339Nano)
	   }
	   if ps.CancelledAt != nil {
		  cancelledAt = ps.CancelledAt.Format(time.RFC3339Nano)
	   }
	  _, err := s.db.ExecContext(ctx, q,
		  ps.ID, ps.UserID, string(ps.FromBook), string(ps.ToBook), string(ps.State),
		  ps.RequestedBy, ps.RequestedAt.Format(time.RFC3339Nano),
		  appliedAt, cancelledAt, ps.CancelNote, ps.FenceEpoch, ps.FenceToken)
	   return err
   }

   func (s *SQLStore) GetPendingSwitch(ctx context.Context, id string) (fundpref.PendingSwitch, error) {
	   const q = `SELECT id, user_id, from_book, to_book, state, requested_by, requested_at, 
		applied_at, cancelled_at, cancel_note, fence_epoch, fence_token
		FROM fp_pending_switches WHERE id=?`
	   row := s.db.QueryRowContext(ctx, q, id)
	   return scanPendingSwitch(row)
   }

  func (s *SQLStore) ListPendingSwitches(ctx context.Context, userID string, state fundpref.PendingSwitchState) ([]fundpref.PendingSwitch, error) {
	   const q = `SELECT id, user_id, from_book, to_book, state, requested_by, requested_at, 
		applied_at, cancelled_at, cancel_note, fence_epoch, fence_token
		FROM fp_pending_switches WHERE user_id=? AND state=? ORDER BY requested_at ASC`
	  rows, err := s.db.QueryContext(ctx, q, userID, string(state))
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  var out []fundpref.PendingSwitch
	   for rows.Next() {
		   ps, err := scanPendingSwitch(rows)
		   if err != nil {
			  return nil, err
		  }
		   out = append(out, ps)
	   }
	   return out, rows.Err()
   }

   func (s *SQLStore) UpdatePendingSwitch(ctx context.Context, ps fundpref.PendingSwitch) error {
	   return s.SavePendingSwitch(ctx, ps)
  }

// ── PrefLease ─────────────────────────────────────────────────────────────

   func (s *SQLStore) SavePrefLease(ctx context.Context, l PrefLease) error {
	  const q = `INSERT INTO fp_pref_leases
		(id, user_id, generation, default_book, state, route_snap_id, credit_txn_id,
		 issued_at, expires_at, bound_at, switched_at, blocked_at, block_reason, issued_by,
		 fence_epoch, fence_token)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE state=VALUES(state), route_snap_id=VALUES(route_snap_id),
		bound_at=VALUES(bound_at), switched_at=VALUES(switched_at),
		blocked_at=VALUES(blocked_at), block_reason=VALUES(block_reason),
		fence_epoch=VALUES(fence_epoch), fence_token=VALUES(fence_token)`
	  _, err := s.db.ExecContext(ctx, q,
		  l.ID,
		   l.UserID, l.Generation, string(l.DefaultBook), string(l.State),
		  l.RouteSnapID, l.CreditTxnID,
		  l.IssuedAt.Format(time.RFC3339Nano), l.ExpiresAt.Format(time.RFC3339Nano),
		  nullTime(l.BoundAt), nullTime(l.SwitchedAt), nullTime(l.BlockedAt),
		  l.BlockReason,
		  l.IssuedBy, l.FenceEpoch, l.FenceToken)
	  return err
   }

   func (s *SQLStore) GetPrefLease(ctx context.Context, id string) (PrefLease, error) {
	   const q = `SELECT id, user_id, generation, default_book, state, route_snap_id, credit_txn_id,
		issued_at, expires_at, bound_at, switched_at, blocked_at, block_reason, issued_by,
		fence_epoch, fence_token
		FROM fp_pref_leases WHERE id=?`
	   row := s.db.QueryRowContext(ctx, q, id)
	   return scanPrefLease(row)
   }

   func (s *SQLStore) UpdatePrefLease(ctx context.Context, l PrefLease) error {
	   return s.SavePrefLease(ctx, l)
  }

   func (s *SQLStore) ListActiveLeases(ctx context.Context, userID string) ([]PrefLease, error) {
	  const q = `SELECT id, user_id, generation, default_book, state, route_snap_id, credit_txn_id, 
		issued_at, expires_at, bound_at, switched_at, blocked_at, block_reason, issued_by,
		fence_epoch, fence_token
		FROM fp_pref_leases WHERE user_id=? AND state IN ('open','bound')
		ORDER BY issued_at DESC`
	  rows, err := s.db.QueryContext(ctx, q, userID)
	  if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   var out []PrefLease
	  for rows.Next() {
		   l, err := scanPrefLease(rows)
		   if err != nil {
			   return nil, err
		  }
		  out = append(out, l)
	  }
	   return out, rows.Err()
  }

   func (s *SQLStore) ExpireStaleLeases(ctx context.Context, userID string, newGen int64, now time.Time) ([]PrefLease, error) {
	  const q = `UPDATE fp_pref_leases SET state='switched', switched_at=?, block_reason=?
		WHERE user_id=? AND state IN ('open','bound') AND generation < ?`
	   nowStr := now.Format(time.RFC3339Nano)
	   reason := fmt.Sprintf("generation advanced to %d", newGen)
	   _, err := s.db.ExecContext(ctx, q, nowStr, reason, userID, newGen)
	   if err != nil {
		   return nil, err
	   }
	  // 返回已废止的租约列表
	   const q2 = `SELECT id, user_id, generation, default_book, state, route_snap_id, credit_txn_id,
		issued_at, expires_at, bound_at, switched_at, blocked_at, block_reason, issued_by,
		fence_epoch, fence_token
		FROM fp_pref_leases WHERE user_id=? AND state='switched' AND switched_at=?`
	  rows,
		  err := s.db.QueryContext(ctx, q2, userID, nowStr)
	   if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	  var out []PrefLease
	   for rows.Next() {
		   l, err := scanPrefLease(rows)
		  if err != nil {
			   return nil, err
		   }
		  out = append(out, l)
	  }
	  return out, rows.Err()
  }

// ── RouteSnap ─────────────────────────────────────────────────────────────

   func (s *SQLStore) SaveRouteSnap(ctx context.Context, r RouteSnap) error {
	  const q = `INSERT INTO fp_route_snaps
		(id, lease_id, user_id, generation, bound_book, credited_book, credit_txn_id,
		 asset, amount, snap_at, credited_at, drift_case_id)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE credited_book=VALUES(credited_book),
		credited_at=VALUES(credited_at), drift_case_id=VALUES(drift_case_id)`
	  _, err := s.db.ExecContext(ctx, q,
		  r.ID, r.LeaseID, r.UserID, r.Generation, string(r.BoundBook), string(r.CreditedBook),
		  r.CreditTxnID, r.Asset, r.Amount,
		  r.SnapAt.Format(time.RFC3339Nano), nullTime(r.CreditedAt), r.DriftCaseID)
	   return err
  }

   func (s *SQLStore) GetRouteSnap(ctx context.Context, id string) (RouteSnap, error) {
	  const q = `SELECT id, lease_id, user_id, generation, bound_book, credited_book, credit_txn_id,
		asset, amount, snap_at, credited_at, drift_case_id
		FROM fp_route_snaps WHERE id=?`
	  row := s.db.QueryRowContext(ctx, q, id)
	  return scanRouteSnap(row)
  }

   func (s *SQLStore) UpdateRouteSnap(ctx context.Context, r RouteSnap) error {
	   return s.SaveRouteSnap(ctx, r)
  }

  func (s *SQLStore) ListRouteSnapsByLease(ctx context.Context, leaseID string) ([]RouteSnap, error) {
	  const q = `SELECT id, lease_id, user_id, generation, bound_book, credited_book, credit_txn_id,
		asset, amount, snap_at, credited_at, drift_case_id
		FROM fp_route_snaps WHERE lease_id=? ORDER BY snap_at ASC`
	  rows, err := s.db.QueryContext(ctx, q, leaseID)
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	  var out []RouteSnap
	  for rows.Next() {
		   r, err := scanRouteSnap(rows)
		  if err != nil {
			   return nil, err
		   }
		  out = append(out, r)
	  }
	   return out, rows.Err()
   }

// ── PrefDriftCase ──────────────────────────────────────────────────────────

  func (s *SQLStore) SaveDriftCase(ctx context.Context, d PrefDriftCase) error {
	  const q = `INSERT INTO fp_pref_drift_cases
		(id, user_id, lease_id, route_snap_id, bound_book, credited_book, generation,
		 credit_txn_id, asset, amount, state, detected_at, proven_at, closed_at,
		 proof_id, proof_summary, closed_by)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE state=VALUES(state), proven_at=VALUES(proven_at),
		closed_at=VALUES(closed_at), proof_id=VALUES(proof_id),
		proof_summary=VALUES(proof_summary), closed_by=VALUES(closed_by)`
	  _, err := s.db.ExecContext(ctx, q,
		  d.ID, d.UserID, d.LeaseID, d.RouteSnapID,
		  string(d.BoundBook), string(d.CreditedBook), d.Generation,
		  d.CreditTxnID, d.Asset, d.Amount, string(d.State),
		   d.DetectedAt.Format(time.RFC3339Nano),
		   nullTime(d.ProvenAt), nullTime(d.ClosedAt),
		  d.ProofID, d.ProofSummary, d.ClosedBy)
	  return err
  }

   func (s *SQLStore) GetDriftCase(ctx context.Context, id string) (PrefDriftCase, error) {
	  const q = `SELECT id, user_id, lease_id, route_snap_id, bound_book, credited_book,
		generation, credit_txn_id, asset, amount, state, detected_at,
		proven_at, closed_at, proof_id, proof_summary, closed_by
		FROM fp_pref_drift_cases WHERE id=?`
	   row := s.db.QueryRowContext(ctx, q, id)
	  return scanDriftCase(row)
   }

   func (s *SQLStore) UpdateDriftCase(ctx context.Context, d PrefDriftCase) error {
	   return s.SaveDriftCase(ctx, d)
  }

  func (s *SQLStore) ListOpenDriftCases(ctx context.Context, userID string) ([]PrefDriftCase, error) {
	  const q = `SELECT id, user_id, lease_id, route_snap_id, bound_book, credited_book,
		generation, credit_txn_id, asset, amount, state, detected_at,
		proven_at, closed_at, proof_id, proof_summary, closed_by
		FROM fp_pref_drift_cases WHERE user_id=? AND state='open'
		ORDER BY detected_at DESC`
	  rows, err := s.db.QueryContext(ctx, q, userID)
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   var out []PrefDriftCase
	  for rows.Next() {
		   d, err := scanDriftCase(rows)
		   if err != nil {
			   return nil, err
		   }
		  out = append(out, d)
	  }
	  return out, rows.Err()
  }

  func (s *SQLStore) HasOpenDriftCase(ctx context.Context, userID string) (bool, string, error) {
	   const q = `SELECT id FROM fp_pref_drift_cases WHERE user_id=? AND state='open' LIMIT 1`
	   row := s.db.QueryRowContext(ctx, q, userID)
	  var id string
	  err := row.Scan(&id)
	   if err == sql.ErrNoRows {
		   return false, "", nil
	   }
	  if err != nil {
		   return false, "", err
	   }
	   return true, id, nil
   }

// ── DriftProof ────────────────────────────────────────────────────────────

   func (s *SQLStore) SaveDriftProof(ctx context.Context, p DriftProof) error {
	   b, _ := json.Marshal(p.Evidence)
	   _ = b
	   const q = `INSERT INTO fp_drift_proofs
		(id, drift_case_id, submitted_by, summary, evidence, submitted_at, confirmed_by, confirmed_at)
		VALUES (?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE confirmed_by=VALUES(confirmed_by), confirmed_at=VALUES(confirmed_at)`
	   _, err := s.db.ExecContext(ctx, q,
		  p.ID, p.DriftCaseID, p.SubmittedBy, p.Summary, p.Evidence,
		   p.SubmittedAt.Format(time.RFC3339Nano), p.ConfirmedBy, nullTime(p.ConfirmedAt))
	   return err
   }

   func (s *SQLStore) GetDriftProof(ctx context.Context, id string) (DriftProof, error) {
	  const q = `SELECT id, drift_case_id, submitted_by, summary, evidence,
		submitted_at, confirmed_by, confirmed_at
		FROM fp_drift_proofs WHERE id=?`
	  row := s.db.QueryRowContext(ctx,
		  q, id)
	   return scanDriftProof(row)
  }

   func (s *SQLStore) ListDriftProofs(ctx context.Context, driftCaseID string) ([]DriftProof, error) {
	   const q = `SELECT id, drift_case_id, submitted_by, summary, evidence,
		submitted_at, confirmed_by, confirmed_at
		FROM fp_drift_proofs WHERE drift_case_id=? ORDER BY submitted_at ASC`
	   rows, err := s.db.QueryContext(ctx, q, driftCaseID)
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   var out []DriftProof
	  for rows.Next() {
		  p, err := scanDriftProof(rows)
		   if err != nil {
			  return nil, err
		  }
		   out = append(out, p)
	  }
	  return out, rows.Err()
   }

// ── InflightCredit ────────────────────────────────────────────────────────

   func (s *SQLStore) RegisterInflightCredit(ctx context.Context, ic InflightCredit) error {
	   const q = `INSERT INTO fp_inflight_credits
		(id, user_id, txn_id, asset, amount, state, lease_id, route_snap_id, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE state=VALUES(state), route_snap_id=VALUES(route_snap_id),
		updated_at=VALUES(updated_at)`
	  _, err := s.db.ExecContext(ctx, q,
		   ic.ID, ic.UserID, ic.TxnID, ic.Asset, ic.Amount, string(ic.State),
		  ic.LeaseID, ic.RouteSnapID,
		   ic.CreatedAt.Format(time.RFC3339Nano), ic.UpdatedAt.Format(time.RFC3339Nano))
	   return err
   }

  func (s *SQLStore) GetInflightCredit(ctx context.Context, txnID string) (InflightCredit, error) {
	  const q = `SELECT id, user_id, txn_id, asset, amount, state, lease_id, route_snap_id,
		created_at, updated_at FROM fp_inflight_credits WHERE txn_id=?`
	   row := s.db.QueryRowContext(ctx, q, txnID)
	   return scanInflightCredit(row)
  }

   func (s *SQLStore) UpdateInflightCredit(ctx context.Context, ic InflightCredit) error {
	  return s.RegisterInflightCredit(ctx,
		  ic)
   }

   func (s *SQLStore) ListInflightCredits(ctx context.Context, userID string, state InflightCreditState) ([]InflightCredit, error) {
	   const q = `SELECT id, user_id, txn_id, asset, amount, state, lease_id, route_snap_id, 
		created_at, updated_at FROM fp_inflight_credits WHERE user_id=? AND state=?
		ORDER BY created_at ASC`
	  rows, err := s.db.QueryContext(ctx, q, userID, string(state))
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   var out []InflightCredit
	   for rows.Next() {
		   ic, err := scanInflightCredit(rows)
		  if err != nil {
			   return nil, err
		  }
		   out = append(out, ic)
	   }
	   return out, rows.Err()
   }

// ── scan helpers ──────────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...any) error
}

  func scanPendingSwitch(row scanner) (fundpref.PendingSwitch, error) {
	   var ps fundpref.PendingSwitch
	   var reqAt, appliedAt, cancelledAt sql.NullString
	  err := row.Scan(&ps.ID, &ps.UserID, &ps.FromBook, &ps.ToBook, &ps.State,
		  &ps.RequestedBy,
		  &reqAt, &appliedAt, &cancelledAt, &ps.CancelNote,
		   &ps.FenceEpoch, &ps.FenceToken)
	  if err == sql.ErrNoRows {
		  return fundpref.PendingSwitch{}, fundpref.ErrNotFound
	  }
	   if err != nil {
		  return fundpref.PendingSwitch{}, err
	   }
	   if reqAt.Valid {
		  ps.RequestedAt, _ = time.Parse(time.RFC3339Nano, reqAt.String)
	   }
	  if appliedAt.Valid {
		   t, _ := time.Parse(time.RFC3339Nano, appliedAt.String)
		  ps.AppliedAt = &t
	  }
	  if cancelledAt.Valid {
		   t,
			  _ := time.Parse(time.RFC3339Nano, cancelledAt.String)
		   ps.CancelledAt = &t
	  }
	   return ps, nil
   }

   func scanPrefLease(row scanner) (PrefLease, error) {
	  var l PrefLease
	  var issuedAt, expiresAt string
	  var boundAt, switchedAt, blockedAt sql.NullString
	   err := row.Scan(&l.ID,
		   &l.UserID, &l.Generation, &l.DefaultBook, &l.State,
		  &l.RouteSnapID, &l.CreditTxnID,
		  &issuedAt, &expiresAt, &boundAt, &switchedAt, &blockedAt,
		  &l.BlockReason, &l.IssuedBy, &l.FenceEpoch, &l.FenceToken)
	   if err == sql.ErrNoRows {
		   return PrefLease{}, ErrNotFound
	   }
	  if err != nil {
		  return PrefLease{}, err
	  }
	  l.IssuedAt, _ = time.Parse(time.RFC3339Nano, issuedAt)
	   l.ExpiresAt, _ = time.Parse(time.RFC3339Nano, expiresAt)
	  if boundAt.Valid {
		   t, _ := time.Parse(time.RFC3339Nano, boundAt.String)
		  l.BoundAt = &t
	  }
	   if switchedAt.Valid {
		   t, _ := time.Parse(time.RFC3339Nano, switchedAt.String)
		   l.SwitchedAt = &t
	   }
	  if blockedAt.Valid {
		  t, _ := time.Parse(time.RFC3339Nano, blockedAt.String)
		  l.BlockedAt = &t
	   }
	  return l, nil
   }

  func scanRouteSnap(row scanner) (RouteSnap, error) {
	   var r RouteSnap
	   var snapAt string
	  var creditedAt sql.NullString
	   err := row.Scan(&r.ID,
		   &r.LeaseID, &r.UserID, &r.Generation, &r.BoundBook, &r.CreditedBook,
		  &r.CreditTxnID, &r.Asset, &r.Amount, &snapAt, &creditedAt, &r.DriftCaseID)
	  if err == sql.ErrNoRows {
		  return RouteSnap{}, ErrNotFound
	   }
	   if err != nil {
		  return RouteSnap{}, err
	  }
	   r.SnapAt, _ = time.Parse(time.RFC3339Nano, snapAt)
	  if creditedAt.Valid {
		   t, _ := time.Parse(time.RFC3339Nano, creditedAt.String)
		   r.CreditedAt = &t
	  }
	   return r, nil
  }

   func scanDriftCase(row scanner) (PrefDriftCase, error) {
	   var d PrefDriftCase
	   var detectedAt string
	   var provenAt, closedAt sql.NullString
	   err := row.Scan(&d.ID,
		  &d.UserID, &d.LeaseID, &d.RouteSnapID,
		  &d.BoundBook, &d.CreditedBook, &d.Generation,
		  &d.CreditTxnID, &d.Asset, &d.Amount, &d.State,
		   &detectedAt,
		  &provenAt, &closedAt, &d.ProofID, &d.ProofSummary, &d.ClosedBy)
	  if err == sql.ErrNoRows {
		   return PrefDriftCase{}, ErrNotFound
	  }
	   if err != nil {
		  return PrefDriftCase{}, err
	   }
	  d.DetectedAt, _ = time.Parse(time.RFC3339Nano, detectedAt)
	  if provenAt.Valid {
		  t, _ := time.Parse(time.RFC3339Nano, provenAt.String)
		   d.ProvenAt = &t
	  }
	   if closedAt.Valid {
		  t, _ := time.Parse(time.RFC3339Nano, closedAt.String)
		  d.ClosedAt = &t
	   }
	   return d, nil
   }

   func scanDriftProof(row scanner) (DriftProof, error) {
	  var p DriftProof
	  var submittedAt string
	   var confirmedAt sql.NullString
	   err := row.Scan(&p.ID, &p.DriftCaseID, &p.SubmittedBy, &p.Summary, &p.Evidence,
		  &submittedAt, &p.ConfirmedBy, &confirmedAt)
	   if err == sql.ErrNoRows {
		  return DriftProof{}, ErrNotFound
	   }
	  if err != nil {
		  return DriftProof{}, err
	   }
	  p.SubmittedAt, _ = time.Parse(time.RFC3339Nano, submittedAt)
	  if confirmedAt.Valid {
		   t, _ := time.Parse(time.RFC3339Nano, confirmedAt.String)
		   p.ConfirmedAt = &t
	  }
	  return p, nil
  }

   func scanInflightCredit(row scanner) (InflightCredit, error) {
	  var ic InflightCredit
	   var createdAt, updatedAt string
	  err := row.Scan(&ic.ID, &ic.UserID, &ic.TxnID, &ic.Asset, &ic.Amount, &ic.State,
		  &ic.LeaseID, &ic.RouteSnapID, &createdAt, &updatedAt)
	  if err == sql.ErrNoRows {
		   return InflightCredit{}, ErrNotFound
	  }
	  if err != nil {
		   return InflightCredit{}, err
	   }
	  ic.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	  ic.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	   return ic, nil
   }

// nullTime 将 *time.Time 转为 SQL NULL 或字符串。

   func nullTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.Format(time.RFC3339Nano)
}
// temporary guard removed after the production check
