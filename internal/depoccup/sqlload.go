package depoccup

import (
	"context"
	"database/sql"
	"time"
)

   func (s *Service) Load(ctx context.Context) error {
	   if s == nil || s.db == nil || s.db.DB == nil {
		   return nil
	   }
	   if ctx == nil {
		   ctx = context.Background()
	  }
	   pols, err := s.readPolicies(ctx)
	  if err != nil {
		   if tableMissing(err) {
			   return nil
		   }
		   s.mu.Lock()
		   s.loadErr = err.Error()
		   s.mu.Unlock()
		   return err
	   }
	   lots, err := s.readLots(ctx)
	  if err != nil && !tableMissing(err) {
		  s.setLoadErr(err)
		  return err
	  }
	   buckets, err := s.readBuckets(ctx)
	   if err != nil && !tableMissing(err) {
		   s.setLoadErr(err)
		   return err
	   }
	   tiers, err := s.readTiers(ctx)
	   if err != nil && !tableMissing(err) {
		   s.setLoadErr(err)
		   return err
	  }
	  sigs, err := s.readSignals(ctx)
	   if err != nil && !tableMissing(err) {
		  s.setLoadErr(err)
		   return err
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  for i := range pols {
		   p := pols[i]
		   if prev := s.byKey[p.key()]; prev != nil {
			   prev.Enabled = false
		  }
		  cp := p
		   s.policies = append(s.policies, &cp)
		  s.byKey[cp.key()] = &cp
	  }
	  if len(lots) > 0 {
		   s.lots = map[string]*Lot{}
		   s.byXfer = map[string]string{}
		  s.byIdem = map[string]string{}
		  for i := range lots {
			   cp := lots[i]
			  s.lots[cp.ID] = &cp
			  if cp.TransferID != "" {
				  s.byXfer[cp.TransferID] = cp.ID
			  }
			   if cp.Idem != "" {
				   s.byIdem[cp.Idem] = cp.ID
			  }
		  }
	   }
	  for i := range buckets {
		   cp := buckets[i]
		  s.buckets[bucketKey(cp.UserID, cp.Asset)] = &cp
	   }
	  for i := range tiers {
		   cp := tiers[i]
		  s.tiers[cp.UserID] = &cp
	  }
	  if len(sigs) > 0 {
		   s.signals = map[string]*Signal{}
		  s.sigByUser = map[string][]string{}
		  for i := range sigs {
			   cp := sigs[i]
			   s.signals[cp.ID] = &cp
			  s.sigByUser[cp.UserID] = append(s.sigByUser[cp.UserID],
				  cp.ID)
		   }
	   }
	  s.loadErr = ""
	   return nil
  }

   func (s *Service) setLoadErr(err error) {
	   if s == nil || err == nil {
		   return
	  }
	   s.mu.Lock()
	  s.loadErr = err.Error()
	  s.mu.Unlock()
   }

  func (s *Service) readPolicies(ctx context.Context) ([]Policy, error) {
	  rows, err := s.db.Conn(ctx).QueryContext(ctx, `SELECT id,kyc,asset,source,chain,ratio_bps,max_window_sec,min_window_sec,tier_floor,version,priority,enabled,note FROM dep_occupancy_policy`)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	  var out []Policy
	  for rows.Next() {
		  var p Policy
		   var src, floor string
		  var en int
		   if err := rows.Scan(&p.ID, &p.KYC, &p.Asset, &src, &p.Chain, &p.RatioBPS, &p.MaxSec, &p.MinSec, &floor, &p.Version, &p.Priority, &en, &p.Note); err != nil {
			   return nil, err
		   }
		   p.Source = Source(src)
		  p.TierFloor = Tier(floor)
		  p.Enabled = en != 0
		  p.MaxWindow = time.Duration(p.MaxSec) * time.Second
		   p.MinWindow = time.Duration(p.MinSec) * time.Second
		   out = append(out, p)
	   }
	  return out, rows.Err()
  }

  func (s *Service) readLots(ctx context.Context) ([]Lot, error) {
	  rows, err := s.db.Conn(ctx).QueryContext(ctx, `SELECT id,transfer_id,idem,user_id,chain,asset,source,kyc,tx_hash,from_addr,observed,occupied,free_amt,state,tier,policy_id,ratio_bps,max_window_sec,min_window_sec,early_credit,confirms,full_need,observed_at,release_at,deadline,cause,window_hit,updated_at FROM dep_occupancy_lot`)
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  var out []Lot
	   for rows.Next() {
		  var lot Lot
		  var src, state, tier string
		  var early, hit int
		  var rel sql.NullTime
		   if err := rows.Scan(&lot.ID, &lot.TransferID, &lot.Idem, &lot.UserID, &lot.Chain, &lot.Asset, &src, &lot.KYC, &lot.TxHash, &lot.FromAddress, &lot.Observed, &lot.Occupied, &lot.Free, &state, &tier, &lot.PolicyID, &lot.RatioBPS, &lot.MaxSec, &lot.MinSec, &early, &lot.Confirms, &lot.FullNeed, &lot.ObservedAt, &rel, &lot.Deadline, &lot.Cause, &hit, &lot.UpdatedAt); err != nil {
			   return nil, err
		  }
		   lot.Source = Source(src)
		   lot.State = LotState(state)
		   lot.Tier = Tier(tier)
		  lot.EarlyCredit = early != 0
		  lot.WindowHit = hit != 0
		  if rel.Valid {
			  lot.ReleaseAt = rel.Time
		  }
		  out = append(out, lot)
	   }
	  return out, rows.Err()
   }

  func (s *Service) readBuckets(ctx context.Context) ([]Bucket, error) {
	   rows, err := s.db.Conn(ctx).QueryContext(ctx, `SELECT user_id,asset,in_transit,available,rolled FROM dep_occupancy_bucket`)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   var out []Bucket
	   for rows.Next() {
		   var b Bucket
		   if err := rows.Scan(&b.UserID, &b.Asset, &b.InTransit, &b.Available, &b.Rolled); err != nil {
			  return nil, err
		   }
		  out = append(out, b)
	   }
	   return out, rows.Err()
  }

   func (s *Service) readTiers(ctx context.Context) ([]TierState, error) {
	  rows, err := s.db.Conn(ctx).QueryContext(ctx, `SELECT user_id,tier,score,clean_run,kyc,last_source,since_at,last_up,last_down,reason,version FROM dep_occupancy_tier`)
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  var out []TierState
	   for rows.Next() {
		  var st TierState
		  var tier, src string
		  var up, down sql.NullTime
		   if err := rows.Scan(&st.UserID, &tier, &st.Score, &st.CleanRun, &st.KYC, &src, &st.Since, &up, &down, &st.Reason, &st.Version); err != nil {
			  return nil, err
		   }
		  st.Tier = Tier(tier)
		  st.LastSource = Source(src)
		  if up.Valid {
			  st.LastUp = up.Time
		   }
		   if down.Valid {
			   st.LastDown = down.Time
		   }
		  out = append(out, st)
	   }
	  return out, rows.Err()
  }

   func (s *Service) readSignals(ctx context.Context) ([]Signal, error) {
	  rows, err := s.db.Conn(ctx).QueryContext(ctx, `SELECT id,user_id,kind,weight,ref,asset,chain,detail,at FROM dep_occupancy_signal ORDER BY at ASC LIMIT 2000`)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   var out []Signal
	   for rows.Next() {
		   var sig Signal
		  var kind string
		   if err := rows.Scan(&sig.ID, &sig.UserID, &kind, &sig.Weight, &sig.Ref, &sig.Asset, &sig.Chain, &sig.Detail, &sig.At); err != nil {
			   return nil, err
		   }
		   sig.Kind = SignalKind(kind)
		   out = append(out, sig)
	   }
	  return out, rows.Err()
  }
