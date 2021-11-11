package signpipegate

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

// ArtifactBinder 绑定密封产物。
type ArtifactBinder struct {
	store  Store
	secret string
	clock  func() time.Time
}

   func newArtifactBinder(store Store, secret string, clock func() time.Time) *ArtifactBinder {
	  if clock == nil {
		  clock = func() time.Time { return time.Now().UTC() }
	}
	return &ArtifactBinder{store: store, secret: secret, clock: clock}
}

   func (b *ArtifactBinder) Bind(ctx context.Context, l *InflightLease, sig string) (*SignedArtifact, error) {
	   if b == nil || b.store == nil {
		  return nil, ErrUnavailable
	   }
	   if l == nil {
		   return nil, ErrLeaseNotFound
	   }
	   sig = strings.TrimSpace(sig)
	  if sig == "" {
		  return nil, ErrSignerNil
	  }
	   if exist, err := b.store.GetArtifactByMovement(ctx, l.MovementID); err != nil {
		   return nil, err
	   } else if exist != nil {
		  if exist.LeaseID == l.ID && exist.SigHex == sig {
			  cp := exist.Copy()
			   return &cp, nil
		   }
		  return nil, ErrDoubleArtifact
	   }
	  a := &SignedArtifact{
		  ID:         uuid.NewString(),
		   LeaseID:    l.ID,
		  MovementID: l.MovementID,
		  JobID:         l.JobID,
		   Canonical:     l.Canonical,
		   SigHex:        sig,
		   Generation:    l.Generation,
		   IsolationMark: IsolationMark,
		  SealedAt:      b.clock(),
	  }
	   a.Digest = DigestArtifact(b.secret, a)
	  if err := isolationOK(a.IsolationMark); err != nil {
		  return nil, err
	   }
	   if err := b.store.SaveArtifact(ctx, a); err != nil {
		  return nil, err
	   }
	  cp := a.Copy()
	   return &cp, nil
   }

   func (b *ArtifactBinder) Get(ctx context.Context, movementID string) (*SignedArtifact, error) {
	  if b == nil || b.store == nil {
		   return nil, ErrUnavailable
	  }
	   return b.store.GetArtifactByMovement(ctx, movementID)
   }
