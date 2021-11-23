package reconflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// Store SQL 仓储操作集合。

type Store struct {
	db *persist.SQLDB
}

// NewStore 构造 Store；db 为 nil 时所有写操作返回 ErrStore，读操作返回空。
  func NewStore(db *persist.SQLDB) *Store {
	  return &Store{db: db}
   }

   func (s *Store) on() bool {
	  return s != nil && s.db != nil && s.db.DB != nil
   }

   func (s *Store) dialect() persist.Dialect {
	   if s.db == nil {
		  return persist.MySQL
	  }
	   return s.db.Dialect
   }

// conn 获取当前上下文连接（支持事务透传）。
  func (s *Store) conn(ctx context.Context) persist.Queryer {
	   return s.db.Conn(ctx)
  }

// ──────────────────────────────────────────────
// FlowRun
// ──────────────────────────────────────────────
// SaveRun 插入或更新 FlowRun（以 id 为主键冲突键）。
  func (s *Store) SaveRun(ctx context.Context, r FlowRun) error {
	  if !s.on() {
		   return ErrStore
	  }
	   gateAt := timePtr(r.GateAt)
	  actionIDs := ""
	   qMy := `INSERT INTO aa_recon_flow_runs
(id, flow_def_id, ticket_id, user_id, asset, chain, diff_kind, state, class_id, confidence, current_step,
 gate_actor, gate_at, closed_by, escalated_by, note, fence_epoch, fence_token, created_by, created_at, updated_at, sla_deadline)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
 flow_def_id=VALUES(flow_def_id), ticket_id=VALUES(ticket_id), state=VALUES(state),
 class_id=VALUES(class_id), confidence=VALUES(confidence), current_step=VALUES(current_step),
 gate_actor=VALUES(gate_actor), gate_at=VALUES(gate_at), closed_by=VALUES(closed_by),
 escalated_by=VALUES(escalated_by), note=VALUES(note), fence_epoch=VALUES(fence_epoch),
 fence_token=VALUES(fence_token), updated_at=VALUES(updated_at)`
	  qPg := `INSERT INTO aa_recon_flow_runs
(id, flow_def_id, ticket_id, user_id, asset, chain, diff_kind, state, class_id, confidence, current_step,
 gate_actor, gate_at, closed_by, escalated_by, note, fence_epoch, fence_token, created_by, created_at, updated_at, sla_deadline)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
ON CONFLICT (id) DO UPDATE SET
 flow_def_id=EXCLUDED.flow_def_id, ticket_id=EXCLUDED.ticket_id, state=EXCLUDED.state,
 class_id=EXCLUDED.class_id, confidence=EXCLUDED.confidence, current_step=EXCLUDED.current_step,
 gate_actor=EXCLUDED.gate_actor, gate_at=EXCLUDED.gate_at, closed_by=EXCLUDED.closed_by,
 escalated_by=EXCLUDED.escalated_by, note=EXCLUDED.note, fence_epoch=EXCLUDED.fence_epoch,
 fence_token=EXCLUDED.fence_token, updated_at=EXCLUDED.updated_at`
	  q := qMy
	  if s.dialect() == persist.Postgres {
		   q = qPg
	   }
	   _ = actionIDs
	   _, err := s.conn(ctx).ExecContext(ctx, q,
		  r.ID, r.FlowDefID, r.TicketID, r.UserID, r.Asset, r.Chain, r.DiffKind,
		   string(r.State), r.ClassID, string(r.Confidence), r.CurrentStep,
		   r.GateActor,
		  gateAt, r.ClosedBy, r.EscalatedBy, r.Note,
		   r.FenceEpoch, r.FenceToken, r.CreatedBy, r.CreatedAt, r.UpdatedAt, r.SLADeadline,
	  )
	  return wrapStoreErr(err)
   }

