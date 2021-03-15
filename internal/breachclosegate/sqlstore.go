package breachclosegate

import (
	"database/sql"
	"errors"
	"strings"
	"time"
	"github.com/google/uuid"
)

// SQLStore SQL 实现。

type SQLStore struct {
	db    *sql.DB
	clock func() time.Time
}

// NewSQLStore 创建 SQL Store。
  func NewSQLStore(db *sql.DB) *SQLStore {
	   return &SQLStore{db: db, clock: func() time.Time { return time.Now().UTC() }}
   }

// WithClock 注入时钟。
   func (s *SQLStore) WithClock(c func() time.Time) *SQLStore {
	   if s != nil && c != nil {
		   s.clock = c
	  }
	  return s
  }

  // ── BreachLease ──────────────────────────────────────────────────────────────

   func (s *SQLStore) CreateLease(lease BreachLease) error {
	  if lease.ID == "" {
		  lease.ID = uuid.NewString()
	  }
	  now := s.clock()
	  if lease.CreatedAt.IsZero() {
		  lease.CreatedAt = now
	  }
	   if lease.UpdatedAt.IsZero() {
		   lease.UpdatedAt = now
	   }
	  _, err := s.db.Exec(`
		INSERT INTO review_breaches
		  (id, case_id, movement_id, user_id, breach_gen, deadline, breached_at,
		   state, case_forced_expired_at, proposer, approver, cleared_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		   lease.ID, lease.CaseID, lease.MovementID, lease.UserID, lease.BreachGen,
		  lease.Deadline, lease.BreachedAt,
		  string(lease.State),
		   nullTime(lease.CaseForcedExpiredAt),
		   nullString(lease.Proposer),
		   nullString(lease.Approver),
		   nullTime(lease.ClearedAt),
		  lease.CreatedAt,
		   lease.UpdatedAt,
	   )
	   if err != nil {
		   if isDupEntry(err) {
			  return ErrDuplicateBreach
		  }
		   return err
	  }
	   return nil
  }

   func (s *SQLStore) GetLeaseByID(id string) (*BreachLease, error) {
	   return s.scanLease(`SELECT * FROM review_breaches WHERE id = ?`, id)
   }

   func (s *SQLStore) GetLeaseByCase(caseID string) (*BreachLease, error) {
	   lease, err := s.scanLease(
		  `SELECT * FROM review_breaches WHERE case_id = ? AND state = 'blocking' ORDER BY created_at DESC LIMIT 1`,
		   caseID)
	  if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrNoActiveBreach
		  }
		  return nil, err
	   }
	  return lease, nil
   }

   func (s *SQLStore) GetLeaseByMovement(movementID string) (*BreachLease, error) {
	  lease, err := s.scanLease(
		  `SELECT * FROM review_breaches WHERE movement_id = ? AND state = 'blocking' ORDER BY created_at DESC LIMIT 1`,
		   movementID)
	  if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrNoActiveBreach
		   }
		   return nil, err
	   }
	   return lease, nil
   }

  func (s *SQLStore) GetLeaseByBreachGen(breachGen string) (*BreachLease, error) {
	   lease, err := s.scanLease(
		  `SELECT * FROM review_breaches WHERE breach_gen = ?`, breachGen)
	  if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrLeaseNotFound
		  }
		  return nil, err
	  }
	   return lease, nil
   }

   func (s *SQLStore) UpdateLeaseState(id string, newState BreachState, update func(*BreachLease)) error {
	  cur, err := s.GetLeaseByID(id)
	   if err != nil {
		  return err
	   }
	   if update != nil {
		  update(cur)
	   }
	   cur.State = newState
	   cur.UpdatedAt = s.clock()
	  _, execErr := s.db.Exec(`
		UPDATE review_breaches
		SET state = ?, case_forced_expired_at = ?, proposer = ?, approver = ?,
		    cleared_at = ?, updated_at = ?
		WHERE id = ?`,
		string(cur.State),
		nullTime(cur.CaseForcedExpiredAt),
		nullString(cur.Proposer),
		nullString(cur.Approver),
		nullTime(cur.ClearedAt),
		cur.UpdatedAt,
		id,
	)
	return execErr
}

  func (s *SQLStore) ListActiveLeases(limit int) []BreachLease {
	   if limit <= 0 {
		   limit = 100
	   }
	   rows, err := s.db.Query(
		  `SELECT * FROM review_breaches WHERE state = 'blocking' ORDER BY created_at DESC LIMIT ?`, limit)
	  if err != nil {
		   return nil
	  }
	  defer rows.Close()
	  return s.scanLeaseRows(rows)
  }

   func (s *SQLStore) ListLeasesByCase(caseID string, limit int) []BreachLease {
	   if limit <= 0 {
		   limit = 50
	   }
	   rows, err := s.db.Query(
		  `SELECT * FROM review_breaches WHERE case_id = ? ORDER BY created_at DESC LIMIT ?`,
		  caseID, limit)
	   if err != nil {
		  return nil
	  }
	   defer rows.Close()
	  return s.scanLeaseRows(rows)
   }

   func (s *SQLStore) scanLease(query string, args ...interface{}) (*BreachLease, error) {
	  row := s.db.QueryRow(query, args...)
	  return scanOneLease(row)
  }

  func scanOneLease(row *sql.Row) (*BreachLease, error) {
	  var l BreachLease
	   var state string
	  var forcedExpired, clearedAt sql.NullTime
	   var proposer, approver sql.NullString
	  err := row.Scan(
		   &l.ID,
		   &l.CaseID, &l.MovementID, &l.UserID, &l.BreachGen,
		  &l.Deadline, &l.BreachedAt, &state,
		   &forcedExpired, &proposer, &approver, &clearedAt,
		   &l.CreatedAt, &l.UpdatedAt,
	  )
	  if err != nil {
		   return nil, err
	  }
	   l.State = BreachState(state)
	   if forcedExpired.Valid {
		   l.CaseForcedExpiredAt = &forcedExpired.Time
	   }
	  if clearedAt.Valid {
		   l.ClearedAt = &clearedAt.Time
	  }
	  l.Proposer = proposer.String
	  l.Approver = approver.String
	   return &l, nil
  }

   func (s *SQLStore) scanLeaseRows(rows *sql.Rows) []BreachLease {
	  out := make([]BreachLease, 0)
	   for rows.Next() {
		  var l BreachLease
		  var state string
		   var forcedExpired, clearedAt sql.NullTime
		  var proposer, approver sql.NullString
		   if err := rows.Scan(
			  &l.ID, &l.CaseID, &l.MovementID, &l.UserID, &l.BreachGen,
			  &l.Deadline, &l.BreachedAt, &state,
			   &forcedExpired, &proposer, &approver, &clearedAt,
			  &l.CreatedAt, &l.UpdatedAt,
		  ); err != nil {
			   continue
		  }
		   l.State = BreachState(state)
		   if forcedExpired.Valid {
			   l.CaseForcedExpiredAt = &forcedExpired.Time
		  }
		  if clearedAt.Valid {
			  l.ClearedAt = &clearedAt.Time
		  }
		   l.Proposer = proposer.String
		  l.Approver = approver.String
		  out = append(out, l)
	   }
	   return out
   }

// ── SlaBreachProof ───────────────────────────────────────────────────────────

   func (s *SQLStore) CreateProof(proof SlaBreachProof) error {
	  if proof.ID == "" {
		  proof.ID = uuid.NewString()
	  }
	   if proof.RecordedAt.IsZero() {
		  proof.RecordedAt = s.clock()
	   }
	   _, err := s.db.Exec(`
		INSERT IGNORE INTO review_breach_proofs
		  (id, case_id, breach_lease_id, movement_id, deadline, breached_at,
		   block_outcome, actor, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		  proof.ID, proof.CaseID, proof.BreachLeaseID, proof.MovementID,
		  proof.Deadline, proof.BreachedAt, proof.BlockOutcome,
		  nullString(proof.Actor), proof.RecordedAt,
	  )
	   if err != nil {
		  if isDupEntry(err) {
			  return nil // 幂等
		  }
		   return err
	  }
	   return nil
  }

   func (s *SQLStore) ListProofsByLease(leaseID string, limit int) []SlaBreachProof {
	  if limit <= 0 {
		   limit = 50
	   }
	   rows, err := s.db.Query(
		   `SELECT id, case_id, breach_lease_id, movement_id, deadline, breached_at,
		        block_outcome, actor, recorded_at
		 FROM review_breach_proofs WHERE breach_lease_id = ? ORDER BY recorded_at DESC LIMIT ?`,
		   leaseID, limit)
	  if err != nil {
		  return nil
	  }
	   defer rows.Close()
	   return s.scanProofRows(rows)
   }

   func (s *SQLStore) ListAllProofs(limit int) []SlaBreachProof {
	  if limit <= 0 {
		  limit = 200
	  }
	   rows, err := s.db.Query(
		   `SELECT id, case_id, breach_lease_id, movement_id, deadline, breached_at,
		        block_outcome, actor, recorded_at
		 FROM review_breach_proofs ORDER BY recorded_at DESC LIMIT ?`, limit)
	  if err != nil {
		   return nil
	  }
	   defer rows.Close()
	   return s.scanProofRows(rows)
   }

  func (s *SQLStore) scanProofRows(rows *sql.Rows) []SlaBreachProof {
	   out := make([]SlaBreachProof, 0)
	   for rows.Next() {
		   var p SlaBreachProof
		   var actor sql.NullString
		   if err := rows.Scan(
			  &p.ID, &p.CaseID, &p.BreachLeaseID, &p.MovementID,
			   &p.Deadline, &p.BreachedAt, &p.BlockOutcome, &actor, &p.RecordedAt,
		   ); err != nil {
			   continue
		   }
		  p.Actor = actor.String
		  out = append(out, p)
	  }
	   return out
   }

// ── BreachClearance ──────────────────────────────────────────────────────────

  func (s *SQLStore) CreateClearance(c BreachClearance) error {
	  if c.ID == "" {
		  c.ID = uuid.NewString()
	   }
	   if c.ProposedAt.IsZero() {
		  c.ProposedAt = s.clock()
	  }
	   _, err := s.db.Exec(`
		INSERT INTO review_breach_clearances
		  (id, breach_lease_id, case_id, movement_id, proposer, approver, reason,
		   state, no_auto_allow, proposed_at, approved_at, rejected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		   c.ID,
		   c.BreachLeaseID, c.CaseID, c.MovementID,
		  c.Proposer, nullString(c.Approver), c.Reason,
		  string(c.State), boolToInt(c.NoAutoAllow),
		  c.ProposedAt, nullTime(c.ApprovedAt), nullTime(c.RejectedAt),
	  )
	  if err != nil {
		  if isDupEntry(err) {
			   return ErrDuplicateBreach
		   }
		   return err
	  }
	  return nil
  }

  func (s *SQLStore) GetClearanceByID(id string) (*BreachClearance, error) {
	  row := s.db.QueryRow(`
		SELECT id, breach_lease_id, case_id, movement_id, proposer, approver, reason,
		       state, no_auto_allow, proposed_at, approved_at, rejected_at
		FROM review_breach_clearances WHERE id = ?`, id)
	   return scanOneClearance(row)
  }

   func (s *SQLStore) GetPendingClearanceByLease(leaseID string) (*BreachClearance, error) {
	   row := s.db.QueryRow(` 
		SELECT id, breach_lease_id, case_id, movement_id, proposer, approver, reason,
		       state, no_auto_allow, proposed_at, approved_at, rejected_at
		FROM review_breach_clearances WHERE breach_lease_id = ? AND state = 'pending'
		ORDER BY proposed_at DESC LIMIT 1`, leaseID)
	  c, err := scanOneClearance(row)
	  if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrClearanceNotFound
		   }
		  return nil, err
	  }
	  return c, nil
  }

   func (s *SQLStore) UpdateClearanceState(id string, update func(*BreachClearance)) error {
	   cur, err := s.GetClearanceByID(id)
	   if err != nil {
		  return err
	   }
	   if update != nil {
		   update(cur)
	   }
	  _, execErr := s.db.Exec(`
		UPDATE review_breach_clearances
		SET approver = ?, state = ?, approved_at = ?, rejected_at = ?
		WHERE id = ?`,
		nullString(cur.Approver),
		string(cur.State),
		nullTime(cur.ApprovedAt),
		nullTime(cur.RejectedAt),
		id,
	)
	return execErr
}

  func (s *SQLStore) ListClearances(limit int) []BreachClearance {
	   if limit <= 0 {
		   limit = 100
	  }
	   rows, err := s.db.Query(`
		SELECT id, breach_lease_id, case_id, movement_id, proposer, approver, reason,
		       state, no_auto_allow, proposed_at, approved_at, rejected_at
		FROM review_breach_clearances ORDER BY proposed_at DESC LIMIT ?`, limit)
	  if err != nil {
		   return nil
	   }
	   defer rows.Close()
	  out := make([]BreachClearance, 0)
	   for rows.Next() {
		  c, err := scanOneClearanceRow(rows)
		  if err != nil {
			  continue
		   }
		  out = append(out, *c)
	  }
	  return out
   }

  func scanOneClearance(row *sql.Row) (*BreachClearance, error) {
	   var c BreachClearance
	  var state string
	  var approver sql.NullString
	  var approvedAt,
		   rejectedAt sql.NullTime
	  var noAutoAllow int
	  err := row.Scan(
		   &c.ID,
		   &c.BreachLeaseID, &c.CaseID, &c.MovementID,
		   &c.Proposer, &approver, &c.Reason,
		   &state, &noAutoAllow, &c.ProposedAt, &approvedAt, &rejectedAt,
	   )
	   if err != nil {
		   return nil, err
	   }
	  c.State = ClearanceState(state)
	   c.Approver = approver.String
	   c.NoAutoAllow = noAutoAllow != 0
	  if approvedAt.Valid {
		   c.ApprovedAt = &approvedAt.Time
	  }
	   if rejectedAt.Valid {
		  c.RejectedAt = &rejectedAt.Time
	  }
	  return &c, nil
   }

   func scanOneClearanceRow(rows *sql.Rows) (*BreachClearance, error) {
	   var c BreachClearance
	  var state string
	  var approver sql.NullString
	  var approvedAt, rejectedAt sql.NullTime
	   var noAutoAllow int
	  err := rows.Scan(
		   &c.ID, &c.BreachLeaseID, &c.CaseID, &c.MovementID,
		  &c.Proposer, &approver, &c.Reason,
		   &state,
		  &noAutoAllow, &c.ProposedAt, &approvedAt, &rejectedAt,
	  )
	   if err != nil {
		  return nil, err
	  }
	  c.State = ClearanceState(state)
	   c.Approver = approver.String
	   c.NoAutoAllow = noAutoAllow != 0
	   if approvedAt.Valid {
		   c.ApprovedAt = &approvedAt.Time
	   }
	   if rejectedAt.Valid {
		   c.RejectedAt = &rejectedAt.Time
	   }
	   return &c, nil
   }

// ── SQL 工具函数 ──────────────────────────────────────────────────────────────

  func nullString(s string) sql.NullString {
	   return sql.NullString{String: s, Valid: s != ""}
  }

  func nullTime(t *time.Time) sql.NullTime {
	   if t == nil {
		   return sql.NullTime{Valid: false}
	  }
	  return sql.NullTime{Time: *t, Valid: true}
   }

  func boolToInt(b bool) int {
	   if b {
		  return 1
	   }
	  return 0
  }

   func isDupEntry(err error) bool {
	  if err == nil {
		   return false
	   }
	   return strings.Contains(err.Error(), "Duplicate entry") ||
		   strings.Contains(err.Error(), "UNIQUE constraint failed")
  }
