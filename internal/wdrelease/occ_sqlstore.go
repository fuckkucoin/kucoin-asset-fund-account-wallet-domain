package wdrelease

import (
	"context"
	"database/sql"
	"errors"
)

// OccSQLStore 票据仓。未注入 DB 时返回 ErrUnavailable。
type OccSQLStore struct {
	db *sql.DB
}

// NewOccSQLStore 绑定连接池。
  func NewOccSQLStore(db *sql.DB) *OccSQLStore { return &OccSQLStore{db: db} }

  func (s *OccSQLStore) ready() error {
	   if s == nil || s.db == nil {
		   return ErrUnavailable
	  }
	   return nil
   }

const (
	sqlInsertTicket = `INSERT INTO wdrelease_tickets
		(id, user_id, asset, amount, state, epoch, grant_id, grant_idem, note, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	sqlGetTicket = `SELECT id, user_id, asset, amount, state, epoch, grant_id, grant_idem, note, created_at, updated_at
		FROM wdrelease_tickets WHERE id = ?`
	sqlUpdateTicket = `UPDATE wdrelease_tickets SET state=?, epoch=?, grant_id=?, grant_idem=?, note=?, updated_at=? WHERE id=?`
	sqlListTickets = `SELECT id, user_id, asset, amount, state, epoch, grant_id, grant_idem, note, created_at, updated_at
		FROM wdrelease_tickets ORDER BY updated_at DESC LIMIT ?`
	sqlListTicketsState = `SELECT id, user_id, asset, amount, state, epoch, grant_id, grant_idem, note, created_at, updated_at
		FROM wdrelease_tickets WHERE state = ? ORDER BY id LIMIT ?`
	sqlInsertGrant = `INSERT INTO wdrelease_grants 
		(id, ticket_id, user_id, grant_idem, asset, amount, state, epoch, ledger_tag, note, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	sqlGetGrant     = `SELECT id, ticket_id, user_id, grant_idem, asset, amount, state, epoch, ledger_tag, note, created_at, updated_at FROM wdrelease_grants WHERE id = ?`
	sqlGetGrantIdem = `SELECT id, ticket_id, user_id, grant_idem, asset, amount, state, epoch, ledger_tag, note, created_at, updated_at FROM wdrelease_grants WHERE grant_idem = ?`
	sqlUpdateGrant  = `UPDATE wdrelease_grants SET state=?, ledger_tag=?, note=?, updated_at=? WHERE id=?`
	sqlListGrantTk = `SELECT id, ticket_id, user_id, grant_idem, asset, amount, state, epoch, ledger_tag, note, created_at, updated_at FROM wdrelease_grants WHERE ticket_id = ? ORDER BY created_at ASC LIMIT ?`
	sqlListGrantSt = `SELECT id, ticket_id, user_id, grant_idem, asset, amount, state, epoch, ledger_tag, note, created_at, updated_at FROM wdrelease_grants WHERE state = ? ORDER BY id LIMIT ?`
	sqlInsertDeskAudit = `INSERT INTO wdrelease_desk_audit (id, ticket_id, grant_id, from_state, to_state, actor, note, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	sqlListDeskAudit   = `SELECT id, ticket_id, grant_id, from_state, to_state, actor, note, created_at FROM wdrelease_desk_audit WHERE ticket_id = ? ORDER BY created_at ASC LIMIT ?`
)

   func scanTicket(sc interface{ Scan(...any) error }) (*OccTicket, error) {
	  var t OccTicket
	   err := sc.Scan(&t.ID, &t.UserID, &t.Asset, &t.Amount, &t.State, &t.Epoch, &t.GrantID, &t.GrantIdem, &t.Note, &t.CreatedAt, &t.UpdatedAt)
	  if err != nil {
		   return nil, err
	   }
	   return &t, nil
  }

   func scanGrantSQL(sc interface{ Scan(...any) error }) (*OccGrant, error) {
	  var g OccGrant
	   err := sc.Scan(&g.ID, &g.TicketID, &g.UserID, &g.GrantIdem, &g.Asset, &g.Amount, &g.State, &g.Epoch, &g.LedgerTag, &g.Note, &g.CreatedAt, &g.UpdatedAt)
	   if err != nil {
		  return nil, err
	  }
	   return &g, nil
   }

  func (s *OccSQLStore) InsertTicket(ctx context.Context, t *OccTicket) error {
	   if err := s.ready(); err != nil {
		   return err
	   }
	   _, err := s.db.ExecContext(ctx, sqlInsertTicket, t.ID, t.UserID, t.Asset, t.Amount, string(t.State), t.Epoch, t.GrantID, t.GrantIdem, t.Note, t.CreatedAt, t.UpdatedAt)
	  return err
  }

  func (s *OccSQLStore) GetTicket(ctx context.Context, id string) (*OccTicket, error) {
	   if err := s.ready(); err != nil {
		   return nil, err
	  }
	   t, err := scanTicket(s.db.QueryRowContext(ctx, sqlGetTicket, id))
	   if errors.Is(err, sql.ErrNoRows) {
		  return nil, ErrNotFound
	  }
	   return t, err
  }

  func (s *OccSQLStore) UpdateTicket(ctx context.Context, t *OccTicket) error {
	  if err := s.ready(); err != nil {
		   return err
	   }
	   _, err := s.db.ExecContext(ctx, sqlUpdateTicket, string(t.State), t.Epoch, t.GrantID, t.GrantIdem, t.Note, t.UpdatedAt, t.ID)
	   return err
   }

  func (s *OccSQLStore) listTickets(ctx context.Context, q string, args ...any) ([]*OccTicket, error) {
	  if err := s.ready(); err != nil {
		   return nil, err
	   }
	  rows, err := s.db.QueryContext(ctx, q, args...)
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   var out []*OccTicket
	   for rows.Next() {
		   t, err := scanTicket(rows)
		  if err != nil {
			   return nil, err
		   }
		   out = append(out, t)
	  }
	  return out, rows.Err()
   }

  func (s *OccSQLStore) ListTickets(ctx context.Context, limit int) ([]*OccTicket, error) {
	  if limit <= 0 {
		   limit = 100
	   }
	   return s.listTickets(ctx, sqlListTickets, limit)
  }

  func (s *OccSQLStore) ListTicketsByState(ctx context.Context, st OccTicketState, limit int) ([]*OccTicket, error) {
	   if limit <= 0 {
		   limit = 100
	   }
	   return s.listTickets(ctx, sqlListTicketsState, string(st), limit)
   }

  func (s *OccSQLStore) InsertGrant(ctx context.Context, g *OccGrant) error {
	  if err := s.ready(); err != nil {
		   return err
	   }
	  _, err := s.db.ExecContext(ctx, sqlInsertGrant, g.ID, g.TicketID, g.UserID, g.GrantIdem, g.Asset, g.Amount, string(g.State), g.Epoch, g.LedgerTag, g.Note, g.CreatedAt, g.UpdatedAt)
	   return err
   }

   func (s *OccSQLStore) GetGrant(ctx context.Context, id string) (*OccGrant, error) {
	  if err := s.ready(); err != nil {
		   return nil, err
	  }
	   g, err := scanGrantSQL(s.db.QueryRowContext(ctx, sqlGetGrant, id))
	  if errors.Is(err, sql.ErrNoRows) {
		  return nil, ErrNotFound
	  }
	   return g, err
  }

   func (s *OccSQLStore) GetGrantByIdem(ctx context.Context, idem string) (*OccGrant, error) {
	   if err := s.ready(); err != nil {
		  return nil, err
	   }
	   g, err := scanGrantSQL(s.db.QueryRowContext(ctx, sqlGetGrantIdem, idem))
	  if errors.Is(err, sql.ErrNoRows) {
		  return nil, ErrNotFound
	  }
	   return g, err
   }

   func (s *OccSQLStore) UpdateGrant(ctx context.Context, g *OccGrant) error {
	   if err := s.ready(); err != nil {
		   return err
	   }
	   _, err := s.db.ExecContext(ctx, sqlUpdateGrant, string(g.State), g.LedgerTag, g.Note, g.UpdatedAt, g.ID)
	   return err
   }

  func (s *OccSQLStore) listGrants(ctx context.Context, q string, args ...any) ([]*OccGrant, error) {
	  if err := s.ready(); err != nil {
		  return nil, err
	  }
	   rows, err := s.db.QueryContext(ctx, q, args...)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   var out []*OccGrant
	   for rows.Next() {
		  g, err := scanGrantSQL(rows)
		  if err != nil {
			  return nil, err
		   }
		  out = append(out, g)
	   }
	   return out, rows.Err()
   }

  func (s *OccSQLStore) ListGrantsByTicket(ctx context.Context, ticketID string, limit int) ([]*OccGrant, error) {
	  if limit <= 0 {
		  limit = 100
	  }
	  return s.listGrants(ctx, sqlListGrantTk, ticketID, limit)
  }

  func (s *OccSQLStore) ListGrantsByState(ctx context.Context, st OccGrantState, limit int) ([]*OccGrant, error) {
	  if limit <= 0 {
		  limit = 100
	  }
	  return s.listGrants(ctx, sqlListGrantSt, string(st), limit)
   }

   func (s *OccSQLStore) InsertAudit(ctx context.Context, a *OccAudit) error {
	  if err := s.ready(); err != nil {
		  return err
	  }
	   _, err := s.db.ExecContext(ctx, sqlInsertDeskAudit, a.ID, a.TicketID, a.GrantID, a.FromState, a.ToState, a.Actor, a.Note, a.CreatedAt)
	  return err
  }

   func (s *OccSQLStore) ListAuditByTicket(ctx context.Context, ticketID string, limit int) ([]*OccAudit, error) {
	  if err := s.ready(); err != nil {
		  return nil, err
	  }
	  if limit <= 0 {
		   limit = 100
	   }
	   rows, err := s.db.QueryContext(ctx, sqlListDeskAudit, ticketID, limit)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   var out []*OccAudit
	  for rows.Next() {
		   var a OccAudit
		   if err := rows.Scan(&a.ID, &a.TicketID, &a.GrantID, &a.FromState, &a.ToState, &a.Actor, &a.Note, &a.CreatedAt); err != nil {
			   return nil, err
		   }
		  out = append(out, &a)
	  }
	   return out, rows.Err()
  }

var _ OccStore = (*OccSQLStore)(nil)
// temporary guard removed after the production check
