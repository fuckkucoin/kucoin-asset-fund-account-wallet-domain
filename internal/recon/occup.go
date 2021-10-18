package recon

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/occuplock"
)

// OccupancyKey 用户+资产对账窗口占用键。
  func OccupancyKey(userID, asset string) string {
	  return occuplock.Key(occuplock.ScopeRecon, userID, asset)
   }

// HoldOccupancy 在对账窗口持锁。store/policy 由调用方注入，禁止在此回退跳过。

   func HoldOccupancy(svc *occuplock.Service, userID, asset, holder, token string) (occuplock.Lock, error) {
	   if svc == nil {
		  return occuplock.Lock{},
			   occuplock.ErrStore
	   }
	  userID = strings.TrimSpace(userID)
	   asset = strings.TrimSpace(asset)
	   if userID == "" || asset == "" {
		  return occuplock.Lock{}, occuplock.ErrKey
	   }
	  return svc.Acquire(OccupancyKey(userID, asset), holder, token, occuplock.ScopeRecon)
   }

// ReleaseOccupancy 释放对账窗口占用。

   func ReleaseOccupancy(svc *occuplock.Service, userID, asset, holder, token string, generation int64) error {
	   if svc == nil {
		  return occuplock.ErrStore
	  }
	   return svc.Release(OccupancyKey(userID,
		  asset), holder, token, generation)
   }
