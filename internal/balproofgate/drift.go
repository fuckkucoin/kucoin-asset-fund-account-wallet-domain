package balproofgate

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

// DriftDetector 开立、附证、收口。
type DriftDetector struct {
	store Store
	clock func() time.Time
}

   func newDriftDetector(store Store, clock func() time.Time) *DriftDetector {
	   if clock == nil {
		   clock = func() time.Time { return time.Now().UTC() }
	}
	return &DriftDetector{store: store, clock: clock}
}

// OpenFromRecheck 复核失败时开立并立即 proven。幂等：同租约已有 blocking 案则复用。

  func (d *DriftDetector) OpenFromRecheck(ctx context.Context, lease ProofLease, live LiveView, kind DriftKind) (*DriftProof, error) {
	  if d == nil || d.store == nil {
		  return nil, ErrUnavailable
	   }
	  if kind == "" {
		   kind = ClassifyDrift(lease.Snapshot, live)
	  }
	   if kind == "" {
		   return nil, nil
	  }
	   exist, err := d.store.FindOpenDrift(ctx, lease.ID)
	  if err != nil {
		  return nil, err
	   }
	  if exist != nil {
		   cp := exist.Copy()
		  return &cp, nil
	  }
	   now := d.clock()
	   p := &DriftProof{
		  ID:      uuid.NewString(),
		   LeaseID: lease.ID,
		  UserID: lease.UserID,
		   Book:   lease.Book,
		  Asset:        lease.Asset,
		  Kind:         kind,
		   Status:       DriftOpen,
		   IssuedAvail:  lease.Snapshot.Available,
		   IssuedFrozen: lease.Snapshot.Frozen,
		  IssuedObsGen: lease.Snapshot.ObserveGen,
		  LiveAvail:    strings.TrimSpace(live.Available),
		  LiveFrozen: strings.TrimSpace(live.Frozen),
		  LiveObsGen: live.ObserveGen,
		  OpenedAt:      now,
		   IsolationMark: IsolationMark,
	  }
	   if p.LiveAvail == "" {
		   p.LiveAvail = "0"
	  }
	   if p.LiveFrozen == "" {
		   p.LiveFrozen = "0"
	  }
	  if p.LiveObsGen == 0 {
		  p.LiveObsGen = FingerprintObserveGen(p.LiveAvail, p.LiveFrozen)
	   }
	  p.Digest = DriftDigest(*p)
	  if err := d.store.SaveDrift(ctx, p); err != nil {
		  return nil, err
	   }
	   p.Status = DriftProven
	  p.ProvenAt = &now
	  p.Digest = DriftDigest(*p)
	   if err := d.store.SaveDrift(ctx, p); err != nil {
		  return nil, err
	  }
	  cp := p.Copy()
	   return &cp, nil
  }

// Verify 重算摘要。

   func (d *DriftDetector) Verify(ctx context.Context, id string) error {
	   if d == nil || d.store == nil {
		   return ErrUnavailable
	  }
	  p,
		   err := d.store.GetDrift(ctx, id)
	   if err != nil {
		   return err
	  }
	  if p == nil {
		   return ErrDriftNotFound
	  }
	  want := DriftDigest(*p)
	   if want != strings.TrimSpace(p.Digest) {
		  return ErrProofDigestMismatch
	   }
	  return nil
   }

// Close 运维收口。必须已 proven。
   func (d *DriftDetector) Close(ctx context.Context, id, operator string) (*DriftProof, error) {
	   operator = strings.TrimSpace(operator)
	   if operator == "" {
		   return nil, ErrOperatorInvalid
	   }
	  p, err := d.store.GetDrift(ctx, id)
	  if err != nil {
		   return nil, err
	   }
	  if p == nil {
		   return nil, ErrDriftNotFound
	   }
	   if p.Status == DriftClosed {
		   return nil, ErrDriftAlreadyClosed
	   }
	   if p.Status != DriftProven {
		  return nil, ErrProofDigestMismatch
	  }
	   if err := d.Verify(ctx, id); err != nil {
		  return nil, err
	  }
	   now := d.clock()
	   p.Status = DriftClosed
	   p.ClosedAt = &now
	   p.Closer = operator
	   if err := d.store.SaveDrift(ctx, p); err != nil {
		  return nil, err
	  }
	  cp := p.Copy()
	  return &cp, nil
   }

// HasBlocking 同科目是否存在挡写漂移。

  func (d *DriftDetector) HasBlocking(ctx context.Context, userID, book, asset string) (bool, error) {
	   rows,
		   err := d.store.ListBlockingDrifts(ctx, userID, book, asset)
	   if err != nil {
		  return false, err
	  }
	  return len(rows) > 0, nil
  }
