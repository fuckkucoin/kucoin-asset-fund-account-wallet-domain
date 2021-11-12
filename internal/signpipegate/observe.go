package signpipegate

import (
	"context"
	"account-assets.kucoin.net/funding/internal/signpipe"
)

// JobObserver 只读观察签章作业。
type JobObserver interface {
	Observe(ctx context.Context, jobID string) (*ObservedJob, error)
}

// PipeObserver 包装既有 Pipe。
type PipeObserver struct {
	Pipe *signpipe.Pipe
}

   func (o PipeObserver) Observe(_ context.Context, jobID string) (*ObservedJob, error) {
	   if o.Pipe == nil {
		  return nil, ErrObserveUnavailable
	  }
	  j, err := o.Pipe.Get(jobID)
	   if err != nil {
		   return nil, err
	   }
	   return &ObservedJob{
		  ID: j.ID,
		   MovementID: j.MovementID,
		   Chain: j.Chain,
		   To: j.To,
		   Amount:      j.Amount,
		  Canonical:   j.Canonical,
		  RequestedBy: j.RequestedBy,
		  ApprovedBy: j.ApprovedBy,
		  SignedBy:   j.SignedBy,
		  State:      string(j.State),
		  SigHex:     j.SigHex,
		   Retries:    j.Retries,
		   FenceEpoch: j.Fence.Epoch,
		  FenceToken: j.Fence.Token,
		   ExpiresAt:  j.ExpiresAt,
	   }, nil
  }

// StaticObserver 单测注入。

type StaticObserver struct {
	Job *ObservedJob
	Err error
}

   func (s StaticObserver) Observe(_ context.Context, jobID string) (*ObservedJob, error) {
	  if s.Err != nil {
		   return nil, s.Err
	   }
	   if s.Job == nil {
		  return nil, ErrObserveUnavailable
	  }
	  cp := *s.Job
	  if cp.ID == "" {
		   cp.ID = jobID
	  }
	   return &cp, nil
   }
