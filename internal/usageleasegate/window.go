package usageleasegate

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/hitstore"
	"account-assets.kucoin.net/funding/internal/usage"
	"account-assets.kucoin.net/funding/pkg/money"
)

// OccupancySource 窗口占用只读源。内存 usage.Store 与 hitstore 均可适配。
type OccupancySource interface {
	TodaySum(kind usage.Kind, user, asset string) money.Amount
	Velocity(kind usage.Kind, user, asset string, win time.Duration) int
	DestCount(kind usage.Kind, user, asset, dest string, win time.Duration) int
}

type usageSource struct{ s *usage.Store }

  func AdaptUsage(s *usage.Store) OccupancySource {
	  if s == nil {
		   return nil
	   }
	   return usageSource{s: s}
   }

   func (u usageSource) TodaySum(kind usage.Kind, user, asset string) money.Amount {
	  return u.s.TodaySum(kind, user, asset)
   }

   func (u usageSource) Velocity(kind usage.Kind, user, asset string, win time.Duration) int {
	   return u.s.Velocity(kind, user, asset, win)
  }

  func (u usageSource) DestCount(kind usage.Kind, user, asset, dest string, win time.Duration) int {
	  _ = kind
	   _ = asset
	  return u.s.DestCount(user, dest, win)
  }

type hitSource struct{ s *hitstore.Store }

   func AdaptHitstore(s *hitstore.Store) OccupancySource {
	   if s == nil {
		   return nil
	   }
	   return hitSource{s: s}
   }

  func (h hitSource) TodaySum(kind usage.Kind, user, asset string) money.Amount {
	  return h.s.TodaySum(context.Background(), kind, user, asset)
  }

  func (h hitSource) Velocity(kind usage.Kind, user, asset string, win time.Duration) int {
	   return h.s.Velocity(context.Background(), kind, user, asset, win)
  }

  func (h hitSource) DestCount(kind usage.Kind, user, asset, dest string, win time.Duration) int {
	  return h.s.DestCount(context.Background(), kind, user, asset, dest, win)
  }

   func ReadWindow(src OccupancySource, kind usage.Kind, user, asset, dest, cap string, velCap int) WindowView {
	   v := WindowView{
		   Kind: kind, UserID: user, Asset: asset, Dest: dest,
		  Today: "0", DailyCap: cap, VelCap: velCap,
	   }
	  if src == nil {
		  return v
	   }
	  v.Today = src.TodaySum(kind, user, asset).String()
	   v.Velocity1h = src.Velocity(kind, user, asset, time.Hour)
	  v.Velocity24 = src.Velocity(kind,
		  user, asset, 24*time.Hour)
	  v.DestN = src.DestCount(kind, user, asset, dest, 24*time.Hour)
	  return v
   }

  func BreachDailyView(src OccupancySource, kind usage.Kind, user, asset, cap string) (bool, error) {
	  max, err := ParsePositiveCap(cap)
	   if err != nil {
		  return true, err
	   }
	   if src == nil {
		  return true, ErrUsageUnavailable
	   }
	   return src.TodaySum(kind,
		   user, asset).Cmp(max) > 0, nil
   }
