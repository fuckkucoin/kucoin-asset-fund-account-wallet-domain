package usageleasegate

import (
	"context"
	"account-assets.kucoin.net/funding/internal/hitstore"
	"account-assets.kucoin.net/funding/internal/usage"
)

// BindUsageRecord 将 usage.Record 升级为租约断言写入。

   func BindUsageRecord(eng *Engine, kind usage.Kind, user, asset, dest, amount, leaseID string) error {
	  if eng == nil {
		   return ErrNilEngine
	  }
	   _, err := eng.RecordAssert(context.Background(), RecordAssertReq{
		   LeaseID: leaseID, Kind: kind, UserID: user, Asset: asset, Dest: dest, Amount: amount,
	  })
	  return err
   }

// BindHitstoreAdd 将 hitstore.Add 升级为租约断言写入。
   func BindHitstoreAdd(eng *Engine, hs *hitstore.Store, kind usage.Kind, user, asset, dest, amount, idem, leaseID string) error {
	   if eng == nil {
		   return ErrNilEngine
	   }
	   if hs != nil {
		  eng.BindHits(hs)
	   }
	   _, err := eng.RecordAssert(context.Background(), RecordAssertReq{
		  LeaseID: leaseID,
		  Kind:    kind, UserID: user, Asset: asset, Dest: dest, Amount: amount, Idem: idem,
	  })
	  return err
  }

// SnapshotUsage 从 usage.Store 只读快照签发租约。
   func SnapshotUsage(eng *Engine, kind usage.Kind, user, asset, dest, amount, cap string) (SnapshotWithLeaseResp, error) {
	   if eng == nil {
		  return SnapshotWithLeaseResp{}, ErrNilEngine
	   }
	   return eng.SnapshotWithLease(context.Background(), SnapshotWithLeaseReq{
		  Kind: kind, UserID: user, Asset: asset, Dest: dest, Amount: amount, DailyCap: cap, Reserve: true,
	   })
   }
