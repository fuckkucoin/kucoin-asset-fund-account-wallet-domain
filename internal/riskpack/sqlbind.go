package riskpack

import (
	"context"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

  func (s *Service) BindSQL(db *persist.SQLDB) *Service {
	  if s != nil && s.eng != nil {
		   s.eng.exp.BindSQL(db)
		  s.eng.esc.BindSQL(db)
	   }
	  return s
   }

  func (b *Book) BindSQL(db *persist.SQLDB) *Book {
	   if b != nil {
		   b.sql = db
		   b.loadSQL()
		  if b.sqlOn() {
			  b.mu.Lock()
			  caps := make([]Cap, 0, len(b.caps))
			   for _, c := range b.caps {
				  caps = append(caps, c)
			   }
			  b.mu.Unlock()
			   for _, c := range caps {
				  _ = b.persistCap(c)
			   }
		   }
	   }
	   return b
  }

   func (b *Book) sqlOn() bool {
	   return b != nil && b.sql != nil && b.sql.DB != nil
  }

   func (b *Book) persistCap(c Cap) error {
	   if !b.sqlOn() {
		   return nil
	  }
	   return sqlrepo.NewRiskPack(b.sql).PutCap(context.Background(), sqlrepo.RiskCap{
		  UserID: c.UserID, Asset: c.Asset, Max: c.Max,
	  })
   }

  func (b *Book) loadSQL() {
	   if !b.sqlOn() {
		  return
	  }
	  got, err := sqlrepo.NewRiskPack(b.sql).ListCaps(context.Background())
	  if err != nil || len(got) == 0 {
		  return
	  }
	  b.mu.Lock()
	   b.caps = map[string]Cap{}
	   for _, c := range got {
		   row := Cap{UserID: c.UserID, Asset: c.Asset, Max: c.Max}
		   b.caps[capKey(row.UserID, row.Asset)] = row
	   }
	   b.mu.Unlock()
  }

  func (q *Queue) BindSQL(db *persist.SQLDB) *Queue {
	  if q != nil {
		   q.sql = db
		  q.loadSQL()
		   if q.sqlOn() {
			  q.mu.Lock()
			  items := make([]Item, 0, len(q.rows))
			  for _, it := range q.rows {
				  items = append(items, *it)
			  }
			  q.mu.Unlock()
			  for _, it := range items {
				  _ = q.persistItem(it)
			   }
		   }
	  }
	   return q
  }

   func (q *Queue) sqlOn() bool {
	  return q != nil && q.sql != nil && q.sql.DB != nil
   }

   func (q *Queue) persistItem(it Item) error {
	   if !q.sqlOn() {
		  return nil
	  }
	   return sqlrepo.NewRiskPack(q.sql).PutItem(context.Background(), sqlrepo.RiskItem{
		  ID:     it.ID,
		  UserID: it.UserID, Reason: it.Reason, Asset: it.Asset, Amount: it.Amount,
		   Dest: it.Dest, Status: string(it.Status), Actor: it.Actor, Fence: it.Fence, OpenedAt: it.OpenedAt,
	  })
  }

  func (q *Queue) loadSQL() {
	   if !q.sqlOn() {
		   return
	   }
	  got, err := sqlrepo.NewRiskPack(q.sql).ListItems(context.Background())
	   if err != nil || len(got) == 0 {
		  return
	   }
	   q.mu.Lock()
	   q.rows = map[string]*Item{}
	   fences := map[string]persist.Fence{}
	   for _, row := range got {
		  it := Item{
			  ID: row.ID, UserID: row.UserID, Reason: row.Reason, Asset: row.Asset, Amount: row.Amount,
			   Dest: row.Dest, Status: Status(row.Status), Actor: row.Actor, OpenedAt: row.OpenedAt, Fence: row.Fence,
		  }
		  cp := it
		   q.rows[it.ID] = &cp
		  fences[it.ID] = it.Fence
	  }
	   q.mu.Unlock()
	  q.fence.Restore(fences)
   }
