package sqlrepo

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

// FundSignalRow 滥用信号一行。source_key 保留原域键（dep:/wd:/rl:）原文。
type FundSignalRow struct {
	ID   string
	Kind string
	Path string
	ScopeKey string
	UserID   string
	Chain    string
	Asset     string
	Weight    int
	SourceKey string
	Note string
	CreatedAt time.Time
}

// FundCircuitRow 熔断器一行。row_key 恒为 fc: 前缀。

type FundCircuitRow struct {
	Key   string
	Scope string
	UserID string
	Chain  string
	Asset  string
	State  string
	Score  int
	Severity string
	Reason       string
	Trips        int
	ProbesIssued int
	Forced bool
	CrossPath bool
	Actor     string
	CaseID    string
	PolicyVer int
	OpenedAt   time.Time
	HalfOpenAt time.Time
	ClosedAt time.Time
	NextAdvanceAt time.Time
	UpdatedAt     time.Time
}

// FundPolicyRow 阻断策略一版。矩阵与作用域开关以 JSON 文档落库。

type FundPolicyRow struct {
	Version int
	Actor   string
	Note string
	DocJSON   string
	UpdatedAt time.Time
}

// FundCaseRow 熔断案件一行。clear_votes 为逗号分隔的票面。
type FundCaseRow struct {
	ID         string
	CircuitKey string
	Scope      string
	UserID string
	Chain string
	Asset string
	State string
	Score    int
	Severity string
	Reason     string
	OpenedBy   string
	Reviewer   string
	ClearReq   string
	ClearVotes string
	AuditJSON  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// FundProbeRow 半开探针一行。

type FundProbeRow struct {
	ID         string
	CircuitKey string
	Path       string
	State      string
	Token string
	Actor string
	Round     int
	Note      string
	IssuedAt  time.Time
	ExpiresAt time.Time
	DecidedAt time.Time
}

// FundCircuit 熔断总线仓储。
type FundCircuit struct{ db *persist.SQLDB }

// NewFundCircuit 建仓储。db 为空时全部方法返回 ErrUnavailable。
   func NewFundCircuit(db *persist.SQLDB) *FundCircuit { return &FundCircuit{db: db} }

// PutSignal 追加一条信号。信号只追加不更新。
  func (r *FundCircuit) PutSignal(ctx context.Context, row FundSignalRow) error {
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	   }
	  if strings.TrimSpace(row.ID) == "" {
		   row.ID = uuid.NewString()
	  }
	   if row.CreatedAt.IsZero() {
		   row.CreatedAt = time.Now().UTC()
	  }
	   stmt := `INSERT INTO aa_fund_signals (id, kind, path, scope_key, user_id, chain, asset, weight, source_key, note, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE weight=VALUES(weight), note=VALUES(note)`
	  if r.db.Dialect == persist.Postgres {
		  stmt = `INSERT INTO aa_fund_signals (id, kind, path, scope_key, user_id, chain, asset, weight, source_key, note, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET weight=EXCLUDED.weight, note=EXCLUDED.note`
	  }
	  _,
		  err := r.db.Conn(ctx).ExecContext(ctx, stmt,
		   row.ID, row.Kind, row.Path, row.ScopeKey, row.UserID, row.Chain, row.Asset,
		   row.Weight, row.SourceKey, row.Note, row.CreatedAt)
	   return err
   }