// GetRun 按 ID 加载 FlowRun。
  func (s *Store) GetRun(ctx context.Context, id string) (FlowRun, error) {
	  if !s.on() {
		  return FlowRun{}, ErrStore
	   }
	   q := `SELECT id, flow_def_id, ticket_id, user_id, asset, chain, diff_kind, state, class_id, confidence,
 current_step, gate_actor, gate_at, closed_by, escalated_by, note, fence_epoch, fence_token,
 created_by, created_at, updated_at, sla_deadline
FROM aa_recon_flow_runs WHERE id=?`
	   if s.dialect() == persist.Postgres {
		  q = strings.ReplaceAll(q, "?", "$1")
	  }
	   var r FlowRun
	  var state, conf string
	  var gateAt sql.NullTime
	   err := s.conn(ctx).QueryRowContext(ctx, q, id).Scan(
		   &r.ID,
		  &r.FlowDefID, &r.TicketID, &r.UserID, &r.Asset, &r.Chain, &r.DiffKind,
		  &state, &r.ClassID, &conf, &r.CurrentStep,
		   &r.GateActor, &gateAt, &r.ClosedBy, &r.EscalatedBy, &r.Note,
		  &r.FenceEpoch,
		  &r.FenceToken, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt, &r.SLADeadline,
	  )
	   if errors.Is(err, sql.ErrNoRows) {
		   return FlowRun{}, ErrNotFound
	   }
	   if err != nil {
		  return FlowRun{}, wrapStoreErr(err)
	   }
	  r.State = RunState(state)
	  r.Confidence = ConfidenceLevel(conf)
	   if gateAt.Valid {
		  t := gateAt.Time
		   r.GateAt = &t
	   }
	   return r, nil
  }

// ListRuns 按条件列举 FlowRun，按创建时间降序。
   func (s *Store) ListRuns(ctx context.Context, q RunQuery) ([]FlowRun, error) {
	   if !s.on() {
		   return nil, nil
	   }
	   limit := q.Limit
	   if limit <= 0 || limit > 200 {
		   limit = 50
	   }
	   offset := q.Offset
	   if offset < 0 {
		  offset = 0
	   }
	   where, args := buildRunWhere(q, s.dialect())
	   baseQ := fmt.Sprintf(
		   `SELECT id, flow_def_id, ticket_id, user_id, asset, chain, diff_kind, state, class_id,
 confidence, current_step, gate_actor, gate_at, closed_by, escalated_by, note,
 fence_epoch, fence_token, created_by, created_at, updated_at, sla_deadline
FROM aa_recon_flow_runs %s ORDER BY created_at DESC LIMIT %d OFFSET %d`, where, limit, offset)
	  rows, err := s.conn(ctx).QueryContext(ctx, baseQ, args...)
	   if err != nil {
		  return nil, wrapStoreErr(err)
	   }
	  defer rows.Close()
	  return scanRuns(rows)
  }

// ListActiveRuns 列举所有非终态运行（供 Worker 使用）。
  func (s *Store) ListActiveRuns(ctx context.Context, limit int) ([]FlowRun, error) {
	   if !s.on() {
		  return nil, nil
	  }
	   if limit <= 0 || limit > 500 {
		  limit = 200
	  }
	   qStr := fmt.Sprintf(
		   `SELECT id, flow_def_id, ticket_id, user_id, asset, chain, diff_kind, state, class_id,
 confidence, current_step, gate_actor, gate_at, closed_by, escalated_by, note,
 fence_epoch, fence_token, created_by, created_at, updated_at, sla_deadline
FROM aa_recon_flow_runs
WHERE state NOT IN ('closed','escalated') ORDER BY created_at ASC LIMIT %d`, limit)
	   rows, err := s.conn(ctx).QueryContext(ctx, qStr)
	   if err != nil {
		   return nil, wrapStoreErr(err)
	   }
	   defer rows.Close()
	   return scanRuns(rows)
  }

// ListBreachedRuns 列举 SLA 超时的非终态运行。

   func (s *Store) ListBreachedRuns(ctx context.Context, now time.Time, limit int) ([]FlowRun, error) {
	   if !s.on() {
		   return nil, nil
	   }
	  if limit <= 0 {
		  limit = 100
	   }
	  qStr := fmt.Sprintf(
		   `SELECT id, flow_def_id, ticket_id, user_id, asset, chain, diff_kind, state, class_id,
 confidence, current_step, gate_actor, gate_at, closed_by, escalated_by, note,
 fence_epoch, fence_token, created_by, created_at, updated_at, sla_deadline
FROM aa_recon_flow_runs
WHERE state NOT IN ('closed','escalated') AND sla_deadline < ? ORDER BY sla_deadline ASC LIMIT %d`, limit)
	  if s.dialect() == persist.Postgres {
		  qStr = strings.Replace(qStr, "?", "$1", 1)
	   }
	  rows, err := s.conn(ctx).QueryContext(ctx, qStr, now)
	   if err != nil {
		   return nil, wrapStoreErr(err)
	   }
	   defer rows.Close()
	   return scanRuns(rows)
   }

