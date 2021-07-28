// @FileName: sqlstore.go
// @Summary: sqlstore.go
// 预测与预留落库。内存是请求内的头寸，SQL 在同一事务里先写成功再改内存。
// CreateDate: 2021.11.09
// @Author: Cason
// @Created: 2021.07.25

package hotreserve

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

   func (s *Service) tx(ctx context.Context, fn func(context.Context) error) error {
	   if s == nil {
		   return ErrUnavailable
	  }
	  if s.sql == nil || s.sql.DB == nil {
		   return fn(ctx)
	  }
	  return s.sql.InTx(ctx, fn)
   }

  func (s *Service) exec(ctx context.Context, q string, args ...any) error {
	   if s == nil || s.sql == nil || s.sql.DB == nil {
		   return nil
	  }
	  _, err := s.sql.Conn(ctx).ExecContext(ctx, q, args...)
	   return err
   }

   func (s *Service) BindSQL(ctx context.Context, db *persist.SQLDB) error {
	  if s == nil {
		   return ErrUnavailable
	  }
	  if db == nil || db.DB == nil {
		   return nil
	  }
	   loaded, err := loadAll(ctx, db)
	   if err != nil {
		   return err
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  s.sql = db
	  s.install(loaded)
	   s.degraded = false
	   return nil
  }

type loaded struct {
	forecast map[string]*HotForecast
	history  map[string][]ForecastHead
	samples  map[string][]Sample
	reserves map[string]*Reservation
	journals map[string][]ReservationJournal
	tracks   map[string]*Track
	observe map[string]string
	plans   map[string][]PlanHold
	drifts map[string]*DriftTicket
	hints  []SweepHint
}

   func (s *Service) install(in loaded) {
	  if in.forecast == nil {
		   in.forecast = map[string]*HotForecast{}
	   }
	   if in.history == nil {
		  in.history = map[string][]ForecastHead{}
	  }
	   if in.samples == nil {
		   in.samples = map[string][]Sample{}
	  }
	  if in.reserves == nil {
		   in.reserves = map[string]*Reservation{}
	  }
	  if in.journals == nil {
		  in.journals = map[string][]ReservationJournal{}
	  }
	  if in.tracks == nil {
		  in.tracks = map[string]*Track{}
	   }
	  if in.observe == nil {
		  in.observe = map[string]string{}
	  }
	  if in.plans == nil {
		  in.plans = map[string][]PlanHold{}
	  }
	  if in.drifts == nil {
		   in.drifts = map[string]*DriftTicket{}
	  }
	  s.forecast = in.forecast
	  s.history = in.history
	   s.samples = in.samples
	   s.reserves = in.reserves
	   s.journals = in.journals
	   s.tracks = in.tracks
	  s.observe = in.observe
	  s.plans = in.plans
	  s.drifts = in.drifts
	   s.hints = in.hints
	   s.byIdem = map[string]string{}
	   s.driftOpen = map[string]string{}
	   for _, r := range s.reserves {
		   if r.Idem != "" {
			   s.byIdem[r.Idem] = r.ID
		  }
	   }
	  for _, tk := range s.drifts {
		  if tk.State == "open" {
			   s.driftOpen[tk.Asset] = tk.ID
		  }
	  }
   }

  func (s *Service) persistForecast(ctx context.Context, row *HotForecast) error {
	  if s.sql == nil || row == nil {
		   return nil
	  }
	   ids := strings.Join(row.SweepIDs, ",")
	   if err := s.exec(ctx, `DELETE FROM hot_forecast_snapshot WHERE asset = ?`, row.Asset); err != nil {
		   return err
	  }
	  return s.exec(ctx, `INSERT INTO hot_forecast_snapshot (
asset, version, window_sec, as_of, hot_avail, hot_frozen, point_est, lower_bound,
inflow_exp, outflow_exp, inflow_kept, outflow_stressed, sweep_hold, in_transit, buffer_amt,
haircut, stress, inputs_hash, sweep_ids, sample_n, bias, realized, event_id
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		  row.Asset, row.Version, row.WindowSec, row.AsOf, row.HotAvail, row.HotFrozen, row.Point, row.Lower,
		   row.Inflow, row.Outflow, row.InflowKept, row.OutflowStressed, row.SweepHold, row.InTransit, row.Buffer,
		   row.Haircut,
		   row.Stress, row.InputsHash, ids, row.SampleN, row.Bias, row.Realized, row.EventID)
  }

   func (s *Service) persistHistory(ctx context.Context, row *HotForecast) error {
	   if s.sql == nil || row == nil {
		   return nil
	  }
	  return s.exec(ctx, `INSERT INTO hot_forecast_history (asset, version, inputs_hash, lower_bound, point_est, as_of, event_id)
VALUES (?,?,?,?,?,?,?)`, row.Asset, row.Version, row.InputsHash, row.Lower, row.Point, row.AsOf, row.EventID)
  }

   func (s *Service) persistSample(ctx context.Context, sm Sample) error {
	  if s.sql == nil {
		   return nil
	  }
	   return s.exec(ctx, `INSERT INTO hot_forecast_sample (id, asset, at, inflow, outflow) VALUES (?,?,?,?,?)`,
		  sm.ID, sm.Asset, sm.At, sm.Inflow, sm.Outflow)
   }

  func (s *Service) persistReservation(ctx context.Context, row *Reservation) error {
	  if s.sql == nil || row == nil {
		  return nil
	   }
	  if err := s.exec(ctx, `DELETE FROM hot_reserve_order WHERE id = ?`, row.ID); err != nil {
		   return err
	   }
	  return s.exec(ctx, `INSERT INTO hot_reserve_order (
id, idem_key, fingerprint, asset, amount, state, user_id, intent_id, movement_id, approval_ticket_id,
note, expires_at, earmarked_at, consumed_at, released_at, release_kind, requester, approver,
broadcast_ref, freeze_ref, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		   row.ID, row.Idem, row.Fingerprint, row.Asset, row.Amount, string(row.State), row.UserID, row.IntentID, row.MovementID, row.ApprovalTicketID,
		  row.Note, row.ExpiresAt, nullTime(row.EarmarkedAt), nullTime(row.ConsumedAt), nullTime(row.ReleasedAt), row.ReleaseKind, row.Requester, row.Approver,
		  row.BroadcastRef, row.FreezeRef, row.CreatedAt, row.UpdatedAt)
  }

   func (s *Service) persistJournal(ctx context.Context, j ReservationJournal) error {
	  if s.sql == nil {
		   return nil
	  }
	   return s.exec(ctx, `INSERT INTO hot_reserve_journal (
id, reservation_id, asset, seq, kind, debit_subject, credit_subject, amount, event_id, note, at
) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		  j.ID, j.ReservationID, j.Asset, j.Seq, j.Kind, j.Debit, j.Credit, j.Amount, j.EventID, j.Note, j.At)
   }

  func (s *Service) persistTrack(ctx context.Context, tr *Track) error {
	  if s.sql == nil || tr == nil {
		  return nil
	  }
	  if err := s.exec(ctx, `DELETE FROM hot_reserve_track WHERE reservation_id = ? AND kind = ?`, tr.ReservationID, tr.Kind); err != nil {
		   return err
	  }
	   return s.exec(ctx, `INSERT INTO hot_reserve_track (reservation_id, kind, asset, amount, ref, updated_at) VALUES (?,?,?,?,?,?)`,
		   tr.ReservationID, tr.Kind, tr.Asset, tr.Amount, tr.Ref, tr.UpdatedAt)
  }

   func (s *Service) deleteTrack(ctx context.Context, id, kind string) error {
	   return s.exec(ctx, `DELETE FROM hot_reserve_track WHERE reservation_id = ? AND kind = ?`, id, kind)
  }

  func (s *Service) persistObserve(ctx context.Context, asset, kind, amount string, at time.Time) error {
	   if err := s.exec(ctx, `DELETE FROM hot_reserve_observe WHERE asset = ? AND kind = ?`, asset, kind); err != nil {
		  return err
	  }
	   return s.exec(ctx, `INSERT INTO hot_reserve_observe (asset, kind, amount, updated_at) VALUES (?,?,?,?)`, asset, kind, amount, at)
   }

  func (s *Service) persistAlign(ctx context.Context, rep AlignReport) error {
	   drift, blocks := 0, 0
	  if rep.Drift {
		  drift = 1
	  }
	   if rep.Blocks {
		   blocks = 1
	   }
	   return s.exec(ctx, `INSERT INTO hot_reserve_align (
id, asset, as_of, reservation_held, earmarked, consumed, open_amt, freeze_track, broadcast_track,
drift, drift_kind, ticket_id, blocks, note, event_id
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		  rep.ID, rep.Asset, rep.AsOf, rep.Held, rep.Earmarked, rep.Consumed, rep.Open, rep.Freeze, rep.Broadcast,
		   drift, rep.DriftKind, rep.TicketID, blocks, rep.Note, rep.EventID)
  }

   func (s *Service) persistDriftState(ctx context.Context, asset string) error {
	   id := s.driftOpen[asset]
	  if id == "" {
		   for _, tk := range s.drifts {
			  if tk.Asset == asset && tk.State == "closed" {
				   if err := s.writeDrift(ctx, tk); err != nil {
					  return err
				  }
			  }
		  }
		  return nil
	  }
	   tk := s.drifts[id]
	  if tk == nil {
		  return nil
	  }
	  return s.writeDrift(ctx, tk)
   }

  func (s *Service) writeDrift(ctx context.Context, tk *DriftTicket) error {
	  if err := s.exec(ctx, `DELETE FROM hot_reserve_drift WHERE id = ?`, tk.ID); err != nil {
		  return err
	  }
	  return s.exec(ctx, `INSERT INTO hot_reserve_drift (
id, asset, kind, state, reservation_held, freeze_track, broadcast_track, note, report_id, opened_at, closed_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		  tk.ID, tk.Asset, tk.Kind, tk.State, tk.Held, tk.Freeze, tk.Broadcast, tk.Note, tk.ReportID, tk.OpenedAt, nullTime(tk.ClosedAt))
  }

  func (s *Service) replacePlans(ctx context.Context, asset string, plans []PlanHold) error {
	  if err := s.exec(ctx, `DELETE FROM hot_reserve_plan WHERE asset = ?`, asset); err != nil {
		  return err
	   }
	   for _, p := range plans {
		   if err := s.exec(ctx, `INSERT INTO hot_reserve_plan (id, asset, amount, noted_at) VALUES (?,?,?,?)`, p.ID, p.Asset, p.Amount, p.Noted); err != nil {
			   return err
		  }
	  }
	  return nil
   }

   func (s *Service) persistHint(ctx context.Context, h SweepHint) error {
	   return s.exec(ctx, `INSERT INTO hot_reserve_hint (id, asset, amount, reason, at) VALUES (?,?,?,?,?)`,
		  h.PlanID, h.Asset, h.Amount, h.Reason, h.At)
  }

  func nullTime(t time.Time) any {
	   if t.IsZero() {
		  return nil
	  }
	   return t.UTC()
   }

type sqlTime struct{ T time.Time }

   func (t *sqlTime) Scan(src any) error {
	  if t == nil {
		  return fmt.Errorf("nil time")
	  }
	  switch v := src.(type) {
	  case nil:
		  t.T = time.Time{}
		   return nil
	  case time.Time:
		  t.T = v.UTC()
		  return nil
	  case []byte:
		   return t.parse(string(v))
	   case string:
		  return t.parse(v)
	   default:
		  return fmt.Errorf("hot reserve time")
	  }
   }

  func (t *sqlTime) parse(s string) error {
	   s = strings.TrimSpace(s)
	  if s == "" {
		  t.T = time.Time{}
		  return nil
	   }
	   for _, layout := range []string{"2006-01-02 15:04:05.000", "2006-01-02 15:04:05", time.RFC3339Nano, time.RFC3339} {
		  if got, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			   t.T = got.UTC()
			  return nil
		  }
	  }
	   return fmt.Errorf("hot reserve time %q", s)
  }

   func loadAll(ctx context.Context, db *persist.SQLDB) (loaded, error) {
	  var out loaded
	   var err error
	   if out.forecast, err = loadForecasts(ctx, db); err != nil {
		  return loaded{}, err
	   }
	  if out.history, err = loadHistory(ctx, db); err != nil {
		  return loaded{}, err
	   }
	   if out.samples, err = loadSamples(ctx, db); err != nil {
		  return loaded{}, err
	   }
	   if out.reserves, err = loadReservations(ctx, db); err != nil {
		   return loaded{}, err
	  }
	   if out.journals, err = loadJournals(ctx, db); err != nil {
		  return loaded{}, err
	  }
	  if out.tracks, err = loadTracks(ctx, db); err != nil {
		  return loaded{}, err
	  }
	   if out.observe, err = loadObserve(ctx, db); err != nil {
		  return loaded{}, err
	   }
	  if out.plans, err = loadPlans(ctx, db); err != nil {
		  return loaded{}, err
	   }
	  if out.drifts, err = loadDrifts(ctx, db); err != nil {
		   return loaded{}, err
	   }
	   if out.hints, err = loadHints(ctx, db); err != nil {
		  return loaded{}, err
	   }
	  return out, nil
  }

   func query(ctx context.Context, db *persist.SQLDB, q string, args ...any) (*sql.Rows, error) {
	  return db.Conn(ctx).QueryContext(ctx, q, args...)
   }

   func loadForecasts(ctx context.Context, db *persist.SQLDB) (map[string]*HotForecast, error) {
	  rows, err := query(ctx, db, `SELECT asset, version, window_sec, as_of, hot_avail, hot_frozen, point_est, lower_bound,
inflow_exp, outflow_exp, inflow_kept, outflow_stressed, sweep_hold, in_transit, buffer_amt,
haircut, stress, inputs_hash, sweep_ids, sample_n, bias, realized, event_id FROM hot_forecast_snapshot`)
	   if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   out := map[string]*HotForecast{}
	  for rows.Next() {
		   var row HotForecast
		  var at sqlTime
		   var ids string
		  if err := rows.Scan(&row.Asset, &row.Version, &row.WindowSec, &at, &row.HotAvail, &row.HotFrozen, &row.Point, &row.Lower,
			   &row.Inflow, &row.Outflow, &row.InflowKept, &row.OutflowStressed, &row.SweepHold, &row.InTransit, &row.Buffer,
			   &row.Haircut, &row.Stress, &row.InputsHash, &ids, &row.SampleN, &row.Bias, &row.Realized, &row.EventID); err != nil {
			   return nil, err
		  }
		  row.AsOf = at.T
		   if ids != "" {
			   row.SweepIDs = strings.Split(ids, ",")
		   }
		   cp := row
		   out[row.Asset] = &cp
	  }
	  return out, rows.Err()
  }

   func loadHistory(ctx context.Context, db *persist.SQLDB) (map[string][]ForecastHead, error) {
	   rows, err := query(ctx, db, `SELECT asset, version, inputs_hash, lower_bound, point_est, as_of, event_id FROM hot_forecast_history ORDER BY asset, version`)
	   if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	  out := map[string][]ForecastHead{}
	  for rows.Next() {
		  var h ForecastHead
		  var at sqlTime
		   if err := rows.Scan(&h.Asset, &h.Version, &h.Hash, &h.Lower, &h.Point, &at, &h.EventID); err != nil {
			   return nil, err
		   }
		   h.AsOf = at.T
		  out[h.Asset] = append(out[h.Asset], h)
	  }
	   return out, rows.Err()
  }

   func loadSamples(ctx context.Context, db *persist.SQLDB) (map[string][]Sample, error) {
	  rows, err := query(ctx, db, `SELECT id, asset, at, inflow, outflow FROM hot_forecast_sample ORDER BY at`)
	   if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  out := map[string][]Sample{}
	   for rows.Next() {
		  var sm Sample
		  var at sqlTime
		   if err := rows.Scan(&sm.ID, &sm.Asset, &at, &sm.Inflow, &sm.Outflow); err != nil {
			   return nil, err
		   }
		   sm.At = at.T
		  out[sm.Asset] = append(out[sm.Asset], sm)
	   }
	  return out, rows.Err()
   }

   func loadReservations(ctx context.Context, db *persist.SQLDB) (map[string]*Reservation, error) {
	   rows, err := query(ctx, db, `SELECT id, idem_key, fingerprint, asset, amount, state, user_id, intent_id, movement_id, approval_ticket_id,
note, expires_at, earmarked_at, consumed_at, released_at, release_kind, requester, approver, broadcast_ref, freeze_ref, created_at, updated_at
FROM hot_reserve_order`)
	  if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   out := map[string]*Reservation{}
	   for rows.Next() {
		  var r Reservation
		   var st string
		  var exp, ear, con, rel, cre, upd sqlTime
		   if err := rows.Scan(&r.ID, &r.Idem, &r.Fingerprint, &r.Asset, &r.Amount, &st, &r.UserID, &r.IntentID, &r.MovementID, &r.ApprovalTicketID,
			  &r.Note, &exp, &ear, &con, &rel, &r.ReleaseKind, &r.Requester, &r.Approver, &r.BroadcastRef, &r.FreezeRef, &cre, &upd); err != nil {
			   return nil, err
		   }
		  r.State = State(st)
		   r.ExpiresAt, r.EarmarkedAt, r.ConsumedAt, r.ReleasedAt = exp.T, ear.T, con.T, rel.T
		   r.CreatedAt, r.UpdatedAt = cre.T, upd.T
		   cp := r
		  out[r.ID] = &cp
	  }
	  return out, rows.Err()
  }

   func loadJournals(ctx context.Context, db *persist.SQLDB) (map[string][]ReservationJournal, error) {
	   rows, err := query(ctx, db, `SELECT id, reservation_id, asset, seq, kind, debit_subject, credit_subject, amount, event_id, note, at
FROM hot_reserve_journal ORDER BY reservation_id, seq`)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   out := map[string][]ReservationJournal{}
	   for rows.Next() {
		  var j ReservationJournal
		  var at sqlTime
		  if err := rows.Scan(&j.ID, &j.ReservationID, &j.Asset, &j.Seq, &j.Kind, &j.Debit, &j.Credit, &j.Amount, &j.EventID, &j.Note, &at); err != nil {
			   return nil, err
		   }
		   j.At = at.T
		  out[j.ReservationID] = append(out[j.ReservationID], j)
	   }
	   return out, rows.Err()
  }

  func loadTracks(ctx context.Context, db *persist.SQLDB) (map[string]*Track, error) {
	   rows, err := query(ctx, db, `SELECT reservation_id, kind, asset, amount, ref, updated_at FROM hot_reserve_track`)
	   if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   out := map[string]*Track{}
	   for rows.Next() {
		   var tr Track
		  var at sqlTime
		  if err := rows.Scan(&tr.ReservationID, &tr.Kind, &tr.Asset, &tr.Amount, &tr.Ref, &at); err != nil {
			  return nil, err
		  }
		  tr.UpdatedAt = at.T
		   cp := tr
		   out[trackKey(tr.ReservationID,
			  tr.Kind)] = &cp
	  }
	   return out, rows.Err()
   }

   func loadObserve(ctx context.Context, db *persist.SQLDB) (map[string]string, error) {
	  rows, err := query(ctx, db, `SELECT asset, kind, amount FROM hot_reserve_observe`)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  out := map[string]string{}
	   for rows.Next() {
		   var asset, kind, amount string
		  if err := rows.Scan(&asset, &kind, &amount); err != nil {
			   return nil, err
		  }
		   out[obsKey(asset, kind)] = amount
	  }
	  return out, rows.Err()
  }

  func loadPlans(ctx context.Context, db *persist.SQLDB) (map[string][]PlanHold, error) {
	   rows, err := query(ctx, db, `SELECT id, asset, amount, noted_at FROM hot_reserve_plan ORDER BY asset, id`)
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   out := map[string][]PlanHold{}
	   for rows.Next() {
		   var p PlanHold
		   var at sqlTime
		   if err := rows.Scan(&p.ID, &p.Asset, &p.Amount, &at); err != nil {
			  return nil, err
		   }
		  p.Noted = at.T
		   out[p.Asset] = append(out[p.Asset], p)
	   }
	   return out, rows.Err()
   }

   func loadDrifts(ctx context.Context, db *persist.SQLDB) (map[string]*DriftTicket, error) {
	   rows, err := query(ctx, db, `SELECT id, asset, kind, state, reservation_held, freeze_track, broadcast_track, note, report_id, opened_at, closed_at FROM hot_reserve_drift`)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  out := map[string]*DriftTicket{}
	   for rows.Next() {
		   var tk DriftTicket
		  var opened, closed sqlTime
		  if err := rows.Scan(&tk.ID, &tk.Asset, &tk.Kind, &tk.State, &tk.Held, &tk.Freeze, &tk.Broadcast, &tk.Note, &tk.ReportID, &opened, &closed); err != nil {
			  return nil, err
		   }
		  tk.OpenedAt, tk.ClosedAt = opened.T, closed.T
		  cp := tk
		  out[tk.ID] = &cp
	   }
	  return out, rows.Err()
  }

   func loadHints(ctx context.Context, db *persist.SQLDB) ([]SweepHint, error) {
	  rows, err := query(ctx, db, `SELECT id, asset, amount, reason, at FROM hot_reserve_hint ORDER BY at`)
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   out := make([]SweepHint, 0)
	   for rows.Next() {
		   var h SweepHint
		   var at sqlTime
		   if err := rows.Scan(&h.PlanID, &h.Asset, &h.Amount, &h.Reason, &at); err != nil {
			  return nil, err
		  }
		  h.At = at.T
		   out = append(out, h)
	  }
	  return out, rows.Err()
  }
