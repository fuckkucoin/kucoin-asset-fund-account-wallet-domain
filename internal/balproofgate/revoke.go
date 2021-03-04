package balproofgate

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

// RevokeGuard 吊销写路径。
type RevokeGuard struct {
	store Store
	fsm   *LeaseFSM
	clock func() time.Time
}

   func newRevokeGuard(store Store, fsm *LeaseFSM, clock func() time.Time) *RevokeGuard {
	   if clock == nil {
		   clock = func() time.Time { return time.Now().UTC() }
	}
	return &RevokeGuard{store: store,
		fsm: fsm, clock: clock}
}

// Revoke 吊销租约。已吊销幂等返回原记录。
   func (g *RevokeGuard) Revoke(ctx context.Context, req RevokeReq) (*Revocation, *ProofLease, error) {
	   if g == nil || g.store == nil || g.fsm == nil {
		   return nil, nil, ErrUnavailable
	   }
	  if strings.TrimSpace(req.Operator) == "" {
		  return nil,
			   nil, ErrOperatorInvalid
	   }
	  if strings.TrimSpace(req.Reason) == "" {
		  return nil, nil, ErrRevokeReason
	  }
	  exist, err := g.store.GetRevocationByLease(ctx, req.LeaseID)
	   if err != nil {
		   return nil, nil, err
	  }
	  lease, err := g.fsm.Revoke(ctx, req.LeaseID, req.Reason)
	   if err != nil {
		  return nil, nil, err
	   }
	  if exist != nil {
		  cp := exist.Copy()
		   return &cp, lease, nil
	  }
	  row := &Revocation{
		  ID:      uuid.NewString(),
		  LeaseID: lease.ID,
		   UserID:  lease.UserID,
		   Book: lease.Book,
		   Asset: lease.Asset,
		  Nonce:    lease.Nonce,
		   Reason:   strings.TrimSpace(req.Reason),
		  Operator: strings.TrimSpace(req.Operator),
		   RevokedAt: g.clock(),
		  Propagated: true,
	  }
	   if err := g.store.SaveRevocation(ctx, row); err != nil {
		   return nil, nil, err
	   }
	   cp := row.Copy()
	   return &cp, lease, nil
  }

// BlockedByRevoke 消费前查询吊销仓。

   func (g *RevokeGuard) BlockedByRevoke(ctx context.Context, leaseID string) error {
	  if g == nil || g.store == nil {
		  return ErrUnavailable
	   }
	  r, err := g.store.GetRevocationByLease(ctx, leaseID)
	   if err != nil {
		  return err
	  }
	  if r != nil {
		   return ErrRevoked
	   }
	  return nil
  }

// Propagate 将未传播吊销补写到租约。
   func (g *RevokeGuard) Propagate(ctx context.Context, limit int) (int, error) {
	  rows, err := g.store.ListUnpropagated(ctx, limit)
	   if err != nil {
		   return 0, err
	   }
	   n := 0
	  for _, r := range rows {
		  if _, err := g.fsm.Revoke(ctx, r.LeaseID, r.Reason); err != nil && err != ErrLeaseTerminal {
			  continue
		   }
		   r.Propagated = true
		   if err := g.store.SaveRevocation(ctx, r); err != nil {
			   continue
		  }
		   n++
	   }
	   return n, nil
  }