// ──────────────────────────────────────────────
// FlowStep
// ──────────────────────────────────────────────
// SaveStep 插入或更新 FlowStep。
   func (s *Store) SaveStep(ctx context.Context, st FlowStep) error {
	   if !s.on() {
		   return ErrStore
	   }
	   qMy := `INSERT INTO aa_recon_flow_steps
(id, run_id, seq, action_def_id, action_kind, state, actor, input, output, err_msg,
 gated_by, gated_at, started_at, finished_at, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
 state=VALUES(state), actor=VALUES(actor), output=VALUES(output), err_msg=VALUES(err_msg),
 gated_by=VALUES(gated_by), gated_at=VALUES(gated_at),
 started_at=VALUES(started_at), finished_at=VALUES(finished_at)`
	  qPg := `INSERT INTO aa_recon_flow_steps
(id, run_id, seq, action_def_id, action_kind, state, actor, input, output, err_msg,
 gated_by, gated_at, started_at, finished_at, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
ON CONFLICT (id) DO UPDATE SET
 state=EXCLUDED.state, actor=EXCLUDED.actor, output=EXCLUDED.output, err_msg=EXCLUDED.err_msg,
 gated_by=EXCLUDED.gated_by, gated_at=EXCLUDED.gated_at,
 started_at=EXCLUDED.started_at, finished_at=EXCLUDED.finished_at`
	  q := qMy
	   if s.dialect() == persist.Postgres {
		   q = qPg
	   }
	  _, err := s.conn(ctx).ExecContext(ctx, q,
		  st.ID, st.RunID, st.Seq, st.ActionDefID, st.ActionKind, string(st.State),
		  st.Actor, st.Input, st.Output, st.ErrMsg,
		   st.GatedBy, timePtr(st.GatedAt), timePtr(st.StartedAt), timePtr(st.FinishedAt), st.CreatedAt,
	   )
	  return wrapStoreErr(err)
   }

// GetStep 按 ID 加载 FlowStep。
  func (s *Store) GetStep(ctx context.Context, id string) (FlowStep, error) {
	   if !s.on() {
		  return FlowStep{}, ErrStore
	   }
	  q := `SELECT id, run_id, seq, action_def_id, action_kind, state, actor, input, output, err_msg,
 gated_by, gated_at, started_at, finished_at, created_at
FROM aa_recon_flow_steps WHERE id=?`
	  if s.dialect() == persist.Postgres {
		  q = strings.ReplaceAll(q, "?", "$1")
	  }
	  var st FlowStep
	   var state string
	  var gatedAt,
		  startedAt, finishedAt sql.NullTime
	  err := s.conn(ctx).QueryRowContext(ctx, q, id).Scan(
		  &st.ID, &st.RunID, &st.Seq, &st.ActionDefID, &st.ActionKind, &state,
		  &st.Actor, &st.Input, &st.Output, &st.ErrMsg,
		   &st.GatedBy, &gatedAt, &startedAt, &finishedAt, &st.CreatedAt,
	  )
	  if errors.Is(err, sql.ErrNoRows) {
		  return FlowStep{}, ErrNotFound
	   }
	   if err != nil {
		  return FlowStep{}, wrapStoreErr(err)
	   }
	   st.State = StepState(state)
	   if gatedAt.Valid {
		   t := gatedAt.Time
		  st.GatedAt = &t
	   }
	   if startedAt.Valid {
		   t := startedAt.Time
		  st.StartedAt = &t
	  }
	   if finishedAt.Valid {
		   t := finishedAt.Time
		   st.FinishedAt = &t
	   }
	   return st, nil
   }

