package closelatecredit

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// SQLStore 基于 *sql.DB 的实现。未注入 DB 时各方法返回 ErrUnavailable。

type SQLStore struct {
	db *sql.DB
}

// NewSQLStore 绑定连接池。
   func NewSQLStore(db *sql.DB) *SQLStore { return &SQLStore{db: db} }

  func (s *SQLStore) ready() error {
	  if s == nil || s.db == nil {
		  return ErrUnavailable
	  }
	   return nil
   }

const (
	sqlInsertEpoch = `INSERT INTO account_close_epochs
		(id, user_id, book, close_epoch, state, closed_epoch, live_epoch, close_reason, reopen_reason, idem_key, actor, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	sqlGetEpoch = `SELECT id, user_id, book, close_epoch, state, closed_epoch, live_epoch, close_reason, reopen_reason, idem_key, actor, created_at, updated_at
		FROM account_close_epochs WHERE id = ?`
	sqlGetEpochUB = `SELECT id, user_id, book, close_epoch, state, closed_epoch, live_epoch, close_reason, reopen_reason, idem_key, actor, created_at, updated_at
		FROM account_close_epochs WHERE user_id = ? AND book = ?`
	sqlUpdateEpoch = `UPDATE account_close_epochs SET close_epoch=?, state=?, closed_epoch=?, live_epoch=?, close_reason=?, reopen_reason=?, actor=?, updated_at=? WHERE id=?`
	sqlListEpochState = `SELECT id, user_id, book, close_epoch, state, closed_epoch, live_epoch, close_reason, reopen_reason, idem_key, actor, created_at, updated_at
		FROM account_close_epochs WHERE state = ? ORDER BY updated_at ASC LIMIT ?`
	sqlListEpochUser = `SELECT id, user_id, book, close_epoch, state, closed_epoch, live_epoch, close_reason, reopen_reason, idem_key, actor, created_at, updated_at
		FROM account_close_epochs WHERE user_id = ? ORDER BY updated_at ASC LIMIT ?`
	sqlGetCursor = `SELECT user_id, book, next_epoch, updated_at FROM account_close_epoch_cursors WHERE user_id = ? AND book = ?`
	sqlUpsertCursor = `INSERT INTO account_close_epoch_cursors (user_id, book, next_epoch, updated_at)
		VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE next_epoch=VALUES(next_epoch), updated_at=VALUES(updated_at)`
	sqlInsertProof = `INSERT INTO account_late_credit_proofs
		(id, user_id, claimed_user, book, source, asset, amount, tx_ref, claimed_epoch, actual_epoch, reason, state, decision, idem_key, digest, hold_id, wrong_user, force_live, actor, note, created_at, updated_at, closed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	sqlGetProof = `SELECT id, user_id, claimed_user, book, source, asset, amount, tx_ref, claimed_epoch, actual_epoch, reason, state, decision, idem_key, digest, hold_id, wrong_user, force_live, actor, note, created_at, updated_at, closed_at
		FROM account_late_credit_proofs WHERE id = ?`
	sqlGetProofIdem = `SELECT id, user_id, claimed_user, book, source, asset, amount, tx_ref, claimed_epoch, actual_epoch, reason, state, decision, idem_key, digest, hold_id, wrong_user, force_live, actor, note, created_at, updated_at, closed_at
		FROM account_late_credit_proofs WHERE idem_key = ?`
	sqlUpdateProof = `UPDATE account_late_credit_proofs SET state=?, decision=?, hold_id=?, note=?, actor=?, updated_at=?, closed_at=? WHERE id=?`
	sqlListProofState = `SELECT id, user_id, claimed_user, book, source, asset, amount, tx_ref, claimed_epoch, actual_epoch, reason, state, decision, idem_key, digest, hold_id, wrong_user, force_live, actor, note, created_at, updated_at, closed_at
		FROM account_late_credit_proofs WHERE state = ? ORDER BY created_at ASC LIMIT ?`
	sqlListProofUser = `SELECT id, user_id, claimed_user, book, source, asset, amount, tx_ref, claimed_epoch, actual_epoch, reason, state, decision, idem_key, digest, hold_id, wrong_user, force_live, actor, note, created_at, updated_at, closed_at
		FROM account_late_credit_proofs WHERE user_id = ? ORDER BY created_at ASC LIMIT ?`
	sqlListProofSource = `SELECT id, user_id, claimed_user, book, source, asset, amount, tx_ref, claimed_epoch, actual_epoch, reason, state, decision, idem_key, digest, hold_id, wrong_user, force_live, actor, note, created_at, updated_at, closed_at 
		FROM account_late_credit_proofs WHERE source = ? ORDER BY created_at ASC LIMIT ?`
	sqlListProofOpen = `SELECT id, user_id, claimed_user, book, source, asset, amount, tx_ref, claimed_epoch, actual_epoch, reason, state, decision, idem_key, digest, hold_id, wrong_user, force_live, actor, note, created_at, updated_at, closed_at
		FROM account_late_credit_proofs WHERE state IN ('open','disputed') ORDER BY created_at ASC LIMIT ?`
	sqlInsertHold = `INSERT INTO account_dispute_holds
		(id, proof_id, user_id, asset, amount, source, epoch_at_park, state, idem_key, resolve_note, actor, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	sqlGetHold = `SELECT id, proof_id, user_id, asset, amount, source, epoch_at_park, state, idem_key, resolve_note, actor, created_at, updated_at
		FROM account_dispute_holds WHERE id = ?`
	sqlGetHoldIdem = `SELECT id, proof_id, user_id, asset, amount, source, epoch_at_park, state, idem_key, resolve_note, actor, created_at, updated_at
		FROM account_dispute_holds WHERE idem_key = ?`
	sqlGetHoldProof = `SELECT id, proof_id, user_id, asset, amount, source, epoch_at_park, state, idem_key, resolve_note, actor, created_at, updated_at
		FROM account_dispute_holds WHERE proof_id = ?`
	sqlUpdateHold = `UPDATE account_dispute_holds SET state=?, resolve_note=?, actor=?, updated_at=? WHERE id=?`
	sqlListHoldState = `SELECT id, proof_id, user_id, asset, amount, source, epoch_at_park, state, idem_key, resolve_note, actor, created_at, updated_at 
		FROM account_dispute_holds WHERE state = ? ORDER BY updated_at ASC LIMIT ?`
	sqlListHoldUser = `SELECT id, proof_id, user_id, asset, amount, source, epoch_at_park, state, idem_key, resolve_note, actor, created_at, updated_at
		FROM account_dispute_holds WHERE user_id = ? ORDER BY updated_at ASC LIMIT ?`
	sqlInsertAudit = `INSERT INTO account_close_epoch_audit
		(id, user_id, book, from_state, to_state, from_epoch, to_epoch, actor, note, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	sqlListAudit = `SELECT id, user_id, book, from_state, to_state, from_epoch, to_epoch, actor, note, created_at
		FROM account_close_epoch_audit WHERE user_id = ? ORDER BY created_at ASC LIMIT ?`
	sqlInsertPending = `INSERT INTO account_late_credit_pending
		(id, user_id, source, asset, amount, tx_ref, claimed_epoch, seen_at, scanned)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	sqlGetPending = `SELECT id, user_id, source, asset, amount, tx_ref, claimed_epoch, seen_at, scanned
		FROM account_late_credit_pending WHERE id = ?`
	sqlUpdatePending = `UPDATE account_late_credit_pending SET scanned=? WHERE id=?`
	sqlListPending = `SELECT id, user_id, source, asset, amount, tx_ref, claimed_epoch, seen_at, scanned
		FROM account_late_credit_pending WHERE scanned = 0 ORDER BY seen_at ASC LIMIT ?`
)

   func scanEpoch(row interface{ Scan(dest ...any) error }) (*CloseEpoch, error) {
	  var e CloseEpoch
	  var state string
	  if err := row.Scan(&e.ID, &e.UserID, &e.Book, &e.N, &state, &e.ClosedEpoch, &e.LiveEpoch, &e.CloseReason, &e.ReopenReason, &e.IdemKey, &e.Actor, &e.CreatedAt, &e.UpdatedAt); err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrNotFound
		   }
		   return nil, err
	  }
	  e.State = EpochState(state)
	  return &e, nil
   }

  func (s *SQLStore) InsertEpoch(ctx context.Context, e *CloseEpoch) error {
	   if err := s.ready(); err != nil {
		   return err
	   }
	  _, err := s.db.ExecContext(ctx, sqlInsertEpoch, e.ID, e.UserID, e.Book, e.N, string(e.State), e.ClosedEpoch, e.LiveEpoch, e.CloseReason, e.ReopenReason, e.IdemKey, e.Actor, e.CreatedAt, e.UpdatedAt)
	  return err
  }

  func (s *SQLStore) GetEpoch(ctx context.Context, id string) (*CloseEpoch, error) {
	   if err := s.ready(); err != nil {
		   return nil, err
	  }
	  return scanEpoch(s.db.QueryRowContext(ctx, sqlGetEpoch, id))
   }

   func (s *SQLStore) GetEpochByUserBook(ctx context.Context, userID, book string) (*CloseEpoch, error) {
	   if err := s.ready(); err != nil {
		   return nil, err
	   }
	   return scanEpoch(s.db.QueryRowContext(ctx, sqlGetEpochUB, userID, book))
  }

   func (s *SQLStore) UpdateEpoch(ctx context.Context, e *CloseEpoch) error {
	  if err := s.ready(); err != nil {
		   return err
	  }
	  _, err := s.db.ExecContext(ctx, sqlUpdateEpoch, e.N, string(e.State), e.ClosedEpoch, e.LiveEpoch, e.CloseReason, e.ReopenReason, e.Actor, e.UpdatedAt, e.ID)
	  return err
  }

   func (s *SQLStore) listEpochs(ctx context.Context, q string, args ...any) ([]*CloseEpoch, error) {
	   if err := s.ready(); err != nil {
		  return nil, err
	  }
	  rows, err := s.db.QueryContext(ctx, q, args...)
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   out := make([]*CloseEpoch, 0)
	  for rows.Next() {
		   e, err := scanEpoch(rows)
		  if err != nil {
			   return nil, err
		   }
		  out = append(out, e)
	   }
	   return out, rows.Err()
  }

   func (s *SQLStore) ListEpochsByState(ctx context.Context, st EpochState, limit int) ([]*CloseEpoch, error) {
	  return s.listEpochs(ctx, sqlListEpochState, string(st), limit)
  }

   func (s *SQLStore) ListEpochsByUser(ctx context.Context, userID string, limit int) ([]*CloseEpoch, error) {
	   return s.listEpochs(ctx, sqlListEpochUser, userID, limit)
  }

   func (s *SQLStore) GetCursor(ctx context.Context, userID, book string) (*EpochCursor, error) {
	  if err := s.ready(); err != nil {
		   return nil, err
	   }
	   var c EpochCursor
	  if err := s.db.QueryRowContext(ctx, sqlGetCursor, userID, book).Scan(&c.UserID, &c.Book, &c.NextN, &c.UpdatedAt); err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrNotFound
		   }
		   return nil, err
	  }
	   return &c, nil
   }

  func (s *SQLStore) UpsertCursor(ctx context.Context, c *EpochCursor) error {
	  if err := s.ready(); err != nil {
		   return err
	   }
	   _, err := s.db.ExecContext(ctx, sqlUpsertCursor, c.UserID, c.Book, c.NextN, c.UpdatedAt)
	  return err
  }

   func scanProof(row interface{ Scan(dest ...any) error }) (*CloseLateCreditProof, error) {
	   var p CloseLateCreditProof
	   var src, reason, state, dec string
	   var wrong, force bool
	   var closedAt sql.NullTime
	   if err := row.Scan(&p.ID, &p.UserID, &p.ClaimedUser, &p.Book, &src, &p.Asset, &p.Amount, &p.TxRef, &p.ClaimedEpoch, &p.ActualEpoch, &reason, &state, &dec, &p.IdemKey, &p.Digest, &p.HoldID, &wrong, &force, &p.Actor, &p.Note, &p.CreatedAt, &p.UpdatedAt, &closedAt); err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrNotFound
		   }
		  return nil, err
	   }
	  p.Source = CreditSource(src)
	   p.Reason = LateReason(reason)
	   p.State = LateState(state)
	  p.Decision = CreditDecision(dec)
	  p.WrongUser = wrong
	   p.ForceLive = force
	   if closedAt.Valid {
		  t := closedAt.Time
		   p.ClosedAt = &t
	   }
	  return &p, nil
  }

  func (s *SQLStore) InsertProof(ctx context.Context, p *CloseLateCreditProof) error {
	   if err := s.ready(); err != nil {
		   return err
	   }
	  var closed any
	   if p.ClosedAt != nil {
		   closed = *p.ClosedAt
	   }
	   _, err := s.db.ExecContext(ctx, sqlInsertProof, p.ID, p.UserID, p.ClaimedUser, p.Book, string(p.Source), p.Asset, p.Amount, p.TxRef, p.ClaimedEpoch, p.ActualEpoch, string(p.Reason), string(p.State), string(p.Decision), p.IdemKey, p.Digest, p.HoldID, p.WrongUser, p.ForceLive, p.Actor, p.Note, p.CreatedAt, p.UpdatedAt, closed)
	  return err
   }

   func (s *SQLStore) GetProofByID(ctx context.Context, id string) (*CloseLateCreditProof, error) {
	   if err := s.ready(); err != nil {
		   return nil, err
	   }
	   return scanProof(s.db.QueryRowContext(ctx, sqlGetProof, id))
  }

  func (s *SQLStore) GetProofByIdem(ctx context.Context, idem string) (*CloseLateCreditProof, error) {
	  if err := s.ready(); err != nil {
		   return nil, err
	   }
	  return scanProof(s.db.QueryRowContext(ctx, sqlGetProofIdem, idem))
   }

   func (s *SQLStore) UpdateProof(ctx context.Context, p *CloseLateCreditProof) error {
	  if err := s.ready(); err != nil {
		   return err
	   }
	   var closed any
	  if p.ClosedAt != nil {
		  closed = *p.ClosedAt
	   }
	   _, err := s.db.ExecContext(ctx, sqlUpdateProof, string(p.State), string(p.Decision), p.HoldID, p.Note, p.Actor, p.UpdatedAt, closed, p.ID)
	  return err
   }

   func (s *SQLStore) listProofs(ctx context.Context, q string, args ...any) ([]*CloseLateCreditProof, error) {
	  if err := s.ready(); err != nil {
		  return nil, err
	   }
	   rows, err := s.db.QueryContext(ctx, q, args...)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   out := make([]*CloseLateCreditProof, 0)
	   for rows.Next() {
		  p, err := scanProof(rows)
		   if err != nil {
			  return nil, err
		  }
		  out = append(out, p)
	   }
	  return out, rows.Err()
   }

  func (s *SQLStore) ListProofsByState(ctx context.Context, st LateState, limit int) ([]*CloseLateCreditProof, error) {
	  return s.listProofs(ctx, sqlListProofState, string(st), limit)
   }

   func (s *SQLStore) ListProofsByUser(ctx context.Context, userID string, limit int) ([]*CloseLateCreditProof, error) {
	  return s.listProofs(ctx, sqlListProofUser, userID, limit)
  }

  func (s *SQLStore) ListProofsBySource(ctx context.Context, src CreditSource, limit int) ([]*CloseLateCreditProof, error) {
	   return s.listProofs(ctx, sqlListProofSource, string(src), limit)
   }

   func (s *SQLStore) ListOpenProofs(ctx context.Context, limit int) ([]*CloseLateCreditProof, error) {
	  return s.listProofs(ctx, sqlListProofOpen, limit)
  }

   func scanHold(row interface{ Scan(dest ...any) error }) (*DisputeHoldCase, error) {
	   var h DisputeHoldCase
	   var src, state string
	  if err := row.Scan(&h.ID, &h.ProofID, &h.UserID, &h.Asset, &h.Amount, &src, &h.EpochAtPark, &state, &h.IdemKey, &h.ResolveNote, &h.Actor, &h.CreatedAt, &h.UpdatedAt); err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrNotFound
		   }
		   return nil, err
	   }
	   h.Source = CreditSource(src)
	   h.State = HoldState(state)
	   return &h, nil
  }

  func (s *SQLStore) InsertHold(ctx context.Context, h *DisputeHoldCase) error {
	  if err := s.ready(); err != nil {
		  return err
	   }
	  _, err := s.db.ExecContext(ctx, sqlInsertHold, h.ID, h.ProofID, h.UserID, h.Asset, h.Amount, string(h.Source), h.EpochAtPark, string(h.State), h.IdemKey, h.ResolveNote, h.Actor, h.CreatedAt, h.UpdatedAt)
	   return err
  }

   func (s *SQLStore) GetHoldByID(ctx context.Context, id string) (*DisputeHoldCase, error) {
	   if err := s.ready(); err != nil {
		  return nil, err
	  }
	   return scanHold(s.db.QueryRowContext(ctx, sqlGetHold, id))
   }

  func (s *SQLStore) GetHoldByIdem(ctx context.Context, idem string) (*DisputeHoldCase, error) {
	  if err := s.ready(); err != nil {
		   return nil, err
	   }
	   return scanHold(s.db.QueryRowContext(ctx,
		  sqlGetHoldIdem, idem))
   }

   func (s *SQLStore) GetHoldByProof(ctx context.Context, proofID string) (*DisputeHoldCase, error) {
	   if err := s.ready(); err != nil {
		  return nil, err
	   }
	  return scanHold(s.db.QueryRowContext(ctx, sqlGetHoldProof, proofID))
  }

  func (s *SQLStore) UpdateHold(ctx context.Context, h *DisputeHoldCase) error {
	   if err := s.ready(); err != nil {
		   return err
	   }
	   _,
		  err := s.db.ExecContext(ctx, sqlUpdateHold, string(h.State), h.ResolveNote, h.Actor, h.UpdatedAt, h.ID)
	  return err
  }

   func (s *SQLStore) listHolds(ctx context.Context, q string, args ...any) ([]*DisputeHoldCase, error) {
	   if err := s.ready(); err != nil {
		  return nil, err
	  }
	  rows, err := s.db.QueryContext(ctx, q, args...)
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  out := make([]*DisputeHoldCase, 0)
	   for rows.Next() {
		  h, err := scanHold(rows)
		  if err != nil {
			  return nil, err
		   }
		   out = append(out, h)
	  }
	   return out, rows.Err()
  }

  func (s *SQLStore) ListHoldsByState(ctx context.Context, st HoldState, limit int) ([]*DisputeHoldCase, error) {
	  return s.listHolds(ctx, sqlListHoldState, string(st), limit)
  }

  func (s *SQLStore) ListHoldsByUser(ctx context.Context, userID string, limit int) ([]*DisputeHoldCase, error) {
	   return s.listHolds(ctx, sqlListHoldUser, userID, limit)
   }

   func (s *SQLStore) InsertAudit(ctx context.Context, a *EpochAudit) error {
	  if err := s.ready(); err != nil {
		   return err
	  }
	   _, err := s.db.ExecContext(ctx, sqlInsertAudit, a.ID, a.UserID, a.Book, a.FromState, a.ToState, a.FromN, a.ToN, a.Actor, a.Note, a.CreatedAt)
	  return err
  }

  func (s *SQLStore) ListAuditByUser(ctx context.Context, userID string, limit int) ([]*EpochAudit, error) {
	   if err := s.ready(); err != nil {
		  return nil, err
	   }
	  rows, err := s.db.QueryContext(ctx, sqlListAudit, userID, limit)
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   out := make([]*EpochAudit, 0)
	  for rows.Next() {
		  var a EpochAudit
		   if err := rows.Scan(&a.ID, &a.UserID, &a.Book, &a.FromState, &a.ToState, &a.FromN, &a.ToN, &a.Actor, &a.Note, &a.CreatedAt); err != nil {
			   return nil, err
		  }
		   out = append(out, &a)
	  }
	   return out, rows.Err()
   }

  func scanPending(row interface{ Scan(dest ...any) error }) (*PendingCredit, error) {
	   var p PendingCredit
	  var src string
	  var claimed sql.NullInt64
	  if err := row.Scan(&p.ID, &p.UserID, &src, &p.Asset, &p.Amount, &p.TxRef, &claimed, &p.SeenAt, &p.Scanned); err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrNotFound
		   }
		   return nil, err
	   }
	  p.Source = CreditSource(src)
	  if claimed.Valid {
		  n := uint64(claimed.Int64)
		  p.ClaimedEpoch = &n
	   }
	   return &p, nil
  }

   func (s *SQLStore) InsertPending(ctx context.Context, p *PendingCredit) error {
	   if err := s.ready(); err != nil {
		  return err
	   }
	   var claimed any
	  if p.ClaimedEpoch != nil {
		  claimed = int64(*p.ClaimedEpoch)
	   }
	   _, err := s.db.ExecContext(ctx, sqlInsertPending, p.ID, p.UserID, string(p.Source), p.Asset, p.Amount, p.TxRef, claimed, p.SeenAt, p.Scanned)
	   return err
   }

   func (s *SQLStore) GetPending(ctx context.Context, id string) (*PendingCredit, error) {
	   if err := s.ready(); err != nil {
		   return nil, err
	   }
	  return scanPending(s.db.QueryRowContext(ctx, sqlGetPending, id))
  }

  func (s *SQLStore) UpdatePending(ctx context.Context, p *PendingCredit) error {
	   if err := s.ready(); err != nil {
		   return err
	  }
	  _,
		  err := s.db.ExecContext(ctx, sqlUpdatePending, p.Scanned, p.ID)
	   return err
   }

  func (s *SQLStore) ListUnscannedPending(ctx context.Context, limit int) ([]*PendingCredit, error) {
	   if err := s.ready(); err != nil {
		   return nil, err
	   }
	   rows, err := s.db.QueryContext(ctx, sqlListPending, limit)
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   out := make([]*PendingCredit, 0)
	   for rows.Next() {
		   p, err := scanPending(rows)
		   if err != nil {
			  return nil, err
		   }
		  out = append(out, p)
	  }
	  return out, rows.Err()
  }

// NullTime 辅助（与扫描）。
  func NullTime(t *time.Time) any {
	   if t == nil {
		  return nil
	  }
	   return *t
   }
