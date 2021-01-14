// 功能：出金提现域之 SQL 绑定。
// 作者：Orin
// 日期：2021.01.13

package withdraw

import (
	"context"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

   func (d *Desk) BindSQL(db *persist.SQLDB) *Desk {
	   if d != nil {
		  d.sql = db
	  }
	   return d
  }

   func (d *Desk) sqlOn() bool {
	  return d != nil && d.sql != nil && d.sql.DB != nil
  }

  func (d *Desk) persistRow(row *Exception, actor, event, from, to string) error {
	  if !d.sqlOn() || row == nil {
		   return nil
	   }
	  ctx := context.Background()
	  err := sqlrepo.NewWDTicket(d.sql).Put(ctx, sqlrepo.TicketRow{
		   ID: row.ID, Kind: string(row.Kind), UserID: row.UserID, TransferID: row.TransferID,
		   Note: row.Note, State: string(row.State), OpenedBy: row.OpenedBy, Assignee: row.Assignee,
		  ClosedBy: row.ClosedBy, Fence: row.Fence, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	  })
	   if err != nil {
		  return err
	   }
	   _ = sqlrepo.RecordTicketEvent(ctx, d.sql, "withdraw", row.ID, actor, event, from, to, row.Note)
	  return nil
  }

   func (d *Desk) loadSQL(id string) (*Exception, bool) {
	  if !d.sqlOn() {
		   return nil, false
	   }
	   got, err := sqlrepo.NewWDTicket(d.sql).Get(context.Background(), id)
	  if err != nil {
		   return nil, false
	  }
	  row := &Exception{
		  ID: got.ID, Kind: TicketKind(got.Kind), UserID: got.UserID, TransferID: got.TransferID,
		   Note:  got.Note,
		   State: TicketState(got.State), OpenedBy: got.OpenedBy, Assignee: got.Assignee,
		   ClosedBy: got.ClosedBy, Fence: got.Fence, CreatedAt: got.CreatedAt, UpdatedAt: got.UpdatedAt,
	   }
	  return row, true
  }
