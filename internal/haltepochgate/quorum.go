package haltepochgate

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

const DefaultQuorumThreshold = 2

// LocalQuorum 进程内投票器。

type LocalQuorum struct {
	threshold int
}

  func NewLocalQuorum(n int) *LocalQuorum {
	  if n < 1 {
		   n = DefaultQuorumThreshold
	  }
	  return &LocalQuorum{threshold: n}
  }

  func (q *LocalQuorum) Threshold() int {
	  if q == nil || q.threshold < 1 {
		  return DefaultQuorumThreshold
	  }
	  return q.threshold
   }

  func (q *LocalQuorum) AllowClear(_ context.Context, _ ScopeKey, actor string) error {
	   if strings.TrimSpace(actor) == "" {
		   return ErrActorRequired
	  }
	  return nil
   }

  func (s *Service) VoteClear(ctx context.Context, epochID, actor, reason string, approve bool) (*QuorumBundle, error) {
	   if s == nil || s.store == nil {
		  return nil, ErrStoreUnavailable
	  }
	  actor = strings.TrimSpace(actor)
	   if actor == "" {
		   return nil, ErrActorRequired
	   }
	   if strings.TrimSpace(epochID) == "" {
		  return nil, ErrEpochNotFound
	  }
	  now := s.now()
	  bundle, err := s.store.GetQuorumByEpoch(ctx, epochID)
	  if err != nil {
		   bundle = &QuorumBundle{
			   QuorumID: "q-" + uuid.NewString(),
			  EpochID:  epochID,
			  Threshold: s.quorumN(),
			   CreatedAt: now,
		   }
	  }
	  for i, v := range bundle.Votes {
		  if v.Actor == actor {
			   bundle.Votes[i].Approve = approve
			   bundle.Votes[i].Reason = strings.TrimSpace(reason)
			   bundle.Satisfied = bundle.DistinctApprovals() >= bundle.Threshold
			  if err := s.store.PutQuorum(ctx, *bundle); err != nil {
				   return nil, err
			  }
			   return bundle, nil
		  }
	   }
	  bundle.Votes = append(bundle.Votes, QuorumVote{
		   VoteID:   "v-" + uuid.NewString(),
		   QuorumID: bundle.QuorumID,
		  EpochID: epochID,
		  Actor: actor,
		   Approve: approve,
		  Reason: strings.TrimSpace(reason),
		  CreatedAt: now,
	  })
	   bundle.Satisfied = bundle.DistinctApprovals() >= bundle.Threshold
	   if err := s.store.PutQuorum(ctx, *bundle); err != nil {
		  return nil, err
	  }
	  return bundle, nil
   }

   func (s *Service) quorumN() int {
	  if s.quorum != nil {
		   return s.quorum.Threshold()
	   }
	  return DefaultQuorumThreshold
   }

  func (s *Service) requireQuorum(ctx context.Context, ep *HaltEpoch, actor string, force bool) (*QuorumBundle, error) {
	  if force {
		   return &QuorumBundle{QuorumID: "force", EpochID: ep.EpochID, Threshold: 0, Satisfied: true, CreatedAt: time.Now().UTC()}, nil
	   }
	  if s.quorum != nil {
		  if err := s.quorum.AllowClear(ctx, ep.Scope, actor); err != nil {
			  return nil, err
		  }
	   }
	   bundle, err := s.store.GetQuorumByEpoch(ctx, ep.EpochID)
	   if err != nil {
		   return nil, ErrQuorumDenied
	   }
	  if bundle.Threshold <= 0 {
		   bundle.Threshold = s.quorumN()
	   }
	  if bundle.DistinctApprovals() < bundle.Threshold {
		  return bundle, ErrQuorumDenied
	  }
	   bundle.Satisfied = true
	   return bundle, nil
   }
