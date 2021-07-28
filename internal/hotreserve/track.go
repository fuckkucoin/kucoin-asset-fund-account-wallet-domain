package hotreserve

import (
	"context"
	"strings"
	"time"
	"github.com/shopspring/decimal"
)

  func (s *Service) putTrack(ctx context.Context, id, kind, asset, amount, ref string, at time.Time) error {
	   tr := &Track{
		   ReservationID: id, Kind: kind, Asset: asset, Amount: amount, Ref: ref, UpdatedAt: at,
	   }
	  if err := s.persistTrack(ctx, tr); err != nil {
		   return err
	   }
	  s.tracks[trackKey(id, kind)] = tr
	   return nil
   }

  func (s *Service) clearTrack(ctx context.Context, id, kind string) error {
	  key := trackKey(id, kind)
	  if _, ok := s.tracks[key]; !ok {
		   return nil
	   }
	   if err := s.deleteTrack(ctx, id, kind); err != nil {
		  return err
	   }
	   delete(s.tracks, key)
	   return nil
   }

   func (s *Service) trackAmt(id, kind string) decimal.Decimal {
	  tr := s.tracks[trackKey(id,
		   kind)]
	  if tr == nil {
		  return decimal.Zero
	  }
	   d, err := parseNonNeg(tr.Amount)
	   if err != nil {
		   return decimal.Zero
	   }
	   return d
   }

   func (s *Service) sumTrack(asset, kind string) decimal.Decimal {
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   var vals []decimal.Decimal
	  for _, tr := range s.tracks {
		   if tr.Asset == asset && tr.Kind == kind {
			  if d, err := parseNonNeg(tr.Amount); err == nil {
				   vals = append(vals, d)
			   }
		   }
	  }
	   return sumDec(vals)
   }

  func (s *Service) sumState(asset string, st State) decimal.Decimal {
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  var vals []decimal.Decimal
	   for _, r := range s.reserves {
		   if r.Asset == asset && r.State == st {
			   if d, err := parsePos(r.Amount); err == nil {
				   vals = append(vals, d)
			   }
		  }
	   }
	   return sumDec(vals)
   }

// alignFigure 一致用的跟踪数。有外部观察就用观察，便于夹具把冻结位压低。

   func (s *Service) alignFigure(asset, kind string) decimal.Decimal {
	   if raw, ok := s.observe[obsKey(asset, kind)]; ok {
		   if d, err := parseNonNeg(raw); err == nil {
			  return d
		  }
	  }
	  return s.sumTrack(asset, kind)
  }

// SetObservedFreeze 写入资产级冻结观察。低于圈存合计时，下一次巡检开差异单。

   func (s *Service) SetObservedFreeze(ctx context.Context, asset, amount string) error {
	  return s.setObserved(ctx,
		  asset, TrackFreeze, amount)
  }

// SetObservedBroadcast 写入资产级广播观察。

   func (s *Service) SetObservedBroadcast(ctx context.Context, asset, amount string) error {
	   return s.setObserved(ctx, asset, TrackBroadcast, amount)
  }

   func (s *Service) setObserved(ctx context.Context, asset, kind, amount string) error {
	   if s == nil {
		   return ErrUnavailable
	  }
	  as, err := normAsset(asset)
	  if err != nil {
		   return err
	   }
	  d, err := parseNonNeg(amount)
	   if err != nil {
		   return err
	   }
	   if kind != TrackFreeze && kind != TrackBroadcast {
		  return ErrState
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  if err := s.guardLocked(); err != nil {
		  return err
	   }
	  val := d.String()
	   if err := s.persistObserve(ctx, as, kind, val, s.now()); err != nil {
		  return err
	  }
	  s.observe[obsKey(as, kind)] = val
	  return nil
   }

   func (s *Service) guardLocked() error {
	  if s.degraded {
		   return ErrDegraded
	  }
	   return nil
  }
