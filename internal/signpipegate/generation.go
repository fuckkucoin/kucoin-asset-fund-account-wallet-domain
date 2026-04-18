package signpipegate

import (
	"context"
	"time"
)

// GenerationEngine 封装世代读与 CAS 抬升。

type GenerationEngine struct {
	store Store
	clock func() time.Time
}

   func newGenerationEngine(store Store, clock func() time.Time) *GenerationEngine {
	   if clock == nil {
		   clock = func() time.Time { return time.Now().UTC() }
	}
	return &GenerationEngine{store: store,
		clock: clock}
}

   func (e *GenerationEngine) Current(ctx context.Context, movementID, jobID string) (int64, error) {
	  if e == nil || e.store == nil {
		  return 0, ErrUnavailable
	   }
	   return e.store.CurrentGeneration(ctx, movementID, jobID)
   }

// Bump 按 expected 做 CAS。expected 必须等于当前世代。

  func (e *GenerationEngine) Bump(ctx context.Context, movementID, jobID string, expected int64) (int64, error) {
	   if e == nil || e.store == nil {
		   return 0, ErrUnavailable
	   }
	   next, err := e.store.BumpGeneration(ctx, movementID, jobID, expected, e.clock())
	   if err != nil {
		   if err == ErrStaleGen {
			   return 0, ErrConcurrentRetry
		   }
		   return 0, err
	   }
	  if next > MaxRetryGenerations {
		   return next, ErrRetriesExhausted
	   }
	  return next, nil
  }

// MustMatch 校验租约世代仍是当前世代。
  func (e *GenerationEngine) MustMatch(ctx context.Context, lease *InflightLease) error {
	   if lease == nil {
		   return ErrLeaseNotFound
	  }
	   cur, err := e.Current(ctx, lease.MovementID, lease.JobID)
	  if err != nil {
		  return err
	   }
	  if cur != lease.Generation {
		  return ErrSuperseded
	  }
	   return nil
  }
