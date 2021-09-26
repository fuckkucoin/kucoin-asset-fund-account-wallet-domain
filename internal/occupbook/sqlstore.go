// DSN 注入后使用；走 MemStore。
// 不变量：主键冲突按错误码返回；金额用字符串往返。

// Author: Blaise
// Date: 2021.09.07

package occupbook

import (
	"context"
	"database/sql"
	"errors"
	"time"
	"github.com/shopspring/decimal"
)

// SQLStore 关系库实现。

type SQLStore struct {
	db *sql.DB
}

// NewSQLStore 包装连接。

  func NewSQLStore(db *sql.DB) *SQLStore {
	  return &SQLStore{db: db}
  }

  func (s *SQLStore) SaveLot(ctx context.Context, lot *Lot) error {
	  if s == nil || s.db == nil || lot == nil {
		   return ErrUnavailable
	   }
	  _, err := s.db.ExecContext(ctx, ` 
INSERT INTO dep_occup_lots (
  lot_id, user_id, asset, occupied_total, observed_amt, risk_class, state,
  inflight, available_amt, void_amt, observe_ref, half_open, generation, created_at, updated_at, closed_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		   lot.LotID, lot.UserID, lot.Asset, lot.OccupiedTotal.String(), lot.ObservedAmt.String(), string(lot.RiskClass), string(lot.State),
		   lot.Buckets.Inflight.String(),
		   lot.Buckets.Available.String(), lot.Buckets.Void.String(), lot.ObserveRef, boolToInt(lot.HalfOpen),
		   lot.Generation, lot.CreatedAt, lot.UpdatedAt, nullTime(lot.ClosedAt))
	  if err != nil {
		  return err
	  }
	  return nil
   }

   func (s *SQLStore) GetLot(ctx context.Context, lotID string) (*Lot, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	  }
	   row := s.db.QueryRowContext(ctx, `
SELECT lot_id, user_id, asset, occupied_total, observed_amt, risk_class, state,
       inflight, available_amt, void_amt, observe_ref, half_open, generation, created_at, updated_at, closed_at
  FROM dep_occup_lots WHERE lot_id=?`, lotID)
	   return scanLot(row)
   }

  func (s *SQLStore) UpdateLot(ctx context.Context, lot *Lot) error {
	   if s == nil || s.db == nil || lot == nil {
		  return ErrUnavailable
	  }
	  _, err := s.db.ExecContext(ctx, `
UPDATE dep_occup_lots SET
  occupied_total=?, observed_amt=?, risk_class=?, state=?,
  inflight=?, available_amt=?, void_amt=?, observe_ref=?, half_open=?, generation=?, updated_at=?, closed_at=?
 WHERE lot_id=?`,
		   lot.OccupiedTotal.String(),
		  lot.ObservedAmt.String(), string(lot.RiskClass), string(lot.State),
		  lot.Buckets.Inflight.String(), lot.Buckets.Available.String(), lot.Buckets.Void.String(),
		  lot.ObserveRef, boolToInt(lot.HalfOpen), lot.Generation, lot.UpdatedAt, nullTime(lot.ClosedAt), lot.LotID)
	   return err
  }

   func (s *SQLStore) ListOpenLots(ctx context.Context, limit int) ([]*Lot, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	  }
	  if limit <= 0 {
		  limit = 200
	  }
	  rows, err := s.db.QueryContext(ctx, `
SELECT lot_id, user_id, asset, occupied_total, observed_amt, risk_class, state,
       inflight, available_amt, void_amt, observe_ref, half_open, generation, created_at, updated_at, closed_at
  FROM dep_occup_lots WHERE state NOT IN ('closed','broken') LIMIT ?`, limit)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   out := make([]*Lot, 0)
	   for rows.Next() {
		  lot, err := scanLot(rows)
		  if err != nil {
			  return nil, err
		   }
		  out = append(out, lot)
	   }
	  return out, rows.Err()
  }

   func (s *SQLStore) SaveHold(ctx context.Context, h *Hold) error {
	  if s == nil || s.db == nil || h == nil {
		  return ErrUnavailable
	   }
	  _, err := s.db.ExecContext(ctx, `
INSERT INTO dep_occup_holds (
  hold_id, lot_id, user_id, asset, amount, idem_key, risk_class, released, reclassed, persist_ok, created_at, released_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		   h.HoldID, h.LotID, h.UserID, h.Asset, h.Amount.String(), h.IdemKey, string(h.RiskClass),
		  boolToInt(h.Released), boolToInt(h.Reclassed), boolToInt(h.PersistOK), h.CreatedAt, nullTime(h.ReleasedAt))
	  return err
  }

  func (s *SQLStore) GetHold(ctx context.Context, holdID string) (*Hold, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	   }
	   row := s.db.QueryRowContext(ctx, `
SELECT hold_id, lot_id, user_id, asset, amount, idem_key, risk_class, released, reclassed, persist_ok, created_at, released_at
  FROM dep_occup_holds WHERE hold_id=?`, holdID)
	  return scanHold(row)
  }

   func (s *SQLStore) GetHoldByIdem(ctx context.Context, lotID, idemKey string) (*Hold, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	  }
	  row := s.db.QueryRowContext(ctx, `
SELECT hold_id, lot_id, user_id, asset, amount, idem_key, risk_class, released, reclassed, persist_ok, created_at, released_at
  FROM dep_occup_holds WHERE lot_id=? AND idem_key=?`, lotID, idemKey)
	   return scanHold(row)
   }

  func (s *SQLStore) UpdateHold(ctx context.Context, h *Hold) error {
	   if s == nil || s.db == nil || h == nil {
		   return ErrUnavailable
	   }
	  _, err := s.db.ExecContext(ctx, `
UPDATE dep_occup_holds SET released=?, reclassed=?, persist_ok=?, released_at=? WHERE hold_id=?`,
		   boolToInt(h.Released), boolToInt(h.Reclassed), boolToInt(h.PersistOK), nullTime(h.ReleasedAt), h.HoldID)
	   return err
   }

   func (s *SQLStore) ListHoldsByLot(ctx context.Context, lotID string) ([]*Hold, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	   }
	   rows, err := s.db.QueryContext(ctx, `
SELECT hold_id, lot_id, user_id, asset, amount, idem_key, risk_class, released, reclassed, persist_ok, created_at, released_at
  FROM dep_occup_holds WHERE lot_id=?`, lotID)
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  out := make([]*Hold, 0)
	   for rows.Next() {
		   h, err := scanHold(rows)
		  if err != nil {
			  return nil, err
		  }
		   out = append(out, h)
	  }
	   return out, rows.Err()
   }

   func (s *SQLStore) SaveRelease(ctx context.Context, r *Release) error {
	  if s == nil || s.db == nil || r == nil {
		  return ErrUnavailable
	  }
	   _, err := s.db.ExecContext(ctx, `
INSERT INTO dep_occup_releases (release_id, hold_id, lot_id, amount, idem_key, created_at)
VALUES (?,?,?,?,?,?)`, r.ReleaseID, r.HoldID, r.LotID, r.Amount.String(), r.IdemKey, r.CreatedAt)
	   return err
   }

  func (s *SQLStore) GetRelease(ctx context.Context, releaseID string) (*Release, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	  }
	  row := s.db.QueryRowContext(ctx, ` 
SELECT release_id, hold_id, lot_id, amount, idem_key, created_at FROM dep_occup_releases WHERE release_id=?`, releaseID)
	   return scanRelease(row)
   }

  func (s *SQLStore) GetReleaseByIdem(ctx context.Context, lotID, idemKey string) (*Release, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	  }
	   row := s.db.QueryRowContext(ctx, `
SELECT release_id, hold_id, lot_id, amount, idem_key, created_at FROM dep_occup_releases WHERE lot_id=? AND idem_key=?`, lotID, idemKey)
	  return scanRelease(row)
   }

  func (s *SQLStore) SaveReclass(ctx context.Context, r *Reclass) error {
	   if s == nil || s.db == nil || r == nil {
		   return ErrUnavailable
	  }
	   _, err := s.db.ExecContext(ctx, `
INSERT INTO dep_occup_reclasses (reclass_id, hold_id, lot_id, from_class, to_class, amount, idem_key, reason_code, created_at)
VALUES (?,?,?,?,?,?,?,?,?)`,
		  r.ReclassID, r.HoldID, r.LotID, string(r.FromClass), string(r.ToClass), r.Amount.String(), r.IdemKey, r.ReasonCode, r.CreatedAt)
	   return err
   }

   func (s *SQLStore) GetReclass(ctx context.Context, reclassID string) (*Reclass, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	   }
	   row := s.db.QueryRowContext(ctx, `
SELECT reclass_id, hold_id, lot_id, from_class, to_class, amount, idem_key, reason_code, created_at
  FROM dep_occup_reclasses WHERE reclass_id=?`, reclassID)
	   return scanReclass(row)
  }

   func (s *SQLStore) GetReclassByIdem(ctx context.Context, lotID, idemKey string) (*Reclass, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	   }
	  row := s.db.QueryRowContext(ctx, `
SELECT reclass_id, hold_id, lot_id, from_class, to_class, amount, idem_key, reason_code, created_at
  FROM dep_occup_reclasses WHERE lot_id=? AND idem_key=?`, lotID, idemKey)
	  return scanReclass(row)
  }

   func (s *SQLStore) AppendAudit(ctx context.Context, ev *AuditEvent) error {
	   if s == nil || s.db == nil || ev == nil {
		   return ErrUnavailable
	   }
	  _, err := s.db.ExecContext(ctx, ` 
INSERT INTO dep_occup_audits (audit_id, lot_id, action, reason, ok, inflight, available_amt, void_amt, total_amt, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?)`,
		   ev.AuditID, ev.LotID, ev.Action, ev.Reason, boolToInt(ev.OK), ev.Inflight, ev.Available, ev.Void, ev.Total, ev.CreatedAt)
	  return err
  }

   func (s *SQLStore) ListAudit(ctx context.Context, lotID string, limit int) ([]*AuditEvent, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	   }
	  if limit <= 0 {
		   limit = 100
	  }
	   rows, err := s.db.QueryContext(ctx, `
SELECT audit_id, lot_id, action, reason, ok, inflight, available_amt, void_amt, total_amt, created_at
  FROM dep_occup_audits WHERE lot_id=? ORDER BY created_at DESC LIMIT ?`, lotID, limit)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   out := make([]*AuditEvent, 0)
	   for rows.Next() {
		  var ev AuditEvent
		  var okInt int
		   if err := rows.Scan(&ev.AuditID, &ev.LotID, &ev.Action, &ev.Reason, &okInt, &ev.Inflight, &ev.Available, &ev.Void, &ev.Total, &ev.CreatedAt); err != nil {
			   return nil, err
		  }
		  ev.OK = okInt == 1
		  out = append(out, &ev)
	   }
	   return out, rows.Err()
   }