// ListSteps 列举某 FlowRun 的所有步骤（按 seq 升序）。
  func (s *Store) ListSteps(ctx context.Context, runID string) ([]FlowStep, error) {
	   if !s.on() {
		  return nil, nil
	  }
	  q := `SELECT id, run_id, seq, action_def_id, action_kind, state, actor, input, output, err_msg, 
 gated_by, gated_at, started_at, finished_at, created_at
FROM aa_recon_flow_steps WHERE run_id=? ORDER BY seq ASC`
	   if s.dialect() == persist.Postgres {
		   q = strings.ReplaceAll(q, "?", "$1")
	  }
	  rows, err := s.conn(ctx).QueryContext(ctx, q, runID)
	   if err != nil {
		  return nil, wrapStoreErr(err)
	  }
	   defer rows.Close()
	  return scanSteps(rows)
  }

// ListGatedSteps 列举所有等待人工放行的步骤。

   func (s *Store) ListGatedSteps(ctx context.Context, limit int) ([]FlowStep, error) {
	  if !s.on() {
		  return nil, nil
	   }
	  if limit <= 0 {
		   limit = 50
	   }
	  qStr := fmt.Sprintf(`SELECT id, run_id, seq, action_def_id, action_kind, state, actor, input, output, err_msg,
 gated_by, gated_at, started_at, finished_at, created_at
FROM aa_recon_flow_steps WHERE state='gated' ORDER BY created_at ASC LIMIT %d`, limit)
	   rows,
		  err := s.conn(ctx).QueryContext(ctx, qStr)
	  if err != nil {
		  return nil, wrapStoreErr(err)
	   }
	  defer rows.Close()
	  return scanSteps(rows)
  }

// ──────────────────────────────────────────────
// FlowDef
// ──────────────────────────────────────────────
// SaveFlowDef 插入或更新 FlowDef。
  func (s *Store) SaveFlowDef(ctx context.Context, d FlowDef) error {
	   if !s.on() {
		  return ErrStore
	  }
	  actionIDsJSON, _ := json.Marshal(d.ActionIDs)
	  slaSecs := int64(d.SLA.Seconds())
	  qMy := `INSERT INTO aa_recon_flow_defs
(id, name, diff_kind, sla_seconds, auto_classify, min_confidence, action_ids_json,
 escalate_on_sla, request_circuit, enabled, description, created_by, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
 name=VALUES(name), diff_kind=VALUES(diff_kind), sla_seconds=VALUES(sla_seconds),
 auto_classify=VALUES(auto_classify), min_confidence=VALUES(min_confidence),
 action_ids_json=VALUES(action_ids_json), escalate_on_sla=VALUES(escalate_on_sla),
 request_circuit=VALUES(request_circuit), enabled=VALUES(enabled),
 description=VALUES(description), updated_at=VALUES(updated_at)`
	   qPg := `INSERT INTO aa_recon_flow_defs 
(id, name, diff_kind, sla_seconds, auto_classify, min_confidence, action_ids_json,
 escalate_on_sla, request_circuit, enabled, description, created_by, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
ON CONFLICT (id) DO UPDATE SET
 name=EXCLUDED.name, diff_kind=EXCLUDED.diff_kind, sla_seconds=EXCLUDED.sla_seconds,
 auto_classify=EXCLUDED.auto_classify, min_confidence=EXCLUDED.min_confidence,
 action_ids_json=EXCLUDED.action_ids_json, escalate_on_sla=EXCLUDED.escalate_on_sla,
 request_circuit=EXCLUDED.request_circuit, enabled=EXCLUDED.enabled,
 description=EXCLUDED.description, updated_at=EXCLUDED.updated_at`
	  q := qMy
	  if s.dialect() == persist.Postgres {
		  q = qPg
	   }
	  _, err := s.conn(ctx).ExecContext(ctx, q,
		  d.ID, d.Name, string(d.DiffKind), slaSecs, boolInt(d.AutoClassify),
		   string(d.MinConfidence), string(actionIDsJSON),
		  boolInt(d.EscalateOnSLA), boolInt(d.RequestCircuit), boolInt(d.Enabled),
		  d.Description, d.CreatedBy, d.CreatedAt, d.UpdatedAt,
	  )
	   return wrapStoreErr(err)
  }

// GetFlowDef 按 ID 加载 FlowDef。
  func (s *Store) GetFlowDef(ctx context.Context, id string) (FlowDef, error) {
	   if !s.on() {
		   return FlowDef{}, ErrStore
	  }
	   q := `SELECT id, name, diff_kind, sla_seconds, auto_classify, min_confidence, action_ids_json,
 escalate_on_sla, request_circuit, enabled, description, created_by, created_at, updated_at
FROM aa_recon_flow_defs WHERE id=?`
	   if s.dialect() == persist.Postgres {
		   q = strings.ReplaceAll(q, "?", "$1")
	  }
	   return s.scanFlowDef(s.conn(ctx).QueryRowContext(ctx, q, id))
   }

