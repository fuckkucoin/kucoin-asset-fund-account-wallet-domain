package depoccup

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

  func (s *Service) persistLotLocked(lot *Lot) error {
	   if s == nil || s.db == nil || s.db.DB == nil || lot == nil {
		   return nil
	  }
	  return s.db.InTx(context.Background(), func(ctx context.Context) error {
		   if err := s.saveLot(ctx, lot); err != nil {
			   return err
		   }
		   if err := s.saveBucket(ctx, s.buckets[bucketKey(lot.UserID, lot.Asset)]); err != nil {
			   return err
		  }
		   if st := s.tiers[lot.UserID]; st != nil {
			   if err := s.saveTierCtx(ctx, st); err != nil {
				   return err
			   }
		   }
		   for _, id := range s.sigByUser[lot.UserID] {
			  if sig := s.signals[id]; sig != nil {
				  if err := s.saveSignalCtx(ctx, sig); err != nil {
					  return err
				  }
			  }
		   }
		  return nil
	   })
  }

  func (s *Service) savePolicy(p *Policy) error {
	  if s == nil || s.db == nil || s.db.DB == nil || p == nil {
		  return nil
	  }
	  return s.savePolicyCtx(context.Background(), p)
  }

  func (s *Service) saveTier(st *TierState) error {
	   if s == nil || s.db == nil || s.db.DB == nil || st == nil {
		  return nil
	  }
	   return s.saveTierCtx(context.Background(), st)
   }

   func (s *Service) saveSignal(sig *Signal) error {
	   if s == nil || s.db == nil || s.db.DB == nil || sig == nil {
		  return nil
	  }
	   return s.saveSignalCtx(context.Background(), sig)
  }

  func (s *Service) saveAudit(a Audit) error {
	  if s == nil || s.db == nil || s.db.DB == nil {
		   return nil
	  }
	   return s.insertAudit(context.Background(), a)
  }

   func (s *Service) upsert(ctx context.Context, mysql, pg string, args ...any) error {
	  q := mysql
	   if s.db.Dialect == persist.Postgres {
		   q = pg
	  }
	  _, err := s.db.Conn(ctx).ExecContext(ctx, q, args...)
	   return err
   }

   func (s *Service) savePolicyCtx(ctx context.Context, p *Policy) error {
	   en := 0
	   if p.Enabled {
		  en = 1
	   }
	  my := `INSERT INTO dep_occupancy_policy
(id,kyc,asset,source,chain,ratio_bps,max_window_sec,min_window_sec,tier_floor,version,priority,enabled,note)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE ratio_bps=VALUES(ratio_bps), max_window_sec=VALUES(max_window_sec), min_window_sec=VALUES(min_window_sec), tier_floor=VALUES(tier_floor), version=VALUES(version), priority=VALUES(priority), enabled=VALUES(enabled), note=VALUES(note)`
	   pg := `INSERT INTO dep_occupancy_policy
(id,kyc,asset,source,chain,ratio_bps,max_window_sec,min_window_sec,tier_floor,version,priority,enabled,note)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET ratio_bps=EXCLUDED.ratio_bps, max_window_sec=EXCLUDED.max_window_sec, min_window_sec=EXCLUDED.min_window_sec, tier_floor=EXCLUDED.tier_floor, version=EXCLUDED.version, priority=EXCLUDED.priority, enabled=EXCLUDED.enabled, note=EXCLUDED.note`
	  return s.upsert(ctx,
		   my, pg, p.ID, p.KYC, p.Asset, string(p.Source), p.Chain, p.RatioBPS, p.MaxSec, p.MinSec, string(p.TierFloor), p.Version, p.Priority, en, p.Note)
   }

  func (s *Service) saveLot(ctx context.Context, lot *Lot) error {
	   early := 0
	   if lot.EarlyCredit {
		  early = 1
	  }
	  hit := 0
	  if lot.WindowHit {
		  hit = 1
	  }
	  rel := sql.NullTime{Time: lot.ReleaseAt,
		  Valid: !lot.ReleaseAt.IsZero()}
	  my := `INSERT INTO dep_occupancy_lot
(id,transfer_id,idem,user_id,chain,asset,source,kyc,tx_hash,from_addr,observed,occupied,free_amt,state,tier,policy_id,ratio_bps,max_window_sec,min_window_sec,early_credit,confirms,full_need,observed_at,release_at,deadline,cause,window_hit,updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE state=VALUES(state), tier=VALUES(tier), policy_id=VALUES(policy_id), ratio_bps=VALUES(ratio_bps), occupied=VALUES(occupied), free_amt=VALUES(free_amt), source=VALUES(source), kyc=VALUES(kyc), early_credit=VALUES(early_credit), confirms=VALUES(confirms), release_at=VALUES(release_at), cause=VALUES(cause), window_hit=VALUES(window_hit), updated_at=VALUES(updated_at)`
	   pg := `INSERT INTO dep_occupancy_lot  
(id,transfer_id,idem,user_id,chain,asset,source,kyc,tx_hash,from_addr,observed,occupied,free_amt,state,tier,policy_id,ratio_bps,max_window_sec,min_window_sec,early_credit,confirms,full_need,observed_at,release_at,deadline,cause,window_hit,updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET state=EXCLUDED.state, tier=EXCLUDED.tier, policy_id=EXCLUDED.policy_id, ratio_bps=EXCLUDED.ratio_bps, occupied=EXCLUDED.occupied, free_amt=EXCLUDED.free_amt, source=EXCLUDED.source, kyc=EXCLUDED.kyc, early_credit=EXCLUDED.early_credit, confirms=EXCLUDED.confirms, release_at=EXCLUDED.release_at, cause=EXCLUDED.cause, window_hit=EXCLUDED.window_hit, updated_at=EXCLUDED.updated_at`
	   return s.upsert(ctx, my, pg,
		  lot.ID,
		  lot.TransferID, lot.Idem, lot.UserID, lot.Chain, lot.Asset, string(lot.Source), lot.KYC, lot.TxHash, lot.FromAddress,
		   lot.Observed, lot.Occupied, lot.Free, string(lot.State), string(lot.Tier), lot.PolicyID, lot.RatioBPS, lot.MaxSec, lot.MinSec,
		  early,
		  lot.Confirms, lot.FullNeed, lot.ObservedAt, rel, lot.Deadline, lot.Cause, hit, lot.UpdatedAt)
  }

  func (s *Service) saveBucket(ctx context.Context, b *Bucket) error {
	   if b == nil {
		  return nil
	  }
	  my := `INSERT INTO dep_occupancy_bucket (user_id,asset,in_transit,available,rolled) VALUES (?,?,?,?,?) 
ON DUPLICATE KEY UPDATE in_transit=VALUES(in_transit), available=VALUES(available), rolled=VALUES(rolled)`
	  pg := `INSERT INTO dep_occupancy_bucket (user_id,asset,in_transit,available,rolled) VALUES (?,?,?,?,?)
ON CONFLICT (user_id, asset) DO UPDATE SET in_transit=EXCLUDED.in_transit, available=EXCLUDED.available, rolled=EXCLUDED.rolled`
	   return s.upsert(ctx, my, pg, b.UserID, b.Asset, zeroAmt(b.InTransit), zeroAmt(b.Available), zeroAmt(b.Rolled))
   }

   func (s *Service) saveTierCtx(ctx context.Context, st *TierState) error {
	   my := `INSERT INTO dep_occupancy_tier
(user_id,tier,score,clean_run,kyc,last_source,since_at,last_up,last_down,reason,version)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE tier=VALUES(tier), score=VALUES(score), clean_run=VALUES(clean_run), kyc=VALUES(kyc), last_source=VALUES(last_source), last_up=VALUES(last_up), last_down=VALUES(last_down), reason=VALUES(reason), version=VALUES(version)`
	  pg := `INSERT INTO dep_occupancy_tier
(user_id,tier,score,clean_run,kyc,last_source,since_at,last_up,last_down,reason,version)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (user_id) DO UPDATE SET tier=EXCLUDED.tier, score=EXCLUDED.score, clean_run=EXCLUDED.clean_run, kyc=EXCLUDED.kyc, last_source=EXCLUDED.last_source, last_up=EXCLUDED.last_up, last_down=EXCLUDED.last_down, reason=EXCLUDED.reason, version=EXCLUDED.version`
	  return s.upsert(ctx, my, pg, st.UserID, string(st.Tier), st.Score, st.CleanRun, st.KYC, string(st.LastSource), st.Since, nullTime(st.LastUp), nullTime(st.LastDown), st.Reason, st.Version)
  }

  func (s *Service) saveSignalCtx(ctx context.Context, sig *Signal) error {
	  my := `INSERT INTO dep_occupancy_signal (id,user_id,kind,weight,ref,asset,chain,detail,at) VALUES (?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE weight=VALUES(weight), detail=VALUES(detail)`
	  pg := `INSERT INTO dep_occupancy_signal (id,user_id,kind,weight,ref,asset,chain,detail,at) VALUES (?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET weight=EXCLUDED.weight, detail=EXCLUDED.detail`
	   return s.upsert(ctx,
		   my, pg, sig.ID, sig.UserID, string(sig.Kind), sig.Weight, sig.Ref, sig.Asset, sig.Chain, sig.Detail, sig.At)
  }

  func (s *Service) insertAudit(ctx context.Context, a Audit) error {
	   my := `INSERT INTO dep_occupancy_audit (id,lot_id,user_id,event,from_state,to_state,amount,actor,at) VALUES (?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE event=VALUES(event)`
	  pg := `INSERT INTO dep_occupancy_audit (id,lot_id,user_id,event,from_state,to_state,amount,actor,at) VALUES (?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO NOTHING`
	   return s.upsert(ctx, my, pg, a.ID, a.LotID, a.UserID, a.Event, string(a.FromState), string(a.ToState), a.Amount, a.Actor, a.At)
  }

  func nullTime(t time.Time) sql.NullTime {
	  if t.IsZero() {
		  return sql.NullTime{}
	   }
	  return sql.NullTime{Time: t,
		  Valid: true}
   }

  func tableMissing(err error) bool {
	   if err == nil {
		  return false
	   }
	  s := strings.ToLower(err.Error())
	   return strings.Contains(s, "doesn't exist") || strings.Contains(s, "does not exist") ||
		  strings.Contains(s, "no such table") || strings.Contains(s, "undefined_table") || strings.Contains(s, "1146")
  }
