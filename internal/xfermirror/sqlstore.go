// 表结构见 migrations/0089_aa_xfer_halfhop.sql（aa_mirror_views, aa_mirror_events）。

// Author: Talon
// Date: 2021.12.30

package xfermirror

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/shopspring/decimal"
)

// SQLMirrorStore SQL 实现，满足 MirrorStore 接口。

type SQLMirrorStore struct {
	db *persist.SQLDB
}

// NewSQLMirrorStore 构造 SQL 镜像仓储。

  func NewSQLMirrorStore(db *persist.SQLDB) *SQLMirrorStore {
	   return &SQLMirrorStore{db: db}
  }

   func (s *SQLMirrorStore) Put(ctx context.Context, m MirrorView) error {
	  now := m.CreatedAt.UTC().Format(time.RFC3339Nano)
	  q := s.db.Conn(ctx)
	   _, err := q.ExecContext(ctx,
		   `INSERT INTO aa_mirror_views
		 (id, hop_id, initiator_user_id, counterparty_user_id, asset, amount,
		  from_book, to_book, state, state_reason, closeout_plan_id,
		  entry_ref, version, override_actor, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		  m.ID, m.HopID, m.InitiatorUserID, m.CounterpartyUserID, m.Asset,
		   m.Amount.String(), m.FromBook, m.ToBook, string(m.State),
		   m.StateReason, m.CloseoutPlanID, m.EntryRef, m.Version,
		  m.OverrideActor, now, now,
	  )
	  if err != nil && isDuplicateMirror(err) {
		   return ErrConflict
	  }
	   return err
   }

  func (s *SQLMirrorStore) Update(ctx context.Context, m MirrorView) error {
	  now := m.UpdatedAt.UTC().Format(time.RFC3339Nano)
	  q := s.db.Conn(ctx)
	  res, err := q.ExecContext(ctx,
		  `UPDATE aa_mirror_views SET
		 state=?, state_reason=?, closeout_plan_id=?, entry_ref=?,
		 version=?, override_actor=?, updated_at=?
		 WHERE id=? AND version=?`,
		   string(m.State), m.StateReason, m.CloseoutPlanID, m.EntryRef,
		   m.Version, m.OverrideActor, now,
		   m.ID, m.Version-1,
	   )
	   if err != nil {
		   return err
	  }
	   n, _ := res.RowsAffected()
	  if n == 0 {
		   return ErrStaleFence
	   }
	  return nil
   }

   func (s *SQLMirrorStore) GetByHopID(ctx context.Context, hopID string) (MirrorView, error) {
	   q := s.db.Conn(ctx)
	  row := q.QueryRowContext(ctx,
		  `SELECT id, hop_id, initiator_user_id, counterparty_user_id, asset, amount,
		 from_book, to_book, state, state_reason, closeout_plan_id,
		 entry_ref, version, override_actor, created_at, updated_at
		 FROM aa_mirror_views WHERE hop_id=?`, hopID)
	  return scanMirror(row)
   }

   func (s *SQLMirrorStore) Get(ctx context.Context, id string) (MirrorView, error) {
	  q := s.db.Conn(ctx)
	  row := q.QueryRowContext(ctx,
		   `SELECT id, hop_id, initiator_user_id, counterparty_user_id, asset, amount,
		 from_book, to_book, state, state_reason, closeout_plan_id,
		 entry_ref, version, override_actor, created_at, updated_at
		 FROM aa_mirror_views WHERE id=?`, id)
	  return scanMirror(row)
  }

   func (s *SQLMirrorStore) List(ctx context.Context, mq MirrorQuery) ([]MirrorView, error) {
	   if mq.Limit <= 0 || mq.Limit > 500 {
		   mq.Limit = 100
	  }
	  var (
		  args  []any
		  where []string
	  )
	   if mq.State != "" {
		  where = append(where, "state=?")
		  args = append(args, string(mq.State))
	  }
	   if mq.Asset != "" {
		  where = append(where, "asset=?")
		  args = append(args, strings.ToUpper(mq.Asset))
	  }
	   if mq.UserID != "" {
		  where = append(where, "(initiator_user_id=? OR counterparty_user_id=?)")
		   args = append(args, mq.UserID, mq.UserID)
	  }
	   wClause := ""
	   if len(where) > 0 {
		   wClause = "WHERE " + strings.Join(where, " AND ")
	  }
	  args = append(args, mq.Limit, mq.Offset)
	  qStr := `SELECT id, hop_id, initiator_user_id, counterparty_user_id, asset, amount,
		 from_book, to_book, state, state_reason, closeout_plan_id,
		 entry_ref, version, override_actor, created_at, updated_at
		 FROM aa_mirror_views ` + wClause + ` ORDER BY created_at ASC LIMIT ? OFFSET ?`
	   dbq := s.db.Conn(ctx)
	  rows, err := dbq.QueryContext(ctx, qStr, args...)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  var out []MirrorView
	  for rows.Next() {
		  m, err2 := scanMirror(rows)
		   if err2 != nil {
			  continue
		   }
		  out = append(out, m)
	  }
	   return out, rows.Err()
  }

   func (s *SQLMirrorStore) PutEvent(ctx context.Context, e MirrorEvent) error {
	  at := e.At.UTC().Format(time.RFC3339Nano)
	   q := s.db.Conn(ctx)
	  _, err := q.ExecContext(ctx,
		   `INSERT INTO aa_mirror_events
		 (id, mirror_id, hop_id, from_state, to_state, reason, actor, at)
		 VALUES (?,?,?,?,?,?,?,?)`,
		   e.ID, e.MirrorID, e.HopID, string(e.FromState), string(e.ToState),
		   e.Reason, e.Actor, at,
	  )
	   return err
   }

   func (s *SQLMirrorStore) ListEvents(ctx context.Context, mirrorID string) ([]MirrorEvent, error) {
	  q := s.db.Conn(ctx)
	  rows, err := q.QueryContext(ctx,
		   `SELECT id, mirror_id, hop_id, from_state, to_state, reason, actor, at
		 FROM aa_mirror_events WHERE mirror_id=? ORDER BY at ASC`, mirrorID)
	  if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   var out []MirrorEvent
	  for rows.Next() {
		   var e MirrorEvent
		  var atStr string
		  if err2 := rows.Scan(&e.ID, &e.MirrorID, &e.HopID, &e.FromState, &e.ToState,
			   &e.Reason, &e.Actor, &atStr); err2 != nil {
			  continue
		  }
		   e.At, _ = time.Parse(time.RFC3339Nano, atStr)
		  out = append(out, e)
	   }
	  return out, rows.Err()
   }

// ── scan helpers ─────────────────────────────────────────────────────────────

type mirrorScanner interface {
	Scan(dest ...any) error
}

   func scanMirror(row mirrorScanner) (MirrorView, error) {
	  var (
		  m       MirrorView
		  amtStr  string
		   created string
		  updated string
	   )
	  err := row.Scan(
		  &m.ID, &m.HopID, &m.InitiatorUserID, &m.CounterpartyUserID,
		   &m.Asset, &amtStr, &m.FromBook, &m.ToBook,
		  &m.State, &m.StateReason, &m.CloseoutPlanID,
		   &m.EntryRef, &m.Version, &m.OverrideActor,
		   &created, &updated,
	  )
	   if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return MirrorView{}, ErrNotFound
		   }
		  return MirrorView{}, err
	  }
	   m.Amount, _ = decimal.NewFromString(amtStr)
	   m.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	   m.UpdatedAt,
		  _ = time.Parse(time.RFC3339Nano, updated)
	  return m, nil
  }

   func isDuplicateMirror(err error) bool {
	   if err == nil {
		  return false
	   }
	   s := err.Error()
	  return strings.Contains(s, "Duplicate entry") ||
		  strings.Contains(s, "duplicate key") ||
		   strings.Contains(s, "UNIQUE constraint")
   }