// GetFlowDefByKind 按差异类型加载首个已启用的 FlowDef。
   func (s *Store) GetFlowDefByKind(ctx context.Context, kind DiffClassKind) (FlowDef, error) {
	   if !s.on() {
		  return FlowDef{}, ErrStore
	   }
	  q := `SELECT id, name, diff_kind, sla_seconds, auto_classify, min_confidence, action_ids_json,
 escalate_on_sla, request_circuit, enabled, description, created_by, created_at, updated_at
FROM aa_recon_flow_defs WHERE diff_kind=? AND enabled=1 ORDER BY created_at ASC LIMIT 1`
	   if s.dialect() == persist.Postgres {
		  q = strings.ReplaceAll(q, "?", "$1")
		   q = strings.Replace(q, "enabled=1", "enabled=true", 1)
	   }
	   return s.scanFlowDef(s.conn(ctx).QueryRowContext(ctx, q, string(kind)))
   }

// ListFlowDefs 列举全部流程定义。
  func (s *Store) ListFlowDefs(ctx context.Context) ([]FlowDef, error) {
	   if !s.on() {
		   return nil, nil
	  }
	  rows, err := s.conn(ctx).QueryContext(ctx,
		   `SELECT id, name, diff_kind, sla_seconds, auto_classify, min_confidence, action_ids_json,
 escalate_on_sla, request_circuit, enabled, description, created_by, created_at, updated_at
FROM aa_recon_flow_defs ORDER BY created_at DESC`)
	  if err != nil {
		  return nil, wrapStoreErr(err)
	  }
	   defer rows.Close()
	  var out []FlowDef
	   for rows.Next() {
		   d, err := s.scanFlowDef(rows)
		  if err != nil {
			   return out, err
		  }
		  out = append(out, d)
	  }
	   return out, rows.Err()
  }

// DeleteFlowDef 软删除（enabled=false）。
  func (s *Store) DeleteFlowDef(ctx context.Context, id, actor string) error {
	  if !s.on() {
		   return ErrStore
	   }
	   q := `UPDATE aa_recon_flow_defs SET enabled=0, updated_at=? WHERE id=?`
	  if s.dialect() == persist.Postgres {
		  q = `UPDATE aa_recon_flow_defs SET enabled=false, updated_at=$1 WHERE id=$2`
	  }
	   _, err := s.conn(ctx).ExecContext(ctx, q, time.Now().UTC(), id)
	   return wrapStoreErr(err)
   }

// ──────────────────────────────────────────────
// ActionDef（自定义动作持久化）
// ──────────────────────────────────────────────
// SaveActionDef 持久化自定义动作定义。
   func (s *Store) SaveActionDef(ctx context.Context, a ActionDef) error {
	  if !s.on() {
		   return ErrStore
	   }
	  paramsJSON,
		   _ := json.Marshal(a.Params)
	   qMy := `INSERT INTO aa_recon_action_defs
(id, name, kind, risk, auto_exec, require_gate, params_json, description, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
 name=VALUES(name), kind=VALUES(kind), risk=VALUES(risk),
 auto_exec=VALUES(auto_exec), require_gate=VALUES(require_gate),
 params_json=VALUES(params_json), description=VALUES(description), updated_at=VALUES(updated_at)`
	  qPg := `INSERT INTO aa_recon_action_defs
(id, name, kind, risk, auto_exec, require_gate, params_json, description, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (id) DO UPDATE SET
 name=EXCLUDED.name, kind=EXCLUDED.kind, risk=EXCLUDED.risk,
 auto_exec=EXCLUDED.auto_exec, require_gate=EXCLUDED.require_gate,
 params_json=EXCLUDED.params_json, description=EXCLUDED.description, updated_at=EXCLUDED.updated_at`
	  q := qMy
	   if s.dialect() == persist.Postgres {
		   q = qPg
	   }
	  _, err := s.conn(ctx).ExecContext(ctx, q,
		  a.ID, a.Name, string(a.Kind), string(a.Risk),
		   boolInt(a.AutoExec), boolInt(a.RequireGate),
		   string(paramsJSON), a.Description, a.CreatedAt, a.UpdatedAt,
	  )
	  return wrapStoreErr(err)
   }

