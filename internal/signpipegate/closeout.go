package signpipegate

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

// CloseoutEngine 开立、核验、关闭收口证明。
type CloseoutEngine struct {
	store Store
	secret string
	clock  func() time.Time
}

   func newCloseoutEngine(store Store,
	  secret string, clock func() time.Time) *CloseoutEngine {
	  if clock == nil {
		  clock = func() time.Time { return time.Now().UTC() }
	}
	return &CloseoutEngine{store: store, secret: secret, clock: clock}
}

  func (e *CloseoutEngine) Open(ctx context.Context, lease *InflightLease, kind CloseoutKind, note string) (*CloseoutProof, error) {
	  if e == nil || e.store == nil {
		   return nil, ErrUnavailable
	   }
	   if lease == nil {
		   return nil, ErrLeaseNotFound
	   }
	   if !KnownCloseoutKind(kind) {
		   kind = CloseFault
	   }
	  if exist, err := e.store.FindOpenCloseout(ctx, lease.ID); err != nil {
		   return nil, err
	  } else if exist != nil {
		  cp := exist.Copy()
		   return &cp, nil
	   }
	   c := &CloseoutProof{
		  ID:         uuid.NewString(),
		  LeaseID:    lease.ID,
		  MovementID: lease.MovementID,
		  JobID:      lease.JobID,
		   Kind: kind,
		   Status: CloseoutOpen,
		  Note:   strings.TrimSpace(note),
		   IsolationMark: IsolationMark,
		   OpenedAt:      e.clock(),
	  }
	  c.Digest = DigestCloseout(e.secret, c)
	  c.Status = CloseoutProven
	  c.ProvenAt = e.clock()
	   if err := e.store.SaveCloseout(ctx, c); err != nil {
		  return nil, err
	  }
	  cp := c.Copy()
	   return &cp, nil
  }

  func (e *CloseoutEngine) Close(ctx context.Context, proofID, operator, reason string) (*CloseoutProof, error) {
	  if err := validateOperator(operator); err != nil {
		   return nil, err
	  }
	   if err := validateReason(reason); err != nil {
		  return nil, err
	  }
	   c, err := e.store.GetCloseout(ctx, proofID)
	   if err != nil {
		  return nil, err
	  }
	  if c.Status == CloseoutClosed {
		  return nil, ErrCloseoutClosed
	  }
	  if err := VerifyCloseoutDigest(e.secret, c); err != nil {
		  return nil, err
	  }
	  c.Status = CloseoutClosed
	  c.Operator = strings.TrimSpace(operator)
	   c.Note = strings.TrimSpace(c.Note + " | " + reason)
	  c.ClosedAt = e.clock()
	   if err := e.store.SaveCloseout(ctx, c); err != nil {
		   return nil, err
	   }
	  cp := c.Copy()
	   return &cp, nil
   }
