package auditchain

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// SQL 挂 aa_audit_* 。表由迁移 0056 建立，这里不建表。

type SQL struct {
	db *persist.SQLDB
}

  func NewSQL(db *persist.SQLDB) *SQL {
	   return &SQL{db: db}
   }

   func (s *SQL) conn(ctx context.Context) (persist.Queryer, error) {
	   if s == nil || s.db == nil || s.db.DB == nil {
		  return nil, ErrUnavailable
	   }
	  return s.db.Conn(ctx), nil
  }

   func nullTime(t time.Time) any {
	  if t.IsZero() {
		   return nil
	  }
	  return t.UTC()
   }

   func scanTime(n sql.NullTime) time.Time {
	   if !n.Valid {
		  return time.Time{}
	  }
	  return n.Time.UTC()
   }

  func (s *SQL) FindIdem(ctx context.Context, source, key string) (Row, bool, error) {
	  q, err := s.conn(ctx)
	   if err != nil {
		   return Row{}, false, err
	   }
	   row := q.QueryRowContext(ctx, `SELECT id, segment_id, seq, idem_key, source, kind, domain, tenant, user_id, asset, amount, ref_id, actor, body, prev_hash, self_hash, occurred_at, ingested_at FROM aa_audit_entry WHERE source=? AND idem_key=?`, source, key)
	   r, err := scanRow(row)
	   if err == sql.ErrNoRows {
		  return Row{}, false, nil
	  }
	   if err != nil {
		  return Row{}, false, err
	  }
	  return r, true, nil
   }