// ListActionDefs 列举所有自定义动作定义。
   func (s *Store) ListActionDefs(ctx context.Context) ([]ActionDef, error) {
	  if !s.on() {
		  return nil, nil
	  }
	  rows, err := s.conn(ctx).QueryContext(ctx,
		  `SELECT id, name, kind, risk, auto_exec, require_gate, params_json, description, created_at, updated_at
FROM aa_recon_action_defs ORDER BY created_at DESC`)
	  if err != nil {
		   return nil, wrapStoreErr(err)
	   }
	   defer rows.Close()
	  var out []ActionDef
	   for rows.Next() {
		   var a ActionDef
		  var kind, risk string
		   var autoExec, requireGate int
		   var paramsJSON string
		  if err := rows.Scan(&a.ID, &a.Name, &kind, &risk, &autoExec, &requireGate,
			  &paramsJSON, &a.Description, &a.CreatedAt, &a.UpdatedAt); err != nil {
			  return out, wrapStoreErr(err)
		  }
		   a.Kind = ActionKind(kind)
		   a.Risk = RiskLevel(risk)
		   a.AutoExec = autoExec != 0
		  a.RequireGate = requireGate != 0
		  _ = json.Unmarshal([]byte(paramsJSON), &a.Params)
		  out = append(out, a)
	   }
	   return out, rows.Err()
   }

// ──────────────────────────────────────────────
// DiffClass 持久化
// ──────────────────────────────────────────────
// SaveDiffClass 持久化分类结果（按 feature_hash 幂等）。
  func (s *Store) SaveDiffClass(ctx context.Context, dc DiffClass) error {
	   if !s.on() {
		  return ErrStore
	  }
	   rulesJSON, _ := json.Marshal(dc.MatchedRules)
	   actsJSON, _ := json.Marshal(dc.SuggestedActs)
	   qMy := `INSERT INTO aa_recon_diff_classes
(id, feature_hash, kind, confidence, score, matched_rules_json, suggested_acts_json, note, classified_at)
VALUES (?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
 kind=VALUES(kind), confidence=VALUES(confidence), score=VALUES(score),
 matched_rules_json=VALUES(matched_rules_json), suggested_acts_json=VALUES(suggested_acts_json),
 note=VALUES(note), classified_at=VALUES(classified_at)`
	   qPg := `INSERT INTO aa_recon_diff_classes 
(id, feature_hash, kind, confidence, score, matched_rules_json, suggested_acts_json, note, classified_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (feature_hash) DO UPDATE SET
 kind=EXCLUDED.kind, confidence=EXCLUDED.confidence, score=EXCLUDED.score,
 matched_rules_json=EXCLUDED.matched_rules_json, suggested_acts_json=EXCLUDED.suggested_acts_json,
 note=EXCLUDED.note, classified_at=EXCLUDED.classified_at`
	   q := qMy
	   if s.dialect() == persist.Postgres {
		   q = qPg
	   }
	   _,
		   err := s.conn(ctx).ExecContext(ctx, q,
		  dc.ID, dc.FeatureHash, string(dc.Kind), string(dc.Confidence), dc.Score,
		   string(rulesJSON), string(actsJSON), dc.Note, dc.ClassifiedAt,
	  )
	  return wrapStoreErr(err)
   }

// ──────────────────────────────────────────────
// 内部扫描工具
// ──────────────────────────────────────────────

