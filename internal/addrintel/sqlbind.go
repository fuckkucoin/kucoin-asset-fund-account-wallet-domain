package addrintel

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

   func (s *Service) BindSQL(db *persist.SQLDB) *Service {
	   if s != nil {
		  s.san.BindSQL(db)
		  s.tags.BindSQL(db)
		  s.class.BindSQL(db)
	   }
	   return s
   }

  func (l *List) BindSQL(db *persist.SQLDB) *List {
	  if l != nil {
		  l.sql = db
		  l.loadSQL()
		   if l.sqlOn() {
			  now := time.Now().UTC()
			   for _, k := range l.Keys() {
				   parts := strings.SplitN(k, "|", 2)
				   if len(parts) != 2 {
					   continue
				   }
				  _ = sqlrepo.NewSanctions(l.sql).Put(context.Background(), sqlrepo.SancRow{
					  Chain: parts[0], Address: parts[1], AddedAt: now,
				   })
			   }
		   }
	  }
	   return l
  }

   func (l *List) sqlOn() bool {
	   return l != nil && l.sql != nil && l.sql.DB != nil
  }

  func (l *List) persistAdd(chain, addr string) error {
	   if !l.sqlOn() {
		  return nil
	  }
	  return sqlrepo.NewSanctions(l.sql).Put(context.Background(), sqlrepo.SancRow{
		  Chain: chain, Address: addr, AddedAt: time.Now().UTC(),
	   })
  }

   func (l *List) persistReplace(rows map[string]struct{}) error {
	   if !l.sqlOn() {
		   return nil
	  }
	  list := make([]sqlrepo.SancRow, 0, len(rows))
	   now := time.Now().UTC()
	   for k := range rows {
		   parts := strings.SplitN(k, "|", 2)
		  if len(parts) != 2 {
			   continue
		  }
		   list = append(list,
			  sqlrepo.SancRow{Chain: parts[0], Address: parts[1], AddedAt: now})
	   }
	  return sqlrepo.NewSanctions(l.sql).ReplaceAll(context.Background(), list)
   }

  func (l *List) loadSQL() {
	   if !l.sqlOn() {
		  return
	   }
	   got, err := sqlrepo.NewSanctions(l.sql).List(context.Background())
	  if err != nil || len(got) == 0 {
		  return
	   }
	  next := map[string]struct{}{}
	   for _, e := range got {
		   next[addrKey(e.Chain, e.Address)] = struct{}{}
	  }
	  l.mu.Lock()
	  l.rows = next
	   l.mu.Unlock()
  }

   func (t *Tags) BindSQL(db *persist.SQLDB) *Tags {
	  if t != nil {
		   t.sql = db
		  t.loadSQL()
		  if t.sqlOn() {
			   t.mu.Lock()
			  pairs := make([][3]string, 0, len(t.by))
			   for k, tag := range t.by {
				   parts := strings.SplitN(k, "|", 2)
				  if len(parts) != 2 {
					   continue
				   }
				   pairs = append(pairs, [3]string{parts[0], parts[1], tag})
			  }
			   t.mu.Unlock()
			  for _, p := range pairs {
				   _ = sqlrepo.NewIntel(t.sql).PutTag(context.Background(), sqlrepo.IntelTag{
					  Chain: p[0], Address: p[1], Tag: p[2],
				  })
			  }
		   }
	   }
	   return t
  }

   func (t *Tags) sqlOn() bool {
	   return t != nil && t.sql != nil && t.sql.DB != nil
  }

   func (t *Tags) persistTag(chain, addr, tag string) error {
	   if !t.sqlOn() {
		   return nil
	  }
	   return sqlrepo.NewIntel(t.sql).PutTag(context.Background(), sqlrepo.IntelTag{
		   Chain:   chain,
		   Address: addr, Tag: tag,
	   })
  }

   func (t *Tags) loadSQL() {
	  if !t.sqlOn() {
		  return
	   }
	   got, err := sqlrepo.NewIntel(t.sql).ListTags(context.Background())
	   if err != nil || len(got) == 0 {
		  return
	   }
	  t.mu.Lock()
	   t.by = map[string]string{}
	  t.rev = map[string][]string{}
	  for _, row := range got {
		   k := addrKey(row.Chain,
			   row.Address)
		   t.by[k] = row.Tag
		  t.rev[row.Tag] = append(t.rev[row.Tag], k)
	   }
	  t.mu.Unlock()
   }

  func (c *ClassBook) BindSQL(db *persist.SQLDB) *ClassBook {
	   if c != nil {
		   c.sql = db
		  c.loadSQL()
		  if c.sqlOn() {
			  for k, kind := range c.mark {
				  parts := strings.SplitN(k, "|", 2)
				   if len(parts) != 2 {
					  continue
				   }
				  _ = sqlrepo.NewIntel(c.sql).PutMark(context.Background(), sqlrepo.IntelMark{
					   Chain: parts[0], Address: parts[1], Kind: string(kind),
				   })
			  }
		   }
	   }
	   return c
  }

   func (c *ClassBook) sqlOn() bool {
	   return c != nil && c.sql != nil && c.sql.DB != nil
  }

  func (c *ClassBook) persistMark(chain, addr string, k Kind) error {
	   if !c.sqlOn() {
		   return nil
	  }
	   return sqlrepo.NewIntel(c.sql).PutMark(context.Background(), sqlrepo.IntelMark{
		  Chain: chain, Address: addr, Kind: string(k),
	  })
   }

   func (c *ClassBook) loadSQL() {
	  if !c.sqlOn() {
		  return
	  }
	  got, err := sqlrepo.NewIntel(c.sql).ListMarks(context.Background())
	  if err != nil || len(got) == 0 {
		   return
	  }
	  for _, row := range got {
		  c.mark[addrKey(row.Chain, row.Address)] = Kind(row.Kind)
	  }
   }