// CountSignals 信号条数，供漂移核对。

   func (r *FundCircuit) CountSignals(ctx context.Context) (int, error) {
	   if !sqlOn(r.db) {
		  return 0, ErrUnavailable
	  }
	  var n int
	  err := r.db.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(1) FROM aa_fund_signals`).Scan(&n)
	   return n, err
  }

// PruneSignals 摘除窗口外信号，返回删除条数。
   func (r *FundCircuit) PruneSignals(ctx context.Context, before time.Time) (int, error) {
	  if !sqlOn(r.db) {
		  return 0, ErrUnavailable
	  }
	   res, err := r.db.Conn(ctx).ExecContext(ctx, `DELETE FROM aa_fund_signals WHERE created_at < ?`, before)
	   if err != nil {
		  return 0, err
	  }
	   n, err := res.RowsAffected()
	  if err != nil {
		  return 0, nil
	  }
	  return int(n), nil
  }

// ListSignals 按作用域键取信号，最近在前。
  func (r *FundCircuit) ListSignals(ctx context.Context, scopeKey string, limit int) ([]FundSignalRow, error) {
	  if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	   }
	  if limit <= 0 || limit > 500 {
		  limit = 100
	   }
	  q := `SELECT id, kind, path, scope_key, user_id, chain, asset, weight, source_key, note, created_at FROM aa_fund_signals`
	  args := make([]any, 0, 2)
	  if strings.TrimSpace(scopeKey) != "" {
		   q += ` WHERE scope_key=?`
		  args = append(args, scopeKey)
	  }
	  q += ` ORDER BY created_at DESC LIMIT ?`
	  args = append(args, limit)
	  rows, err := r.db.Conn(ctx).QueryContext(ctx, q, args...)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   out := make([]FundSignalRow, 0, limit)
	  for rows.Next() {
		  var row FundSignalRow
		  if err := rows.Scan(&row.ID, &row.Kind, &row.Path, &row.ScopeKey, &row.UserID, &row.Chain,
			   &row.Asset, &row.Weight, &row.SourceKey, &row.Note, &row.CreatedAt); err != nil {
			  return nil, err
		  }
		   out = append(out, row)
	  }
	  return out, rows.Err()
   }

// PutCircuit 按键覆盖熔断器一行。

  func (r *FundCircuit) PutCircuit(ctx context.Context, row FundCircuitRow) error {
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	  if strings.TrimSpace(row.Key) == "" {
		   return ErrConflict
	  }
	  if row.UpdatedAt.IsZero() {
		   row.UpdatedAt = time.Now().UTC()
	  }
	   stmt := `INSERT INTO aa_fund_circuits (row_key, scope, user_id, chain, asset, state, score, severity, reason, trips, probes_issued, forced, cross_path, actor, case_id, policy_ver, opened_at, half_open_at, closed_at, next_advance_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE scope=VALUES(scope), user_id=VALUES(user_id), chain=VALUES(chain), asset=VALUES(asset), state=VALUES(state), score=VALUES(score), severity=VALUES(severity), reason=VALUES(reason), trips=VALUES(trips), probes_issued=VALUES(probes_issued), forced=VALUES(forced), cross_path=VALUES(cross_path), actor=VALUES(actor), case_id=VALUES(case_id), policy_ver=VALUES(policy_ver), opened_at=VALUES(opened_at), half_open_at=VALUES(half_open_at), closed_at=VALUES(closed_at), next_advance_at=VALUES(next_advance_at), updated_at=VALUES(updated_at)`
	  if r.db.Dialect == persist.Postgres {
		   stmt = `INSERT INTO aa_fund_circuits (row_key, scope, user_id, chain, asset, state, score, severity, reason, trips, probes_issued, forced, cross_path, actor, case_id, policy_ver, opened_at, half_open_at, closed_at, next_advance_at, updated_at) 
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (row_key) DO UPDATE SET scope=EXCLUDED.scope, user_id=EXCLUDED.user_id, chain=EXCLUDED.chain, asset=EXCLUDED.asset, state=EXCLUDED.state, score=EXCLUDED.score, severity=EXCLUDED.severity, reason=EXCLUDED.reason, trips=EXCLUDED.trips, probes_issued=EXCLUDED.probes_issued, forced=EXCLUDED.forced, cross_path=EXCLUDED.cross_path, actor=EXCLUDED.actor, case_id=EXCLUDED.case_id, policy_ver=EXCLUDED.policy_ver, opened_at=EXCLUDED.opened_at, half_open_at=EXCLUDED.half_open_at, closed_at=EXCLUDED.closed_at, next_advance_at=EXCLUDED.next_advance_at, updated_at=EXCLUDED.updated_at`
	   }
	  _,
		  err := r.db.Conn(ctx).ExecContext(ctx, stmt,
		   row.Key, row.Scope, row.UserID, row.Chain, row.Asset, row.State, row.Score, row.Severity,
		  row.Reason, row.Trips, row.ProbesIssued, bit(row.Forced), bit(row.CrossPath), row.Actor,
		  row.CaseID, row.PolicyVer, nullTime(row.OpenedAt), nullTime(row.HalfOpenAt),
		  nullTime(row.ClosedAt), nullTime(row.NextAdvanceAt), row.UpdatedAt)
	  return err
  }

// ListCircuits 取熔断器清单。
   func (r *FundCircuit) ListCircuits(ctx context.Context, limit int) ([]FundCircuitRow, error) {
	  if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	   }
	  if limit <= 0 || limit > 1000 {
		   limit = 200
	   }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx,
		   `SELECT row_key, scope, user_id, chain, asset, state, score, severity, reason, trips, probes_issued, forced, cross_path, actor, case_id, policy_ver, opened_at, half_open_at, closed_at, next_advance_at, updated_at
FROM aa_fund_circuits ORDER BY row_key LIMIT ?`, limit)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   out := make([]FundCircuitRow, 0, limit)
	   for rows.Next() {
		  var row FundCircuitRow
		   var forced, cross int
		   var opened,
			   half, closed, next *time.Time
		   if err := rows.Scan(&row.Key, &row.Scope, &row.UserID, &row.Chain, &row.Asset, &row.State,
			  &row.Score, &row.Severity, &row.Reason, &row.Trips, &row.ProbesIssued, &forced, &cross,
			  &row.Actor, &row.CaseID, &row.PolicyVer, &opened, &half, &closed, &next, &row.UpdatedAt); err != nil {
			   return nil, err
		   }
		  row.Forced = forced != 0
		  row.CrossPath = cross != 0
		  row.OpenedAt = deref(opened)
		  row.HalfOpenAt = deref(half)
		   row.ClosedAt = deref(closed)
		   row.NextAdvanceAt = deref(next)
		   out = append(out, row)
	  }
	  return out, rows.Err()
  }

// PutPolicy 落一版策略。同代次覆盖，供回放。
  func (r *FundCircuit) PutPolicy(ctx context.Context, row FundPolicyRow) error {
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	  if row.Version <= 0 {
		   return ErrConflict
	   }
	  if row.UpdatedAt.IsZero() {
		   row.UpdatedAt = time.Now().UTC()
	   }
	   stmt := `INSERT INTO aa_fund_policies (version, actor, note, doc_json, updated_at) 
VALUES (?,?,?,?,?)
ON DUPLICATE KEY UPDATE actor=VALUES(actor), note=VALUES(note), doc_json=VALUES(doc_json), updated_at=VALUES(updated_at)`
	  if r.db.Dialect == persist.Postgres {
		  stmt = `INSERT INTO aa_fund_policies (version, actor, note, doc_json, updated_at)
VALUES (?,?,?,?,?)
ON CONFLICT (version) DO UPDATE SET actor=EXCLUDED.actor, note=EXCLUDED.note, doc_json=EXCLUDED.doc_json, updated_at=EXCLUDED.updated_at`
	   }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, stmt, row.Version, row.Actor, row.Note, row.DocJSON, row.UpdatedAt)
	   return err
  }

// LatestPolicy 取最新一版策略。无记录返回 ErrNotFound。
  func (r *FundCircuit) LatestPolicy(ctx context.Context) (FundPolicyRow, error) {
	   if !sqlOn(r.db) {
		  return FundPolicyRow{},
			  ErrUnavailable
	   }
	  var row FundPolicyRow
	   err := r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT version, actor, note, doc_json, updated_at FROM aa_fund_policies ORDER BY version DESC LIMIT 1`).
		   Scan(&row.Version, &row.Actor, &row.Note, &row.DocJSON, &row.UpdatedAt)
	   if err != nil {
		  return FundPolicyRow{}, err
	  }
	   return row, nil
   }