type rowScanner interface {
	Scan(dest ...any) error
}

  func (s *Store) scanFlowDef(row rowScanner) (FlowDef, error) {
	   var d FlowDef
	  var diffKind, minConf string
	  var slaSecs int64
	  var autoClassify,
		  escalateOnSLA, requestCircuit, enabled int
	   var actionIDsJSON string
	   err := row.Scan(
		  &d.ID, &d.Name, &diffKind, &slaSecs, &autoClassify, &minConf, &actionIDsJSON,
		  &escalateOnSLA, &requestCircuit, &enabled, &d.Description, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt,
	  )
	  if errors.Is(err, sql.ErrNoRows) {
		   return FlowDef{}, ErrNotFound
	  }
	  if err != nil {
		  return FlowDef{}, wrapStoreErr(err)
	   }
	   d.DiffKind = DiffClassKind(diffKind)
	  d.SLA = time.Duration(slaSecs) * time.Second
	   d.MinConfidence = ConfidenceLevel(minConf)
	   d.AutoClassify = autoClassify != 0
	  d.EscalateOnSLA = escalateOnSLA != 0
	   d.RequestCircuit = requestCircuit != 0
	  d.Enabled = enabled != 0
	  _ = json.Unmarshal([]byte(actionIDsJSON), &d.ActionIDs)
	  return d, nil
  }

   func scanRuns(rows *sql.Rows) ([]FlowRun, error) {
	   var out []FlowRun
	   for rows.Next() {
		   var r FlowRun
		  var state, conf string
		   var gateAt sql.NullTime
		   if err := rows.Scan(
			  &r.ID,
			  &r.FlowDefID, &r.TicketID, &r.UserID, &r.Asset, &r.Chain, &r.DiffKind,
			  &state, &r.ClassID, &conf, &r.CurrentStep,
			   &r.GateActor, &gateAt, &r.ClosedBy, &r.EscalatedBy, &r.Note,
			  &r.FenceEpoch, &r.FenceToken, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt, &r.SLADeadline,
		   ); err != nil {
			   return out, fmt.Errorf("%w: %v", ErrStore, err)
		   }
		  r.State = RunState(state)
		  r.Confidence = ConfidenceLevel(conf)
		   if gateAt.Valid {
			  t := gateAt.Time
			   r.GateAt = &t
		  }
		   out = append(out, r)
	  }
	  return out, rows.Err()
  }

   func scanSteps(rows *sql.Rows) ([]FlowStep, error) {
	  var out []FlowStep
	   for rows.Next() {
		  var st FlowStep
		  var state string
		   var gatedAt, startedAt, finishedAt sql.NullTime
		  if err := rows.Scan(
			  &st.ID, &st.RunID, &st.Seq, &st.ActionDefID, &st.ActionKind, &state,
			  &st.Actor,
			   &st.Input, &st.Output, &st.ErrMsg,
			   &st.GatedBy, &gatedAt, &startedAt, &finishedAt, &st.CreatedAt,
		  ); err != nil {
			  return out, fmt.Errorf("%w: %v", ErrStore, err)
		   }
		   st.State = StepState(state)
		  if gatedAt.Valid {
			   t := gatedAt.Time
			   st.GatedAt = &t
		  }
		  if startedAt.Valid {
			   t := startedAt.Time
			  st.StartedAt = &t
		  }
		   if finishedAt.Valid {
			   t := finishedAt.Time
			  st.FinishedAt = &t
		  }
		   out = append(out, st)
	   }
	  return out, rows.Err()
  }

// ──────────────────────────────────────────────
// 工具函数
// ──────────────────────────────────────────────

   func wrapStoreErr(err error) error {
	  if err == nil {
		  return nil
	  }
	  return fmt.Errorf("%w: %v", ErrStore, err)
  }

  func timePtr(t *time.Time) any {
	  if t == nil {
		  return nil
	   }
	  return *t
   }

  func boolInt(b bool) int {
	  if b {
		   return 1
	   }
	  return 0
   }

// buildRunWhere 根据查询条件构建 WHERE 子句。

   func buildRunWhere(q RunQuery, _ persist.Dialect) (string, []any) {
	   var clauses []string
	   var args []any
	  if q.UserID != "" {
		  clauses = append(clauses, "user_id=?")
		  args = append(args, q.UserID)
	  }
	  if q.Asset != "" {
		   clauses = append(clauses, "asset=?")
		   args = append(args, q.Asset)
	   }
	   if q.DiffKind != "" {
		  clauses = append(clauses, "diff_kind=?")
		   args = append(args, q.DiffKind)
	  }
	  if q.State != "" {
		   clauses = append(clauses, "state=?")
		   args = append(args, string(q.State))
	  }
	  if q.TicketID != "" {
		  clauses = append(clauses, "ticket_id=?")
		   args = append(args, q.TicketID)
	  }
	  if len(clauses) == 0 {
		   return "", args
	   }
	  return "WHERE " + strings.Join(clauses, " AND "), args
   }
