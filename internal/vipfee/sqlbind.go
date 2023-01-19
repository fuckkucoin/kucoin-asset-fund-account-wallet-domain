package vipfee

import (
	"context"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

   func (s *Service) BindSQL(db *persist.SQLDB) *Service {
	  if s != nil {
		  s.sql = db
		   s.loadSQL()
	  }
	  return s
  }

  func (s *Service) StoreBacked() bool {
	  if s == nil {
		  return false
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  return s.storeBacked
  }

   func (s *Service) sqlOn() bool {
	  return s != nil && s.sql != nil && s.sql.DB != nil
   }

   func (s *Service) persistSchedule(sch Schedule) error {
	   if !s.sqlOn() {
		  return nil
	   }
	   return sqlrepo.NewVIPFee(s.sql).PutSchedule(context.Background(), sqlrepo.VIPSchedule{
		  ID: sch.ID, Tier: string(sch.Tier), Kind: string(sch.Kind), Asset: sch.Asset,
		   BPS: sch.BPS, Active: sch.Active, Version: sch.Version,
	  })
  }

   func (s *Service) persistPin(p Pin) error {
	  if !s.sqlOn() {
		   return nil
	  }
	   return sqlrepo.NewVIPFee(s.sql).PutPin(context.Background(), sqlrepo.VIPPin{
		  UserID: p.UserID, Tier: string(p.Tier), Actor: p.Actor, Note: p.Note, At: p.At,
	   })
  }

  func (s *Service) persistClearPin(userID string) error {
	   if !s.sqlOn() {
		   return nil
	  }
	   return sqlrepo.NewVIPFee(s.sql).DeletePin(context.Background(), userID)
  }

   func (s *Service) persistFill(f Fill) error {
	   if !s.sqlOn() {
		  return nil
	  }
	   return sqlrepo.NewVIPFee(s.sql).PutFill(context.Background(), sqlrepo.VIPFill{
		   ID: f.ID, UserID: f.UserID, Asset: f.Asset, Notional: f.Notional, Day: f.Day, At: f.At,
	  })
   }

  func (s *Service) loadSQL() {
	  if !s.sqlOn() {
		  return
	   }
	  ctx := context.Background()
	  repo := sqlrepo.NewVIPFee(s.sql)
	   scheds,
		   err := repo.ListSchedules(ctx)
	  if err == nil && len(scheds) > 0 {
		   s.mu.Lock()
		   s.storeBacked = true
		   s.rows = map[string]Schedule{}
		   for _, row := range scheds {
			  sch := Schedule{
				   ID: row.ID, Tier: Tier(row.Tier), Kind: Kind(row.Kind), Asset: row.Asset,
				   BPS: row.BPS, Active: row.Active, Version: row.Version,
			  }
			   s.rows[scheduleKey(sch.Tier, sch.Kind, sch.Asset)] = sch
			   if sch.Version > s.seq {
				   s.seq = sch.Version
			  }
		  }
		  s.mu.Unlock()
	  }
	   pins, err := repo.ListPins(ctx)
	  if err == nil && len(pins) > 0 {
		  s.mu.Lock()
		  s.pins = map[string]Pin{}
		   for _, p := range pins {
			   s.pins[p.UserID] = Pin{UserID: p.UserID, Tier: Tier(p.Tier), Actor: p.Actor, Note: p.Note, At: p.At}
		  }
		   s.mu.Unlock()
	  }
	   fills, err := repo.ListFills(ctx)
	  if err == nil && len(fills) > 0 {
		   s.mu.Lock()
		  s.fills = s.fills[:0]
		   for _, f := range fills {
			   s.fills = append(s.fills, Fill{ID: f.ID, UserID: f.UserID, Asset: f.Asset, Notional: f.Notional, Day: f.Day, At: f.At})
		   }
		   s.mu.Unlock()
	   }
   }
