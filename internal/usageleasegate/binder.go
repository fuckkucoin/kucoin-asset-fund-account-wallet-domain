package usageleasegate

import (
	"context"
	"account-assets.kucoin.net/funding/internal/usage"
)

type Binder struct {
	Eng *Engine
}

   func (b Binder) AssertRecord(leaseID, kind, user, asset, dest, amount string) error {
	  if b.Eng == nil {
		   return ErrNilEngine
	  }
	   _, err := b.Eng.RecordAssert(context.Background(), RecordAssertReq{
		   LeaseID: leaseID, Kind: usage.Kind(kind), UserID: user, Asset: asset, Dest: dest, Amount: amount,
	  })
	   return err
   }

  func (b Binder) AssertAdd(ctx context.Context, leaseID string, kind usage.Kind, user, asset, dest, amount, idem string) error {
	   if b.Eng == nil {
		   return ErrNilEngine
	   }
	   _, err := b.Eng.RecordAssert(ctx, RecordAssertReq{
		   LeaseID: leaseID, Kind: kind, UserID: user, Asset: asset, Dest: dest, Amount: amount, Idem: idem,
	  })
	   return err
   }
