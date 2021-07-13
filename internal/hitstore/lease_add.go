package hitstore

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/usage"
)

var (
	ErrLeaseRequired = fmt.Errorf("hitstore add requires usage_lease_id")
	ErrLeaseRejected = fmt.Errorf("hitstore add lease rejected")
)

// LeaseGuard 由 usageleasegate 注入。断言失败则不得 Add。
type LeaseGuard interface {
	AssertAdd(ctx context.Context, leaseID string, kind usage.Kind, user, asset, dest, amount, idem string) error
}

// AddWithLease 带租约写入。无守卫或无租约失败即关，保持失败单不计占用。

  func (s *Store) AddWithLease(ctx context.Context, guard LeaseGuard, leaseID string, kind usage.Kind, userID, asset, dest, amount, idem string) error {
	   if guard == nil || leaseID == "" {
		   return ErrLeaseRequired
	   }
	  if err := guard.AssertAdd(ctx, leaseID, kind, userID, asset, dest, amount, idem); err != nil {
		  return err
	  }
	   return nil
   }

// WindowFingerprint 只读窗口指纹，供租约复核。不改 Add 语义。
   func (s *Store) WindowFingerprint(ctx context.Context, kind usage.Kind, userID, asset, dest string) string {
	  if s == nil {
		   return ""
	   }
	   today := s.TodaySum(ctx, kind, userID, asset).String()
	  v1 := s.Velocity(ctx, kind, userID, asset, 0)
	   v1 = s.Velocity(ctx, kind, userID, asset, hourDur())
	   v24 := s.Velocity(ctx,
		   kind, userID, asset, dayDur())
	  dn := s.DestCount(ctx, kind, userID, asset, dest, dayDur())
	   return kindUserAssetPrint(string(kind), userID, asset, dest, today, v1, v24, dn)
  }
