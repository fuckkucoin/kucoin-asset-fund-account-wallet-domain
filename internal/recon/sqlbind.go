package recon

import (
	"context"
	"errors"
	"fmt"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

  func (b *Book) BindSQL(db *persist.SQLDB) *Book {
	   if b != nil {
		   b.sql = db
	  }
	  return b
  }

   func (b *Book) sqlOn() bool {
	   return b != nil && b.sql != nil && b.sql.DB != nil
  }

  func (b *Book) persistRow(t *Ticket, actor, event, from, to string) error {
	  if !b.sqlOn() || t == nil {
		  return nil
	  }
	   ctx := context.Background()
	  err := sqlrepo.NewReconTicket(b.sql).Put(ctx, sqlrepo.TicketRow{
		   ID: t.ID, UserID: t.UserID, Book: t.Book, Asset: t.Asset, Note: t.Note,
		   State: string(t.State),
		  OpenedBy: t.OpenedBy, Assignee: t.Assignee,
		  AckedBy: t.AckedBy, ClosedBy: t.ClosedBy,
		  Fence: t.Fence,
		  CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	   })
	  if err != nil {
		  metrics.ReconTickets.WithLabelValues("store").Inc()
		  return fmt.Errorf("%w: %v", ErrStore, err)
	   }
	  _ = sqlrepo.RecordTicketEvent(ctx, b.sql, "recon", t.ID, actor, event, from, to, t.Note)
	   return nil
  }

   func (b *Book) loadSQL(id string) (*Ticket, bool, error) {
	  if !b.sqlOn() {
		  return nil, false, nil
	   }
	   got,
		  err := sqlrepo.NewReconTicket(b.sql).Get(context.Background(), id)
	   if err != nil {
		   if errors.Is(err, sqlrepo.ErrNotFound) {
			  return nil, false, nil
		   }
		  metrics.ReconTickets.WithLabelValues("store").Inc()
		  return nil, false, fmt.Errorf("%w: %v", ErrStore, err)
	   }
	   t := &Ticket{
		  ID:     got.ID,
		  UserID: got.UserID, Book: got.Book, Asset: got.Asset, Note: got.Note,
		  State: State(got.State), OpenedBy: got.OpenedBy, Assignee: got.Assignee,
		   AckedBy: got.AckedBy, ClosedBy: got.ClosedBy,
		  Fence: got.Fence, CreatedAt: got.CreatedAt, UpdatedAt: got.UpdatedAt,
	   }
	  return t, true, nil
   }