// PutCase 覆盖案件一行。

   func (r *FundCircuit) PutCase(ctx context.Context, row FundCaseRow) error {
	   if !sqlOn(r.db) {
		   return ErrUnavailable
	   }
	  if strings.TrimSpace(row.ID) == "" {
		   return ErrConflict
	   }
	   if row.UpdatedAt.IsZero() {
		   row.UpdatedAt = time.Now().UTC()
	  }
	  if row.CreatedAt.IsZero() {
		  row.CreatedAt = row.UpdatedAt
	  }
	   stmt := `INSERT INTO aa_fund_cases (id, circuit_key, scope, user_id, chain, asset, state, score, severity, reason, opened_by, reviewer, clear_req, clear_votes, audit_json, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE state=VALUES(state), score=VALUES(score), severity=VALUES(severity), reason=VALUES(reason), reviewer=VALUES(reviewer), clear_req=VALUES(clear_req), clear_votes=VALUES(clear_votes), audit_json=VALUES(audit_json), updated_at=VALUES(updated_at)`
	  if r.db.Dialect == persist.Postgres {
		   stmt = `INSERT INTO aa_fund_cases (id, circuit_key, scope, user_id, chain, asset, state, score, severity, reason, opened_by, reviewer, clear_req, clear_votes, audit_json, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET state=EXCLUDED.state, score=EXCLUDED.score, severity=EXCLUDED.severity, reason=EXCLUDED.reason, reviewer=EXCLUDED.reviewer, clear_req=EXCLUDED.clear_req, clear_votes=EXCLUDED.clear_votes, audit_json=EXCLUDED.audit_json, updated_at=EXCLUDED.updated_at`
	   }
	   _,
		   err := r.db.Conn(ctx).ExecContext(ctx, stmt,
		  row.ID, row.CircuitKey, row.Scope, row.UserID, row.Chain, row.Asset, row.State, row.Score,
		  row.Severity, row.Reason, row.OpenedBy, row.Reviewer, row.ClearReq, row.ClearVotes,
		  row.AuditJSON, row.CreatedAt, row.UpdatedAt)
	   return err
  }

