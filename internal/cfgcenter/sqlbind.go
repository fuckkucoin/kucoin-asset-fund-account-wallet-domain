package cfgcenter

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

  func (s *Store) BindSQL(db *persist.SQLDB) *Store {
	   if s != nil {
		   s.sql = db
		   s.loadSQL()
		   if s.sqlOn() {
			   s.mu.Lock()
			   packs := make([]LimitPack, 0)
			  for _, rows := range s.byName {
				  packs = append(packs, rows...)
			  }
			   pin := map[string]int{}
			   for n, v := range s.pin {
				   pin[n] = v
			  }
			  kill := map[string]bool{}
			   for n, v := range s.kill {
				  kill[n] = v
			  }
			  global := s.global
			   s.mu.Unlock()
			   for _, p := range packs {
				   _ = s.persistPack(p)
			   }
			   for n, v := range pin {
				  _ = s.persistFlag(n, v, kill[n])
			  }
			   for n, v := range kill {
				   if _, ok := pin[n]; ok {
					   continue
				  }
				   _ = s.persistFlag(n, 0, v)
			   }
			  if global {
				   _ = s.persistFlag("*", 0, true)
			   }
		  }
	   }
	  return s
   }

  func (s *Store) sqlOn() bool {
	   return s != nil && s.sql != nil && s.sql.DB != nil
   }

  func (s *Store) persistPack(p LimitPack) error {
	   if !s.sqlOn() {
		  return nil
	  }
	   return sqlrepo.NewCfgStore(s.sql).PutPack(context.Background(), sqlrepo.CfgPack{
		  Name: p.Name, Version: p.Version, MaxSingle: p.MaxSingle, DailyWithdraw: p.DailyWithdraw,
		   HourlyVel: p.HourlyVel, DailyVel: p.DailyVel, AssetMax: p.AssetMax, Killed: p.Killed,
	   })
   }

  func (s *Store) persistFlag(name string, pin int, killed bool) error {
	  if !s.sqlOn() {
		  return nil
	   }
	  return sqlrepo.NewCfgStore(s.sql).PutFlag(context.Background(), sqlrepo.CfgFlag{
		  Name: name, PinVersion: pin, Killed: killed,
	   })
  }

   func (s *Store) loadSQL() {
	   if !s.sqlOn() {
		  return
	   }
	  ctx := context.Background()
	   repo := sqlrepo.NewCfgStore(s.sql)
	  packs, err := repo.ListPacks(ctx)
	   if err == nil && len(packs) > 0 {
		   s.mu.Lock()
		  s.byName = map[string][]LimitPack{}
		   for _, p := range packs {
			   s.byName[p.Name] = append(s.byName[p.Name], LimitPack{
				   Name:    p.Name,
				   Version: p.Version, MaxSingle: p.MaxSingle, DailyWithdraw: p.DailyWithdraw,
				   HourlyVel: p.HourlyVel,
				   DailyVel: p.DailyVel, AssetMax: p.AssetMax, Killed: p.Killed,
			   })
		   }
		  s.mu.Unlock()
	  }
	   flags, err := repo.ListFlags(ctx)
	   if err == nil && len(flags) > 0 {
		   s.mu.Lock()
		   s.pin = map[string]int{}
		  s.kill = map[string]bool{}
		   s.global = false
		   for _, f := range flags {
			  name := strings.TrimSpace(f.Name)
			   if name == "*" {
				   s.global = f.Killed
				   continue
			   }
			   if f.PinVersion > 0 {
				  s.pin[name] = f.PinVersion
			  }
			  if f.Killed {
				  s.kill[name] = true
			  }
		   }
		   s.mu.Unlock()
	   }
   }
