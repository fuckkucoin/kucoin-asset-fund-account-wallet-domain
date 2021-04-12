// @FileName: sqlstore.go
// @Summary: SQL 持久化实现 — CalendarStore、AuditStore、InflightStore、WindowStore、BypassStore。
// 仅对 dest_entry_lives / cooldown_windows / inflight_refs / bypass_proofs / lifecycle_audit 五张表操作。
// Dialect 支持 MySQL（ON DUPLICATE KEY）和 Postgres（ON CONFLICT）。
// 不变量：所有写操作必须在同一 DB 连接上；不做跨表事务（事务由上层管理）。
// @Author: Knox
// @Created: 2021.04.11

package destlife

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// SQLCalendarStore SQL 实现的 CalendarStore。

type SQLCalendarStore struct {
	db *persist.SQLDB
}

   func NewSQLCalendarStore(db *persist.SQLDB) *SQLCalendarStore {
	   return &SQLCalendarStore{db: db}
   }

   func (s *SQLCalendarStore) GetByAddr(ctx context.Context, userID, chain, addr string) (AllowEntryLife, error) {
	  q := `SELECT entry_life_id, user_id, chain, address, label, state,
		added_at, cooling_from, active_at, revoking_at, revoked_at, generation, created_at, updated_at
		FROM dest_entry_lives
		WHERE user_id=? AND chain=? AND address=? AND state != 'revoked'
		ORDER BY created_at DESC LIMIT 1`
	  return s.scanOne(ctx, q, userID, strings.ToUpper(chain), addr)
   }

   func (s *SQLCalendarStore) GetByID(ctx context.Context, id string) (AllowEntryLife, error) {
	  q := `SELECT entry_life_id, user_id, chain, address, label, state,
		added_at, cooling_from, active_at, revoking_at, revoked_at, generation, created_at, updated_at
		FROM dest_entry_lives WHERE entry_life_id=?`
	  return s.scanOne(ctx, q, id)
  }

  func (s *SQLCalendarStore) Insert(ctx context.Context, e AllowEntryLife) error {
	   q := `INSERT INTO dest_entry_lives
		(entry_life_id, user_id, chain, address, label, state, added_at, cooling_from,
		 active_at, revoking_at, revoked_at, generation, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	   var activeAt, revokingAt, revokedAt interface{}
	   if !e.ActiveAt.IsZero() {
		  activeAt = e.ActiveAt
	  }
	   if !e.RevokingAt.IsZero() {
		  revokingAt = e.RevokingAt
	  }
	  if !e.RevokedAt.IsZero() {
		   revokedAt = e.RevokedAt
	   }
	  _, err := s.db.Conn(ctx).ExecContext(ctx, q,
		   e.EntryLifeID,
		  e.UserID, strings.ToUpper(e.Chain), e.Address, e.Label, string(e.State),
		  e.AddedAt, e.CoolingFrom, activeAt, revokingAt, revokedAt,
		  e.Generation, e.CreatedAt, e.UpdatedAt,
	  )
	  return err
   }

   func (s *SQLCalendarStore) UpdateState(ctx context.Context, entryLifeID string, to EntryState, at time.Time, expectedGen, newGen int64) error {
	   var q string
	  var args []interface{}
	   switch to {
	   case StateActive:
		   q = `UPDATE dest_entry_lives SET state=?, active_at=?, generation=?, updated_at=? WHERE entry_life_id=? AND generation=?`
		  args = []interface{}{string(to), at, newGen, at, entryLifeID, expectedGen}
	  case StateRevoking:
		  q = `UPDATE dest_entry_lives SET state=?, revoking_at=?, generation=?, updated_at=? WHERE entry_life_id=? AND generation=?`
		   args = []interface{}{string(to), at, newGen, at, entryLifeID, expectedGen}
	  case StateRevoked:
		  q = `UPDATE dest_entry_lives SET state=?, revoked_at=?, generation=?, updated_at=? WHERE entry_life_id=? AND generation=?`
		   args = []interface{}{string(to), at, newGen, at, entryLifeID, expectedGen}
	  default:
		  q = `UPDATE dest_entry_lives SET state=?, generation=?, updated_at=? WHERE entry_life_id=? AND generation=?`
		  args = []interface{}{string(to),
			   newGen, at, entryLifeID, expectedGen}
	  }
	   res, err := s.db.Conn(ctx).ExecContext(ctx, q, args...)
	   if err != nil {
		   return err
	   }
	   n, _ := res.RowsAffected()
	   if n == 0 {
		  return fmt.Errorf("destlife: update state: generation mismatch or entry not found (%s)", entryLifeID)
	   }
	   return nil
   }

   func (s *SQLCalendarStore) ListByUser(ctx context.Context, userID string) ([]AllowEntryLife, error) {
	   q := `SELECT entry_life_id, user_id, chain, address, label, state,
		added_at, cooling_from, active_at, revoking_at, revoked_at, generation, created_at, updated_at
		FROM dest_entry_lives WHERE user_id=? AND state != 'revoked' ORDER BY created_at DESC`
	  return s.scanMany(ctx, q, userID)
   }

   func (s *SQLCalendarStore) ListCooling(ctx context.Context, before time.Time, limit int) ([]AllowEntryLife, error) {
	  q := fmt.Sprintf(`SELECT entry_life_id, user_id, chain, address, label, state,
		added_at, cooling_from, active_at, revoking_at, revoked_at, generation, created_at, updated_at
		FROM dest_entry_lives WHERE state='cooling' AND cooling_from<=? ORDER BY cooling_from ASC LIMIT %d`, limit)
	  return s.scanMany(ctx, q, before)
  }

  func (s *SQLCalendarStore) ListRevoking(ctx context.Context, limit int) ([]AllowEntryLife, error) {
	   q := fmt.Sprintf(`SELECT entry_life_id, user_id, chain, address, label, state,
		added_at, cooling_from, active_at, revoking_at, revoked_at, generation, created_at, updated_at
		FROM dest_entry_lives WHERE state='revoking' ORDER BY revoking_at ASC LIMIT %d`, limit)
	  return s.scanMany(ctx, q)
   }

  func (s *SQLCalendarStore) scanOne(ctx context.Context, q string, args ...interface{}) (AllowEntryLife, error) {
	   rows, err := s.scanMany(ctx, q, args...)
	  if err != nil {
		   return AllowEntryLife{}, err
	   }
	   if len(rows) == 0 {
		   return AllowEntryLife{}, ErrEntryNotFound
	  }
	   return rows[0], nil
   }

   func (s *SQLCalendarStore) scanMany(ctx context.Context, q string, args ...interface{}) ([]AllowEntryLife, error) {
	  rows, err := s.db.Conn(ctx).QueryContext(ctx, q, args...)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	  var out []AllowEntryLife
	  for rows.Next() {
		  var e AllowEntryLife
		   var state string
		  var activeAt,
			  revokingAt, revokedAt sql.NullTime
		  if err := rows.Scan(
			   &e.EntryLifeID,
			  &e.UserID, &e.Chain, &e.Address, &e.Label, &state,
			   &e.AddedAt, &e.CoolingFrom, &activeAt, &revokingAt, &revokedAt,
			   &e.Generation, &e.CreatedAt, &e.UpdatedAt,
		   ); err != nil {
			  return nil, err
		  }
		  e.State = EntryState(state)
		   if activeAt.Valid {
			  e.ActiveAt = activeAt.Time
		   }
		  if revokingAt.Valid {
			   e.RevokingAt = revokingAt.Time
		   }
		   if revokedAt.Valid {
			  e.RevokedAt = revokedAt.Time
		   }
		  out = append(out, e)
	   }
	  return out, rows.Err()
  }

// SQLAuditStore SQL 实现的 AuditStore。
type SQLAuditStore struct {
	db *persist.SQLDB
}

   func NewSQLAuditStore(db *persist.SQLDB) *SQLAuditStore {
	   return &SQLAuditStore{db: db}
  }

   func (s *SQLAuditStore) AppendAudit(ctx context.Context, row LifecycleAuditRow) error {
	  q := `INSERT INTO lifecycle_audit
		(audit_id, entry_life_id, user_id, chain, address, actor, event, from_state, to_state, note, generation, at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`
	  _, err := s.db.Conn(ctx).ExecContext(ctx, q,
		   row.AuditID, row.EntryLifeID, row.UserID, row.Chain, row.Address,
		   row.Actor, row.Event, string(row.FromState), string(row.ToState),
		   row.Note, row.Generation, row.At,
	   )
	   return err
  }

   func (s *SQLAuditStore) ListAudit(ctx context.Context, entryLifeID string, limit int) ([]LifecycleAuditRow, error) {
	  q := fmt.Sprintf(`SELECT audit_id, entry_life_id, user_id, chain, address, actor, event, from_state, to_state, note, generation, at
		FROM lifecycle_audit WHERE entry_life_id=? ORDER BY at ASC LIMIT %d`, limit)
	   return s.scanAuditMany(ctx, q, entryLifeID)
  }

  func (s *SQLAuditStore) ListAuditByUser(ctx context.Context, userID string, limit int) ([]LifecycleAuditRow, error) {
	   q := fmt.Sprintf(`SELECT audit_id, entry_life_id, user_id, chain, address, actor, event, from_state, to_state, note, generation, at
		FROM lifecycle_audit WHERE user_id=? ORDER BY at DESC LIMIT %d`, limit)
	   return s.scanAuditMany(ctx,
		   q, userID)
   }

  func (s *SQLAuditStore) scanAuditMany(ctx context.Context, q string, args ...interface{}) ([]LifecycleAuditRow, error) {
	  rows, err := s.db.Conn(ctx).QueryContext(ctx, q, args...)
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  var out []LifecycleAuditRow
	  for rows.Next() {
		  var r LifecycleAuditRow
		   var from, to string
		   if err := rows.Scan(
			  &r.AuditID, &r.EntryLifeID, &r.UserID, &r.Chain, &r.Address,
			  &r.Actor,
			   &r.Event, &from, &to, &r.Note, &r.Generation, &r.At,
		   ); err != nil {
			  return nil, err
		   }
		  r.FromState = EntryState(from)
		  r.ToState = EntryState(to)
		  out = append(out, r)
	   }
	   return out, rows.Err()
  }

// SQLInflightStore SQL 实现的 InflightStore。
type SQLInflightStore struct {
	db *persist.SQLDB
}

   func NewSQLInflightStore(db *persist.SQLDB) *SQLInflightStore {
	  return &SQLInflightStore{db: db}
   }

   func (s *SQLInflightStore) InsertRef(ctx context.Context, ref InflightRef) error {
	   q := `INSERT INTO inflight_refs
		(ref_id, entry_life_id, user_id, chain, address, movement_id, acquired_at, released, released_at)
		VALUES (?,?,?,?,?,?,?,?,?)`
	   var releasedAt interface{}
	  if !ref.ReleasedAt.IsZero() {
		   releasedAt = ref.ReleasedAt
	   }
	  _, err := s.db.Conn(ctx).ExecContext(ctx, q,
		   ref.RefID, ref.EntryLifeID, ref.UserID, ref.Chain, ref.Address,
		   ref.MovementID, ref.AcquiredAt, ref.Released, releasedAt,
	  )
	  if err != nil && strings.Contains(err.Error(), "Duplicate") {
		  return ErrDuplicateRef
	  }
	  return err
  }

   func (s *SQLInflightStore) ReleaseRef(ctx context.Context, refID string, releasedAt time.Time) error {
	  _, err := s.db.Conn(ctx).ExecContext(ctx,
		   `UPDATE inflight_refs SET released=1, released_at=? WHERE ref_id=?`,
		   releasedAt, refID,
	   )
	  return err
  }

  func (s *SQLInflightStore) GetRef(ctx context.Context, refID string) (InflightRef, error) {
	  return s.scanRef(ctx, `SELECT ref_id, entry_life_id, user_id, chain, address, movement_id, acquired_at, released, released_at
		FROM inflight_refs WHERE ref_id=?`, refID)
   }

  func (s *SQLInflightStore) GetRefByMovement(ctx context.Context, movementID string) (InflightRef, bool, error) {
	   ref, err := s.scanRef(ctx, `SELECT ref_id, entry_life_id, user_id, chain, address, movement_id, acquired_at, released, released_at
		FROM inflight_refs WHERE movement_id=?`, movementID)
	   if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return InflightRef{}, false, nil
		   }
		   return InflightRef{}, false, err
	   }
	  return ref, true, nil
   }

  func (s *SQLInflightStore) ActiveCount(ctx context.Context, entryLifeID string) (int, error) {
	   var cnt int
	  err := s.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT COUNT(*) FROM inflight_refs WHERE entry_life_id=? AND released=0`,
		  entryLifeID,
	   ).Scan(&cnt)
	  return cnt, err
  }

   func (s *SQLInflightStore) ListByEntry(ctx context.Context, entryLifeID string, limit int) ([]InflightRef, error) {
	   q := fmt.Sprintf(`SELECT ref_id, entry_life_id, user_id, chain, address, movement_id, acquired_at, released, released_at
		FROM inflight_refs WHERE entry_life_id=? ORDER BY acquired_at DESC LIMIT %d`, limit)
	   return s.scanRefs(ctx, q, entryLifeID)
  }

   func (s *SQLInflightStore) ActiveSummary(ctx context.Context, entryLifeID string) (InflightCount, error) {
	  ic := InflightCount{EntryLifeID: entryLifeID}
	   err := s.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT user_id, chain, address, SUM(CASE WHEN released=0 THEN 1 ELSE 0 END), COUNT(*)
		 FROM inflight_refs WHERE entry_life_id=? GROUP BY user_id, chain, address`,
		  entryLifeID,
	  ).Scan(&ic.UserID, &ic.Chain, &ic.Address, &ic.Active, &ic.Total)
	   if errors.Is(err, sql.ErrNoRows) {
		  return ic, nil
	   }
	   return ic, err
  }

  func (s *SQLInflightStore) scanRef(ctx context.Context, q string, args ...interface{}) (InflightRef, error) {
	  refs,
		   err := s.scanRefs(ctx, q, args...)
	  if err != nil {
		  return InflightRef{}, err
	  }
	   if len(refs) == 0 {
		   return InflightRef{},
			   sql.ErrNoRows
	  }
	   return refs[0], nil
  }

   func (s *SQLInflightStore) scanRefs(ctx context.Context, q string, args ...interface{}) ([]InflightRef, error) {
	  rows,
		   err := s.db.Conn(ctx).QueryContext(ctx, q, args...)
	  if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   var out []InflightRef
	  for rows.Next() {
		   var r InflightRef
		  var releasedAt sql.NullTime
		  var released int
		  if err := rows.Scan(
			   &r.RefID, &r.EntryLifeID, &r.UserID, &r.Chain, &r.Address,
			  &r.MovementID, &r.AcquiredAt, &released, &releasedAt,
		   ); err != nil {
			   return nil, err
		   }
		   r.Released = released != 0
		  if releasedAt.Valid {
			  r.ReleasedAt = releasedAt.Time
		   }
		  out = append(out, r)
	   }
	  return out, rows.Err()
  }

// SQLWindowStore SQL 实现的 WindowStore。
type SQLWindowStore struct {
	db *persist.SQLDB
}

   func NewSQLWindowStore(db *persist.SQLDB) *SQLWindowStore {
	  return &SQLWindowStore{db: db}
  }

  func (s *SQLWindowStore) GetWindow(ctx context.Context, scope string) (CooldownWindow, error) {
	   var w CooldownWindow
	  err := s.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT id, scope, seconds, created_at, updated_at FROM cooldown_windows WHERE scope=? ORDER BY updated_at DESC LIMIT 1`,
		  scope,
	  ).Scan(&w.ID, &w.Scope, &w.Seconds, &w.CreatedAt, &w.UpdatedAt)
	   if errors.Is(err, sql.ErrNoRows) {
		  return CooldownWindow{}, fmt.Errorf("destlife: window not found for scope %q", scope)
	  }
	  if err != nil {
		   return CooldownWindow{}, err
	  }
	  w.Duration = time.Duration(w.Seconds) * time.Second
	   return w, nil
  }

  func (s *SQLWindowStore) SetWindow(ctx context.Context, w CooldownWindow) error {
	  q := `INSERT INTO cooldown_windows (id, scope, seconds, created_at, updated_at) VALUES (?,?,?,?,?)
ON DUPLICATE KEY UPDATE seconds=VALUES(seconds), updated_at=VALUES(updated_at)`
	  if s.db.Dialect == persist.Postgres {
		  q = `INSERT INTO cooldown_windows (id, scope, seconds, created_at, updated_at) VALUES (?,?,?,?,?)
ON CONFLICT (scope) DO UPDATE SET seconds=EXCLUDED.seconds, updated_at=EXCLUDED.updated_at`
	   }
	   _,
		   err := s.db.Conn(ctx).ExecContext(ctx, q, w.ID, w.Scope, w.Seconds, w.CreatedAt, w.UpdatedAt)
	  return err
  }

