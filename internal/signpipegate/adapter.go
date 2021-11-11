package signpipegate

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/signpipe"
)

// PipeBridge 把闸决策映射到既有 Pipe 只读字段。
type PipeBridge struct {
	Pipe *signpipe.Pipe
}

   func NewPipeBridge(p *signpipe.Pipe) *PipeBridge {
	   return &PipeBridge{Pipe: p}
   }

   func (b *PipeBridge) Snapshot(jobID string) (*ObservedJob, error) {
	  if b == nil || b.Pipe == nil {
		  return nil, ErrPipeUnavailable
	   }
	   return PipeObserver{Pipe: b.Pipe}.Observe(context.Background(), jobID)
  }

  func (b *PipeBridge) AlignCanonical(lease *InflightLease) error {
	   if lease == nil {
		  return ErrLeaseNotFound
	  }
	   job, err := b.Snapshot(lease.JobID)
	  if err != nil {
		   return err
	   }
	  want := Canonical(lease.MovementID, lease.Chain, lease.To, lease.Amount)
	   if job.Canonical != want && job.Canonical != lease.Canonical {
		   return ErrCanonical
	   }
	   if !strings.EqualFold(job.MovementID, lease.MovementID) {
		   return ErrMovementInvalid
	   }
	  return nil
   }

  func (b *PipeBridge) FenceMatch(lease *InflightLease) error {
	   if lease == nil {
		   return ErrLeaseNotFound
	   }
	  job, err := b.Snapshot(lease.JobID)
	  if err != nil {
		   return err
	  }
	  if lease.FenceToken == "" {
		  return nil
	   }
	   if job.FenceEpoch != lease.FenceEpoch || job.FenceToken != lease.FenceToken {
		  return ErrFenceStale
	  }
	   return nil
   }
