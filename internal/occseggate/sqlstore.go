// 不执行金额计算；仅持久化世代、证明与吊销。 DSN 由进程配置注入，本文件不硬编码。

// Author: Cason
// Date: 2021.09.06

package occseggate

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// OccSQLStore 基于 *sql.DB 的实现。未注入 DB 时各方法返回 ErrUnavailable。

type OccSQLStore struct {
	db *sql.DB
}

// NewOccSQLStore 绑定连接池。
  func NewOccSQLStore(db *sql.DB) *OccSQLStore { return &OccSQLStore{db: db} }

  func (s *OccSQLStore) ready() error {
	   if s == nil || s.db == nil {
		   return ErrUnavailable
	  }
	   return nil
   }

const (
	sqlInsertSeg = `INSERT INTO wdrelease_occ_segs
		(id, ticket_id, user_id, grant_id, grant_idem, asset, amount, gen, epoch, state, idem_key, proof_id, revoke_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	sqlGetSeg = `SELECT id, ticket_id, user_id, grant_id, grant_idem, asset, amount, gen, epoch, state, idem_key, proof_id, revoke_id, created_at, updated_at 
		FROM wdrelease_occ_segs WHERE id = ?`
	sqlGetSegIdem = `SELECT id, ticket_id, user_id, grant_id, grant_idem, asset, amount, gen, epoch, state, idem_key, proof_id, revoke_id, created_at, updated_at
		FROM wdrelease_occ_segs WHERE grant_idem = ?`
	sqlUpdateSeg = `UPDATE wdrelease_occ_segs SET state=?, proof_id=?, revoke_id=?, updated_at=? WHERE id=?`
	sqlListSegTicket = `SELECT id, ticket_id, user_id, grant_id, grant_idem, asset, amount, gen, epoch, state, idem_key, proof_id, revoke_id, created_at, updated_at 
		FROM wdrelease_occ_segs WHERE ticket_id = ? ORDER BY created_at ASC LIMIT ?`
	sqlListSegState = `SELECT id, ticket_id, user_id, grant_id, grant_idem, asset, amount, gen, epoch, state, idem_key, proof_id, revoke_id, created_at, updated_at
		FROM wdrelease_occ_segs WHERE state = ? ORDER BY created_at ASC LIMIT ?`
	sqlListSegActive = `SELECT id, ticket_id, user_id, grant_id, grant_idem, asset, amount, gen, epoch, state, idem_key, proof_id, revoke_id, created_at, updated_at
		FROM wdrelease_occ_segs WHERE state = 'reserved' ORDER BY created_at ASC LIMIT ?`
	sqlGetCursor = `SELECT ticket_id, user_id, next_gen, epoch, updated_at FROM wdrelease_occ_cursors WHERE ticket_id = ?`
	sqlUpsertCursor = `INSERT INTO wdrelease_occ_cursors (ticket_id, user_id, next_gen, epoch, updated_at)
		VALUES (?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE user_id=VALUES(user_id), next_gen=VALUES(next_gen), epoch=VALUES(epoch), updated_at=VALUES(updated_at)`
	sqlInsertProof = `INSERT INTO wdrelease_ghost_proofs 
		(id, kind, ticket_id, user_id, grant_idem, seg_id, expected_gen, actual_gen, reason, state, idem_key, digest, ledger_frozen, abort_would_ok, phase_at_issue, actor, note, acked_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	sqlGetProof = `SELECT id, kind, ticket_id, user_id, grant_idem, seg_id, expected_gen, actual_gen, reason, state, idem_key, digest, ledger_frozen, abort_would_ok, phase_at_issue, actor, note, acked_at, created_at, updated_at
		FROM wdrelease_ghost_proofs WHERE id = ?`
	sqlGetProofIdem = `SELECT id, kind, ticket_id, user_id, grant_idem, seg_id, expected_gen, actual_gen, reason, state, idem_key, digest, ledger_frozen, abort_would_ok, phase_at_issue, actor, note, acked_at, created_at, updated_at
		FROM wdrelease_ghost_proofs WHERE idem_key = ?`
	sqlUpdateProof = `UPDATE wdrelease_ghost_proofs SET state=?, note=?, acked_at=?, updated_at=? WHERE id=?`
	sqlListProofState = `SELECT id, kind, ticket_id, user_id, grant_idem, seg_id, expected_gen, actual_gen, reason, state, idem_key, digest, ledger_frozen, abort_would_ok, phase_at_issue, actor, note, acked_at, created_at, updated_at
		FROM wdrelease_ghost_proofs WHERE state = ? ORDER BY created_at ASC LIMIT ?`
	sqlListProofTicket = `SELECT id, kind, ticket_id, user_id, grant_idem, seg_id, expected_gen, actual_gen, reason, state, idem_key, digest, ledger_frozen, abort_would_ok, phase_at_issue, actor, note, acked_at, created_at, updated_at
		FROM wdrelease_ghost_proofs WHERE ticket_id = ? ORDER BY created_at ASC LIMIT ?`
	sqlListProofKind = `SELECT id, kind, ticket_id, user_id, grant_idem, seg_id, expected_gen, actual_gen, reason, state, idem_key, digest, ledger_frozen, abort_would_ok, phase_at_issue, actor, note, acked_at, created_at, updated_at
		FROM wdrelease_ghost_proofs WHERE kind = ? ORDER BY created_at ASC LIMIT ?`
	sqlInsertRev = `INSERT INTO wdrelease_expire_revocations
		(id, ticket_id, user_id, from_epoch, to_epoch, revoked_n, state, idem_key, note, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	sqlGetRev = `SELECT id, ticket_id, user_id, from_epoch, to_epoch, revoked_n, state, idem_key, note, created_at, updated_at 
		FROM wdrelease_expire_revocations WHERE id = ?`
	sqlGetRevIdem = `SELECT id, ticket_id, user_id, from_epoch, to_epoch, revoked_n, state, idem_key, note, created_at, updated_at 
		FROM wdrelease_expire_revocations WHERE idem_key = ?`
	sqlUpdateRev = `UPDATE wdrelease_expire_revocations SET state=?, revoked_n=?, note=?, updated_at=? WHERE id=?`
	sqlListRev = `SELECT id, ticket_id, user_id, from_epoch, to_epoch, revoked_n, state, idem_key, note, created_at, updated_at
		FROM wdrelease_expire_revocations ORDER BY created_at DESC LIMIT ?`
	sqlListRevTicket = `SELECT id, ticket_id, user_id, from_epoch, to_epoch, revoked_n, state, idem_key, note, created_at, updated_at
		FROM wdrelease_expire_revocations WHERE ticket_id = ? ORDER BY created_at ASC LIMIT ?`
	sqlInsertAudit = `INSERT INTO wdrelease_occ_audit
		(id, ticket_id, grant_idem, seg_id, from_seg, to_seg, gen, actor, note, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	sqlListAudit = `SELECT id, ticket_id, grant_idem, seg_id, from_seg, to_seg, gen, actor, note, created_at
		FROM wdrelease_occ_audit WHERE ticket_id = ? ORDER BY created_at ASC LIMIT ?`
	sqlInsertMux = `INSERT INTO wdrelease_occ_mutex (grant_idem, owner, kind, gen, acquired_at) VALUES (?, ?, ?, ?, ?)`
	sqlDeleteMux = `DELETE FROM wdrelease_occ_mutex WHERE grant_idem = ? AND owner = ?`
	sqlGetMux    = `SELECT grant_idem, owner, kind, gen, acquired_at FROM wdrelease_occ_mutex WHERE grant_idem = ?`
)

  func scanSeg(sc interface{ Scan(...any) error }) (*OccSegGen, error) {
	  var s OccSegGen
	  err := sc.Scan(&s.ID, &s.TicketID, &s.UserID, &s.GrantID, &s.GrantIdem, &s.Asset, &s.Amount,
		   &s.Gen,
		  &s.Epoch, &s.State, &s.IdemKey, &s.ProofID, &s.RevokeID, &s.CreatedAt, &s.UpdatedAt)
	   if err != nil {
		   return nil, err
	   }
	   return &s, nil
   }

  func scanProofSQL(sc interface{ Scan(...any) error }) (*GhostOccupancyProof, error) {
	  var p GhostOccupancyProof
	  var acked sql.NullTime
	   var frozen, abortOK int
	  err := sc.Scan(&p.ID, &p.Kind, &p.TicketID, &p.UserID, &p.GrantIdem, &p.SegID, &p.ExpectedGen, &p.ActualGen,
		  &p.Reason, &p.State, &p.IdemKey, &p.Digest, &frozen, &abortOK, &p.PhaseAtIssue, &p.Actor, &p.Note, &acked, &p.CreatedAt, &p.UpdatedAt)
	  if err != nil {
		   return nil, err
	  }
	  p.LedgerFrozen = frozen != 0
	   p.AbortWouldOK = abortOK != 0
	  if acked.Valid {
		  p.AckedAt = &acked.Time
	  }
	   return &p, nil
   }

   func scanRev(sc interface{ Scan(...any) error }) (*ExpireRevocation, error) {
	   var r ExpireRevocation
	   err := sc.Scan(&r.ID,
		  &r.TicketID, &r.UserID, &r.FromEpoch, &r.ToEpoch, &r.RevokedN, &r.State, &r.IdemKey, &r.Note, &r.CreatedAt, &r.UpdatedAt)
	   if err != nil {
		  return nil, err
	  }
	  return &r, nil
  }

  func boolInt(b bool) int {
	   if b {
		   return 1
	  }
	   return 0
   }

   func nullTime(t *time.Time) any {
	  if t == nil {
		   return nil
	   }
	  return *t
   }

   func (s *OccSQLStore) InsertSeg(ctx context.Context, seg *OccSegGen) error {
	   if err := s.ready(); err != nil {
		   return err
	   }
	   _, err := s.db.ExecContext(ctx, sqlInsertSeg, seg.ID, seg.TicketID, seg.UserID, seg.GrantID, seg.GrantIdem,
		   seg.Asset, seg.Amount, seg.Gen, seg.Epoch, string(seg.State), seg.IdemKey, seg.ProofID, seg.RevokeID, seg.CreatedAt, seg.UpdatedAt)
	   return err
   }

   func (s *OccSQLStore) GetSegByID(ctx context.Context, id string) (*OccSegGen, error) {
	  if err := s.ready(); err != nil {
		  return nil, err
	   }
	  seg, err := scanSeg(s.db.QueryRowContext(ctx, sqlGetSeg, id))
	   if errors.Is(err, sql.ErrNoRows) {
		   return nil, ErrNotFound
	   }
	  return seg, err
  }

   func (s *OccSQLStore) GetSegByGrantIdem(ctx context.Context, grantIdem string) (*OccSegGen, error) {
	  if err := s.ready(); err != nil {
		  return nil, err
	  }
	   seg, err := scanSeg(s.db.QueryRowContext(ctx, sqlGetSegIdem, grantIdem))
	  if errors.Is(err, sql.ErrNoRows) {
		  return nil, ErrNotFound
	   }
	   return seg, err
  }

  func (s *OccSQLStore) UpdateSeg(ctx context.Context, seg *OccSegGen) error {
	  if err := s.ready(); err != nil {
		   return err
	  }
	  _, err := s.db.ExecContext(ctx, sqlUpdateSeg, string(seg.State), seg.ProofID, seg.RevokeID, seg.UpdatedAt, seg.ID)
	  return err
   }

   func (s *OccSQLStore) listSegs(ctx context.Context, q string, args ...any) ([]*OccSegGen, error) {
	  if err := s.ready(); err != nil {
		  return nil, err
	  }
	  rows, err := s.db.QueryContext(ctx, q, args...)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  var out []*OccSegGen
	  for rows.Next() {
		   seg, err := scanSeg(rows)
		  if err != nil {
			  return nil, err
		  }
		  out = append(out, seg)
	   }
	   return out, rows.Err()
   }

   func (s *OccSQLStore) ListSegsByTicket(ctx context.Context, ticketID string, limit int) ([]*OccSegGen, error) {
	  if limit <= 0 {
		   limit = 100
	   }
	   return s.listSegs(ctx, sqlListSegTicket, ticketID, limit)
  }

  func (s *OccSQLStore) ListSegsByState(ctx context.Context, st SegState, limit int) ([]*OccSegGen, error) {
	  if limit <= 0 {
		   limit = 100
	   }
	   return s.listSegs(ctx, sqlListSegState, string(st), limit)
   }

  func (s *OccSQLStore) ListActiveSegs(ctx context.Context, limit int) ([]*OccSegGen, error) {
	  if limit <= 0 {
		  limit = 100
	   }
	   return s.listSegs(ctx, sqlListSegActive, limit)
  }

   func (s *OccSQLStore) GetCursor(ctx context.Context, ticketID string) (*TicketCursor, error) {
	  if err := s.ready(); err != nil {
		   return nil, err
	  }
	  var c TicketCursor
	  err := s.db.QueryRowContext(ctx, sqlGetCursor, ticketID).Scan(&c.TicketID, &c.UserID, &c.NextGen, &c.Epoch, &c.UpdatedAt)
	  if errors.Is(err, sql.ErrNoRows) {
		  return nil, ErrNotFound
	   }
	   if err != nil {
		  return nil, err
	   }
	  return &c, nil
  }

  func (s *OccSQLStore) UpsertCursor(ctx context.Context, c *TicketCursor) error {
	   if err := s.ready(); err != nil {
		  return err
	   }
	   _, err := s.db.ExecContext(ctx, sqlUpsertCursor, c.TicketID, c.UserID, c.NextGen, c.Epoch, c.UpdatedAt)
	   return err
  }

  func (s *OccSQLStore) InsertProof(ctx context.Context, p *GhostOccupancyProof) error {
	   if err := s.ready(); err != nil {
		  return err
	  }
	  _, err := s.db.ExecContext(ctx, sqlInsertProof, p.ID, string(p.Kind), p.TicketID, p.UserID, p.GrantIdem, p.SegID,
		  p.ExpectedGen, p.ActualGen, string(p.Reason), string(p.State), p.IdemKey, p.Digest, boolInt(p.LedgerFrozen),
		   boolInt(p.AbortWouldOK), p.PhaseAtIssue, p.Actor, p.Note, nullTime(p.AckedAt), p.CreatedAt, p.UpdatedAt)
	   return err
   }

  func (s *OccSQLStore) getProof(ctx context.Context, q string, arg any) (*GhostOccupancyProof, error) {
	   if err := s.ready(); err != nil {
		   return nil, err
	   }
	  p, err := scanProofSQL(s.db.QueryRowContext(ctx, q, arg))
	  if errors.Is(err, sql.ErrNoRows) {
		   return nil, ErrNotFound
	   }
	  return p, err
   }

  func (s *OccSQLStore) GetProofByID(ctx context.Context, id string) (*GhostOccupancyProof, error) {
	  return s.getProof(ctx, sqlGetProof, id)
   }

  func (s *OccSQLStore) GetProofByIdem(ctx context.Context, idem string) (*GhostOccupancyProof, error) {
	   return s.getProof(ctx, sqlGetProofIdem, idem)
   }

   func (s *OccSQLStore) UpdateProof(ctx context.Context, p *GhostOccupancyProof) error {
	   if err := s.ready(); err != nil {
		  return err
	  }
	   _, err := s.db.ExecContext(ctx, sqlUpdateProof, string(p.State), p.Note, nullTime(p.AckedAt), p.UpdatedAt, p.ID)
	  return err
   }

   func (s *OccSQLStore) listProofs(ctx context.Context, q string, args ...any) ([]*GhostOccupancyProof, error) {
	  if err := s.ready(); err != nil {
		   return nil, err
	   }
	   rows,
		   err := s.db.QueryContext(ctx, q, args...)
	  if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	   var out []*GhostOccupancyProof
	  for rows.Next() {
		  p, err := scanProofSQL(rows)
		  if err != nil {
			   return nil, err
		  }
		  out = append(out, p)
	  }
	   return out, rows.Err()
  }

  func (s *OccSQLStore) ListProofsByState(ctx context.Context, st ProofState, limit int) ([]*GhostOccupancyProof, error) {
	  if limit <= 0 {
		   limit = 100
	   }
	   return s.listProofs(ctx, sqlListProofState, string(st), limit)
   }

   func (s *OccSQLStore) ListProofsByTicket(ctx context.Context, ticketID string, limit int) ([]*GhostOccupancyProof, error) {
	   if limit <= 0 {
		  limit = 100
	  }
	  return s.listProofs(ctx, sqlListProofTicket, ticketID, limit)
   }

  func (s *OccSQLStore) ListProofsByKind(ctx context.Context, kind ProofKind, limit int) ([]*GhostOccupancyProof, error) {
	  if limit <= 0 {
		   limit = 100
	  }
	   return s.listProofs(ctx, sqlListProofKind, string(kind), limit)
   }

  func (s *OccSQLStore) InsertRevocation(ctx context.Context, r *ExpireRevocation) error {
	   if err := s.ready(); err != nil {
		   return err
	   }
	   _, err := s.db.ExecContext(ctx, sqlInsertRev, r.ID, r.TicketID, r.UserID, r.FromEpoch, r.ToEpoch, r.RevokedN, r.State, r.IdemKey, r.Note, r.CreatedAt, r.UpdatedAt)
	   return err
   }

  func (s *OccSQLStore) getRev(ctx context.Context, q string, arg any) (*ExpireRevocation, error) {
	   if err := s.ready(); err != nil {
		   return nil, err
	  }
	  r, err := scanRev(s.db.QueryRowContext(ctx, q, arg))
	   if errors.Is(err, sql.ErrNoRows) {
		   return nil, ErrNotFound
	  }
	  return r, err
  }

   func (s *OccSQLStore) GetRevocation(ctx context.Context, id string) (*ExpireRevocation, error) {
	  return s.getRev(ctx, sqlGetRev, id)
  }

  func (s *OccSQLStore) GetRevocationByIdem(ctx context.Context, idem string) (*ExpireRevocation, error) {
	   return s.getRev(ctx, sqlGetRevIdem, idem)
   }

  func (s *OccSQLStore) UpdateRevocation(ctx context.Context, r *ExpireRevocation) error {
	   if err := s.ready(); err != nil {
		  return err
	   }
	   _, err := s.db.ExecContext(ctx, sqlUpdateRev, r.State, r.RevokedN, r.Note, r.UpdatedAt, r.ID)
	  return err
  }

  func (s *OccSQLStore) listRevs(ctx context.Context, q string, args ...any) ([]*ExpireRevocation, error) {
	  if err := s.ready(); err != nil {
		   return nil, err
	   }
	  rows, err := s.db.QueryContext(ctx, q, args...)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	  var out []*ExpireRevocation
	   for rows.Next() {
		   r, err := scanRev(rows)
		  if err != nil {
			  return nil, err
		   }
		   out = append(out, r)
	  }
	  return out, rows.Err()
   }

  func (s *OccSQLStore) ListRevocations(ctx context.Context, limit int) ([]*ExpireRevocation, error) {
	   if limit <= 0 {
		   limit = 100
	  }
	  return s.listRevs(ctx, sqlListRev, limit)
   }

  func (s *OccSQLStore) ListRevocationsByTicket(ctx context.Context, ticketID string, limit int) ([]*ExpireRevocation, error) {
	  if limit <= 0 {
		   limit = 100
	  }
	   return s.listRevs(ctx, sqlListRevTicket, ticketID, limit)
   }

   func (s *OccSQLStore) InsertAudit(ctx context.Context, a *OccAudit) error {
	  if err := s.ready(); err != nil {
		   return err
	  }
	   _, err := s.db.ExecContext(ctx, sqlInsertAudit, a.ID, a.TicketID, a.GrantIdem, a.SegID, a.FromSeg, a.ToSeg, a.Gen, a.Actor, a.Note, a.CreatedAt)
	   return err
  }

   func (s *OccSQLStore) ListAuditByTicket(ctx context.Context, ticketID string, limit int) ([]*OccAudit, error) {
	  if err := s.ready(); err != nil {
		  return nil, err
	   }
	   if limit <= 0 {
		  limit = 100
	   }
	  rows, err := s.db.QueryContext(ctx, sqlListAudit, ticketID, limit)
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  var out []*OccAudit
	   for rows.Next() {
		  var a OccAudit
		   if err := rows.Scan(&a.ID, &a.TicketID, &a.GrantIdem, &a.SegID, &a.FromSeg, &a.ToSeg, &a.Gen, &a.Actor, &a.Note, &a.CreatedAt); err != nil {
			  return nil, err
		  }
		   out = append(out, &a)
	  }
	  return out, rows.Err()
   }

  func (s *OccSQLStore) TryAcquireMutex(ctx context.Context, rec *MutexRecord) error {
	  if err := s.ready(); err != nil {
		   return err
	   }
	  _, err := s.db.ExecContext(ctx, sqlInsertMux, rec.GrantIdem, rec.Owner, rec.Kind, rec.Gen, rec.AcquiredAt)
	  if err != nil {
		   return ErrMutexHeld
	  }
	  return nil
  }

   func (s *OccSQLStore) ReleaseMutex(ctx context.Context, grantIdem, owner string) error {
	  if err := s.ready(); err != nil {
		  return err
	  }
	  _, err := s.db.ExecContext(ctx, sqlDeleteMux, grantIdem, owner)
	  return err
  }

   func (s *OccSQLStore) GetMutex(ctx context.Context, grantIdem string) (*MutexRecord, error) {
	   if err := s.ready(); err != nil {
		   return nil, err
	   }
	  var m MutexRecord
	   err := s.db.QueryRowContext(ctx, sqlGetMux, grantIdem).Scan(&m.GrantIdem, &m.Owner, &m.Kind, &m.Gen, &m.AcquiredAt)
	  if errors.Is(err, sql.ErrNoRows) {
		   return nil, ErrNotFound
	   }
	  if err != nil {
		  return nil, err
	  }
	  return &m, nil
  }

var _ Store = (*OccSQLStore)(nil)
