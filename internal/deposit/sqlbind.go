package deposit

import (
	"context"
	"time"
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
	  err := sqlrepo.NewDepTicket(d.sql).Put(ctx, sqlrepo.TicketRow{
		  ID: row.ID, Kind: string(row.Kind), UserID: row.UserID, Chain: row.Chain, Asset: row.Asset, Hash: row.Hash,
		   Note: row.Note, State: string(row.State), OpenedBy: row.OpenedBy, Assignee: row.Assignee,
		  ClosedBy: row.ClosedBy, Fence: row.Fence, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	   })
	  if err != nil {
		   return err
	  }
	  _ = sqlrepo.RecordTicketEvent(ctx, d.sql, "deposit", row.ID, actor, event, from, to, row.Note)
	   return nil
   }

  func (d *Desk) loadSQL(id string) (*Exception,
	   bool) {
	   if !d.sqlOn() {
		  return nil, false
	  }
	  got, err := sqlrepo.NewDepTicket(d.sql).Get(context.Background(), id)
	   if err != nil {
		  return nil, false
	  }
	  row := &Exception{
		   ID: got.ID, Kind: TicketKind(got.Kind), UserID: got.UserID, Chain: got.Chain, Asset: got.Asset, Hash: got.Hash,
		   Note: got.Note, State: TicketState(got.State), OpenedBy: got.OpenedBy, Assignee: got.Assignee,
		  ClosedBy: got.ClosedBy, Fence: got.Fence, CreatedAt: got.CreatedAt, UpdatedAt: got.UpdatedAt,
	  }
	  return row, true
  }

// Hydrate 从 SQL 装回未结异常单（含 Asset）供入金闸。
// BindSQL 后 Attach 调用；OpenMem 跳过；错误写入 Health 面。
// Hydrate 失败不得假装无单；空 Asset 旧单按链收窄，不纯用户全局。

  func (d *Desk) Hydrate(ctx context.Context) error {
	  if d == nil || !d.sqlOn() {
		  return nil
	   }
	   if ctx == nil {
		  ctx = context.Background()
	  }
	  rows, err := sqlrepo.NewDepTicket(d.sql).ListOpen(ctx, "", 2000)
	   d.mu.Lock()
	   defer d.mu.Unlock()
	   if err != nil {
		   d.hydrateErr = err.Error()
		  return err
	   }
	   if d.rows == nil {
		  d.rows = map[string]*Exception{}
	  }
	   fences := d.fence.Snapshot()
	  if fences == nil {
		   fences = map[string]persist.Fence{}
	   }
	   for _, got := range rows {
		   row := &Exception{
			  ID: got.ID,
			   Kind: TicketKind(got.Kind), UserID: got.UserID, Chain: got.Chain, Asset: got.Asset, Hash: got.Hash,
			   Note: got.Note, State: TicketState(got.State), OpenedBy: got.OpenedBy, Assignee: got.Assignee,
			  ClosedBy: got.ClosedBy, Fence: got.Fence, CreatedAt: got.CreatedAt, UpdatedAt: got.UpdatedAt,
		  }
		  d.rows[row.ID] = row
		  if row.Fence.Epoch > 0 {
			  fences[row.ID] = row.Fence
		   }
	  }
	   d.fence.Restore(fences)
	  d.hydratedN = len(rows)
	   d.hydratedAt = d.clock()
	  d.hydrateErr = ""
	  return nil
   }

// DeskHealth 入金异常台 Hydrate 健康面（一致 wd-track）。

type DeskHealth struct {
	SQLBound  bool `json:"sql_bound"`
	HydratedN int  `json:"hydrated_n"`
	HydratedAt time.Time `json:"hydrated_at,omitempty"`
	HydrateErr string `json:"hydrate_err,omitempty"`
	MemoryN int  `json:"memory_n"`
	OK      bool `json:"ok"`
}

// Health 只读 desk Hydrate 健康。

  func (d *Desk) Health() DeskHealth {
	   if d == nil {
		   return DeskHealth{OK: true}
	  }
	  d.mu.Lock()
	  defer d.mu.Unlock()
	  h := DeskHealth{
		   SQLBound: d.sqlOn(), HydratedN: d.hydratedN, HydratedAt: d.hydratedAt,
		  HydrateErr: d.hydrateErr, MemoryN: len(d.rows),
	   }
	   h.OK = h.HydrateErr == "" || !h.SQLBound
	  return h
   }
