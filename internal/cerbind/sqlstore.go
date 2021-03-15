// 冷签绑定的 SQL 存储。占位符走 persist.Conn，Postgres 由方言层改写。
// 做什么：把策略、会话、绑定、回执、审计、摘要和别名落到 aa_cer_*。
// 表由迁移建立，这里不建表；不在存储层推进仪式状态。
// 不变量：同一别名不能改指向另一批次；读不到行是未找到，不是空会话。

// Author: Blaise
// Date: 2021.03.10

package cerbind

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// SQL 挂 aa_cer_*。

type SQL struct {
	db *persist.SQLDB
}

// NewSQL 绑定已打开的库。库为空时后续调用失败即关。
   func NewSQL(db *persist.SQLDB) *SQL { return &SQL{db: db} }

  func (s *SQL) conn(ctx context.Context) (persist.Queryer, error) {
	   if s == nil || s.db == nil || s.db.DB == nil {
		   return nil, ErrUnavailable
	  }
	   if ctx == nil {
		  ctx = context.Background()
	  }
	  return s.db.Conn(ctx), nil
  }

  func (s *SQL) PutPolicy(ctx context.Context, p Policy) error {
	  q, err := s.conn(ctx)
	  if err != nil {
		   return err
	  }
	   res, err := q.ExecContext(ctx, `UPDATE aa_cer_policy SET name=?, asset=?, chain=?, purpose=?, min_amount=?, max_amount=?, dest_risk=?, side=?, force_cold=?, threshold=?, shares=?, bind_mode=?, priority=?, enabled=?, approval_need=?, max_age_sec=?, limit_ref=?, updated_by=?, updated_at=? WHERE id=?`,
		   p.Name,
		  p.Asset, p.Chain, p.Purpose, p.MinAmount, p.MaxAmount, p.DestRisk, p.Side,
		   boolInt(p.ForceCold), p.Threshold, p.Shares, p.BindMode, p.Priority, boolInt(p.Enabled),
		   boolInt(p.ApprovalNeed), p.MaxAgeSec, p.LimitRef, p.UpdatedBy, p.UpdatedAt.UTC(), p.ID)
	  if err != nil {
		   return err
	  }
	   n, _ := res.RowsAffected()
	  if n > 0 {
		   return nil
	  }
	   _, err = q.ExecContext(ctx, `INSERT INTO aa_cer_policy (id, name, asset, chain, purpose, min_amount, max_amount, dest_risk, side, force_cold, threshold, shares, bind_mode, priority, enabled, approval_need, max_age_sec, limit_ref, updated_by, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		  p.ID, p.Name, p.Asset, p.Chain, p.Purpose, p.MinAmount, p.MaxAmount, p.DestRisk, p.Side,
		   boolInt(p.ForceCold), p.Threshold, p.Shares, p.BindMode, p.Priority, boolInt(p.Enabled),
		   boolInt(p.ApprovalNeed), p.MaxAgeSec, p.LimitRef, p.UpdatedBy, p.CreatedAt.UTC(), p.UpdatedAt.UTC())
	  if isDup(err) {
		   return s.PutPolicy(ctx, p)
	   }
	   return err
  }

  func (s *SQL) Policies(ctx context.Context) ([]Policy, error) {
	  q, err := s.conn(ctx)
	  if err != nil {
		   return nil, err
	  }
	  rows, err := q.QueryContext(ctx, `SELECT id, name, asset, chain, purpose, min_amount, max_amount, dest_risk, side, force_cold, threshold, shares, bind_mode, priority, enabled, approval_need, max_age_sec, limit_ref, updated_by, created_at, updated_at FROM aa_cer_policy ORDER BY priority DESC, id`)
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   out := make([]Policy, 0)
	   for rows.Next() {
		   p, err := scanPolicy(rows)
		  if err != nil {
			   return nil, err
		   }
		   out = append(out, p)
	   }
	  return out, rows.Err()
  }

  func scanPolicy(sc rower) (Policy, error) {
	  var p Policy
	  var force, enabled, appr int
	  var created, updated time.Time
	   err := sc.Scan(&p.ID, &p.Name, &p.Asset, &p.Chain, &p.Purpose, &p.MinAmount, &p.MaxAmount, &p.DestRisk, &p.Side,
		  &force, &p.Threshold, &p.Shares, &p.BindMode, &p.Priority, &enabled, &appr, &p.MaxAgeSec, &p.LimitRef, &p.UpdatedBy, &created, &updated)
	  if err != nil {
		  return Policy{}, err
	  }
	   p.ForceCold, p.Enabled, p.ApprovalNeed = force == 1, enabled == 1, appr == 1
	  p.CreatedAt, p.UpdatedAt = created.UTC(), updated.UTC()
	  return p, nil
  }

  func (s *SQL) PutSession(ctx context.Context, ses Session) error {
	  q, err := s.conn(ctx)
	   if err != nil {
		   return err
	  }
	   res, err := q.ExecContext(ctx, `UPDATE aa_cer_session SET policy_id=?, state=?, asset=?, chain=?, amount=?, dest=?, dest_risk=?, side=?, purpose=?, batch_id=?, threshold=?, shares=?, bind_mode=?, receipt_n=?, approved=?, airgap_id=?, keycer_ticket=?, custody_id=?, digest=?, starter=?, last_actor=?, deadline=?, abort_reason=?, note=?, fence_epoch=?, fence_token=?, alerted_at=?, updated_at=? WHERE id=?`,
		  ses.PolicyID, string(ses.State), ses.Asset, ses.Chain, ses.Amount, ses.Dest, ses.DestRisk, ses.Side, ses.Purpose, ses.BatchID,
		  ses.Threshold, ses.Shares, ses.BindMode, ses.ReceiptN, boolInt(ses.Approved), ses.AirgapID, ses.KeycerTicket, ses.CustodyID, ses.Digest,
		   ses.Starter, ses.LastActor, ses.Deadline.UTC(), ses.AbortReason, ses.Note, ses.FenceEpoch, ses.FenceToken, nullTime(ses.AlertedAt), ses.UpdatedAt.UTC(), ses.ID)
	   if err != nil {
		  return err
	  }
	  n, _ := res.RowsAffected()
	  if n > 0 {
		  return nil
	  }
	   _, err = q.ExecContext(ctx, `INSERT INTO aa_cer_session (id, policy_id, state, asset, chain, amount, dest, dest_risk, side, purpose, batch_id, threshold, shares, bind_mode, receipt_n, approved, airgap_id, keycer_ticket, custody_id, digest, starter, last_actor, deadline, abort_reason, note, fence_epoch, fence_token, alerted_at, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		  ses.ID, ses.PolicyID, string(ses.State), ses.Asset, ses.Chain, ses.Amount, ses.Dest, ses.DestRisk, ses.Side, ses.Purpose, ses.BatchID,
		  ses.Threshold, ses.Shares, ses.BindMode, ses.ReceiptN, boolInt(ses.Approved), ses.AirgapID, ses.KeycerTicket, ses.CustodyID, ses.Digest,
		  ses.Starter,
		  ses.LastActor, ses.Deadline.UTC(), ses.AbortReason, ses.Note, ses.FenceEpoch, ses.FenceToken, nullTime(ses.AlertedAt), ses.CreatedAt.UTC(), ses.UpdatedAt.UTC())
	  if isDup(err) {
		   return s.PutSession(ctx, ses)
	   }
	   return err
   }

  func (s *SQL) Session(ctx context.Context, id string) (Session, bool, error) {
	  q, err := s.conn(ctx)
	  if err != nil {
		  return Session{}, false, err
	  }
	   row := q.QueryRowContext(ctx, sessionSelect+` WHERE id=?`, id)
	  ses, err := scanSession(row)
	  if err == sql.ErrNoRows {
		  return Session{}, false, nil
	  }
	  if err != nil {
		  return Session{}, false, err
	  }
	   return ses, true, nil
   }

   func (s *SQL) LiveSessions(ctx context.Context) ([]Session, error) {
	   return s.querySessions(ctx, sessionSelect+` WHERE state NOT IN ('closed','aborted') ORDER BY id`)
   }

   func (s *SQL) ListSessions(ctx context.Context, limit int) ([]Session, error) {
	   return s.querySessions(ctx, sessionSelect+` ORDER BY updated_at DESC LIMIT ?`, clampLimit(limit))
  }

  func (s *SQL) querySessions(ctx context.Context, query string, args ...any) ([]Session, error) {
	  q, err := s.conn(ctx)
	  if err != nil {
		   return nil, err
	  }
	  rows, err := q.QueryContext(ctx, query, args...)
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  out := make([]Session, 0)
	  for rows.Next() {
		  ses, err := scanSession(rows)
		   if err != nil {
			  return nil, err
		   }
		  out = append(out, ses)
	   }
	  return out, rows.Err()
  }

const sessionSelect = `SELECT id, policy_id, state, asset, chain, amount, dest, dest_risk, side, purpose, batch_id, threshold, shares, bind_mode, receipt_n, approved, airgap_id, keycer_ticket, custody_id, digest, starter, last_actor, deadline, abort_reason, note, fence_epoch, fence_token, alerted_at, created_at, updated_at FROM aa_cer_session`

   func scanSession(sc rower) (Session, error) {
	   var ses Session
	   var state string
	   var approved int
	  var deadline,
		   created, updated time.Time
	  var alerted sql.NullTime
	   err := sc.Scan(&ses.ID, &ses.PolicyID, &state, &ses.Asset, &ses.Chain, &ses.Amount, &ses.Dest, &ses.DestRisk, &ses.Side, &ses.Purpose, &ses.BatchID,
		   &ses.Threshold, &ses.Shares, &ses.BindMode, &ses.ReceiptN, &approved, &ses.AirgapID, &ses.KeycerTicket, &ses.CustodyID, &ses.Digest,
		  &ses.Starter, &ses.LastActor, &deadline, &ses.AbortReason, &ses.Note, &ses.FenceEpoch, &ses.FenceToken, &alerted, &created, &updated)
	  if err != nil {
		  return Session{}, err
	   }
	   ses.State = State(state)
	   ses.Approved = approved == 1
	  ses.Deadline, ses.CreatedAt, ses.UpdatedAt = deadline.UTC(), created.UTC(), updated.UTC()
	  if alerted.Valid {
		   ses.AlertedAt = alerted.Time.UTC()
	   }
	   return ses, nil
  }

   func (s *SQL) PutBinding(ctx context.Context, b Binding) error {
	  q, err := s.conn(ctx)
	   if err != nil {
		   return err
	   }
	   res, err := q.ExecContext(ctx, `UPDATE aa_cer_binding SET batch_id=?, session_id=?, digest=?, state=?, bind_mode=?, actor=?, bound_at=?, released_at=? WHERE id=?`,
		   b.BatchID, b.SessionID, b.Digest, b.State, b.BindMode, b.Actor, b.BoundAt.UTC(), nullTime(b.ReleasedAt), b.ID)
	  if err != nil {
		  return err
	  }
	  n, _ := res.RowsAffected()
	  if n > 0 {
		   return nil
	   }
	  _, err = q.ExecContext(ctx, `INSERT INTO aa_cer_binding (id, batch_id, session_id, digest, state, bind_mode, actor, bound_at, released_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		  b.ID, b.BatchID, b.SessionID, b.Digest, b.State, b.BindMode, b.Actor, b.BoundAt.UTC(), nullTime(b.ReleasedAt))
	  if isDup(err) {
		  return s.PutBinding(ctx, b)
	   }
	   return err
   }

   func (s *SQL) BindingsBySession(ctx context.Context, sessionID string) ([]Binding, error) {
	   return s.queryBinds(ctx, bindSelect+` WHERE session_id=? ORDER BY bound_at, id`, sessionID)
   }

  func (s *SQL) ActiveBindings(ctx context.Context) ([]Binding, error) {
	  return s.queryBinds(ctx, bindSelect+` WHERE state=? ORDER BY bound_at, id`, BindActive)
   }

   func (s *SQL) ActiveByBatch(ctx context.Context, batchID string) (Binding, bool, error) {
	   rows, err := s.queryBinds(ctx, bindSelect+` WHERE batch_id=? AND state=? ORDER BY bound_at, id`, batchID, BindActive)
	   if err != nil {
		  return Binding{}, false, err
	  }
	   if len(rows) == 0 {
		  return Binding{}, false, nil
	  }
	   if len(rows) > 1 {
		   return Binding{}, false, ErrBindMode
	  }
	  return rows[0], true, nil
   }

   func (s *SQL) queryBinds(ctx context.Context, query string, args ...any) ([]Binding, error) {
	   q, err := s.conn(ctx)
	   if err != nil {
		  return nil, err
	   }
	   rows, err := q.QueryContext(ctx, query, args...)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  out := make([]Binding, 0)
	   for rows.Next() {
		  b, err := scanBinding(rows)
		   if err != nil {
			  return nil, err
		   }
		  out = append(out, b)
	  }
	  return out, rows.Err()
   }

const bindSelect = `SELECT id, batch_id, session_id, digest, state, bind_mode, actor, bound_at, released_at FROM aa_cer_binding`

   func scanBinding(sc rower) (Binding, error) {
	  var b Binding
	   var bound time.Time
	   var released sql.NullTime
	   err := sc.Scan(&b.ID, &b.BatchID, &b.SessionID, &b.Digest, &b.State, &b.BindMode, &b.Actor, &bound, &released)
	   if err != nil {
		   return Binding{}, err
	   }
	   b.BoundAt = bound.UTC()
	  if released.Valid {
		  b.ReleasedAt = released.Time.UTC()
	   }
	   return b, nil
   }

   func (s *SQL) PutReceipt(ctx context.Context, r Receipt) error {
	   q, err := s.conn(ctx)
	   if err != nil {
		   return err
	  }
	   _, err = q.ExecContext(ctx, `INSERT INTO aa_cer_receipt (id, session_id, actor, share_ref, slot_label, note, at) VALUES (?,?,?,?,?,?,?)`,
		  r.ID, r.SessionID, r.Actor, r.ShareRef, r.SlotLabel, r.Note, r.At.UTC())
	   if isDup(err) {
		  return nil
	   }
	   return err
  }

  func (s *SQL) Receipts(ctx context.Context, sessionID string) ([]Receipt, error) {
	  q, err := s.conn(ctx)
	   if err != nil {
		   return nil, err
	   }
	  rows, err := q.QueryContext(ctx, `SELECT id, session_id, actor, share_ref, slot_label, note, at FROM aa_cer_receipt WHERE session_id=? ORDER BY at, actor`, sessionID)
	  if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	  out := make([]Receipt, 0)
	   for rows.Next() {
		  var r Receipt
		  var at time.Time
		   if err := rows.Scan(&r.ID, &r.SessionID, &r.Actor, &r.ShareRef, &r.SlotLabel, &r.Note, &at); err != nil {
			   return nil, err
		  }
		  r.At = at.UTC()
		  out = append(out, r)
	   }
	   return out, rows.Err()
   }

  func (s *SQL) PutAudit(ctx context.Context, a Audit) error {
	  q, err := s.conn(ctx)
	   if err != nil {
		  return err
	   }
	  _, err = q.ExecContext(ctx, `INSERT INTO aa_cer_audit (id, session_id, batch_id, kind, actor, detail, at) VALUES (?,?,?,?,?,?,?)`,
		  a.ID, a.SessionID, a.BatchID, a.Kind, a.Actor, clip(a.Detail, 255), a.At.UTC())
	   return err
   }

  func (s *SQL) Audits(ctx context.Context, sessionID, batchID string, limit int) ([]Audit, error) {
	   q, err := s.conn(ctx)
	   if err != nil {
		  return nil, err
	  }
	  query := `SELECT id, session_id, batch_id, kind, actor, detail, at FROM aa_cer_audit`
	  var args []any
	  if sessionID != "" {
		  query += ` WHERE session_id=?`
		  args = append(args, sessionID)
	   } else if batchID != "" {
		  query += ` WHERE batch_id=?`
		  args = append(args, batchID)
	   }
	  if sessionID != "" && batchID != "" {
		  query += ` AND batch_id=?`
		   args = append(args, batchID)
	   }
	  query += ` ORDER BY at DESC LIMIT ?`
	   args = append(args, clampLimit(limit))
	  rows, err := q.QueryContext(ctx, query, args...)
	   if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  out := make([]Audit, 0)
	  for rows.Next() {
		  var a Audit
		  var at time.Time
		   if err := rows.Scan(&a.ID, &a.SessionID, &a.BatchID, &a.Kind, &a.Actor, &a.Detail, &at); err != nil {
			  return nil, err
		  }
		  a.At = at.UTC()
		   out = append(out, a)
	  }
	   return out, rows.Err()
  }

  func (s *SQL) PutSnap(ctx context.Context, snap Snapshot) error {
	  body, err := json.Marshal(snap.Lines)
	   if err != nil {
		  return err
	  }
	  q, err := s.conn(ctx)
	   if err != nil {
		  return err
	   }
	   res, err := q.ExecContext(ctx, `UPDATE aa_cer_snap SET body=?, digest=?, at=? WHERE batch_id=?`, string(body), snap.Digest, snap.At.UTC(), snap.BatchID)
	  if err != nil {
		  return err
	  }
	  n, _ := res.RowsAffected()
	   if n > 0 {
		   return nil
	  }
	  _, err = q.ExecContext(ctx, `INSERT INTO aa_cer_snap (batch_id, body, digest, at) VALUES (?,?,?,?)`, snap.BatchID, string(body), snap.Digest, snap.At.UTC())
	   if isDup(err) {
		   return s.PutSnap(ctx, snap)
	  }
	  return err
  }

   func (s *SQL) Snap(ctx context.Context, batchID string) (Snapshot, bool, error) {
	  q, err := s.conn(ctx)
	  if err != nil {
		   return Snapshot{}, false, err
	   }
	  var body, digest string
	   var at time.Time
	  err = q.QueryRowContext(ctx, `SELECT body, digest, at FROM aa_cer_snap WHERE batch_id=?`, batchID).Scan(&body, &digest, &at)
	  if err == sql.ErrNoRows {
		  return Snapshot{}, false, nil
	   }
	   if err != nil {
		   return Snapshot{}, false, err
	  }
	  var lines []Line
	   if err := json.Unmarshal([]byte(body), &lines); err != nil {
		  return Snapshot{}, false, err
	   }
	  return Snapshot{BatchID: batchID, Lines: lines, Digest: digest, At: at.UTC()}, true, nil
  }

   func (s *SQL) PutAlias(ctx context.Context, ref, batchID, kind string) error {
	   ref = strings.TrimSpace(ref)
	  if ref == "" || batchID == "" {
		  return ErrAlias
	  }
	   q, err := s.conn(ctx)
	   if err != nil {
		   return err
	  }
	  var cur string
	  err = q.QueryRowContext(ctx, `SELECT batch_id FROM aa_cer_alias WHERE ref_id=?`, ref).Scan(&cur)
	   if err == nil {
		   if cur != batchID {
			  return ErrAlias
		   }
		   return nil
	   }
	  if err != sql.ErrNoRows {
		  return err
	  }
	  _, err = q.ExecContext(ctx, `INSERT INTO aa_cer_alias (ref_id, batch_id, kind) VALUES (?,?,?)`, ref, batchID, kind)
	  if isDup(err) {
		  return s.PutAlias(ctx, ref, batchID, kind)
	   }
	  return err
  }

  func (s *SQL) Alias(ctx context.Context, ref string) (string, bool, error) {
	  q, err := s.conn(ctx)
	   if err != nil {
		   return "", false, err
	   }
	  var batchID string
	   err = q.QueryRowContext(ctx, `SELECT batch_id FROM aa_cer_alias WHERE ref_id=?`, strings.TrimSpace(ref)).Scan(&batchID)
	   if err == sql.ErrNoRows {
		   return "", false, nil
	   }
	   if err != nil {
		  return "", false, err
	  }
	   return batchID, true, nil
  }

type rower interface {
	Scan(dest ...any) error
}

  func boolInt(v bool) int {
	  if v {
		   return 1
	   }
	  return 0
   }

  func nullTime(t time.Time) any {
	   if t.IsZero() {
		   return nil
	   }
	   return t.UTC()
  }

  func isDup(err error) bool {
	  if err == nil {
		   return false
	  }
	   msg := strings.ToLower(err.Error())
	  return strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique")
   }

var _ Store = (*SQL)(nil)
