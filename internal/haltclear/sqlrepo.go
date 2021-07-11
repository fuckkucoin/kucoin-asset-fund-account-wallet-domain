package haltclear

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ---- SQLCaseRepo ---------------------------------------------------
// SQLCaseRepo MisClearCase SQL 实现；操作 misclear_cases 表。
// 功能：持久化误清案例全生命周期；open_diff_keys 序列化为 JSON 存入 TEXT 列。
// 不写 reengage_legs；不写对账工单。
// 不变量：misclear_id 是主键，INSERT 冲突返回 ErrDuplicateCase。

type SQLCaseRepo struct {
	db *sql.DB
}

// NewSQLCaseRepo 构造 SQL 实现。
   func NewSQLCaseRepo(db *sql.DB) *SQLCaseRepo {
	  return &SQLCaseRepo{db: db}
   }

  func (r *SQLCaseRepo) Create(ctx context.Context, c *MisClearCase) error {
	  if r == nil || r.db == nil {
		  return ErrStoreHaltclear
	  }
	  now := time.Now().UTC()
	  if c.CreatedAt.IsZero() {
		   c.CreatedAt = now
	   }
	  if c.UpdatedAt.IsZero() {
		   c.UpdatedAt = now
	   }
	   keysJSON, _ := json.Marshal(c.OpenDiffKeys)
	   q := `INSERT INTO misclear_cases
		(misclear_id, scope, chain, asset, trigger, state, detected_at,
		 open_diff_count, open_diff_keys, forced_audit_id, clear_plan_id,
		 reengaged_at, reengage_actor_id, reengage_leg_id,
		 closed_at, closed_by, close_note, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	   _, err := r.db.ExecContext(ctx, q,
		  c.MisclearID, c.Scope, c.Chain, c.Asset, string(c.Trigger),
		   string(c.State), c.DetectedAt,
		   c.OpenDiffCount, string(keysJSON),
		   c.ForcedAuditID, c.ClearPlanID,
		   nullTime(c.ReengagedAt), c.ReengageActorID, c.ReengageLegID,
		   nullTime(c.ClosedAt), c.ClosedBy, c.CloseNote,
		  c.CreatedAt, c.UpdatedAt,
	  )
	  if err != nil {
		   if isDuplicateKey(err) {
			  return ErrDuplicateCase
		  }
		  return fmt.Errorf("misclear create: %w", err)
	  }
	   return nil
   }

   func (r *SQLCaseRepo) Get(ctx context.Context, id string) (*MisClearCase, error) {
	   if r == nil || r.db == nil {
		  return nil, ErrStoreHaltclear
	   }
	   q := `SELECT misclear_id, scope, chain, asset, trigger, state, detected_at,
		open_diff_count, open_diff_keys, forced_audit_id, clear_plan_id,
		reengaged_at, reengage_actor_id, reengage_leg_id,
		closed_at, closed_by, close_note, created_at, updated_at
		FROM misclear_cases WHERE misclear_id = ?`
	  row := r.db.QueryRowContext(ctx, q, id)
	   return scanCase(row)
   }

   func (r *SQLCaseRepo) Update(ctx context.Context, c *MisClearCase) error {
	  if r == nil || r.db == nil {
		   return ErrStoreHaltclear
	  }
	   c.UpdatedAt = time.Now().UTC()
	  keysJSON, _ := json.Marshal(c.OpenDiffKeys)
	  q := `UPDATE misclear_cases SET
		scope=?, chain=?, asset=?, trigger=?, state=?, detected_at=?,
		open_diff_count=?, open_diff_keys=?, forced_audit_id=?, clear_plan_id=?,
		reengaged_at=?, reengage_actor_id=?, reengage_leg_id=?,
		closed_at=?, closed_by=?, close_note=?, updated_at=?
		WHERE misclear_id=?`
	  res, err := r.db.ExecContext(ctx, q,
		   c.Scope, c.Chain, c.Asset, string(c.Trigger), string(c.State), c.DetectedAt,
		   c.OpenDiffCount, string(keysJSON), c.ForcedAuditID, c.ClearPlanID,
		   nullTime(c.ReengagedAt), c.ReengageActorID, c.ReengageLegID,
		  nullTime(c.ClosedAt), c.ClosedBy, c.CloseNote, c.UpdatedAt,
		   c.MisclearID,
	   )
	  if err != nil {
		  return fmt.Errorf("misclear update: %w", err)
	  }
	   n, _ := res.RowsAffected()
	   if n == 0 {
		  return ErrCaseNotFound
	  }
	   return nil
   }

  func (r *SQLCaseRepo) List(ctx context.Context, f CaseFilter) ([]*MisClearCase, error) {
	  if r == nil || r.db == nil {
		   return nil, ErrStoreHaltclear
	   }
	  limit := f.Limit
	  if limit <= 0 || limit > 200 {
		  limit = 50
	  }
	  var conds []string
	   var args []any
	   if f.Scope != "" {
		   conds = append(conds, "scope = ?")
		  args = append(args, f.Scope)
	   }
	   if f.Chain != "" {
		  conds = append(conds, "chain = ?")
		  args = append(args, strings.ToUpper(f.Chain))
	   }
	   if f.Asset != "" {
		   conds = append(conds, "asset = ?")
		  args = append(args, strings.ToUpper(f.Asset))
	  }
	  if f.State != "" {
		   conds = append(conds, "state = ?")
		   args = append(args, string(f.State))
	  }
	   if f.Trigger != "" {
		   conds = append(conds, "trigger = ?")
		  args = append(args, string(f.Trigger))
	  }
	  if !f.After.IsZero() {
		  conds = append(conds, "detected_at > ?")
		   args = append(args, f.After)
	   }
	  where := ""
	   if len(conds) > 0 {
		   where = "WHERE " + strings.Join(conds, " AND ")
	   }
	  args = append(args, limit)
	   q := fmt.Sprintf(`SELECT misclear_id, scope, chain, asset, trigger, state, detected_at,
		open_diff_count, open_diff_keys, forced_audit_id, clear_plan_id,
		reengaged_at, reengage_actor_id, reengage_leg_id,
		closed_at, closed_by, close_note, created_at, updated_at
		FROM misclear_cases %s ORDER BY detected_at ASC LIMIT ?`, where)
	  rows, err := r.db.QueryContext(ctx, q, args...)
	  if err != nil {
		   return nil, fmt.Errorf("misclear list: %w", err)
	   }
	   defer rows.Close()
	   var out []*MisClearCase
	   for rows.Next() {
		   c, err := scanCaseRow(rows)
		  if err != nil {
			  return nil, err
		   }
		   out = append(out, c)
	   }
	   return out, rows.Err()
  }

  func (r *SQLCaseRepo) CountOpen(ctx context.Context, scope, chain, asset string) (int, error) {
	  if r == nil || r.db == nil {
		  return 0, ErrStoreHaltclear
	   }
	  var conds []string
	   var args []any
	  conds = append(conds, "state = ?")
	  args = append(args, string(MisClearStateOpen))
	   if scope != "" {
		  conds = append(conds, "scope = ?")
		  args = append(args, scope)
	   }
	   if chain != "" {
		  conds = append(conds, "chain = ?")
		   args = append(args, strings.ToUpper(chain))
	  }
	   if asset != "" {
		   conds = append(conds, "asset = ?")
		   args = append(args,
			  strings.ToUpper(asset))
	   }
	  q := fmt.Sprintf("SELECT COUNT(*) FROM misclear_cases WHERE %s", strings.Join(conds, " AND "))
	  var n int
	   if err := r.db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		   return 0, fmt.Errorf("misclear count open: %w", err)
	   }
	  return n, nil
   }

// ---- SQLLegRepo ---------------------------------------------------
// SQLLegRepo ReEngageLeg SQL 实现；操作 reengage_legs 表。
type SQLLegRepo struct {
	db *sql.DB
}

  func NewSQLLegRepo(db *sql.DB) *SQLLegRepo {
	  return &SQLLegRepo{db: db}
  }

   func (r *SQLLegRepo) Create(ctx context.Context, leg *ReEngageLeg) error {
	   if r == nil || r.db == nil {
		  return ErrStoreHaltclear
	   }
	   now := time.Now().UTC()
	  if leg.CreatedAt.IsZero() {
		  leg.CreatedAt = now
	   }
	   if leg.UpdatedAt.IsZero() {
		   leg.UpdatedAt = now
	   }
	   var proofJSON []byte
	  if leg.RollbackProof != nil {
		   proofJSON, _ = json.Marshal(leg.RollbackProof)
	   }
	  q := `INSERT INTO reengage_legs
		(leg_id, misclear_id, scope, chain, asset, state, actor_id, reason, is_auto,
		 scheduled_at, executed_at, engaged_at, failed_at, fail_reason,
		 rollback_proof, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	  _, err := r.db.ExecContext(ctx, q,
		   leg.LegID, leg.MisclearID, leg.Scope, leg.Chain, leg.Asset,
		   string(leg.State), leg.ActorID, leg.Reason, leg.IsAuto,
		   leg.ScheduledAt, nullTime(leg.ExecutedAt), nullTime(leg.EngagedAt),
		   nullTime(leg.FailedAt), leg.FailReason, nullBytes(proofJSON),
		  leg.CreatedAt, leg.UpdatedAt,
	  )
	  if err != nil {
		  if isDuplicateKey(err) {
			  return ErrDuplicateLeg
		   }
		  return fmt.Errorf("reengage leg create: %w", err)
	   }
	   return nil
  }

  func (r *SQLLegRepo) Get(ctx context.Context, id string) (*ReEngageLeg, error) {
	   if r == nil || r.db == nil {
		   return nil, ErrStoreHaltclear
	  }
	  q := `SELECT leg_id, misclear_id, scope, chain, asset, state, actor_id, reason, is_auto,
		scheduled_at, executed_at, engaged_at, failed_at, fail_reason,
		rollback_proof, created_at, updated_at
		FROM reengage_legs WHERE leg_id = ?`
	   row := r.db.QueryRowContext(ctx,
		  q, id)
	   return scanLeg(row)
  }

   func (r *SQLLegRepo) Update(ctx context.Context, leg *ReEngageLeg) error {
	  if r == nil || r.db == nil {
		  return ErrStoreHaltclear
	   }
	  leg.UpdatedAt = time.Now().UTC()
	   var proofJSON []byte
	   if leg.RollbackProof != nil {
		  proofJSON,
			  _ = json.Marshal(leg.RollbackProof)
	   }
	   q := `UPDATE reengage_legs SET
		scope=?, chain=?, asset=?, state=?, actor_id=?, reason=?, is_auto=?,
		scheduled_at=?, executed_at=?, engaged_at=?, failed_at=?, fail_reason=?,
		rollback_proof=?, updated_at=?
		WHERE leg_id=?`
	  res, err := r.db.ExecContext(ctx, q,
		  leg.Scope, leg.Chain, leg.Asset, string(leg.State), leg.ActorID, leg.Reason, leg.IsAuto,
		  leg.ScheduledAt,
		   nullTime(leg.ExecutedAt), nullTime(leg.EngagedAt),
		  nullTime(leg.FailedAt), leg.FailReason, nullBytes(proofJSON),
		  leg.UpdatedAt, leg.LegID,
	  )
	   if err != nil {
		   return fmt.Errorf("reengage leg update: %w", err)
	  }
	  n, _ := res.RowsAffected()
	  if n == 0 {
		   return ErrLegNotFound
	  }
	  return nil
  }

   func (r *SQLLegRepo) ListByCase(ctx context.Context, misclearID string, limit int) ([]*ReEngageLeg, error) {
	   if r == nil || r.db == nil {
		  return nil, ErrStoreHaltclear
	  }
	   if limit <= 0 || limit > 200 {
		  limit = 50
	  }
	   q := `SELECT leg_id, misclear_id, scope, chain, asset, state, actor_id, reason, is_auto, 
		scheduled_at, executed_at, engaged_at, failed_at, fail_reason,
		rollback_proof, created_at, updated_at
		FROM reengage_legs WHERE misclear_id = ? ORDER BY created_at ASC LIMIT ?`
	   rows, err := r.db.QueryContext(ctx, q, misclearID, limit)
	   if err != nil {
		   return nil, fmt.Errorf("reengage leg list: %w", err)
	  }
	   defer rows.Close()
	  var out []*ReEngageLeg
	  for rows.Next() {
		   l, err := scanLegRow(rows)
		  if err != nil {
			  return nil, err
		  }
		  out = append(out, l)
	   }
	   return out, rows.Err()
  }

  func (r *SQLLegRepo) ActiveLeg(ctx context.Context, misclearID string) (*ReEngageLeg, error) {
	   if r == nil || r.db == nil {
		  return nil, ErrStoreHaltclear
	   }
	   q := `SELECT leg_id, misclear_id, scope, chain, asset, state, actor_id, reason, is_auto,
		scheduled_at, executed_at, engaged_at, failed_at, fail_reason,
		rollback_proof, created_at, updated_at
		FROM reengage_legs WHERE misclear_id = ? AND state IN ('pending','executing')
		ORDER BY created_at DESC LIMIT 1`
	   row := r.db.QueryRowContext(ctx, q, misclearID)
	  leg, err := scanLeg(row)
	  if errors.Is(err, sql.ErrNoRows) {
		   return nil, nil
	  }
	   return leg, err
   }

// ---- 扫描辅助 -------------------------------------------------------

type caseScanner interface {
	Scan(dest ...any) error
}

   func scanCase(row caseScanner) (*MisClearCase, error) {
	   return scanCaseRow(row)
  }

   func scanCaseRow(row caseScanner) (*MisClearCase, error) {
	  var (
		  c           MisClearCase
		  reengagedAt sql.NullTime
		  closedAt    sql.NullTime
		  keysJSON string
		   trigger string
		   state   string
	  )
	  err := row.Scan(
		  &c.MisclearID,
		   &c.Scope, &c.Chain, &c.Asset, &trigger, &state, &c.DetectedAt,
		   &c.OpenDiffCount, &keysJSON, &c.ForcedAuditID, &c.ClearPlanID,
		   &reengagedAt, &c.ReengageActorID, &c.ReengageLegID,
		   &closedAt, &c.ClosedBy, &c.CloseNote, &c.CreatedAt, &c.UpdatedAt,
	   )
	   if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrCaseNotFound
		   }
		  return nil, fmt.Errorf("scan misclear case: %w", err)
	   }
	   c.Trigger = MisClearTrigger(trigger)
	  c.State = MisClearCaseState(state)
	   if reengagedAt.Valid {
		  c.ReengagedAt = reengagedAt.Time
	   }
	   if closedAt.Valid {
		  c.ClosedAt = closedAt.Time
	  }
	  if keysJSON != "" && keysJSON != "null" {
		   _ = json.Unmarshal([]byte(keysJSON), &c.OpenDiffKeys)
	   }
	  return &c, nil
   }

