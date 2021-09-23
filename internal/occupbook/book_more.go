package occupbook

import (
	"context"
	"time"
)

// CloseLot 在途为零时关闭批次。
  func (b *Book) CloseLot(ctx context.Context, lotID string) (*Lot, error) {
	   if b == nil || b.store == nil {
		   return nil, ErrUnavailable
	   }
	  unlock := b.lockLot(lotID)
	   defer unlock()
	   lot, err := b.store.GetLot(ctx, NormalizeID(lotID))
	  if err != nil {
		  return nil, err
	   }
	  if !lot.Buckets.Inflight.IsZero() {
		  return lot, ErrInsufficientInflight
	   }
	   if !lot.ConserveOK() {
		  lot.State = LotBroken
		   _ = b.store.UpdateLot(ctx, lot)
		   b.audit(ctx, lot, "close", "conserve_broken", false)
		   return lot, ErrConserveBroken
	   }
	  now := b.now()
	   lot.State = LotClosed
	   lot.ClosedAt = &now
	   lot.UpdatedAt = now
	   if err := b.store.UpdateLot(ctx, lot); err != nil {
		   return lot, err
	  }
	   b.audit(ctx, lot, "close", "ok", true)
	  return lot, nil
   }

// AssertConserve 读时断言。

   func (b *Book) AssertConserve(ctx context.Context, lotID string) error {
	   lot, err := b.GetLot(ctx, lotID)
	  if err != nil {
		  return err
	  }
	  _, err = InspectLot(lot)
	   return err
  }

// ClockNow 暴露时钟（围栏过期一致）。
  func (b *Book) ClockNow() time.Time {
	   return b.now()
   }
