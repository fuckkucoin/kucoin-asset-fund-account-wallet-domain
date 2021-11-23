package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

type TicketRow struct {
	ID     string
	Kind   string
	UserID string
	Chain string
	Hash       string
	TransferID string
	Book string
	Asset string
	Note  string
	State string
	OpenedBy string
	Assignee string
	AckedBy  string
	ClosedBy  string
	Fence     persist.Fence
	CreatedAt time.Time
	UpdatedAt time.Time
}

type DepTicket struct{ db *persist.SQLDB }
type WDTicket struct{ db *persist.SQLDB }
type ReconTicket struct{ db *persist.SQLDB }

  func NewDepTicket(db *persist.SQLDB) *DepTicket { return &DepTicket{db: db} }

  func NewWDTicket(db *persist.SQLDB) *WDTicket       { return &WDTicket{db: db} }
   func NewReconTicket(db *persist.SQLDB) *ReconTicket { return &ReconTicket{db: db} }

   func (r *DepTicket) Put(ctx context.Context, row TicketRow) error {
	  if !sqlOn(r.db) {
		   return ErrUnavailable
	  }
	   return upsertTicket(ctx, r.db, "aa_dep_tickets",
		   `INSERT INTO aa_dep_tickets (id, kind, user_id, chain, asset, tx_hash, note, state, opened_by, assignee, closed_by, fence_epoch, fence_token, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		   `INSERT INTO aa_dep_tickets (id, kind, user_id, chain, asset, tx_hash, note, state, opened_by, assignee, closed_by, fence_epoch, fence_token, created_at, updated_at) 
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE kind=VALUES(kind), user_id=VALUES(user_id), chain=VALUES(chain), asset=VALUES(asset), tx_hash=VALUES(tx_hash), note=VALUES(note), state=VALUES(state), opened_by=VALUES(opened_by), assignee=VALUES(assignee), closed_by=VALUES(closed_by), fence_epoch=VALUES(fence_epoch), fence_token=VALUES(fence_token), updated_at=VALUES(updated_at)`,
		  `INSERT INTO aa_dep_tickets (id, kind, user_id, chain, asset, tx_hash, note, state, opened_by, assignee, closed_by, fence_epoch, fence_token, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET kind=EXCLUDED.kind, user_id=EXCLUDED.user_id, chain=EXCLUDED.chain, asset=EXCLUDED.asset, tx_hash=EXCLUDED.tx_hash, note=EXCLUDED.note, state=EXCLUDED.state, opened_by=EXCLUDED.opened_by, assignee=EXCLUDED.assignee, closed_by=EXCLUDED.closed_by, fence_epoch=EXCLUDED.fence_epoch, fence_token=EXCLUDED.fence_token, updated_at=EXCLUDED.updated_at`,
		  row.ID, row.Kind, row.UserID, row.Chain, row.Asset, row.Hash, row.Note, row.State, row.OpenedBy, row.Assignee, row.ClosedBy, row.Fence.Epoch, row.Fence.Token, row.CreatedAt, row.UpdatedAt)
  }

  func (r *WDTicket) Put(ctx context.Context, row TicketRow) error {
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	   }
	  return upsertTicket(ctx, r.db, "aa_wd_tickets",
		  `INSERT INTO aa_wd_tickets (id, kind, user_id, transfer_id, note, state, opened_by, assignee, closed_by, fence_epoch, fence_token, created_at, updated_at) 
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		  `INSERT INTO aa_wd_tickets (id, kind, user_id, transfer_id, note, state, opened_by, assignee, closed_by, fence_epoch, fence_token, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE kind=VALUES(kind), user_id=VALUES(user_id), transfer_id=VALUES(transfer_id), note=VALUES(note), state=VALUES(state), opened_by=VALUES(opened_by), assignee=VALUES(assignee), closed_by=VALUES(closed_by), fence_epoch=VALUES(fence_epoch), fence_token=VALUES(fence_token), updated_at=VALUES(updated_at)`,
		   `INSERT INTO aa_wd_tickets (id, kind, user_id, transfer_id, note, state, opened_by, assignee, closed_by, fence_epoch, fence_token, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET kind=EXCLUDED.kind, user_id=EXCLUDED.user_id, transfer_id=EXCLUDED.transfer_id, note=EXCLUDED.note, state=EXCLUDED.state, opened_by=EXCLUDED.opened_by, assignee=EXCLUDED.assignee, closed_by=EXCLUDED.closed_by, fence_epoch=EXCLUDED.fence_epoch, fence_token=EXCLUDED.fence_token, updated_at=EXCLUDED.updated_at`,
		  row.ID, row.Kind, row.UserID, row.TransferID, row.Note, row.State, row.OpenedBy, row.Assignee, row.ClosedBy, row.Fence.Epoch, row.Fence.Token, row.CreatedAt, row.UpdatedAt)
   }

   func (r *ReconTicket) Put(ctx context.Context, row TicketRow) error {
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	   return upsertTicket(ctx, r.db, "aa_recon_tickets",
		   `INSERT INTO aa_recon_tickets (id, user_id, book, asset, note, state, opened_by, assignee, acked_by, closed_by, fence_epoch, fence_token, created_at, updated_at) 
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		   `INSERT INTO aa_recon_tickets (id, user_id, book, asset, note, state, opened_by, assignee, acked_by, closed_by, fence_epoch, fence_token, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE user_id=VALUES(user_id), book=VALUES(book), asset=VALUES(asset), note=VALUES(note), state=VALUES(state), opened_by=VALUES(opened_by), assignee=VALUES(assignee), acked_by=VALUES(acked_by), closed_by=VALUES(closed_by), fence_epoch=VALUES(fence_epoch), fence_token=VALUES(fence_token), updated_at=VALUES(updated_at)`,
		   `INSERT INTO aa_recon_tickets (id, user_id, book, asset, note, state, opened_by, assignee, acked_by, closed_by, fence_epoch, fence_token, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET user_id=EXCLUDED.user_id, book=EXCLUDED.book, asset=EXCLUDED.asset, note=EXCLUDED.note, state=EXCLUDED.state, opened_by=EXCLUDED.opened_by, assignee=EXCLUDED.assignee, acked_by=EXCLUDED.acked_by, closed_by=EXCLUDED.closed_by, fence_epoch=EXCLUDED.fence_epoch, fence_token=EXCLUDED.fence_token, updated_at=EXCLUDED.updated_at`,
		   row.ID, row.UserID, row.Book, row.Asset, row.Note, row.State, row.OpenedBy, row.Assignee, row.AckedBy, row.ClosedBy, row.Fence.Epoch, row.Fence.Token, row.CreatedAt, row.UpdatedAt)
   }

   func upsertTicket(ctx context.Context, db *persist.SQLDB, _ string, insert, mysql, pg string, args ...any) error {
	  q := mysql
	  if db.Dialect == persist.Postgres {
		  q = pg
	  }
	  _ = insert
	  _, err := db.Conn(ctx).ExecContext(ctx, q, args...)
	   return err
  }

  func (r *DepTicket) Get(ctx context.Context, id string) (TicketRow, error) {
	   if !sqlOn(r.db) {
		  return TicketRow{},
			  ErrUnavailable
	   }
	  var row TicketRow
	   err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT id, kind, user_id, chain, asset, tx_hash, note, state, opened_by, assignee, closed_by, fence_epoch, fence_token, created_at, updated_at FROM aa_dep_tickets WHERE id=?`,
		   strings.TrimSpace(id),
	   ).Scan(&row.ID, &row.Kind, &row.UserID, &row.Chain, &row.Asset, &row.Hash, &row.Note, &row.State, &row.OpenedBy, &row.Assignee, &row.ClosedBy, &row.Fence.Epoch, &row.Fence.Token, &row.CreatedAt, &row.UpdatedAt)
	  if errors.Is(err, sql.ErrNoRows) {
		  return TicketRow{}, ErrNotFound
	  }
	  return row, err
   }

   func (r *WDTicket) Get(ctx context.Context, id string) (TicketRow, error) {
	   if !sqlOn(r.db) {
		   return TicketRow{}, ErrUnavailable
	   }
	  var row TicketRow
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT id, kind, user_id, transfer_id, note, state, opened_by, assignee, closed_by, fence_epoch, fence_token, created_at, updated_at FROM aa_wd_tickets WHERE id=?`,
		  strings.TrimSpace(id),
	   ).Scan(&row.ID, &row.Kind, &row.UserID, &row.TransferID, &row.Note, &row.State, &row.OpenedBy, &row.Assignee, &row.ClosedBy, &row.Fence.Epoch, &row.Fence.Token, &row.CreatedAt, &row.UpdatedAt)
	   if errors.Is(err, sql.ErrNoRows) {
		   return TicketRow{}, ErrNotFound
	  }
	   return row, err
   }

  func (r *ReconTicket) Get(ctx context.Context, id string) (TicketRow, error) {
	   if !sqlOn(r.db) {
		   return TicketRow{}, ErrUnavailable
	  }
	   var row TicketRow
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT id, user_id, book, asset, note, state, opened_by, assignee, acked_by, closed_by, fence_epoch, fence_token, created_at, updated_at FROM aa_recon_tickets WHERE id=?`,
		   strings.TrimSpace(id),
	  ).Scan(&row.ID, &row.UserID, &row.Book, &row.Asset, &row.Note, &row.State, &row.OpenedBy, &row.Assignee, &row.AckedBy, &row.ClosedBy, &row.Fence.Epoch, &row.Fence.Token, &row.CreatedAt, &row.UpdatedAt)
	   if errors.Is(err, sql.ErrNoRows) {
		   return TicketRow{}, ErrNotFound
	   }
	  return row, err
   }

  func RecordTicketEvent(ctx context.Context, db *persist.SQLDB, scope, ticketID, actor, event, from, to, note string) error {
	  if !sqlOn(db) {
		   return ErrUnavailable
	  }
	   _, err := db.Conn(ctx).ExecContext(ctx,
		  `INSERT INTO aa_ticket_events (id, scope, ticket_id, actor, event, from_state, to_state, note, at) VALUES (?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP(3))`,
		   uuid.NewString(), scope, ticketID, actor, event, from, to, note)
	  return err
  }

// ListOpen 列出未结入金异常单（desk Hydrate / 资产闸）。
// 含 asset；空 user 全台；limit<=0 默认 500。
// 仅 open/assigned；closed 不装回，避免结案后仍挡入账。
   func (r *DepTicket) ListOpen(ctx context.Context, userID string, limit int) ([]TicketRow, error) {
	   if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	   }
	  if limit <= 0 {
		   limit = 500
	   }
	  userID = strings.TrimSpace(userID)
	  q := `SELECT id, kind, user_id, chain, asset, tx_hash, note, state, opened_by, assignee, closed_by, fence_epoch, fence_token, created_at, updated_at
FROM aa_dep_tickets WHERE state IN ('open','assigned')`
	   args := []any{}
	   if userID != "" {
		  q += ` AND user_id=?`
		  args = append(args, userID)
	   }
	  q += ` ORDER BY created_at ASC LIMIT ?`
	   args = append(args, limit)
	   rows, err := r.db.Conn(ctx).QueryContext(ctx, q, args...)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   out := make([]TicketRow, 0)
	  for rows.Next() {
		  var row TicketRow
		   if err := rows.Scan(&row.ID, &row.Kind, &row.UserID, &row.Chain, &row.Asset, &row.Hash, &row.Note, &row.State, &row.OpenedBy, &row.Assignee, &row.ClosedBy, &row.Fence.Epoch, &row.Fence.Token, &row.CreatedAt, &row.UpdatedAt); err != nil {
			   return nil, err
		   }
		   out = append(out, row)
	   }
	  return out, rows.Err()
  }