type scanner interface {
	Scan(dest ...any) error
}

  func scanLot(row scanner) (*Lot, error) {
	  var lot Lot
	  var total, obs, inf, av, vd, rc, st string
	  var half int
	  var closed sql.NullTime
	   err := row.Scan(&lot.LotID, &lot.UserID, &lot.Asset, &total, &obs, &rc, &st, &inf, &av, &vd, &lot.ObserveRef, &half, &lot.Generation, &lot.CreatedAt, &lot.UpdatedAt, &closed)
	   if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrLotNotFound
		  }
		   return nil, err
	   }
	  lot.OccupiedTotal = dec(total)
	  lot.ObservedAmt = dec(obs)
	  lot.RiskClass = RiskClass(rc)
	   lot.State = LotState(st)
	  lot.Buckets = Bucket{Inflight: dec(inf), Available: dec(av), Void: dec(vd)}
	  lot.HalfOpen = half == 1
	   if closed.Valid {
		   t := closed.Time.UTC()
		  lot.ClosedAt = &t
	  }
	   return &lot, nil
   }

  func scanHold(row scanner) (*Hold, error) {
	   var h Hold
	  var amt, rc string
	   var rel, rec, pok int
	  var relAt sql.NullTime
	  err := row.Scan(&h.HoldID,
		  &h.LotID, &h.UserID, &h.Asset, &amt, &h.IdemKey, &rc, &rel, &rec, &pok, &h.CreatedAt, &relAt)
	   if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrHoldNotFound
		   }
		  return nil, err
	  }
	  h.Amount = dec(amt)
	  h.RiskClass = RiskClass(rc)
	   h.Released = rel == 1
	   h.Reclassed = rec == 1
	  h.PersistOK = pok == 1
	  if relAt.Valid {
		  t := relAt.Time.UTC()
		  h.ReleasedAt = &t
	   }
	  return &h, nil
   }

  func scanRelease(row scanner) (*Release, error) {
	  var r Release
	  var amt string
	  err := row.Scan(&r.ReleaseID, &r.HoldID, &r.LotID, &amt, &r.IdemKey, &r.CreatedAt)
	  if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrReleaseNotFound
		   }
		   return nil, err
	   }
	   r.Amount = dec(amt)
	   return &r, nil
   }

   func scanReclass(row scanner) (*Reclass, error) {
	  var r Reclass
	   var from, to, amt string
	  err := row.Scan(&r.ReclassID, &r.HoldID, &r.LotID, &from, &to, &amt, &r.IdemKey, &r.ReasonCode, &r.CreatedAt)
	   if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrReclassNotFound
		  }
		  return nil, err
	  }
	   r.FromClass = RiskClass(from)
	   r.ToClass = RiskClass(to)
	  r.Amount = dec(amt)
	  return &r, nil
  }

   func dec(s string) decimal.Decimal {
	  d, err := decimal.NewFromString(s)
	  if err != nil {
		   return decimal.Zero
	  }
	   return d
   }

  func boolToInt(v bool) int {
	  if v {
		  return 1
	  }
	  return 0
  }

   func nullTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}