// SQLBypassStore SQL 实现的 BypassStore。
type SQLBypassStore struct {
	db *persist.SQLDB
}

   func NewSQLBypassStore(db *persist.SQLDB) *SQLBypassStore {
	  return &SQLBypassStore{db: db}
  }

  func (s *SQLBypassStore) InsertProof(ctx context.Context, proof BypassProof) error {
	  q := `INSERT INTO bypass_proofs
		(proof_id, entry_life_id, user_id, chain, address, reason, clock_drift_ms, blocked, blocked_by, detected_at, blocked_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`
	   var blockedAt interface{}
	   if !proof.BlockedAt.IsZero() {
		   blockedAt = proof.BlockedAt
	   }
	  _, err := s.db.Conn(ctx).ExecContext(ctx, q,
		   proof.ProofID, proof.EntryLifeID, proof.UserID, proof.Chain, proof.Address,
		   proof.Reason, proof.ClockDrift, proof.Blocked, proof.BlockedBy, proof.DetectedAt, blockedAt,
	   )
	  return err
   }

  func (s *SQLBypassStore) BlockProof(ctx context.Context, proofID, blockedBy string, at time.Time) error {
	   _, err := s.db.Conn(ctx).ExecContext(ctx,
		  `UPDATE bypass_proofs SET blocked=1, blocked_by=?, blocked_at=? WHERE proof_id=?`,
		  blockedBy, at, proofID,
	   )
	   return err
  }

   func (s *SQLBypassStore) ListByEntry(ctx context.Context, entryLifeID string, limit int) ([]BypassProof, error) {
	  q := fmt.Sprintf(`SELECT proof_id, entry_life_id, user_id, chain, address, reason, clock_drift_ms, blocked, blocked_by, detected_at, blocked_at
		FROM bypass_proofs WHERE entry_life_id=? ORDER BY detected_at DESC LIMIT %d`, limit)
	  return s.scanProofs(ctx, q, entryLifeID)
   }

   func (s *SQLBypassStore) ListUnblocked(ctx context.Context, limit int) ([]BypassProof, error) {
	  q := fmt.Sprintf(`SELECT proof_id, entry_life_id, user_id, chain, address, reason, clock_drift_ms, blocked, blocked_by, detected_at, blocked_at
		FROM bypass_proofs WHERE blocked=0 ORDER BY detected_at ASC LIMIT %d`, limit)
	   return s.scanProofs(ctx, q)
   }

  func (s *SQLBypassStore) scanProofs(ctx context.Context, q string, args ...interface{}) ([]BypassProof, error) {
	  rows, err := s.db.Conn(ctx).QueryContext(ctx, q, args...)
	  if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   var out []BypassProof
	  for rows.Next() {
		  var p BypassProof
		  var blocked int
		   var blockedBy sql.NullString
		  var blockedAt sql.NullTime
		   if err := rows.Scan(
			   &p.ProofID, &p.EntryLifeID, &p.UserID, &p.Chain, &p.Address,
			  &p.Reason, &p.ClockDrift, &blocked, &blockedBy, &p.DetectedAt, &blockedAt,
		  ); err != nil {
			   return nil, err
		  }
		  p.Blocked = blocked != 0
		  if blockedBy.Valid {
			  p.BlockedBy = blockedBy.String
		   }
		   if blockedAt.Valid {
			  p.BlockedAt = blockedAt.Time
		   }
		   out = append(out, p)
	   }
	  return out, rows.Err()
  }
