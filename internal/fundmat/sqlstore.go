package fundmat

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// SQLStore 按迁移表读写投影。占位符在执行前按方言改写。
type SQLStore struct {
	db *sql.DB
	dialect persist.Dialect
}

// NewSQLStore 绑定已打开的库。db 为空时所有方法返回 ErrUnavailable。

  func NewSQLStore(db *sql.DB, d persist.Dialect) *SQLStore {
	  return &SQLStore{db: db,
		  dialect: d}
  }

  func (s *SQLStore) q(query string) string { return persist.Rewrite(query, s.dialect) }

   func (s *SQLStore) avail() error {
	  if s == nil || s.db == nil {
		  return ErrUnavailable
	  }
	   return nil
  }

const eventCols = `event_id, stream, seq, user_id, asset, subject, op, amount, bucket, causal_id, source_event_id, occurred_at`

   func (s *SQLStore) Append(ctx context.Context, ev FundingEvent) (FundingEvent, error) {
	   if err := s.avail(); err != nil {
		   return FundingEvent{}, err
	  }
	  if err := ev.Validate(); err != nil {
		   return FundingEvent{}, err
	   }
	   tx, err := s.db.BeginTx(ctx, nil)
	  if err != nil {
		  return FundingEvent{}, err
	  }
	  defer func() { _ = tx.Rollback() }()
	   if old, err := scanEvent(tx.QueryRowContext(ctx, s.q(`SELECT `+eventCols+` FROM fund_mat_events WHERE event_id=?`), ev.ID)); err == nil {
		   return old, nil
	  } else if err != ErrNotFound {
		   return FundingEvent{}, err
	   }
	  if ev.Seq == 0 {
		  var head int64
		   if err := tx.QueryRowContext(ctx, s.q(`SELECT COALESCE(MAX(seq),0) FROM fund_mat_events WHERE stream=?`), string(ev.Stream)).Scan(&head); err != nil {
			   return FundingEvent{}, err
		  }
		  ev.Seq = head + 1
	   }
	  _, err = tx.ExecContext(ctx, s.q(`INSERT INTO fund_mat_events (`+eventCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`),
		  ev.ID, string(ev.Stream), ev.Seq, ev.UserID, ev.Asset, ev.Subject, string(ev.Op), ev.Amount,
		   string(ev.Bucket), ev.CausalID, ev.SourceEventID, ev.OccurredAt.UTC())
	  if err != nil {
		   if isDup(err) {
			  return FundingEvent{}, ErrSeqConflict
		   }
		   return FundingEvent{}, err
	   }
	  if err := tx.Commit(); err != nil {
		   return FundingEvent{}, err
	   }
	   return ev, nil
   }

  func (s *SQLStore) Pending(ctx context.Context, stream Stream, limit int) ([]FundingEvent, error) {
	  if err := s.avail(); err != nil {
		   return nil, err
	  }
	  if limit <= 0 {
		  limit = 100
	  }
	  rows, err := s.db.QueryContext(ctx, s.q(`SELECT `+eventCols+` FROM fund_mat_events e
WHERE e.stream=? AND NOT EXISTS (SELECT 1 FROM fund_mat_applied a WHERE a.event_id=e.event_id)
ORDER BY e.seq LIMIT ?`), string(stream), limit)
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  return collectEvents(rows)
  }

   func (s *SQLStore) ApplyOne(ctx context.Context, ev FundingEvent, now time.Time) (ApplyOutcome, error) {
	   if err := s.avail(); err != nil {
		  return ApplyOutcome{}, err
	  }
	   if now.IsZero() {
		  now = time.Now().UTC()
	  }
	   tx, err := s.db.BeginTx(ctx, nil)
	   if err != nil {
		   return ApplyOutcome{}, err
	  }
	   defer func() { _ = tx.Rollback() }()
	  stored, err := scanEvent(tx.QueryRowContext(ctx, s.q(`SELECT `+eventCols+` FROM fund_mat_events WHERE event_id=?`), ev.ID))
	  if err != nil {
		   return ApplyOutcome{}, err
	   }
	   var applied int
	  if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(1) FROM fund_mat_applied WHERE event_id=?`), stored.ID).Scan(&applied); err != nil {
		  return ApplyOutcome{}, err
	   }
	   row, ok, err := s.getRowTx(ctx, tx, stored.UserID, stored.Asset, stored.Subject)
	   if err != nil {
		  return ApplyOutcome{}, err
	  }
	  if applied > 0 {
		   if !ok {
			  row = BlankRow(stored.UserID, stored.Asset, stored.Subject, now)
		  }
		  return ApplyOutcome{Applied: false, Row: row}, tx.Commit()
	   }
	   if !ok {
		  row = BlankRow(stored.UserID, stored.Asset, stored.Subject, now)
	   }
	   next, err := Mutate(row, stored)
	   if err != nil {
		   return ApplyOutcome{}, err
	   }
	  next.RowVersion = row.RowVersion + 1
	  next.UpdatedAt = now.UTC()
	  next.LastEventID = stored.ID
	   if stored.CausalID != "" {
		  next.CausalID = stored.CausalID
	  }
	   if err := s.upsertRowTx(ctx, tx, next); err != nil {
		   return ApplyOutcome{}, err
	  }
	  if _, err := tx.ExecContext(ctx, s.q(`INSERT INTO fund_mat_applied (event_id, stream, seq, applied_at) VALUES (?,?,?,?)`),
		  stored.ID, string(stored.Stream), stored.Seq, now.UTC()); err != nil {
		  return ApplyOutcome{}, err
	   }
	  if err := tx.Commit(); err != nil {
		  return ApplyOutcome{}, err
	  }
	   return ApplyOutcome{Applied: true, Row: next}, nil
   }

   func (s *SQLStore) GetRow(ctx context.Context, user, asset, subject string) (MatRow, bool, error) {
	   if err := s.avail(); err != nil {
		  return MatRow{}, false, err
	   }
	   return s.getRowTx(ctx,
		   s.db, user, asset, subject)
  }

  func (s *SQLStore) PutRow(ctx context.Context, row MatRow) error {
	   if err := s.avail(); err != nil {
		  return err
	  }
	   row.UserID = NormUser(row.UserID)
	   row.Asset = NormAsset(row.Asset)
	   row.Subject = NormSubject(row.Subject)
	  if row.UserID == "" || row.Asset == "" || row.Subject == "" {
		   return ErrBadEvent
	   }
	   row = fillZero(row)
	  if err := row.recompute(); err != nil {
		   return err
	  }
	   return s.upsertRowTx(ctx, s.db, row)
  }

  func (s *SQLStore) ListUser(ctx context.Context, user string) ([]MatRow, error) {
	  if err := s.avail(); err != nil {
		  return nil, err
	   }
	  rows, err := s.db.QueryContext(ctx, s.q(`SELECT `+rowCols+` FROM fund_mat_rows WHERE user_id=? ORDER BY asset, subject`), NormUser(user))
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   return collectRows(rows)
   }

   func (s *SQLStore) ListUsers(ctx context.Context, limit int) ([]string, error) {
	   if err := s.avail(); err != nil {
		   return nil, err
	  }
	  if limit <= 0 {
		  limit = 100
	  }
	  rows, err := s.db.QueryContext(ctx, s.q(`SELECT DISTINCT user_id FROM fund_mat_rows ORDER BY user_id LIMIT ?`), limit)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   var out []string
	  for rows.Next() {
		   var u string
		   if err := rows.Scan(&u); err != nil {
			   return nil, err
		   }
		   out = append(out, u)
	   }
	   return out, rows.Err()
  }

   func (s *SQLStore) AppliedSeqs(ctx context.Context, stream Stream) ([]int64, error) {
	   if err := s.avail(); err != nil {
		  return nil, err
	   }
	   rows, err := s.db.QueryContext(ctx, s.q(`SELECT seq FROM fund_mat_applied WHERE stream=?`), string(stream))
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  var out []int64
	  for rows.Next() {
		   var n int64
		   if err := rows.Scan(&n); err != nil {
			  return nil, err
		   }
		   out = append(out, n)
	  }
	   return out, rows.Err()
   }

  func (s *SQLStore) HeadSeq(ctx context.Context, stream Stream) (int64, error) {
	   if err := s.avail(); err != nil {
		   return 0, err
	  }
	   var n int64
	   err := s.db.QueryRowContext(ctx, s.q(`SELECT COALESCE(MAX(seq),0) FROM fund_mat_events WHERE stream=?`), string(stream)).Scan(&n)
	   return n, err
  }

   func (s *SQLStore) PutWM(ctx context.Context, wm MatWatermark) error {
	   if err := s.avail(); err != nil {
		   return err
	   }
	   if !wm.Stream.Valid() {
		   return ErrBadEvent
	  }
	   within := 0
	  if wm.WithinSLA {
		  within = 1
	   }
	  if s.dialect == persist.Postgres {
		  _, err := s.db.ExecContext(ctx, s.q(`INSERT INTO fund_mat_wm (
stream, epoch, head_seq, contiguous_seq, applied_count, hole_count, lag_events, lag_ms, sla_ms, within_sla, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (stream) DO UPDATE SET
epoch=EXCLUDED.epoch, head_seq=EXCLUDED.head_seq, contiguous_seq=EXCLUDED.contiguous_seq,
applied_count=EXCLUDED.applied_count, hole_count=EXCLUDED.hole_count, lag_events=EXCLUDED.lag_events,
lag_ms=EXCLUDED.lag_ms, sla_ms=EXCLUDED.sla_ms, within_sla=EXCLUDED.within_sla, updated_at=EXCLUDED.updated_at`),
			   string(wm.Stream), wm.Epoch, wm.HeadSeq, wm.ContiguousSeq, wm.AppliedCount, wm.HoleCount,
			  wm.LagEvents,
			   wm.LagMs, wm.SLAMs, within, wm.UpdatedAt.UTC())
		   return err
	  }
	  _, err := s.db.ExecContext(ctx, s.q(`INSERT INTO fund_mat_wm (
stream, epoch, head_seq, contiguous_seq, applied_count, hole_count, lag_events, lag_ms, sla_ms, within_sla, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
epoch=VALUES(epoch), head_seq=VALUES(head_seq), contiguous_seq=VALUES(contiguous_seq),
applied_count=VALUES(applied_count), hole_count=VALUES(hole_count), lag_events=VALUES(lag_events),
lag_ms=VALUES(lag_ms), sla_ms=VALUES(sla_ms), within_sla=VALUES(within_sla), updated_at=VALUES(updated_at)`),
		  string(wm.Stream), wm.Epoch, wm.HeadSeq, wm.ContiguousSeq, wm.AppliedCount, wm.HoleCount,
		  wm.LagEvents, wm.LagMs, wm.SLAMs, within, wm.UpdatedAt.UTC())
	  return err
  }

   func (s *SQLStore) ListWM(ctx context.Context) ([]MatWatermark, error) {
	  if err := s.avail(); err != nil {
		  return nil, err
	  }
	  rows, err := s.db.QueryContext(ctx, s.q(`SELECT stream, epoch, head_seq, contiguous_seq, applied_count, hole_count, lag_events, lag_ms, sla_ms, within_sla, updated_at FROM fund_mat_wm`))
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  have := map[Stream]MatWatermark{}
	  for rows.Next() {
		   var wm MatWatermark
		   var stream string
		   var within int
		   if err := rows.Scan(&stream, &wm.Epoch, &wm.HeadSeq, &wm.ContiguousSeq, &wm.AppliedCount, &wm.HoleCount, &wm.LagEvents, &wm.LagMs, &wm.SLAMs, &within, &wm.UpdatedAt); err != nil {
			  return nil, err
		  }
		  wm.Stream = Stream(stream)
		  wm.WithinSLA = within != 0
		   wm.UpdatedAt = wm.UpdatedAt.UTC()
		   have[wm.Stream] = wm
	   }
	  if err := rows.Err(); err != nil {
		  return nil, err
	   }
	   out := make([]MatWatermark, 0, len(Streams()))
	   for _, st := range Streams() {
		   if wm, ok := have[st]; ok {
			  out = append(out, wm)
			  continue
		   }
		   out = append(out, MatWatermark{Stream: st, Epoch: 1, WithinSLA: true})
	   }
	  return out, nil
  }

  func (s *SQLStore) SealUserBalance(ctx context.Context, userID string, at time.Time) (int, error) {
	   if err := s.avail(); err != nil {
		   return 0, err
	  }
	  if at.IsZero() {
		   at = time.Now().UTC()
	   }
	  res, err := s.db.ExecContext(ctx, s.q(`INSERT INTO fund_mat_applied (event_id, stream, seq, applied_at) 
SELECT e.event_id, e.stream, e.seq, ? FROM fund_mat_events e
WHERE e.user_id=? AND e.op IN ('freeze','unfreeze','credit_available','debit_available','debit_frozen','set_available','set_frozen')
AND NOT EXISTS (SELECT 1 FROM fund_mat_applied a WHERE a.event_id=e.event_id)`), at.UTC(), NormUser(userID))
	  if err != nil {
		   return 0, err
	  }
	   n, _ := res.RowsAffected()
	  return int(n), nil
  }

  func (s *SQLStore) PutFence(ctx context.Context, f ReadFence) error {
	   if err := s.avail(); err != nil {
		   return err
	  }
	   if f.ID == "" {
		  return ErrBadEvent
	  }
	   _, err := s.db.ExecContext(ctx, s.q(`INSERT INTO read_fences (id, user_id, stream, epoch, min_seq, token, issued_at, expires_at)
VALUES (?,?,?,?,?,?,?,?)`), f.ID, f.UserID, string(f.Stream), f.Epoch, f.MinSeq, f.Token, f.IssuedAt.UTC(), f.ExpiresAt.UTC())
	   if err != nil && isDup(err) {
		   _, err = s.db.ExecContext(ctx, s.q(`UPDATE read_fences SET user_id=?, stream=?, epoch=?, min_seq=?, token=?, issued_at=?, expires_at=? WHERE id=?`),
			   f.UserID, string(f.Stream), f.Epoch, f.MinSeq, f.Token, f.IssuedAt.UTC(), f.ExpiresAt.UTC(), f.ID)
	   }
	  return err
   }

  func (s *SQLStore) GetFence(ctx context.Context, id string) (ReadFence, error) {
	  if err := s.avail(); err != nil {
		   return ReadFence{}, err
	  }
	   var f ReadFence
	   var stream string
	  err := s.db.QueryRowContext(ctx, s.q(`SELECT id, user_id, stream, epoch, min_seq, token, issued_at, expires_at FROM read_fences WHERE id=?`), id).
		   Scan(&f.ID, &f.UserID, &stream, &f.Epoch, &f.MinSeq, &f.Token, &f.IssuedAt, &f.ExpiresAt)
	   if err == sql.ErrNoRows {
		  return ReadFence{}, ErrNotFound
	  }
	  if err != nil {
		   return ReadFence{}, err
	   }
	  f.Stream = Stream(stream)
	  f.IssuedAt = f.IssuedAt.UTC()
	   f.ExpiresAt = f.ExpiresAt.UTC()
	   return f, nil
  }

   func (s *SQLStore) PutVersion(ctx context.Context, v ViewVersion) error {
	  if err := s.avail(); err != nil {
		   return err
	  }
	   if v.ID == "" {
		   return ErrBadEvent
	  }
	   if v.Scope == "" {
		  v.Scope = "user"
	   }
	  _, err := s.db.ExecContext(ctx, s.q(`INSERT INTO view_versions (id, scope, user_id, version, epoch, wm_seq, digest, row_count, built_at)
VALUES (?,?,?,?,?,?,?,?,?)`), v.ID, v.Scope, v.UserID, v.Version, v.Epoch, v.WMSeq, v.Digest, v.RowCount, v.BuiltAt.UTC())
	  return err
  }

  func (s *SQLStore) LatestVersion(ctx context.Context, userID string) (ViewVersion, error) {
	  if err := s.avail(); err != nil {
		   return ViewVersion{}, err
	  }
	  var v ViewVersion
	  err := s.db.QueryRowContext(ctx, s.q(`SELECT id, scope, user_id, version, epoch, wm_seq, digest, row_count, built_at
FROM view_versions WHERE scope='user' AND user_id=? ORDER BY version DESC LIMIT 1`), NormUser(userID)).
		  Scan(&v.ID, &v.Scope, &v.UserID, &v.Version, &v.Epoch, &v.WMSeq, &v.Digest, &v.RowCount, &v.BuiltAt)
	  if err == sql.ErrNoRows {
		  return ViewVersion{}, ErrNotFound
	   }
	  if err != nil {
		  return ViewVersion{}, err
	   }
	   v.BuiltAt = v.BuiltAt.UTC()
	   return v, nil
   }

  func (s *SQLStore) PutJob(ctx context.Context, j RebuildJob) error {
	   if err := s.avail(); err != nil {
		  return err
	   }
	  if j.ID == "" || !j.Status.Valid() {
		  return ErrBadEvent
	  }
	  _, err := s.db.ExecContext(ctx, s.q(`INSERT INTO rebuild_jobs (
id, scope, user_id, status, tolerance, drift_count, drift_sample, error_text, queued_at, started_at, finished_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?)`),
		  j.ID, j.Scope, j.UserID, string(j.Status), j.Tolerance, j.DriftCount, j.DriftSample, j.Error,
		   j.QueuedAt.UTC(), nullTime(j.StartedAt), nullTime(j.FinishedAt))
	  if isDup(err) {
		   return ErrDuplicate
	  }
	   return err
   }

   func (s *SQLStore) GetJob(ctx context.Context, id string) (RebuildJob, error) {
	  if err := s.avail(); err != nil {
		   return RebuildJob{}, err
	   }
	   return scanJob(s.db.QueryRowContext(ctx, s.q(`SELECT `+jobCols+` FROM rebuild_jobs WHERE id=?`), id))
  }

  func (s *SQLStore) ListJobs(ctx context.Context, status JobStatus, limit int) ([]RebuildJob, error) {
	   if err := s.avail(); err != nil {
		   return nil, err
	  }
	  if limit <= 0 {
		  limit = 50
	  }
	  var (
		  rows *sql.Rows
		   err error
	  )
	   if status == "" {
		   rows, err = s.db.QueryContext(ctx, s.q(`SELECT `+jobCols+` FROM rebuild_jobs ORDER BY queued_at LIMIT ?`), limit)
	   } else {
		  rows, err = s.db.QueryContext(ctx, s.q(`SELECT `+jobCols+` FROM rebuild_jobs WHERE status=? ORDER BY queued_at LIMIT ?`), string(status), limit)
	  }
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   var out []RebuildJob
	  for rows.Next() {
		  j, err := scanJob(rows)
		   if err != nil {
			  return nil, err
		   }
		   out = append(out, j)
	   }
	   return out, rows.Err()
  }

  func (s *SQLStore) CasJob(ctx context.Context, id string, from JobStatus, next RebuildJob) (RebuildJob, error) {
	   if err := s.avail(); err != nil {
		   return RebuildJob{}, err
	  }
	  if err := CanStep(from, next.Status); err != nil {
		   cur, gerr := s.GetJob(ctx, id)
		   if gerr != nil {
			   return RebuildJob{}, gerr
		   }
		   return cur, err
	   }
	  res, err := s.db.ExecContext(ctx, s.q(`UPDATE rebuild_jobs SET status=?, tolerance=?, drift_count=?, drift_sample=?, error_text=?, started_at=?, finished_at=?
WHERE id=? AND status=?`),
		   string(next.Status), next.Tolerance, next.DriftCount, next.DriftSample, next.Error,
		  nullTime(next.StartedAt), nullTime(next.FinishedAt), id, string(from))
	  if err != nil {
		  return RebuildJob{}, err
	   }
	   n, _ := res.RowsAffected()
	   cur, gerr := s.GetJob(ctx, id)
	  if gerr != nil {
		  return RebuildJob{}, gerr
	   }
	   if n == 0 {
		   return cur, ErrTransition
	  }
	   return cur, nil
   }

   func (s *SQLStore) PutAlert(ctx context.Context, a Alert) error {
	   if err := s.avail(); err != nil {
		   return err
	  }
	  if a.ID == "" || a.Kind == "" {
		   return ErrBadEvent
	  }
	  open := 0
	   if a.Open {
		   open = 1
	  }
	  if s.dialect == persist.Postgres {
		   _, err := s.db.ExecContext(ctx, s.q(`INSERT INTO fund_mat_alerts (id, kind, stream, user_id, detail, open_flag, raised_at)
VALUES (?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET kind=EXCLUDED.kind, stream=EXCLUDED.stream, user_id=EXCLUDED.user_id,
detail=EXCLUDED.detail, open_flag=EXCLUDED.open_flag, raised_at=EXCLUDED.raised_at`),
			  a.ID, a.Kind, a.Stream, a.UserID, a.Detail, open, a.RaisedAt.UTC())
		   return err
	   }
	   _, err := s.db.ExecContext(ctx, s.q(`INSERT INTO fund_mat_alerts (id, kind, stream, user_id, detail, open_flag, raised_at)
VALUES (?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE kind=VALUES(kind), stream=VALUES(stream), user_id=VALUES(user_id),
detail=VALUES(detail), open_flag=VALUES(open_flag), raised_at=VALUES(raised_at)`),
		  a.ID, a.Kind, a.Stream, a.UserID, a.Detail, open, a.RaisedAt.UTC())
	  return err
  }

   func (s *SQLStore) ListAlerts(ctx context.Context, openOnly bool, limit int) ([]Alert, error) {
	  if err := s.avail(); err != nil {
		  return nil, err
	   }
	   if limit <= 0 {
		   limit = 50
	  }
	   q := `SELECT id, kind, stream, user_id, detail, open_flag, raised_at FROM fund_mat_alerts`
	  args := []any{limit}
	  if openOnly {
		   q += ` WHERE open_flag=1`
	   }
	  q += ` ORDER BY raised_at DESC LIMIT ?`
	  rows, err := s.db.QueryContext(ctx, s.q(q), args...)
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	  var out []Alert
	   for rows.Next() {
		  var a Alert
		  var open int
		   if err := rows.Scan(&a.ID, &a.Kind, &a.Stream, &a.UserID, &a.Detail, &open, &a.RaisedAt); err != nil {
			   return nil, err
		  }
		  a.Open = open != 0
		  a.RaisedAt = a.RaisedAt.UTC()
		   out = append(out, a)
	  }
	  return out, rows.Err()
  }

   func (s *SQLStore) SeenSource(ctx context.Context, id string) (bool, error) {
	   if err := s.avail(); err != nil {
		   return false, err
	  }
	  var n int
	   err := s.db.QueryRowContext(ctx, s.q(`SELECT COUNT(1) FROM fund_mat_source WHERE source_id=?`), id).Scan(&n)
	  if err != nil {
		  return false, err
	  }
	  return n > 0, nil
   }

  func (s *SQLStore) MarkSource(ctx context.Context, id string, at time.Time) error {
	   if err := s.avail(); err != nil {
		   return err
	   }
	  if id == "" {
		  return ErrBadEvent
	  }
	   if at.IsZero() {
		   at = time.Now().UTC()
	  }
	  _, err := s.db.ExecContext(ctx, s.q(`INSERT INTO fund_mat_source (source_id, seen_at) VALUES (?,?)`), id, at.UTC())
	   if isDup(err) {
		  return nil
	   }
	   return err
  }