type rower interface {
	Scan(dest ...any) error
}

   func scanRow(sc rower) (Row, error) {
	   var r Row
	   var occurred, ingested time.Time
	  err := sc.Scan(&r.ID, &r.SegmentID, &r.Seq, &r.IdemKey, &r.Source, &r.Kind, &r.Domain, &r.Tenant, &r.UserID, &r.Asset, &r.Amount, &r.RefID, &r.Actor, &r.Body, &r.PrevHash, &r.SelfHash, &occurred, &ingested)
	   if err != nil {
		   return Row{}, err
	  }
	   r.OccurredAt = occurred.UTC()
	   r.IngestedAt = ingested.UTC()
	   return r, nil
   }

  func (s *SQL) Segments(ctx context.Context, partKey string) ([]Segment, error) {
	  q, err := s.conn(ctx)
	  if err != nil {
		   return nil, err
	   }
	  rows, err := q.QueryContext(ctx, `SELECT id, part_key, domain, tenant, day_key, ordinal, state, tier, entry_count, head_hash, tail_hash, merkle_root, hold_reason, opened_at, sealed_at, frozen_at FROM aa_audit_segment WHERE part_key=? ORDER BY ordinal`, partKey)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  return collectSegs(rows)
  }

  func (s *SQL) AllSegments(ctx context.Context) ([]Segment, error) {
	  q, err := s.conn(ctx)
	   if err != nil {
		  return nil, err
	  }
	  rows, err := q.QueryContext(ctx, `SELECT id, part_key, domain, tenant, day_key, ordinal, state, tier, entry_count, head_hash, tail_hash, merkle_root, hold_reason, opened_at, sealed_at, frozen_at FROM aa_audit_segment ORDER BY part_key, ordinal`)
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   return collectSegs(rows)
  }

   func collectSegs(rows *sql.Rows) ([]Segment, error) {
	   var out []Segment
	  for rows.Next() {
		  seg, err := scanSeg(rows)
		   if err != nil {
			   return nil, err
		   }
		   out = append(out, seg)
	  }
	   return out, rows.Err()
   }

   func scanSeg(sc rower) (Segment, error) {
	   var seg Segment
	   var opened time.Time
	   var sealed, frozen sql.NullTime
	   err := sc.Scan(&seg.ID, &seg.PartKey, &seg.Domain, &seg.Tenant, &seg.DayKey, &seg.Ordinal, &seg.State, &seg.Tier, &seg.Count, &seg.HeadHash, &seg.TailHash, &seg.MerkleRoot, &seg.HoldReason, &opened, &sealed, &frozen)
	  if err != nil {
		   return Segment{}, err
	  }
	  seg.OpenedAt = opened.UTC()
	  seg.SealedAt = scanTime(sealed)
	   seg.FrozenAt = scanTime(frozen)
	   return seg, nil
  }

   func (s *SQL) Segment(ctx context.Context, id string) (Segment, bool, error) {
	  q, err := s.conn(ctx)
	  if err != nil {
		   return Segment{}, false, err
	  }
	   row := q.QueryRowContext(ctx, `SELECT id, part_key, domain, tenant, day_key, ordinal, state, tier, entry_count, head_hash, tail_hash, merkle_root, hold_reason, opened_at, sealed_at, frozen_at FROM aa_audit_segment WHERE id=?`, id)
	   seg, err := scanSeg(row)
	   if err == sql.ErrNoRows {
		  return Segment{}, false, nil
	   }
	   if err != nil {
		   return Segment{}, false, err
	   }
	   return seg, true, nil
  }

  func (s *SQL) CreateSegment(ctx context.Context, seg Segment) error {
	   q, err := s.conn(ctx)
	   if err != nil {
		  return err
	  }
	   _, err = q.ExecContext(ctx, `INSERT INTO aa_audit_segment (id, part_key, domain, tenant, day_key, ordinal, state, tier, entry_count, head_hash, tail_hash, merkle_root, hold_reason, opened_at, sealed_at, frozen_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		  seg.ID, seg.PartKey, seg.Domain, seg.Tenant, seg.DayKey, seg.Ordinal, seg.State, seg.Tier, seg.Count, seg.HeadHash, seg.TailHash, seg.MerkleRoot, seg.HoldReason, seg.OpenedAt.UTC(), nullTime(seg.SealedAt), nullTime(seg.FrozenAt))
	  return err
  }

  func (s *SQL) SaveSegment(ctx context.Context, seg Segment) error {
	   cur,
		   ok, err := s.Segment(ctx, seg.ID)
	   if err != nil {
		   return err
	   }
	   if !ok {
		  return ErrSegment
	   }
	   if err := Transit(cur.State, seg.State); err != nil {
		  return err
	  }
	   q, err := s.conn(ctx)
	  if err != nil {
		  return err
	  }
	   _, err = q.ExecContext(ctx, `UPDATE aa_audit_segment SET state=?, tier=?, entry_count=?, head_hash=?, tail_hash=?, merkle_root=?, hold_reason=?, sealed_at=?, frozen_at=? WHERE id=?`,
		  seg.State, seg.Tier, seg.Count, seg.HeadHash, seg.TailHash, seg.MerkleRoot, seg.HoldReason, nullTime(seg.SealedAt), nullTime(seg.FrozenAt), seg.ID)
	  return err
  }

  func (s *SQL) InsertRow(ctx context.Context, r Row) error {
	   seg, ok, err := s.Segment(ctx, r.SegmentID)
	   if err != nil {
		  return err
	   }
	   if !ok {
		   return ErrSegment
	  }
	  if seg.State != SegOpen {
		   return ErrSealed
	  }
	  q, err := s.conn(ctx)
	  if err != nil {
		  return err
	   }
	  _, err = q.ExecContext(ctx, `INSERT INTO aa_audit_entry (id, segment_id, seq, idem_key, source, kind, domain, tenant, user_id, asset, amount, ref_id, actor, body, prev_hash, self_hash, occurred_at, ingested_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		   r.ID,
		  r.SegmentID, r.Seq, r.IdemKey, r.Source, r.Kind, r.Domain, r.Tenant, r.UserID, r.Asset, r.Amount, r.RefID, r.Actor, r.Body, r.PrevHash, r.SelfHash, r.OccurredAt.UTC(), r.IngestedAt.UTC())
	  return err
   }

   func (s *SQL) Rows(ctx context.Context, segmentID string) ([]Row, error) {
	   q, err := s.conn(ctx)
	  if err != nil {
		   return nil, err
	  }
	  rows, err := q.QueryContext(ctx, `SELECT id, segment_id, seq, idem_key, source, kind, domain, tenant, user_id, asset, amount, ref_id, actor, body, prev_hash, self_hash, occurred_at, ingested_at FROM aa_audit_entry WHERE segment_id=? ORDER BY seq`, segmentID)
	   if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	   return collectRows(rows)
  }

   func collectRows(rows *sql.Rows) ([]Row, error) {
	   var out []Row
	  for rows.Next() {
		   r, err := scanRow(rows)
		  if err != nil {
			   return nil, err
		   }
		   out = append(out, r)
	  }
	  return out, rows.Err()
   }

   func (s *SQL) UserRows(ctx context.Context, user, asset string, from, to time.Time) ([]Row, error) {
	   q, err := s.conn(ctx)
	   if err != nil {
		  return nil, err
	   }
	  qtext := `SELECT id, segment_id, seq, idem_key, source, kind, domain, tenant, user_id, asset, amount, ref_id, actor, body, prev_hash, self_hash, occurred_at, ingested_at FROM aa_audit_entry WHERE 1=1`
	   var args []any
	   if strings.TrimSpace(user) != "" {
		   qtext += ` AND user_id=?`
		  args = append(args, strings.TrimSpace(user))
	   }
	  if strings.TrimSpace(asset) != "" {
		  qtext += ` AND asset=?`
		  args = append(args, strings.TrimSpace(asset))
	   }
	   if !from.IsZero() {
		   qtext += ` AND occurred_at>=?`
		  args = append(args, from.UTC())
	   }
	  if !to.IsZero() {
		   qtext += ` AND occurred_at<=?`
		   args = append(args, to.UTC())
	   }
	   qtext += ` ORDER BY occurred_at, segment_id, seq`
	  rows, err := q.QueryContext(ctx, qtext, args...)
	   if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	  return collectRows(rows)
   }

  func (s *SQL) PutCheckpoint(ctx context.Context, c Checkpoint) error {
	  q, err := s.conn(ctx)
	  if err != nil {
		   return err
	   }
	  tip := 0
	  if c.Tip {
		  tip = 1
	  }
	   var root string
	   err = q.QueryRowContext(ctx, `SELECT merkle_root FROM aa_audit_checkpoint WHERE segment_id=? AND seq=? AND tip=?`, c.SegmentID, c.Seq, tip).Scan(&root)
	   if err == nil {
		  if root != c.MerkleRoot {
			   return ErrDrift
		   }
		  return nil
	   }
	  if err != sql.ErrNoRows {
		  return err
	   }
	  _, err = q.ExecContext(ctx, `INSERT INTO aa_audit_checkpoint (id, segment_id, seq, merkle_root, prev_root, entry_count, tip, actor, at) VALUES (?,?,?,?,?,?,?,?,?)`,
		  c.ID, c.SegmentID, c.Seq, c.MerkleRoot, c.PrevRoot, c.Count, tip, c.Actor, c.At.UTC())
	  return err
   }

  func (s *SQL) Checkpoints(ctx context.Context, segmentID string) ([]Checkpoint, error) {
	  q, err := s.conn(ctx)
	   if err != nil {
		   return nil, err
	   }
	  rows, err := q.QueryContext(ctx, `SELECT id, segment_id, seq, merkle_root, prev_root, entry_count, tip, actor, at FROM aa_audit_checkpoint WHERE segment_id=? ORDER BY seq, tip`, segmentID)
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  return collectCkpt(rows)
  }

   func (s *SQL) AllCheckpoints(ctx context.Context) ([]Checkpoint, error) {
	  q, err := s.conn(ctx)
	  if err != nil {
		   return nil, err
	   }
	  rows, err := q.QueryContext(ctx, `SELECT id, segment_id, seq, merkle_root, prev_root, entry_count, tip, actor, at FROM aa_audit_checkpoint ORDER BY at, id`)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   return collectCkpt(rows)
   }

   func collectCkpt(rows *sql.Rows) ([]Checkpoint, error) {
	  var out []Checkpoint
	   for rows.Next() {
		  var c Checkpoint
		   var tip int
		  var at time.Time
		   if err := rows.Scan(&c.ID, &c.SegmentID, &c.Seq, &c.MerkleRoot, &c.PrevRoot, &c.Count, &tip, &c.Actor, &at); err != nil {
			  return nil, err
		   }
		   c.Tip = tip != 0
		  c.At = at.UTC()
		   out = append(out, c)
	   }
	   return out, rows.Err()
   }

  func (s *SQL) Cursor(ctx context.Context, source string) (Cursor, bool, error) {
	   q, err := s.conn(ctx)
	   if err != nil {
		   return Cursor{}, false, err
	  }
	  var c Cursor
	   var lag int64
	  var at time.Time
	   err = q.QueryRowContext(ctx, `SELECT source, high_water, last_id, lag_ms, updated_at FROM aa_audit_cursor WHERE source=?`, source).Scan(&c.Source, &c.HighWater, &c.LastID, &lag, &at)
	  if err == sql.ErrNoRows {
		   return Cursor{}, false, nil
	   }
	   if err != nil {
		  return Cursor{}, false, err
	   }
	   c.Lag = time.Duration(lag) * time.Millisecond
	  c.UpdatedAt = at.UTC()
	   return c, true, nil
  }

  func (s *SQL) PutCursor(ctx context.Context, c Cursor) error {
	   cur,
		  ok, err := s.Cursor(ctx, c.Source)
	  if err != nil {
		   return err
	   }
	  if ok && c.HighWater < cur.HighWater {
		   return ErrCursor
	   }
	   q, err := s.conn(ctx)
	  if err != nil {
		  return err
	  }
	  lag := c.Lag.Milliseconds()
	  if !ok {
		   _, err = q.ExecContext(ctx, `INSERT INTO aa_audit_cursor (source, high_water, last_id, lag_ms, updated_at) VALUES (?,?,?,?,?)`, c.Source, c.HighWater, c.LastID, lag, c.UpdatedAt.UTC())
		  return err
	   }
	  _, err = q.ExecContext(ctx, `UPDATE aa_audit_cursor SET high_water=?, last_id=?, lag_ms=?, updated_at=? WHERE source=?`, c.HighWater, c.LastID, lag, c.UpdatedAt.UTC(), c.Source)
	  return err
  }

  func (s *SQL) Cursors(ctx context.Context) ([]Cursor, error) {
	  q, err := s.conn(ctx)
	   if err != nil {
		  return nil, err
	   }
	   rows, err := q.QueryContext(ctx, `SELECT source, high_water, last_id, lag_ms, updated_at FROM aa_audit_cursor ORDER BY source`)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	  var out []Cursor
	   for rows.Next() {
		   var c Cursor
		  var lag int64
		  var at time.Time
		  if err := rows.Scan(&c.Source, &c.HighWater, &c.LastID, &lag, &at); err != nil {
			   return nil, err
		   }
		   c.Lag = time.Duration(lag) * time.Millisecond
		  c.UpdatedAt = at.UTC()
		   out = append(out, c)
	  }
	   return out, rows.Err()
  }

  func (s *SQL) PutAlert(ctx context.Context, a Alert) error {
	   q, err := s.conn(ctx)
	  if err != nil {
		  return err
	   }
	  open := 0
	  if a.Open {
		  open = 1
	   }
	  _, err = q.ExecContext(ctx, `INSERT INTO aa_audit_alert (id, name, segment_id, detail, open_flag, at) VALUES (?,?,?,?,?,?)`, a.ID, a.Name, a.SegmentID, clip(a.Detail, 255), open, a.At.UTC())
	  return err
  }

  func (s *SQL) Alerts(ctx context.Context, openOnly bool) ([]Alert, error) {
	  q, err := s.conn(ctx)
	   if err != nil {
		   return nil, err
	   }
	  qtext := `SELECT id, name, segment_id, detail, open_flag, at FROM aa_audit_alert`
	  if openOnly {
		  qtext += ` WHERE open_flag=1`
	  }
	   qtext += ` ORDER BY at`
	   rows, err := q.QueryContext(ctx, qtext)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   var out []Alert
	  for rows.Next() {
		  var a Alert
		  var open int
		  var at time.Time
		   if err := rows.Scan(&a.ID, &a.Name, &a.SegmentID, &a.Detail, &open, &at); err != nil {
			  return nil, err
		   }
		   a.Open = open != 0
		  a.At = at.UTC()
		   out = append(out, a)
	  }
	  return out, rows.Err()
   }

  func (s *SQL) PutJob(ctx context.Context, j Job) error {
	   cur, ok, err := s.Job(ctx, j.ID)
	   if err != nil {
		  return err
	  }
	  if ok {
		   if err := jobTransit(cur.State, j.State); err != nil {
			   return err
		   }
	  }
	  q, err := s.conn(ctx)
	  if err != nil {
		   return err
	  }
	   if !ok {
		   _, err = q.ExecContext(ctx, `INSERT INTO aa_audit_replay_job (id, user_id, asset, from_at, to_at, state, fence_epoch, fence_token, dual_id, report_json, error_text, actor, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			   j.ID,
			   j.UserID, j.Asset, j.From.UTC(), j.To.UTC(), j.State, j.FenceEpoch, j.FenceToken, j.DualID, j.ReportJSON, clip(j.ErrorText, 255), j.Actor, j.CreatedAt.UTC(), j.UpdatedAt.UTC())
		   return err
	   }
	   _, err = q.ExecContext(ctx, `UPDATE aa_audit_replay_job SET state=?, fence_epoch=?, fence_token=?, dual_id=?, report_json=?, error_text=?, updated_at=? WHERE id=?`,
		   j.State, j.FenceEpoch, j.FenceToken, j.DualID, j.ReportJSON, clip(j.ErrorText, 255), j.UpdatedAt.UTC(), j.ID)
	  return err
  }

  func (s *SQL) Job(ctx context.Context, id string) (Job, bool, error) {
	  q, err := s.conn(ctx)
	  if err != nil {
		   return Job{}, false, err
	  }
	   var j Job
	   var from, to, created, updated time.Time
	   err = q.QueryRowContext(ctx, `SELECT id, user_id, asset, from_at, to_at, state, fence_epoch, fence_token, dual_id, report_json, error_text, actor, created_at, updated_at FROM aa_audit_replay_job WHERE id=?`, id).Scan(
		   &j.ID, &j.UserID, &j.Asset, &from, &to, &j.State, &j.FenceEpoch, &j.FenceToken, &j.DualID, &j.ReportJSON, &j.ErrorText, &j.Actor, &created, &updated)
	  if err == sql.ErrNoRows {
		  return Job{}, false, nil
	   }
	   if err != nil {
		  return Job{}, false, err
	  }
	  j.From, j.To = from.UTC(), to.UTC()
	  j.CreatedAt,
		  j.UpdatedAt = created.UTC(), updated.UTC()
	  return j, true, nil
   }

   func (s *SQL) Jobs(ctx context.Context, state string) ([]Job, error) {
	  q, err := s.conn(ctx)
	   if err != nil {
		  return nil, err
	  }
	   qtext := `SELECT id, user_id, asset, from_at, to_at, state, fence_epoch, fence_token, dual_id, report_json, error_text, actor, created_at, updated_at FROM aa_audit_replay_job`
	   var args []any
	  if state != "" {
		   qtext += ` WHERE state=?`
		  args = append(args, state)
	   }
	   qtext += ` ORDER BY created_at`
	  rows, err := q.QueryContext(ctx, qtext, args...)
	  if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   var out []Job
	  for rows.Next() {
		  var j Job
		   var from, to, created, updated time.Time
		   if err := rows.Scan(&j.ID, &j.UserID, &j.Asset, &from, &to, &j.State, &j.FenceEpoch, &j.FenceToken, &j.DualID, &j.ReportJSON, &j.ErrorText, &j.Actor, &created, &updated); err != nil {
			   return nil, err
		  }
		  j.From, j.To = from.UTC(), to.UTC()
		   j.CreatedAt, j.UpdatedAt = created.UTC(), updated.UTC()
		  out = append(out, j)
	   }
	  return out, rows.Err()
   }

   func (s *SQL) PutExport(ctx context.Context, e ExportRec) error {
	  _, ok, err := s.Export(ctx, e.ID)
	   if err != nil {
		   return err
	  }
	   q, err := s.conn(ctx)
	   if err != nil {
		  return err
	  }
	  if !ok {
		   _, err = q.ExecContext(ctx, `INSERT INTO aa_audit_export (id, segment_id, user_id, asset, from_at, to_at, approval_id, requested_by, state, created_at, released_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			   e.ID, e.SegmentID, e.UserID, e.Asset, e.From.UTC(), e.To.UTC(), e.ApprovalID, e.RequestedBy, e.State, e.CreatedAt.UTC(), nullTime(e.ReleasedAt))
		  return err
	   }
	  _, err = q.ExecContext(ctx, `UPDATE aa_audit_export SET state=?, approval_id=?, released_at=? WHERE id=?`, e.State, e.ApprovalID, nullTime(e.ReleasedAt), e.ID)
	   return err
   }

  func (s *SQL) Export(ctx context.Context, id string) (ExportRec, bool, error) {
	  q, err := s.conn(ctx)
	   if err != nil {
		   return ExportRec{}, false, err
	  }
	   var e ExportRec
	   var from, to, created time.Time
	  var released sql.NullTime
	  err = q.QueryRowContext(ctx, `SELECT id, segment_id, user_id, asset, from_at, to_at, approval_id, requested_by, state, created_at, released_at FROM aa_audit_export WHERE id=?`, id).Scan(
		  &e.ID, &e.SegmentID, &e.UserID, &e.Asset, &from, &to, &e.ApprovalID, &e.RequestedBy, &e.State, &created, &released)
	  if err == sql.ErrNoRows {
		  return ExportRec{}, false, nil
	   }
	   if err != nil {
		  return ExportRec{}, false, err
	  }
	   e.From, e.To, e.CreatedAt = from.UTC(), to.UTC(), created.UTC()
	  e.ReleasedAt = scanTime(released)
	   return e, true, nil
  }

  func (s *SQL) CorruptBody(context.Context, string, int, string) error {
	  return fmt.Errorf("%w: sql history rewrite refused", ErrFrozen)
   }

   func (s *SQL) RewriteRoot(context.Context, string, string) error {
	   return fmt.Errorf("%w: sql history rewrite refused", ErrFrozen)
  }

   func clip(s string, n int) string {
	  if len(s) <= n {
		  return s
	   }
	  return s[:n]
  }
