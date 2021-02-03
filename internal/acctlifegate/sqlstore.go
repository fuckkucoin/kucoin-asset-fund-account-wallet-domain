package acctlifegate

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SQLStore Store 的数据库实现。
type SQLStore struct {
	db *sql.DB
}

// NewSQLStore 构造 SQLStore。
  func NewSQLStore(db *sql.DB) *SQLStore { return &SQLStore{db: db} }

// ── 辅助 ─────────────────────────────────────────────────────────────────────

  func (s *SQLStore) exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	   return s.db.ExecContext(ctx, query, args...)
   }

   func (s *SQLStore) queryRow(ctx context.Context, query string, args ...any) *sql.Row {
	   return s.db.QueryRowContext(ctx, query, args...)
  }

   func (s *SQLStore) query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	  return s.db.QueryContext(ctx, query, args...)
   }

   func casErr(res sql.Result, err error) error {
	   if err != nil {
		  return err
	  }
	  n, _ := res.RowsAffected()
	  if n == 0 {
		   return ErrGenerationMismatch
	  }
	   return nil
   }

// ── BookLifeGeneration ───────────────────────────────────────────────────────

   func (s *SQLStore) CurrentGeneration(ctx context.Context, userID, book string) (BookLifeGeneration, error) {
	  var g BookLifeGeneration
	   row := s.queryRow(ctx,
		   `SELECT gen_id, user_id, book, generation, action, actor, created_at
		   FROM account_life_generations
		  WHERE user_id=? AND book=?
		  ORDER BY generation DESC LIMIT 1`,
		   userID, book,
	   )
	   err := row.Scan(&g.GenID, &g.UserID, &g.Book, &g.Generation, &g.Action, &g.Actor, &g.CreatedAt)
	  if err == sql.ErrNoRows {
		   return BookLifeGeneration{UserID: userID,
			  Book: book, Generation: 0}, nil
	  }
	  if err != nil {
		   return g, fmt.Errorf("acctlifegate sql: current_generation: %w", err)
	   }
	  return g, nil
  }

   func (s *SQLStore) BumpGeneration(ctx context.Context, userID, book, action, actor string) (BookLifeGeneration, error) {
	   cur,
		   err := s.CurrentGeneration(ctx, userID, book)
	  if err != nil {
		  return BookLifeGeneration{}, err
	  }
	   next := cur.Generation + 1
	  genID := fmt.Sprintf("gen-%s|%s-%d", userID, book, next)
	   now := time.Now().UTC()
	   _, err = s.exec(ctx,
		   `INSERT INTO account_life_generations (gen_id, user_id, book, generation, action, actor, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		  genID, userID, book, next, action, actor, now,
	   )
	  if err != nil {
		   return BookLifeGeneration{}, fmt.Errorf("acctlifegate sql: bump_generation: %w", err)
	  }
	  return BookLifeGeneration{
		  GenID: genID, UserID: userID, Book: book,
		  Generation: next, Action: action, Actor: actor, CreatedAt: now,
	   }, nil
   }

// ── LifeLease ────────────────────────────────────────────────────────────────

   func scanLease(row *sql.Row) (*LifeLease, error) {
	  var l LifeLease
	  var usedAt, expiredAt, revokedAt sql.NullTime
	  err := row.Scan(
		  &l.LeaseID, &l.UserID, &l.Book, &l.Generation, &l.State,
		  &l.IssuedAt,
		   &l.ExpiresAt, &usedAt, &expiredAt, &revokedAt, &l.RevokeNote,
	  )
	  if err == sql.ErrNoRows {
		   return nil, ErrLeaseNotFound
	  }
	   if err != nil {
		  return nil, fmt.Errorf("acctlifegate sql: scan_lease: %w", err)
	   }
	   if usedAt.Valid {
		  l.UsedAt = &usedAt.Time
	  }
	  if expiredAt.Valid {
		   l.ExpiredAt = &expiredAt.Time
	  }
	   if revokedAt.Valid {
		   l.RevokedAt = &revokedAt.Time
	   }
	  return &l, nil
  }

   func (s *SQLStore) GetLease(ctx context.Context, leaseID string) (*LifeLease, error) {
	  return scanLease(s.queryRow(ctx,
		   `SELECT lease_id, user_id, book, generation, state,
		        issued_at, expires_at, used_at, expired_at, revoked_at, revoke_note
		   FROM account_life_leases WHERE lease_id=?`, leaseID,
	  ))
   }

  func (s *SQLStore) InsertLease(ctx context.Context, l LifeLease) error {
	  _, err := s.exec(ctx,
		   `INSERT INTO account_life_leases
		 (lease_id, user_id, book, generation, state, issued_at, expires_at, revoke_note)
		 VALUES (?, ?, ?, ?, ?, ?, ?, '')`,
		  l.LeaseID, l.UserID, l.Book, l.Generation, l.State, l.IssuedAt, l.ExpiresAt,
	  )
	  if err != nil {
		  return fmt.Errorf("acctlifegate sql: insert_lease: %w", err)
	  }
	  return nil
   }

  func (s *SQLStore) UpdateLeaseState(ctx context.Context, leaseID string, expected, next LeaseState, now time.Time, note string) error {
	   res, err := s.exec(ctx,
		  `UPDATE account_life_leases  
		    SET state=?,
		        used_at=CASE WHEN ?='used' THEN ? ELSE used_at END,
		        expired_at=CASE WHEN ?='expired' THEN ? ELSE expired_at END,
		        revoked_at=CASE WHEN ?='revoked' THEN ? ELSE revoked_at END,
		        revoke_note=CASE WHEN ?='revoked' THEN ? ELSE revoke_note END
		  WHERE lease_id=? AND state=?`,
		   next,
		  next, now,
		   next, now,
		  next, now,
		   next, note,
		  leaseID, expected,
	   )
	  return casErr(res, err)
  }

  func (s *SQLStore) RevokeOpenLeases(ctx context.Context, userID, book string, now time.Time, note string) (int, error) {
	  res, err := s.exec(ctx,
		  `UPDATE account_life_leases
		    SET state='revoked', revoked_at=?, revoke_note=?
		  WHERE user_id=? AND book=? AND state='open'`,
		   now, note, userID, book,
	  )
	   if err != nil {
		   return 0, fmt.Errorf("acctlifegate sql: revoke_open_leases: %w", err)
	  }
	  n, _ := res.RowsAffected()
	  return int(n), nil
  }

  func (s *SQLStore) ListOpenLeases(ctx context.Context, userID, book string) ([]LifeLease, error) {
	  rows, err := s.query(ctx,
		   `SELECT lease_id, user_id, book, generation, state,
		        issued_at, expires_at, used_at, expired_at, revoked_at, revoke_note
		   FROM account_life_leases
		  WHERE user_id=? AND book=? AND state='open'`,
		   userID, book,
	   )
	  if err != nil {
		   return nil, fmt.Errorf("acctlifegate sql: list_open_leases: %w", err)
	   }
	   defer rows.Close()
	   return scanLeases(rows)
  }

  func (s *SQLStore) ListExpiredLeases(ctx context.Context, now time.Time, limit int) ([]LifeLease, error) {
	   rows, err := s.query(ctx,
		   `SELECT lease_id, user_id, book, generation, state,
		        issued_at, expires_at, used_at, expired_at, revoked_at, revoke_note
		   FROM account_life_leases
		  WHERE state='open' AND expires_at < ?
		  LIMIT ?`,
		   now, limit,
	   )
	  if err != nil {
		   return nil, fmt.Errorf("acctlifegate sql: list_expired_leases: %w", err)
	   }
	   defer rows.Close()
	  return scanLeases(rows)
  }

  func scanLeases(rows *sql.Rows) ([]LifeLease, error) {
	  var out []LifeLease
	  for rows.Next() {
		   var l LifeLease
		  var usedAt, expiredAt, revokedAt sql.NullTime
		   if err := rows.Scan(
			   &l.LeaseID, &l.UserID, &l.Book, &l.Generation, &l.State,
			   &l.IssuedAt, &l.ExpiresAt, &usedAt, &expiredAt, &revokedAt, &l.RevokeNote,
		   ); err != nil {
			  return nil, fmt.Errorf("acctlifegate sql: scan_leases: %w", err)
		   }
		  if usedAt.Valid {
			   l.UsedAt = &usedAt.Time
		  }
		   if expiredAt.Valid {
			  l.ExpiredAt = &expiredAt.Time
		   }
		   if revokedAt.Valid {
			  l.RevokedAt = &revokedAt.Time
		  }
		  out = append(out, l)
	  }
	  return out, rows.Err()
   }

// ── MoveBind ─────────────────────────────────────────────────────────────────

   func scanBind(row *sql.Row) (*MoveBind, error) {
	   var b MoveBind
	   var resolvedAt sql.NullTime
	   err := row.Scan(
		   &b.BindID,
		   &b.UserID, &b.FromBook, &b.ToBook,
		  &b.Asset, &b.Amount, &b.LeaseID, &b.Generation,
		   &b.IdemKey, &b.State, &b.BoundAt, &b.ExpiresAt,
		   &resolvedAt, &b.Note,
	   )
	   if err == sql.ErrNoRows {
		  return nil, ErrMoveBindNotFound
	  }
	  if err != nil {
		   return nil, fmt.Errorf("acctlifegate sql: scan_bind: %w", err)
	   }
	  if resolvedAt.Valid {
		   b.ResolvedAt = &resolvedAt.Time
	   }
	   return &b, nil
  }

   func (s *SQLStore) GetMoveBind(ctx context.Context, bindID string) (*MoveBind, error) {
	  return scanBind(s.queryRow(ctx,
		   `SELECT bind_id, user_id, from_book, to_book, asset, amount,
		        lease_id, generation, idem_key, state, bound_at, expires_at, resolved_at, note
		   FROM account_move_binds WHERE bind_id=?`, bindID,
	  ))
  }

   func (s *SQLStore) GetMoveBindByIdem(ctx context.Context, idemKey string) (*MoveBind, error) {
	   return scanBind(s.queryRow(ctx,
		  `SELECT bind_id, user_id, from_book, to_book, asset, amount,
		        lease_id, generation, idem_key, state, bound_at, expires_at, resolved_at, note
		   FROM account_move_binds WHERE idem_key=?`, idemKey,
	   ))
   }

   func (s *SQLStore) InsertMoveBind(ctx context.Context, b MoveBind) error {
	  _, err := s.exec(ctx,
		  `INSERT INTO account_move_binds
		 (bind_id, user_id, from_book, to_book, asset, amount,
		  lease_id, generation, idem_key, state, bound_at, expires_at, note)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		   b.BindID, b.UserID, b.FromBook, b.ToBook, b.Asset, b.Amount,
		   b.LeaseID, b.Generation, b.IdemKey, b.State,
		   b.BoundAt, b.ExpiresAt, b.Note,
	   )
	   if err != nil {
		   return fmt.Errorf("acctlifegate sql: insert_move_bind: %w", err)
	  }
	   return nil
   }

   func (s *SQLStore) UpdateMoveBindState(ctx context.Context, bindID string, expected, next MoveBindState, now time.Time, note string) error {
	   res, err := s.exec(ctx,
		   `UPDATE account_move_binds
		    SET state=?, resolved_at=?, note=?
		  WHERE bind_id=? AND state=?`,
		   next, now, note, bindID, expected,
	   )
	   return casErr(res, err)
   }

   func (s *SQLStore) ListPendingMoveBinds(ctx context.Context, userID, book string) ([]MoveBind, error) {
	  rows, err := s.query(ctx,
		  `SELECT bind_id, user_id, from_book, to_book, asset, amount,
		        lease_id, generation, idem_key, state, bound_at, expires_at, resolved_at, note
		   FROM account_move_binds
		  WHERE user_id=? AND (from_book=? OR to_book=?) AND state='pending'`,
		  userID, book, book,
	  )
	  if err != nil {
		   return nil, fmt.Errorf("acctlifegate sql: list_pending_move_binds: %w", err)
	  }
	  defer rows.Close()
	   return scanBindRows(rows)
  }

  func (s *SQLStore) ListExpiredMoveBinds(ctx context.Context, now time.Time, limit int) ([]MoveBind, error) {
	  rows, err := s.query(ctx,
		  `SELECT bind_id, user_id, from_book, to_book, asset, amount,
		        lease_id, generation, idem_key, state, bound_at, expires_at, resolved_at, note
		   FROM account_move_binds
		  WHERE state='pending' AND expires_at < ?
		  LIMIT ?`,
		  now, limit,
	   )
	   if err != nil {
		  return nil, fmt.Errorf("acctlifegate sql: list_expired_move_binds: %w", err)
	   }
	  defer rows.Close()
	  return scanBindRows(rows)
  }

  func scanBindRows(rows *sql.Rows) ([]MoveBind, error) {
	   var out []MoveBind
	   for rows.Next() {
		  var b MoveBind
		  var resolvedAt sql.NullTime
		  if err := rows.Scan(
			  &b.BindID, &b.UserID, &b.FromBook, &b.ToBook,
			   &b.Asset, &b.Amount, &b.LeaseID, &b.Generation,
			  &b.IdemKey, &b.State, &b.BoundAt, &b.ExpiresAt,
			   &resolvedAt, &b.Note,
		   ); err != nil {
			   return nil, fmt.Errorf("acctlifegate sql: scan_bind_rows: %w", err)
		   }
		   if resolvedAt.Valid {
			  b.ResolvedAt = &resolvedAt.Time
		   }
		   out = append(out, b)
	  }
	   return out, rows.Err()
  }

