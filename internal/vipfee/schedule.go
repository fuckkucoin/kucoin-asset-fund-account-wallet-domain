package vipfee

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"account-assets.kucoin.net/funding/internal/validate"
)

  func (s *Service) seedLocked() {
	  type row struct {
		  tier Tier
		  kind Kind
		   bps int
	   }
	   seeds := []row{
		   {TierKYC0, KindMaker, 10}, {TierKYC0, KindTaker, 20}, {TierKYC0, KindRebate, 0}, {TierKYC0, KindWithdraw, 15},
		   {TierStandard, KindMaker, 5}, {TierStandard, KindTaker, 15}, {TierStandard, KindRebate, 5000}, {TierStandard, KindWithdraw, 10},
		   {TierVIP,
			  KindMaker, 2}, {TierVIP, KindTaker, 8}, {TierVIP, KindRebate, 8000}, {TierVIP, KindWithdraw, 5},
		  {TierVIPPlus, KindMaker, 0}, {TierVIPPlus, KindTaker, 4}, {TierVIPPlus, KindRebate, 10000}, {TierVIPPlus, KindWithdraw, 2},
	  }
	  for _, r := range seeds {
		   _ = s.putLocked(Schedule{Tier: r.tier, Kind: r.kind, Asset: "*", BPS: r.bps, Active: true})
	  }
	  _ = s.putLocked(Schedule{Tier: TierVIPPlus, Kind: KindWithdraw, Asset: "BTC", BPS: 1, Active: true})
  }

  func (s *Service) Put(sch Schedule) error {
	  if s == nil {
		  return ErrUnavailable
	   }
	   s.mu.Lock()
	   err := s.putLocked(sch)
	  var saved Schedule
	   if err == nil {
		   tier, _ := ParseTier(string(sch.Tier))
		  kind, _ := ParseKind(string(sch.Kind))
		   asset := strings.ToUpper(strings.TrimSpace(sch.Asset))
		   if asset == "" {
			   asset = "*"
		  }
		  saved = s.rows[scheduleKey(tier, kind, asset)]
	   }
	   s.mu.Unlock()
	   if err != nil {
		   return err
	   }
	   if err := s.persistSchedule(saved); err != nil {
		   s.mu.Lock()
		  delete(s.rows, scheduleKey(saved.Tier, saved.Kind, saved.Asset))
		  s.mu.Unlock()
		   return err
	   }
	   return nil
  }

   func (s *Service) putLocked(sch Schedule) error {
	  tier, err := ParseTier(string(sch.Tier))
	  if err != nil {
		   return err
	   }
	  kind, err := ParseKind(string(sch.Kind))
	   if err != nil {
		   return err
	  }
	   if err := validBPS(sch.BPS); err != nil {
		  return err
	   }
	  asset := strings.ToUpper(strings.TrimSpace(sch.Asset))
	   if asset == "" {
		   asset = "*"
	  }
	  if asset != "*" {
		  if _, err := validate.Asset(asset); err != nil {
			   return ErrAsset
		  }
	   }
	  s.seq++
	  sch.Tier, sch.Kind, sch.Asset = tier, kind, asset
	  sch.Version = s.seq
	   if sch.ID == "" {
		  sch.ID = fmt.Sprintf("vip-%d", s.seq)
	  }
	   sch.Active = true
	   s.rows[scheduleKey(tier, kind, asset)] = sch
	  return nil
  }

   func (s *Service) Get(tier Tier, kind Kind, asset string) (Schedule, error) {
	  if s == nil {
		   return Schedule{}, ErrUnavailable
	   }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  if s.sqlOn() {
		   if row, err := sqlrepo.NewVIPFee(s.sql).GetSchedule(context.Background(), string(tier), string(kind), asset); err == nil && row.Active {
			  return Schedule{ID: row.ID, Tier: Tier(row.Tier), Kind: Kind(row.Kind), Asset: row.Asset, BPS: row.BPS, Active: row.Active, Version: row.Version}, nil
		  } else if err == nil || errors.Is(err, sqlrepo.ErrNotFound) {
			  if row, err := sqlrepo.NewVIPFee(s.sql).GetSchedule(context.Background(), string(tier), string(kind), "*"); err == nil && row.Active {
				  return Schedule{ID: row.ID, Tier: Tier(row.Tier), Kind: Kind(row.Kind), Asset: row.Asset, BPS: row.BPS, Active: row.Active, Version: row.Version}, nil
			   }
		   } else if err != nil && !errors.Is(err, sqlrepo.ErrNotFound) {
			  return Schedule{}, ErrUnavailable
		  }
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   return s.getLocked(tier, kind, asset)
   }

  func (s *Service) getLocked(tier Tier, kind Kind, asset string) (Schedule, error) {
	   if row, ok := s.rows[scheduleKey(tier, kind, asset)]; ok && row.Active {
		   return row.Copy(), nil
	  }
	   if row, ok := s.rows[scheduleKey(tier, kind, "*")]; ok && row.Active {
		   return row.Copy(), nil
	  }
	   return Schedule{}, ErrNotFound
  }

  func (s *Service) Disable(tier Tier, kind Kind, asset string) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   s.mu.Lock()
	   k := scheduleKey(tier, kind, asset)
	   row, ok := s.rows[k]
	   if !ok {
		   s.mu.Unlock()
		   return ErrNotFound
	  }
	   row.Active = false
	  s.rows[k] = row
	   s.mu.Unlock()
	  if err := s.persistSchedule(row); err != nil {
		   s.mu.Lock()
		   row.Active = true
		  s.rows[k] = row
		   s.mu.Unlock()
		   return err
	   }
	  return nil
  }

  func (s *Service) List() []Schedule {
	   if s == nil {
		  return nil
	  }
	   if s.sqlOn() {
		   got, err := sqlrepo.NewVIPFee(s.sql).ListSchedules(context.Background())
		  if err == nil {
			  out := make([]Schedule, 0, len(got))
			   for _, row := range got {
				  out = append(out, Schedule{
					   ID: row.ID, Tier: Tier(row.Tier), Kind: Kind(row.Kind), Asset: row.Asset,
					  BPS: row.BPS, Active: row.Active, Version: row.Version,
				  })
			  }
			   return out
		  }
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]Schedule,
		   0, len(s.rows))
	  for _, row := range s.rows {
		  out = append(out, row.Copy())
	   }
	  return out
   }