type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

  func (s *SQLStore) getRowTx(ctx context.Context, q queryRower, user, asset, subject string) (MatRow, bool, error) {
	  row, err := scanRow(q.QueryRowContext(ctx, s.q(`SELECT `+rowCols+` FROM fund_mat_rows WHERE user_id=? AND asset=? AND subject=?`),
		  NormUser(user), NormAsset(asset), NormSubject(subject)))
	   if err == ErrNotFound {
		   return MatRow{}, false, nil
	   }
	   if err != nil {
		   return MatRow{}, false, err
	  }
	  return row, true, nil
   }

   func (s *SQLStore) upsertRowTx(ctx context.Context, ex execer, row MatRow) error {
	   args := []any{
		  row.UserID, row.Asset, row.Subject, row.Available, row.Frozen, row.InFlight,
		  row.InFlightDeposit, row.InFlightWithdraw, row.InFlightTransfer,
		  row.Occupancy, row.OccupancyRisk, row.OccupancyReserve,
		  row.CausalID, row.LastEventID, row.RowVersion, row.UpdatedAt.UTC(),
	   }
	  if s.dialect == persist.Postgres {
		  _, err := ex.ExecContext(ctx, s.q(`INSERT INTO fund_mat_rows (`+rowCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (user_id, asset, subject) DO UPDATE SET
available=EXCLUDED.available, frozen=EXCLUDED.frozen, in_flight=EXCLUDED.in_flight,
in_flight_deposit=EXCLUDED.in_flight_deposit, in_flight_withdraw=EXCLUDED.in_flight_withdraw,
in_flight_transfer=EXCLUDED.in_flight_transfer, occupancy=EXCLUDED.occupancy,
occupancy_risk=EXCLUDED.occupancy_risk, occupancy_reserve=EXCLUDED.occupancy_reserve,
causal_id=EXCLUDED.causal_id, last_event_id=EXCLUDED.last_event_id,
row_version=EXCLUDED.row_version, updated_at=EXCLUDED.updated_at`), args...)
		  return err
	  }
	  _, err := ex.ExecContext(ctx, s.q(`INSERT INTO fund_mat_rows (`+rowCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
available=VALUES(available), frozen=VALUES(frozen), in_flight=VALUES(in_flight),
in_flight_deposit=VALUES(in_flight_deposit), in_flight_withdraw=VALUES(in_flight_withdraw),
in_flight_transfer=VALUES(in_flight_transfer), occupancy=VALUES(occupancy),
occupancy_risk=VALUES(occupancy_risk), occupancy_reserve=VALUES(occupancy_reserve),
causal_id=VALUES(causal_id), last_event_id=VALUES(last_event_id),
row_version=VALUES(row_version), updated_at=VALUES(updated_at)`), args...)
	   return err
  }

const rowCols = `user_id, asset, subject, available, frozen, in_flight, in_flight_deposit, in_flight_withdraw, in_flight_transfer, occupancy, occupancy_risk, occupancy_reserve, causal_id, last_event_id, row_version, updated_at`

const jobCols = `id, scope, user_id, status, tolerance, drift_count, drift_sample, error_text, queued_at, started_at, finished_at`

type rowScanner interface {
	Scan(dest ...any) error
}

   func scanEvent(sc rowScanner) (FundingEvent, error) {
	  var ev FundingEvent
	  var stream, op, bucket string
	  err := sc.Scan(&ev.ID, &stream, &ev.Seq, &ev.UserID, &ev.Asset, &ev.Subject, &op, &ev.Amount, &bucket, &ev.CausalID, &ev.SourceEventID, &ev.OccurredAt)
	  if err == sql.ErrNoRows {
		   return FundingEvent{}, ErrNotFound
	  }
	   if err != nil {
		   return FundingEvent{}, err
	   }
	   ev.Stream = Stream(stream)
	   ev.Op = Op(op)
	   ev.Bucket = Bucket(bucket)
	   ev.OccurredAt = ev.OccurredAt.UTC()
	   return ev, nil
   }

  func collectEvents(rows *sql.Rows) ([]FundingEvent, error) {
	   var out []FundingEvent
	   for rows.Next() {
		   ev, err := scanEvent(rows)
		  if err != nil {
			  return nil, err
		  }
		   out = append(out, ev)
	  }
	   return out, rows.Err()
   }

   func scanRow(sc rowScanner) (MatRow, error) {
	   var row MatRow
	   err := sc.Scan(&row.UserID, &row.Asset, &row.Subject, &row.Available, &row.Frozen, &row.InFlight,
		  &row.InFlightDeposit, &row.InFlightWithdraw, &row.InFlightTransfer,
		  &row.Occupancy, &row.OccupancyRisk, &row.OccupancyReserve,
		  &row.CausalID, &row.LastEventID, &row.RowVersion, &row.UpdatedAt)
	   if err == sql.ErrNoRows {
		  return MatRow{}, ErrNotFound
	   }
	  if err != nil {
		  return MatRow{}, err
	  }
	  row.UpdatedAt = row.UpdatedAt.UTC()
	  return row, nil
  }

  func collectRows(rows *sql.Rows) ([]MatRow, error) {
	   var out []MatRow
	   for rows.Next() {
		   row, err := scanRow(rows)
		  if err != nil {
			  return nil, err
		   }
		   out = append(out, row)
	  }
	  return out, rows.Err()
  }

   func scanJob(sc rowScanner) (RebuildJob, error) {
	   var j RebuildJob
	   var status string
	  var started, finished sql.NullTime
	   err := sc.Scan(&j.ID, &j.Scope, &j.UserID, &status, &j.Tolerance, &j.DriftCount, &j.DriftSample, &j.Error, &j.QueuedAt, &started, &finished)
	   if err == sql.ErrNoRows {
		  return RebuildJob{}, ErrNotFound
	  }
	   if err != nil {
		  return RebuildJob{}, err
	   }
	  j.Status = JobStatus(status)
	  j.QueuedAt = j.QueuedAt.UTC()
	  j.StartedAt = nullToPtr(started)
	   j.FinishedAt = nullToPtr(finished)
	   return j, nil
  }

  func nullTime(t *time.Time) any {
	  if t == nil || t.IsZero() {
		  return nil
	  }
	   return t.UTC()
  }

  func nullToPtr(n sql.NullTime) *time.Time {
	   if !n.Valid {
		   return nil
	  }
	   t := n.Time.UTC()
	  return &t
  }

   func isDup(err error) bool {
	   if err == nil {
		  return false
	   }
	   msg := strings.ToLower(err.Error())
	  return strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique")
  }