// ── LifeDriftCase ────────────────────────────────────────────────────────────

   func (s *SQLStore) GetDriftCase(ctx context.Context, caseID string) (*LifeDriftCase, error) {
	   var c LifeDriftCase
	  var blockedAt, provenAt, closedAt sql.NullTime
	  err := s.queryRow(ctx,
		   `SELECT case_id, user_id, book, detected_at, check_hop_at, move_attempt_at,
		        status_at_check, status_at_move, lease_id, move_idem_key,
		        life_gen_at_check, life_gen_at_move, state,
		        blocked_at, proven_at, closed_at, actor1, actor2, proof_note, close_note
		   FROM account_life_drift_cases WHERE case_id=?`, caseID,
	   ).Scan(
		  &c.CaseID, &c.UserID, &c.Book, &c.DetectedAt, &c.CheckHopAt, &c.MoveAttemptAt,
		   &c.StatusAtCheck, &c.StatusAtMove, &c.LeaseID, &c.MoveIdemKey,
		  &c.LifeGenAtCheck, &c.LifeGenAtMove, &c.State,
		   &blockedAt,
		   &provenAt, &closedAt, &c.Actor1, &c.Actor2, &c.ProofNote, &c.CloseNote,
	  )
	   if err == sql.ErrNoRows {
		   return nil, ErrDriftCaseNotFound
	   }
	  if err != nil {
		  return nil, fmt.Errorf("acctlifegate sql: get_drift_case: %w", err)
	  }
	  if blockedAt.Valid {
		   c.BlockedAt = &blockedAt.Time
	  }
	  if provenAt.Valid {
		   c.ProvenAt = &provenAt.Time
	  }
	  if closedAt.Valid {
		   c.ClosedAt = &closedAt.Time
	  }
	   return &c, nil
  }

   func (s *SQLStore) ListOpenDriftCases(ctx context.Context, userID, book string) ([]LifeDriftCase, error) {
	  rows, err := s.query(ctx,
		   `SELECT case_id, user_id, book, detected_at, check_hop_at, move_attempt_at,
		        status_at_check, status_at_move, lease_id, move_idem_key,
		        life_gen_at_check, life_gen_at_move, state,
		        blocked_at, proven_at, closed_at, actor1, actor2, proof_note, close_note
		   FROM account_life_drift_cases
		  WHERE user_id=? AND book=? AND state!='closed'`,
		  userID, book,
	   )
	   if err != nil {
		  return nil, fmt.Errorf("acctlifegate sql: list_open_drift_cases: %w", err)
	  }
	  defer rows.Close()
	  return scanDriftRows(rows)
   }

  func (s *SQLStore) ListAllOpenDriftCases(ctx context.Context, limit int) ([]LifeDriftCase, error) {
	   rows, err := s.query(ctx,
		   `SELECT case_id, user_id, book, detected_at, check_hop_at, move_attempt_at,
		        status_at_check, status_at_move, lease_id, move_idem_key,
		        life_gen_at_check, life_gen_at_move, state,
		        blocked_at, proven_at, closed_at, actor1, actor2, proof_note, close_note
		   FROM account_life_drift_cases
		  WHERE state!='closed'
		  LIMIT ?`, limit,
	   )
	  if err != nil {
		   return nil, fmt.Errorf("acctlifegate sql: list_all_open_drift_cases: %w", err)
	  }
	  defer rows.Close()
	  return scanDriftRows(rows)
   }

   func (s *SQLStore) InsertDriftCase(ctx context.Context, c LifeDriftCase) error {
	   _, err := s.exec(ctx,
		  `INSERT INTO account_life_drift_cases
		 (case_id, user_id, book, detected_at, check_hop_at, move_attempt_at,
		  status_at_check, status_at_move, lease_id, move_idem_key,
		  life_gen_at_check, life_gen_at_move, state, blocked_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		   c.CaseID, c.UserID, c.Book, c.DetectedAt, c.CheckHopAt, c.MoveAttemptAt,
		   c.StatusAtCheck,
		  c.StatusAtMove, c.LeaseID, c.MoveIdemKey,
		  c.LifeGenAtCheck, c.LifeGenAtMove, c.State, c.BlockedAt,
	  )
	  if err != nil {
		  return fmt.Errorf("acctlifegate sql: insert_drift_case: %w", err)
	  }
	  return nil
   }

  func (s *SQLStore) UpdateDriftState(ctx context.Context, caseID string, expected, next DriftState, now time.Time, actor1, actor2, note string) error {
	   var res sql.Result
	   var err error
	  switch next {
	   case DriftStateProven:
		   res, err = s.exec(ctx,
			  `UPDATE account_life_drift_cases
			    SET state=?, proven_at=?, actor1=?, actor2=?, proof_note=?
			  WHERE case_id=? AND state=?`,
			  next, now, actor1, actor2, note, caseID, expected,
		  )
	   case DriftStateClosed:
		   res, err = s.exec(ctx,
			  `UPDATE account_life_drift_cases
			    SET state=?, closed_at=?, close_note=?
			  WHERE case_id=? AND state=?`,
			   next, now, note, caseID, expected,
		  )
	  default:
		  res, err = s.exec(ctx,
			   `UPDATE account_life_drift_cases SET state=? WHERE case_id=? AND state=?`,
			  next, caseID, expected,
		   )
	  }
	   return casErr(res, err)
  }

  func scanDriftRows(rows *sql.Rows) ([]LifeDriftCase, error) {
	  var out []LifeDriftCase
	  for rows.Next() {
		  var c LifeDriftCase
		  var blockedAt, provenAt, closedAt sql.NullTime
		   if err := rows.Scan(
			   &c.CaseID, &c.UserID, &c.Book, &c.DetectedAt, &c.CheckHopAt, &c.MoveAttemptAt,
			  &c.StatusAtCheck, &c.StatusAtMove, &c.LeaseID, &c.MoveIdemKey,
			  &c.LifeGenAtCheck, &c.LifeGenAtMove, &c.State,
			  &blockedAt, &provenAt, &closedAt, &c.Actor1, &c.Actor2, &c.ProofNote, &c.CloseNote,
		  ); err != nil {
			   return nil, fmt.Errorf("acctlifegate sql: scan_drift_rows: %w", err)
		   }
		   if blockedAt.Valid {
			   c.BlockedAt = &blockedAt.Time
		   }
		   if provenAt.Valid {
			   c.ProvenAt = &provenAt.Time
		   }
		   if closedAt.Valid {
			   c.ClosedAt = &closedAt.Time
		   }
		   out = append(out, c)
	  }
	   return out, rows.Err()
   }
