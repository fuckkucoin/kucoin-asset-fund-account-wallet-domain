package occupbook

import "context"

// Snapshot 批次只读快照。

type Snapshot struct {
	Lot   *Lot    `json:"lot"`
	Holds []*Hold `json:"holds"`
	Audits []*AuditEvent `json:"audits,omitempty"`
	Conserve bool `json:"conserve"`
}

// SnapshotLot 组装只读快照。

  func (b *Book) SnapshotLot(ctx context.Context, lotID string, withAudit bool) (*Snapshot, error) {
	   if b == nil || b.store == nil {
		   return nil, ErrUnavailable
	   }
	  lot, err := b.store.GetLot(ctx, NormalizeID(lotID))
	  if err != nil {
		   return nil, err
	  }
	  holds, err := b.store.ListHoldsByLot(ctx, lot.LotID)
	   if err != nil {
		  return nil, err
	  }
	   sn := &Snapshot{Lot: lot,
		   Holds: holds, Conserve: lot.ConserveOK()}
	   if withAudit {
		   aud, err := b.store.ListAudit(ctx, lot.LotID, 50)
		   if err != nil {
			   return nil, err
		   }
		  sn.Audits = aud
	  }
	   return sn, nil
  }

// HoldByID 只读 Hold。

  func (b *Book) HoldByID(ctx context.Context, holdID string) (*Hold, error) {
	   if b == nil || b.store == nil {
		   return nil, ErrUnavailable
	   }
	   return b.store.GetHold(ctx, NormalizeID(holdID))
  }