type legScanner interface {
	Scan(dest ...any) error
}

   func scanLeg(row legScanner) (*ReEngageLeg, error) {
	   return scanLegRow(row)
   }

   func scanLegRow(row legScanner) (*ReEngageLeg, error) {
	  var (
		   leg        ReEngageLeg
		   executedAt sql.NullTime
		   engagedAt  sql.NullTime
		  failedAt   sql.NullTime
		   proofJSON []byte
		   state string
	  )
	  err := row.Scan(
		  &leg.LegID, &leg.MisclearID, &leg.Scope, &leg.Chain, &leg.Asset,
		  &state,
		  &leg.ActorID, &leg.Reason, &leg.IsAuto,
		   &leg.ScheduledAt, &executedAt, &engagedAt, &failedAt, &leg.FailReason,
		   &proofJSON, &leg.CreatedAt, &leg.UpdatedAt,
	  )
	  if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrLegNotFound
		  }
		   return nil, fmt.Errorf("scan reengage leg: %w", err)
	  }
	   leg.State = ReEngageLegState(state)
	  if executedAt.Valid {
		   leg.ExecutedAt = executedAt.Time
	   }
	   if engagedAt.Valid {
		   leg.EngagedAt = engagedAt.Time
	  }
	  if failedAt.Valid {
		   leg.FailedAt = failedAt.Time
	   }
	   if len(proofJSON) > 0 {
		  leg.RollbackProof = &RollbackProof{}
		   _ = json.Unmarshal(proofJSON, leg.RollbackProof)
	  }
	   return &leg, nil
  }

// ---- SQL 辅助 -------------------------------------------------------

  func nullTime(t time.Time) any {
	   if t.IsZero() {
		   return nil
	   }
	   return t
  }

   func nullBytes(b []byte) any {
	  if len(b) == 0 {
		   return nil
	   }
	   return string(b)
  }

  func isDuplicateKey(err error) bool {
	   if err == nil {
		  return false
	   }
	  msg := err.Error()
	   return strings.Contains(msg, "Duplicate entry") ||
		   strings.Contains(msg, "duplicate key") ||
		   strings.Contains(msg, "UNIQUE constraint") ||
		   strings.Contains(msg, "unique_violation")
  }
