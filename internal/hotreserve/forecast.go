package hotreserve

import (
	"context"
	"fmt"
	"sort"
	"time"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"github.com/google/uuid"
)

   func (s *Service) readSurface(ctx context.Context, asset string) (hotinv.Surface, error) {
	   if s == nil || s.hot == nil || s.books == nil {
		  return hotinv.Surface{}, ErrUnread
	   }
	  sur, err := s.hot.Surface(ctx, s.books, asset)
	   if err != nil {
		  return hotinv.Surface{}, fmt.Errorf("%w: %v", ErrUnread, err)
	  }
	   return sur, nil
  }

// Refresh 重算单资产快照。输入摘要变化才升代次。
   func (s *Service) Refresh(ctx context.Context, asset string) (HotForecast, error) {
	  if s == nil {
		  return HotForecast{}, ErrUnavailable
	   }
	  as, err := normAsset(asset)
	  if err != nil {
		  return HotForecast{}, err
	   }
	   sur, err := s.readSurface(ctx, as)
	  if err != nil {
		   return HotForecast{}, err
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if err := s.guardLocked(); err != nil {
		  return HotForecast{}, err
	  }
	   return s.publishLocked(ctx, sur)
   }

// RefreshAll 对当前热库存里出现的资产各刷新一次。

  func (s *Service) RefreshAll(ctx context.Context) ([]HotForecast, error) {
	   if s == nil || s.hot == nil || s.books == nil {
		  return nil, ErrUnread
	  }
	   rows,
		   err := s.hot.Surfaces(ctx, s.books)
	  if err != nil {
		   return nil, fmt.Errorf("%w: %v", ErrUnread, err)
	   }
	   out := make([]HotForecast, 0, len(rows))
	   for _, sur := range rows {
		  s.mu.Lock()
		  if err := s.guardLocked(); err != nil {
			  s.mu.Unlock()
			   return nil, err
		   }
		  got,
			  perr := s.publishLocked(ctx, sur)
		  s.mu.Unlock()
		   if perr != nil {
			  return nil, perr
		  }
		   out = append(out, got)
	   }
	  return out, nil
   }

   func (s *Service) publishLocked(ctx context.Context, sur hotinv.Surface) (HotForecast, error) {
	   asset := sur.Asset
	  now := s.now()
	   est, err := estimate(s.samples[asset], now, s.pol.Lookback, s.pol.Window)
	  if err != nil {
		   return HotForecast{}, err
	  }
	   sweep := s.planSum(asset)
	  transit := s.sumTrack(asset, TrackBroadcast)
	  ids := make([]string, 0, len(s.plans[asset]))
	  for _, p := range s.plans[asset] {
		  ids = append(ids, p.ID)
	  }
	   sort.Strings(ids)
	   in := ProjectInput{
		  HotAvail: sur.Available.String(),
		   Inflow: est.In.String(),
		  Outflow:   est.Out.String(),
		   SweepHold: sweep.String(),
		   InTransit: transit.String(),
		   Haircut:   s.pol.Haircut,
		   Stress:    s.pol.Stress,
		   Buffer:    s.pol.Buffer,
	  }
	  res, err := Project(in)
	  if err != nil {
		   return HotForecast{}, err
	   }
	  sum := Digest(asset,
		  in, int64(s.pol.Window/time.Second), est.N, ids)
	   ver := int64(1)
	  bias, realized := "", ""
	   if prev := s.forecast[asset]; prev != nil {
		   ver = prev.Version
		   bias,
			  realized = prev.Bias, prev.Realized
		  if prev.InputsHash != sum {
			  ver++
		   }
	  }
	  row := &HotForecast{
		   Asset: asset, Version: ver, WindowSec: int64(s.pol.Window / time.Second), AsOf: now,
		   HotAvail: sur.Available.String(), HotFrozen: sur.Frozen.String(),
		   Point: res.Point, Lower: res.Lower,
		  Inflow: in.Inflow, Outflow: in.Outflow,
		   InflowKept: res.InflowKept, OutflowStressed: res.OutflowStressed,
		   SweepHold: in.SweepHold, InTransit: in.InTransit, Buffer: in.Buffer,
		   Haircut: in.Haircut, Stress: in.Stress, InputsHash: sum, SweepIDs: ids,
		   SampleN: est.N,
		  Bias:    bias, Realized: realized,
		  EventID: fmt.Sprintf("hr.forecast.%s.%d", asset, ver),
	   }
	   if err := s.persistForecast(ctx, row); err != nil {
		   return HotForecast{}, err
	   }
	  if prev := s.forecast[asset]; prev == nil || prev.Version != row.Version {
		  if err := s.persistHistory(ctx, row); err != nil {
			  return HotForecast{}, err
		  }
		   s.history[asset] = append(s.history[asset], ForecastHead{
			   Asset: asset, Version: row.Version, Hash: row.InputsHash,
			  Lower: row.Lower,
			   Point: row.Point, AsOf: row.AsOf, EventID: row.EventID,
		  })
		  if len(s.history[asset]) > 64 {
			  s.history[asset] = s.history[asset][len(s.history[asset])-64:]
		  }
	  }
	  s.forecast[asset] = row
	  s.paintForecast(row)
	  cp := *row
	  cp.SweepIDs = append([]string(nil), row.SweepIDs...)
	  return cp, nil
  }

// Forecast 返回当前快照。没有快照视为缺失，不返回零头寸。

   func (s *Service) Forecast(asset string) (HotForecast, error) {
	  if s == nil {
		   return HotForecast{}, ErrUnavailable
	   }
	   as, err := normAsset(asset)
	  if err != nil {
		  return HotForecast{}, err
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  row := s.forecast[as]
	   if row == nil {
		  return HotForecast{}, ErrForecastMissing
	  }
	   cp := *row
	   cp.SweepIDs = append([]string(nil), row.SweepIDs...)
	   return cp, nil
   }

// History 返回该资产已发布过的代次头。

   func (s *Service) History(asset string) ([]ForecastHead, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	  as, err := normAsset(asset)
	   if err != nil {
		  return nil, err
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   src := s.history[as]
	  out := make([]ForecastHead, len(src))
	   copy(out, src)
	   return out, nil
  }

// RecordSample 记下一段历史消耗。负值拒绝。样本本身不升代次，下次刷新才升。
   func (s *Service) RecordSample(ctx context.Context, asset, inflow, outflow string, at time.Time) (Sample, error) {
	   if s == nil {
		  return Sample{}, ErrUnavailable
	   }
	   as, err := normAsset(asset)
	   if err != nil {
		   return Sample{}, err
	   }
	   in, err := parseNonNeg(inflow)
	   if err != nil {
		  return Sample{}, err
	   }
	   out, err := parseNonNeg(outflow)
	  if err != nil {
		  return Sample{}, err
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   if at.IsZero() {
		  at = s.now()
	  }
	   at = at.UTC()
	   if at.After(s.now().Add(timeSkew())) {
		  return Sample{}, ErrAmount
	  }
	   sm := Sample{ID: uuid.NewString(),
		   Asset: as, Inflow: in.String(), Outflow: out.String(), At: at}
	  if err := s.persistSample(ctx, sm); err != nil {
		   return Sample{}, err
	  }
	   s.samples[as] = append(s.samples[as], sm)
	  return sm, nil
  }

   func timeSkew() time.Duration { return 2 * time.Second }
