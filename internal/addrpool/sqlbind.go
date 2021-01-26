package addrpool

import (
	"context"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

   func (p *Pool) BindSQL(db *persist.SQLDB) *Pool {
	  if p != nil {
		  p.sql = db
	   }
	  return p
   }

  func (p *Pool) sqlOn() bool {
	  return p != nil && p.sql != nil && p.sql.DB != nil
   }

   func (p *Pool) persistBind(b Binding) error {
	  if !p.sqlOn() {
		   return nil
	   }
	  return sqlrepo.NewAddress(p.sql).Put(context.Background(), sqlrepo.AddrBind{
		  BindKey: key(b.UserID, b.Chain), UserID: b.UserID, Chain: b.Chain, Family: b.Family,
		  Address: b.Address, Allocated: b.Allocated, BoundAt: b.BoundAt, ReuseAfter: b.Cooldown,
	  })
  }

   func (p *Pool) listSQL(userID string) ([]Binding, error) {
	  if !p.sqlOn() {
		   return nil,
			  sqlrepo.ErrUnavailable
	  }
	   rows,
		  err := sqlrepo.NewAddress(p.sql).ListUser(context.Background(), userID)
	   if err != nil {
		   return nil, err
	  }
	  out := make([]Binding, 0, len(rows))
	   for _, row := range rows {
		  if row.Allocated {
			   out = append(out, bindFromRow(row))
		   }
	   }
	  return out, nil
  }

   func (p *Pool) persistRelease(userID, chain string) error {
	  if !p.sqlOn() {
		   return nil
	  }
	  return sqlrepo.NewAddress(p.sql).SetAllocated(context.Background(), key(userID, chain), false)
  }

   func bindFromRow(row sqlrepo.AddrBind) Binding {
	  return Binding{
		  UserID: row.UserID, Chain: row.Chain, Family: row.Family, Address: row.Address,
		  BoundAt: row.BoundAt,
		  Cooldown: row.ReuseAfter, Allocated: row.Allocated,
	  }
   }

   func (p *Pool) loadByAddr(addr string) (Binding, error) {
	  if !p.sqlOn() {
		  return Binding{},
			   sqlrepo.ErrUnavailable
	  }
	   got, err := sqlrepo.NewAddress(p.sql).GetByAddr(context.Background(), addr)
	  if err != nil {
		   return Binding{}, err
	  }
	  return bindFromRow(got), nil
  }

  func (p *Pool) loadSQL(userID, chain string) (Binding, bool) {
	   if !p.sqlOn() {
		   return Binding{}, false
	  }
	  got, err := sqlrepo.NewAddress(p.sql).Get(context.Background(), userID, chain)
	   if err != nil || !got.Allocated {
		   // family key may differ from chain|user; fall back to list
		   rows, lerr := sqlrepo.NewAddress(p.sql).ListUser(context.Background(), userID)
		  if lerr != nil {
			  return Binding{}, false
		   }
		   want := key(userID, chain)
		   for _, row := range rows {
			  if row.BindKey == want || (row.Family == family(chain) && row.Allocated) {
				   return bindFromRow(row), true
			  }
		  }
		  return Binding{}, false
	   }
	   return bindFromRow(got), true
   }

  func (inv *Inventory) BindSQL(db *persist.SQLDB) *Inventory {
	   if inv != nil {
		  inv.sql = db
	   }
	  return inv
   }

  func (inv *Inventory) sqlOn() bool {
	   return inv != nil && inv.sql != nil && inv.sql.DB != nil
   }

   func (inv *Inventory) persistSlot(s *Slot) error {
	  if !inv.sqlOn() || s == nil {
		   return nil
	  }
	   return sqlrepo.NewSlots(inv.sql).Put(context.Background(), sqlrepo.AddrSlot{
		  Address: s.Address, Chain: s.Chain, Family: s.Family, UserID: s.UserID,
		  Status: string(s.Status), Reserved: s.Reserved, BoundAt: s.BoundAt, CoolUntil: s.CoolUntil,
	   })
   }