// ListCases 按状态取案件；state 为空取全部。
   func (r *FundCircuit) ListCases(ctx context.Context, state string, limit int) ([]FundCaseRow, error) {
	  if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	   }
	  if limit <= 0 || limit > 500 {
		   limit = 100
	  }
	   q := `SELECT id, circuit_key, scope, user_id, chain, asset, state, score, severity, reason, opened_by, reviewer, clear_req, clear_votes, audit_json, created_at, updated_at FROM aa_fund_cases`
	   args := make([]any, 0, 2)
	  if strings.TrimSpace(state) != "" {
		  q += ` WHERE state=?`
		   args = append(args, state)
	   }
	   q += ` ORDER BY updated_at DESC LIMIT ?`
	  args = append(args, limit)
	  rows,
		   err := r.db.Conn(ctx).QueryContext(ctx, q, args...)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  out := make([]FundCaseRow, 0, limit)
	  for rows.Next() {
		   var row FundCaseRow
		  if err := rows.Scan(&row.ID, &row.CircuitKey, &row.Scope, &row.UserID, &row.Chain, &row.Asset,
			  &row.State, &row.Score, &row.Severity, &row.Reason, &row.OpenedBy, &row.Reviewer,
			   &row.ClearReq, &row.ClearVotes, &row.AuditJSON, &row.CreatedAt, &row.UpdatedAt); err != nil {
			   return nil, err
		   }
		   out = append(out, row)
	   }
	  return out, rows.Err()
   }

// PutProbe 覆盖探针一行。
  func (r *FundCircuit) PutProbe(ctx context.Context, row FundProbeRow) error {
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	   if strings.TrimSpace(row.ID) == "" {
		   return ErrConflict
	   }
	   if row.IssuedAt.IsZero() {
		  row.IssuedAt = time.Now().UTC()
	  }
	   stmt := `INSERT INTO aa_fund_probes (id, circuit_key, path, state, token, actor, round, note, issued_at, expires_at, decided_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE state=VALUES(state), actor=VALUES(actor), note=VALUES(note), decided_at=VALUES(decided_at)`
	  if r.db.Dialect == persist.Postgres {
		   stmt = `INSERT INTO aa_fund_probes (id, circuit_key, path, state, token, actor, round, note, issued_at, expires_at, decided_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET state=EXCLUDED.state, actor=EXCLUDED.actor, note=EXCLUDED.note, decided_at=EXCLUDED.decided_at`
	  }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, stmt,
		   row.ID, row.CircuitKey, row.Path, row.State, row.Token, row.Actor, row.Round, row.Note,
		  row.IssuedAt, nullTime(row.ExpiresAt), nullTime(row.DecidedAt))
	  return err
   }

// ListProbes 按状态取探针；state 为空取全部。

   func (r *FundCircuit) ListProbes(ctx context.Context, state string, limit int) ([]FundProbeRow, error) {
	   if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	   }
	   if limit <= 0 || limit > 500 {
		   limit = 100
	   }
	   q := `SELECT id, circuit_key, path, state, token, actor, round, note, issued_at, expires_at, decided_at FROM aa_fund_probes`
	  args := make([]any, 0, 2)
	   if strings.TrimSpace(state) != "" {
		  q += ` WHERE state=?`
		   args = append(args, state)
	   }
	   q += ` ORDER BY issued_at DESC LIMIT ?`
	   args = append(args, limit)
	  rows, err := r.db.Conn(ctx).QueryContext(ctx, q, args...)
	  if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	   out := make([]FundProbeRow, 0, limit)
	  for rows.Next() {
		  var row FundProbeRow
		  var expires, decided *time.Time
		  if err := rows.Scan(&row.ID, &row.CircuitKey, &row.Path, &row.State, &row.Token, &row.Actor,
			  &row.Round, &row.Note, &row.IssuedAt, &expires, &decided); err != nil {
			   return nil, err
		   }
		  row.ExpiresAt = deref(expires)
		  row.DecidedAt = deref(decided)
		   out = append(out, row)
	  }
	   return out, rows.Err()
  }

// deref 可空时间列回填零值，供内存台账装回。
   func deref(t *time.Time) time.Time {
	  if t == nil {
		  return time.Time{}
	  }
	  return *t
  }
